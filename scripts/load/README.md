# Load test

`catalog.js` is the k6 script for the BE-6 performance target: at 20 rps, `GET /v1/dishes` p95 < 100ms, the recipe endpoint p95 < 50ms, and 0 errors. The thresholds encode the target, so k6 exits non-zero (99) when one is missed. Record results in [`docs/perf-baseline.md`](../../docs/perf-baseline.md).

k6 runs from the official image, pinned by tag and digest in the Makefile (`K6_IMAGE`), so nothing needs installing. The script goes in on stdin (`k6 run -`) and has no local imports, so the same file runs on a laptop or on the VM without copying anything.

## Settings

| Env | Default | Meaning |
|-----|---------|---------|
| `BASE_URL` | required | Origin without a trailing path, e.g. `http://localhost:8080` |
| `RATE` | `20` | Requests per second (constant arrival rate, one request per iteration) |
| `DURATION` | `2m` | k6 duration, e.g. `20s`, `5m` |
| `TARGET` | `api` | `api`: the `/v1` endpoints, with the latency thresholds. `pages`: the frontend pages that call them, with the error thresholds only |

`TARGET=api` cycles through `GET /v1/regions`, `GET /v1/dishes` (twice the share), `GET /v1/dishes/{dishKey}` and the recipe endpoint, in `en`, `es` and `ru`, for keys from `testdata/content.design.json`. `TARGET=pages` cycles through `/:lang/`, `/:lang/dishes/:dishKey` and `/:lang/dishes/:dishKey/:regionKey` with the same keys. Both need the design seed (`make seed` locally); any non-200 counts as an error.

The summary shows p50 (`med`), p95, p99 and the error rate for every endpoint (the `{ endpoint:... }` rows).

k6 doesn't cache or send `If-None-Match`, so every API request is a full 200: the worst case for the server.

## Local

```bash
make docker-up
make seed
make load BASE_URL=http://localhost:8080 DURATION=20s
```

`make load` runs `docker run --network host`, so `localhost` is the host's. Set `LOAD_NETWORK=<network>` to join a Docker network instead. Local numbers only check that the script and the thresholds work; they're not a baseline.

## Demo

The API isn't reachable from the internet (stack.md X4): Caddy proxies only to the SvelteKit frontend, and the frontend doesn't proxy `/v1`. Its pages call the API server-side over the internal network (`recetas-frontend/src/lib/server/api-source.ts`):

- every page under `/:lang/` loads `GET /v1/regions` and `GET /v1/dishes` through a per-locale in-memory cache, fresh for 60s and then revalidated with the ETag (mostly a 304);
- `/:lang/dishes/:dishKey` adds `GET /v1/dishes/{dishKey}` on every request;
- `/:lang/dishes/:dishKey/:regionKey` adds the recipe call on every request.

So the headline `GET /v1/dishes` target can't be measured from outside at all: public traffic reaches it about once a minute per locale. The baseline takes two runs.

### 1. API, from a container on the demo network

This measures the backend alone (Go and Postgres, no TLS or proxy), which is what the BE-6 targets are about. The network name and the backend's service name are as planned in INF-2: compose project `recetas-demo` (always passed as `-p recetas-demo`, and planned as `name:` in the compose file) with network `internal` gives `recetas-demo_internal`, and the service is `backend`. Check with `docker network ls` before the run.

```bash
ssh <vps> 'docker run --rm -i --network recetas-demo_internal \
  -e BASE_URL=http://backend:8080 -e DURATION=2m \
  grafana/k6:2.3.0@sha256:2dd4756ec6e14e62778916e1eb850108e4b499304c02df56760fa0bdb0fba0b8 \
  run --quiet -' < scripts/load/catalog.js
```

Keep the image reference in step with `K6_IMAGE` in the Makefile. k6 shares the VM's CPU with the system under test, so note the VM's load (`uptime`) next to the result.

### 2. Pages, from outside the VM through Caddy

This measures what a visitor sees for an uncached page: TLS, Caddy, SvelteKit's server render and the API call behind it. The stage sets no latency target for pages, so the run checks for errors and records the numbers.

```bash
make load BASE_URL=https://demo.asadorov.es TARGET=pages DURATION=2m
```

The difference between the page and API numbers for the recipe is the cost of the frontend and the edge. A redeploy under load (BE-6, graceful shutdown) uses the same command with a longer `DURATION` while `docker compose -p recetas-demo up -d` runs on the VM; any failed request shows in `http_req_failed`.

Don't run it against prod, where the synthetic load would compete with real visitors. The baseline is taken on demo, which shares the VM with prod (D-INF0-3), so schedule the run for a quiet hour.
