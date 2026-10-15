# recetas-backend

Backend for Asadorov, a non-profit atlas of Spanish cooking. Modular Go monolith — one binary, one Postgres.

## Stack

- **Go 1.25** — `net/http.ServeMux` with method+path routing, no third-party router.
- **PostgreSQL 16** — pgx/v5 driver.
- **[sqlc](https://sqlc.dev/)** — SQL-first, generates type-safe Go from `.sql` files.
- **[goose](https://github.com/pressly/goose)** — numbered up/down migrations.
- **Docker + Docker Compose** for local dev.
- **GitHub Actions** for CI.

## Layout

```
cmd/server/            # entry point — the only place modules are wired together
internal/
  config/              # env loading
  platform/database/   # pgxpool bootstrap
  dish/                # dish domain module
    dish.go            # domain model
    repository.go      # Repository interface + pg implementation
    service.go         # business logic
    handler.go         # HTTP handlers
    dishdb/            # sqlc-generated code (do not edit)
db/
  migrations/          # goose .sql files
  queries/             # sqlc .sql inputs
docker/
  entrypoint.sh        # runs migrations then execs the server
```

Domain modules never import each other — wiring happens only in `cmd/server/main.go`.

## Prerequisites

- Go 1.25+
- Docker + Docker Compose (for `make docker-*` targets)
- Optional local tools (only if you want to run outside Docker):
  - `make goose-install` — pinned goose CLI
  - [sqlc](https://docs.sqlc.dev/en/latest/overview/install.html) — for regenerating `dishdb`

## Quick start — Docker (recommended)

```bash
cp .env.example .env
make docker-up          # builds image, starts postgres + backend
curl -i http://localhost:8080/dishes
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
| Test | `make test` |
| Build binary → `bin/server` | `make build` |
| Regenerate sqlc code | `make sqlc-generate` |
| Apply migrations | `make migrate` |
| Revert last migration | `make migrate-down` |
| Migration status | `make migrate-status` |
| New migration | `make migrate-create NAME=add_recipes` |
| List all targets | `make help` |

## Endpoints (iteration 1)

- `POST /dishes` — create a dish (`{name, description}`)
- `GET /dishes` — list all dishes
- `GET /dishes/{id}` — get one dish
- `GET /dishes/total/count` — total dish count

## Configuration

All config comes from environment variables (with `.env` loaded automatically). See `.env.example` for the full list. The only two the app itself reads:

| Var | Default | Purpose |
|-----|---------|---------|
| `DATABASE_URL` | required | Postgres DSN, e.g. `postgres://user:pass@host:5432/db?sslmode=disable` |
| `HTTP_ADDR` | `:8080` | Bind address for the HTTP server |

Inside `docker-compose.yaml`, `DATABASE_URL` is derived from `POSTGRES_*` and points to the compose-internal `database` service.

## CI

See `.github/workflows/ci.yml`. Two jobs run on every PR and push:

1. **lint-and-vet**: `gofmt -l`, `go vet`, golangci-lint, `go build`.
2. **test**: starts `postgres:16-alpine`, applies migrations with goose, runs `go test -race`.

Publishing images to GHCR and deploying are not wired up yet (BE-0 and INF-2 in `../specs/`).

## Iteration plans

Numbered plans live in `docs/plans/`. Current: `docs/plans/iteration-01.md`.
