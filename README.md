# recetas-backend

Backend for Asadorov, a non-profit atlas of Spanish cooking. Modular Go monolith — one binary, one Postgres.

## Stack

- **Go 1.27** — `net/http.ServeMux` with method+path routing, no third-party router.
- **PostgreSQL 16** — pgx/v5 driver.
- **[sqlc](https://sqlc.dev/)** — SQL-first, generates type-safe Go from `.sql` files.
- **[goose](https://github.com/pressly/goose)** — numbered up/down migrations.
- **[oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)** — spec-first: the strict server interface is generated from `api/openapi.yaml`.
- **Docker + Docker Compose** for local dev.
- **GitHub Actions** for CI.

## Layout

```
api/
  openapi.yaml         # the API contract (OpenAPI 3.0.3), hand-written
  oapi-codegen.yaml    # codegen config
  content-bundle.schema.json  # JSON Schema of a content import bundle
cmd/server/            # entry point — the only place modules are wired together
cmd/import/            # content import CLI (/app/import in the image)
internal/
  api/                 # adapts the generated strict interface; serves /openapi.json
    apigen/            # oapi-codegen output (do not edit)
  config/              # env loading
  platform/database/   # pgxpool bootstrap
  platform/logging/    # slog JSON logger
  platform/httpx/      # http.Server timeouts + middleware (access log, panic recovery, body limit, conditional GET)
  platform/health/     # GET /healthz (DB ping)
  catalog/             # catalog module: regions, dishes, variations, recipes
    catalogdb/         # sqlc-generated code (do not edit)
    catalogtest/       # test-only fixture loader
    importer/          # content bundle parse, validate, diff and apply
db/
  migrations/          # goose .sql files (schema: docs/schema.md)
  queries/<module>/    # sqlc .sql inputs, one dir per module
docker/
  entrypoint.sh        # runs migrations then execs the server (or /app/import)
scripts/designbundle/  # converts the design content.json into testdata/content.design.json
testdata/
  content.design.json  # the design content as an import bundle (make seed)
```

Domain modules never import each other — wiring happens only in `cmd/server/main.go`.

## Prerequisites

- Go 1.27+
- Docker + Docker Compose (for `make docker-*` targets)
- Optional local tools (only if you want to run outside Docker):
  - `make goose-install` — pinned goose CLI
  - nothing else: `make generate` runs the pinned oapi-codegen (go.mod `tool`) and sqlc (`go run`)

## Quick start — Docker (recommended)

```bash
cp .env.example .env
make docker-up          # builds image, starts postgres + backend
curl -i http://localhost:8080/healthz
make docker-logs        # tail backend logs
make docker-down        # stop
```

The `backend` container runs migrations (`goose up`) before starting the server. Postgres is exposed on `127.0.0.1:5435` for local tools (psql, DataGrip).

## Quick start — bare-metal Go

Requires a running Postgres reachable via `DATABASE_URL`.

```bash
cp .env.example .env         # edit DATABASE_URL
make goose-install
make migrate                # apply schema
make run                    # go run ./cmd/server
```

## Common tasks

| What | Command |
|------|---------|
| Format | `make fmt` |
| Lint (fmt-check + vet + golangci-lint + build) | `make lint` |
| Test (integration tests skip) | `make test` |
| Integration tests (compose Postgres, migrated `recetas_test` DB) | `make test-integration` |
| Build binaries → `bin/server`, `bin/import` | `make build` |
| Import the design content into `DATABASE_URL` (idempotent) | `make seed` |
| Regenerate `testdata/content.design.json` from the design | `make design-bundle` |
| Regenerate all generated code (oapi-codegen + sqlc) | `make generate` |
| Regenerate the API server code from `api/openapi.yaml` | `make api-generate` |
| Lint the OpenAPI spec (vacuum, fails on errors) | `make api-lint` |
| Regenerate sqlc code | `make sqlc-generate` |
| Apply migrations | `make migrate` |
| Revert last migration | `make migrate-down` |
| Migration status | `make migrate-status` |
| New migration | `make migrate-create NAME=add_recipes` |
| List all targets | `make help` |

## Endpoints

The contract is `api/openapi.yaml`, also served as JSON at `GET /openapi.json`. The `/v1` catalog endpoints take `?lang=en|es|ru` (default `en`; anything else is a 400):

- `GET /v1/regions`: every region in map order, with `nameEs` and its regional dishes
- `GET /v1/regions/{regionKey}`: one region with every dish's variation there, in dish order
- `GET /v1/dishes`: every dish in dish order, with its variations in region order
- `GET /v1/dishes/{dishKey}`: one dish with its variations
- `GET /v1/dishes/{dishKey}/variations/{regionKey}/recipe`: the recipe of that variation

Before the first content import the lists are `[]` and the details 404; `make seed` loads the design content. Content arrives only through the import CLI, never through the API: see `docs/content-import.md` (bundle format, validation, `ssh <vps> docker compose run --rm -T backend /app/import --source <label> < bundle.json`, dry runs, exit codes). Unknown or malformed keys are a 404 `not_found`. Responses carry a weak `ETag` (`W/"<contentVersion>-<locale>"`, or `W/"<contentVersion>-<locale>-<dish>-<region>"` for a recipe) and `Cache-Control: public, max-age=60, stale-while-revalidate=600`; a matching `If-None-Match` gets a 304.

Errors use `{"error":{"code":"...","message":"..."}}`. Unknown paths, and wrong methods on known ones, return a 404 in that shape.

- `GET /healthz` — liveness + DB ping (1s timeout): 200 `{"status":"ok"}`, or 503 `{"status":"degraded","checks":{"db":"timeout|unavailable"}}`

## Configuration

All config comes from environment variables (with `.env` loaded automatically). See `.env.example` for the full list. The app itself reads:

| Var | Default | Purpose |
|-----|---------|---------|
| `APP_ENV` | required | `local`, `demo` or `prod`; anything else fails at startup |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `DATABASE_URL` | required | Postgres DSN, e.g. `postgres://user:pass@host:5432/db?sslmode=disable` |
| `HTTP_ADDR` | `:8080` | Bind address for the HTTP server |

Inside `docker-compose.yaml`, `DATABASE_URL` is derived from `POSTGRES_*` and points to the compose-internal `database` service.

## CI

See `.github/workflows/ci.yml`. Two jobs run on every PR and push, and a third on pushes to `main` and tags:

1. **lint-and-vet**: `gofmt -l`, `go vet`, golangci-lint, `go build`, `make generate` + `git diff --exit-code` (fails on stale generated code), `make api-lint`.
2. **test**: starts `postgres:16-alpine`, applies migrations with goose, runs `go test -race`.

3. **build-and-push-image**: pushes `ghcr.io/cosmofactory/recetas-backend:sha-<7>` (and `:vX.Y.Z` for a git tag), with the version baked into the binary.

Deploying is not wired up yet (INF-2 in `../specs/`).

## Iteration plans

Numbered plans live in `docs/plans/`. Current: `docs/plans/iteration-01.md`.
