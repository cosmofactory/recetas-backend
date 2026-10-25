# recetas-backend

The API behind Asadorov, a non-profit atlas of Spanish cooking: a read-only catalog of regions, dishes, their regional variations and one recipe per variation, in English, Spanish and Russian. A modular Go monolith: one binary, one Postgres. The public site is the SvelteKit frontend; this API is reachable only from it, on the internal Docker network.

## Quick start

Needs Docker with Compose, and Go 1.27 for the `make` targets that run Go.

```bash
cp .env.example .env
make docker-up                  # builds the image, starts Postgres + backend (migrations run on start)
make seed                       # imports the design content (re-running prints "no changes")
curl -s localhost:8080/healthz  # {"status":"ok"}
curl -s 'localhost:8080/v1/dishes/tortilla?lang=es'
make docker-logs                # follow the backend's JSON logs
make docker-down
```

Postgres is published on `127.0.0.1:5435`, which is the `DATABASE_URL` in `.env.example`, so `make run`, `make seed`, `make migrate` and psql reach it from the host. To run the server outside Docker, start only the database (`docker compose up -d database`), then `make migrate` (after `make goose-install`) and `make run`.

## Commands

| What | Command |
|------|---------|
| Start / stop / follow the local stack | `make docker-up` / `make docker-down` / `make docker-logs` |
| Run the server bare-metal | `make run` |
| Lint, as CI does: gofmt, vet, golangci-lint (with the clean-code limits), build | `make lint` |
| Format | `make fmt` |
| Unit tests (integration tests skip) | `make test` |
| All tests, against a migrated `recetas_test` DB in the compose Postgres | `make test-integration` |
| Vulnerability check of the source / of an image's binaries | `make vulncheck` / `make vulncheck-image IMAGE_REF=<image>` |
| Build `bin/server` and `bin/import` | `make build` |
| Import the design content into `DATABASE_URL` | `make seed` |
| Regenerate `testdata/content.design.json` from the design's `content.json` | `make design-bundle` |
| Regenerate all generated code (oapi-codegen + sqlc; CI fails when it's stale) | `make generate` |
| Lint `api/openapi.yaml` | `make api-lint` |
| Migrations: apply, revert one, status, new | `make migrate`, `make migrate-down`, `make migrate-status`, `make migrate-create NAME=xyz` |
| Install the pinned goose CLI (for the migrate targets) | `make goose-install` |
| Load test (k6 in Docker) | `make load BASE_URL=http://localhost:8080 [RATE=20] [DURATION=2m] [TARGET=api\|pages]` |
| Everything else | `make help` |

Every tool is pinned. oapi-codegen, sqlc, golangci-lint, govulncheck, vacuum and k6 run at their pinned versions without installing anything.

## API

The contract is [`api/openapi.yaml`](api/openapi.yaml), also served at `GET /openapi.json`. Catalog endpoints take `?lang=en|es|ru` (default `en`; anything else is a 400):

| Endpoint | Returns |
|----------|---------|
| `GET /v1/regions` | every region in map order, with its regional dishes |
| `GET /v1/regions/{regionKey}` | one region with every dish's variation there |
| `GET /v1/dishes` | every dish, with its variations in region order |
| `GET /v1/dishes/{dishKey}` | one dish with its variations |
| `GET /v1/dishes/{dishKey}/variations/{regionKey}/recipe` | the recipe of that variation |
| `GET /healthz` | 200 `{"status":"ok"}`, or 503 `{"status":"degraded","checks":{"db":"timeout\|unavailable"}}` |

Responses carry a weak `ETag` that changes with every content import, and `Cache-Control: public, max-age=60, stale-while-revalidate=600`; a matching `If-None-Match` gets a 304. Errors are `{"error":{"code":"...","message":"..."}}`, unknown keys and paths a 404. Before the first import the lists are empty and the details 404.

Content never arrives through the API: the content pipeline hands a JSON bundle to the import CLI (`/app/import` in the image), which validates it and replaces the catalog in one transaction. See [`docs/content-import.md`](docs/content-import.md).

## Configuration

Environment variables, with `.env` loaded when present. [`.env.example`](.env.example) lists all of them with comments.

| Var | Read by | Default | Purpose |
|-----|---------|---------|---------|
| `APP_ENV` | server | required | `local`, `demo` or `prod` |
| `LOG_LEVEL` | server | `info` | `debug`, `info`, `warn` or `error` |
| `HTTP_ADDR` | server | `:8080` | Listen address |
| `DATABASE_URL` | server, import | required | Postgres DSN. In compose it's derived from `POSTGRES_*` |
| `DB_MAX_CONNS` | server | `10` | Pool size, 1–100 |
| `DB_MIN_CONNS` | server | `1` | Idle connections kept open, 0–`DB_MAX_CONNS` |
| `LOGFIRE_TOKEN` | server | blank (off) | Logfire write token; blank turns telemetry off |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | server | required with a token | https OTLP base URL, e.g. `https://logfire-eu.pydantic.dev` |
| `OTEL_SERVICE_NAME` | server | `recetas-backend` | `service.name` on spans and logs |
| `OTEL_TRACES_SAMPLER`, `OTEL_TRACES_SAMPLER_ARG` | OpenTelemetry SDK | `parentbased_always_on` | Trace sampling |
| `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` | compose | `recetas`, `recetas`, `recetas_dev` | The local Postgres container |
| `BACKEND_HOST_PORT`, `DB_HOST_PORT`, `DB_BIND_ADDRESS` | compose | `8080`, `5435`, `127.0.0.1` | Host ports of the local stack |
| `TEST_DATABASE_URL` | integration tests | unset (skip) | Never `DATABASE_URL`; `make test-integration` sets it |

## Layout

```
api/                   openapi.yaml (the contract), oapi-codegen config, content-bundle.schema.json
cmd/server/            the server: config, wiring, graceful shutdown
cmd/import/            the content import CLI (/app/import)
internal/api/          adapts the generated strict server; /openapi.json
  apigen/              oapi-codegen output (generated)
internal/catalog/      the catalog module: handler -> service -> Repository -> sqlc
  catalogdb/           sqlc output (generated)
  importer/            bundle parse, validate, diff and apply
internal/config/       env parsing
internal/platform/     database (pgxpool), telemetry (OTel -> Logfire), logging (slog JSON),
                       httpx (server, middleware, conditional GET), health (/healthz)
db/migrations/         goose migrations, the schema source of truth (docs/schema.md)
db/queries/<module>/   sqlc inputs
docker/entrypoint.sh   applies migrations, then execs the server or the import
scripts/designbundle/  converts the design's content.json into testdata/content.design.json
scripts/load/          k6 load test
```

## CI

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) runs on every PR and push:

1. **Lint And Vet**: gofmt, vet, golangci-lint, build, generated-code drift (`make generate` + `git diff`), OpenAPI lint.
2. **Test**: Postgres 16, migrations, `go test -race` with the integration tests on.
3. **Vulnerability Check**: govulncheck on the source.
4. **Build, Scan And Push Image**: builds the image, scans it with Trivy (fails on fixable CRITICAL) and govulncheck in binary mode, and on `main` or a tag pushes `ghcr.io/cosmofactory/recetas-backend:sha-<7>` (plus `vX.Y.Z` for a tag). There's no `:latest`.

Deploying to demo and prod is INF-2 and INF-5 in `../specs/`.

## Docs

- [`docs/content-import.md`](docs/content-import.md): the bundle format, validation, import semantics, exit codes
- [`docs/schema.md`](docs/schema.md): the database schema
- [`docs/runbook.md`](docs/runbook.md): logs, health, imports, rollbacks, failure modes
- [`docs/perf-baseline.md`](docs/perf-baseline.md) and [`scripts/load/README.md`](scripts/load/README.md): the performance target and how it's measured
- [`CLAUDE.md`](CLAUDE.md): architecture and conventions in detail
- `docs/plans/`: the early iteration plans, kept as history
