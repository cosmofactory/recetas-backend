# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project context

**recetas-backend** is the production backend for a public, non-profit website about Spanish cuisine. It's a modular Go monolith: one binary, one Postgres. `main` is protected and must always be releasable. Deploys to the demo and prod environments come back in INF-2 (see CI/CD below).

Write the code a senior Go engineer would ship. Don't add teaching comments, "Concept" notes, or comparisons with Python/SQLAlchemy/Alembic to code, plans, or commits. The `docs/plans/` documents were written as learning material while the project was a sandbox. Treat them as history: they describe how the current code came about, but their scope limits (e.g. "no tests, no structured logging") no longer apply.

## Commands

All `make` targets load `.env` automatically (copy from `.env.example`).

| What | Command |
|------|---------|
| Full local stack (Postgres + backend, migrations run on start) | `make docker-up` / `make docker-down` / `make docker-logs` |
| Run server bare-metal (needs reachable `DATABASE_URL`) | `make run` |
| CI lint gate: `gofmt -l` + `go vet` + golangci-lint (pinned, `.golangci.yml`) + `go build` | `make lint` |
| Format | `make fmt` |
| All tests | `make test` (`go test -race -count=1 ./...`) |
| Single test | `go test -race -count=1 -run '^TestName$' ./internal/dish/` |
| Regenerate sqlc code after editing `db/queries/*.sql` or migrations | `make sqlc-generate` |
| Migrations | `make migrate`, `make migrate-down`, `make migrate-status`, `make migrate-create NAME=xyz` |

The compose Postgres is published on `127.0.0.1:5435`, which is the `DATABASE_URL` in `.env.example`. Tools: `make goose-install` installs the pinned goose (v3.24.0, same as CI and the Dockerfile). sqlc has to be installed separately.

## Architecture

- **Domain modules** live in `internal/<module>/`. Today there's only a minimal `dish`. The target domain model (regions, dishes with flags, one variation per dish×region, one recipe per variation, slug keys) is defined by the design handoff and described in `../CLAUDE.md`. Build toward that model rather than extending the current `dish` shape. Each module has the same layers:
  - `<module>.go`: domain types with JSON tags. These are the HTTP response shapes.
  - `repository.go`: a `Repository` interface plus a `pgRepository` built on the sqlc `Queries`. It maps sqlc rows to domain types (`toDomain`) and turns `pgx.ErrNoRows` into the module's sentinel `ErrNotFound`.
  - `service.go`: business logic. It depends only on the `Repository` interface, so tests can swap in a fake.
  - `handler.go`: `Register(mux)` attaches routes using Go 1.22 method+path patterns on the stdlib `http.ServeMux` (no third-party router). Handlers check input, map sentinel errors to status codes, log unexpected errors, and return a generic `{"error": "..."}`.
- **Wiring** happens only in `cmd/server/main.go`, which builds config → pgx pool → queries → repo → service → handler. It also handles graceful shutdown on SIGINT/SIGTERM. Modules never import each other.
- **Shared infrastructure** lives in `internal/platform/` (the pgxpool bootstrap) and `internal/config/` (env vars; `.env` is loaded via godotenv when it exists).
- **SQL-first data access.** Migrations in `db/migrations/` (goose) are the schema source of truth. sqlc reads them together with `db/queries/*.sql` and generates `internal/dish/dishdb/`. Never hand-edit generated code. `sqlc.yaml` currently has one `sql` entry that writes everything into `dishdb`. When you add a new module, add a separate entry (with its own queries dir and output package) so generated code stays inside its module.
- **Migrations run at container start.** `docker/entrypoint.sh` runs `goose up` and then `exec`s the server. In production, a migration is applied before the new binary takes over, while the previous container may still be serving. Keep migrations backward-compatible with the running release (expand/contract), and always write a working `-- +goose Down`.

## Conventions

- Module path: `github.com/cosmosfactory/recetas-backend`. Go 1.25; dependencies are limited to pgx/v5 and godotenv. Prefer the stdlib, and justify every new dependency.
- Every I/O function takes `ctx context.Context` as its first argument, and handlers pass `r.Context()`.
- Wrap errors with `fmt.Errorf("doing x: %w", err)` at each layer boundary. Use `errors.Is` against module sentinels instead of comparing strings. Don't panic in request paths.
- Never leak internal error text to clients. 5xx responses get a generic message, and the detail goes to the log.
- Handlers and services never touch `*pgxpool.Pool` or `dishdb` directly. Everything goes through the repository.
- Code must pass `make lint` (gofmt-clean) and `make test` before it's done. CI runs exactly these checks.

## CI/CD (`.github/workflows/ci.yml`)

- On every PR or push, the lint job runs gofmt, vet, golangci-lint and build. The test job starts `postgres:16-alpine`, applies the migrations, and runs `go test -race` with `DATABASE_URL` set. Integration tests can therefore rely on a migrated database through `DATABASE_URL`.
- There is no image push or deploy yet. BE-0 adds the push to `ghcr.io/cosmofactory/recetas-backend` with immutable `sha-<7>` tags; INF-2 adds the deploy to the Contabo VPS.
- Config the app reads: `DATABASE_URL` (required) and `HTTP_ADDR` (default `:8080`).
