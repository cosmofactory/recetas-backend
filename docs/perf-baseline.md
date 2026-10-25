# Performance baseline

The BE-6 baseline, measured on demo with [`scripts/load/catalog.js`](../scripts/load/README.md). Fill it in once demo runs (INF-2), and re-measure when the catalog queries, the pool or the VM size change.

## Target

| Endpoint | Rate | p95 | Errors |
|----------|------|-----|--------|
| `GET /v1/dishes` | 20 rps (whole mix) | < 100ms | 0 |
| `GET /v1/dishes/{dishKey}/variations/{regionKey}/recipe` | 20 rps (whole mix) | < 50ms | 0 |

The other endpoints in the mix (`GET /v1/regions`, `GET /v1/dishes/{dishKey}`) and the frontend pages have no target; they're recorded for reference.

## Method

| | |
|-|-|
| Tool | k6, `grafana/k6:2.3.0@sha256:2dd4756e…` (`K6_IMAGE` in the Makefile) |
| Script | `scripts/load/catalog.js` at commit _TBD_ |
| Rate, duration | constant arrival rate, 20 rps for 2m (defaults) |
| Content | design seed (`testdata/content.design.json`), content version _TBD_ |
| Backend image | `ghcr.io/cosmofactory/recetas-backend:sha-_TBD_` |
| Frontend image | `ghcr.io/cosmofactory/recetas-frontend:sha-_TBD_` (pages run only) |
| Pool | `DB_MAX_CONNS` _TBD_, `DB_MIN_CONNS` _TBD_ |
| VM | _TBD_ (size; load average before the run) |
| Date | _TBD_ |

Two runs, because the API isn't public (see the [load test README](../scripts/load/README.md#demo)):

1. **API**: `TARGET=api`, from a k6 container on the demo Docker network, `BASE_URL=http://backend:8080`. This is the run the targets apply to.
2. **Pages**: `TARGET=pages`, from outside the VM through Caddy, `BASE_URL=https://demo.asadorov.es`. Run from _TBD_ (machine and network).

## Results

### 1. API (demo network)

| Endpoint | Requests | p50 | p95 | p99 | Error rate | Target met |
|----------|----------|-----|-----|-----|------------|------------|
| `GET /v1/regions` | | | | | | n/a |
| `GET /v1/dishes` | | | | | | |
| `GET /v1/dishes/{dishKey}` | | | | | | n/a |
| `GET /v1/dishes/{dishKey}/variations/{regionKey}/recipe` | | | | | | |

### 2. Pages (through Caddy)

| Page | Requests | p50 | p95 | p99 | Error rate |
|------|----------|-----|-----|-----|------------|
| `/:lang/` | | | | | |
| `/:lang/dishes/:dishKey` | | | | | |
| `/:lang/dishes/:dishKey/:regionKey` | | | | | |

### N+1 check

One catalog request of each kind in Logfire, with its `query …` span count (BE-6). The counts are fixed whatever the catalog size: every request reads the content version (`query LatestContentVersion`); `GET /v1/dishes` adds `ListDishes` and `ListVariations` only when its (locale, version) list isn't memoised yet, so expect 3 spans cold and 1 warm; the recipe always adds `GetRecipeByDishRegion`, `ListRecipeIngredients` and `ListRecipeSteps`, so 4. Anything that grows with the number of dishes or variations is an N+1.

| Request | trace_id | Memo | SQL spans | Expected |
|---------|----------|------|-----------|----------|
| `GET /v1/dishes` | | cold | | 3 |
| `GET /v1/dishes` | | warm | | 1 |
| `GET /v1/dishes/{dishKey}/variations/{regionKey}/recipe` | | n/a | | 4 |

## Local smoke run, not a baseline

On 2026-10-25, `make load BASE_URL=http://localhost:8080 DURATION=20s` against `make docker-up` and `make seed`, on a developer workstation (WSL2, Docker Engine in Ubuntu 24.04, 16 CPUs, 32 GB; k6 and the stack on the same machine, no TLS or proxy). It shows the script and thresholds work; the numbers say nothing about demo hardware.

| Endpoint | Requests | p50 | p95 | p99 | Error rate |
|----------|----------|-----|-----|-----|------------|
| `GET /v1/regions` | 41 | 1.10ms | 2.31ms | 5.43ms | 0% |
| `GET /v1/dishes` | 80 | 1.25ms | 1.90ms | 4.73ms | 0% |
| `GET /v1/dishes/{dishKey}` | 120 | 1.17ms | 1.39ms | 1.59ms | 0% |
| recipe | 160 | 2.57ms | 3.03ms | 5.65ms | 0% |
