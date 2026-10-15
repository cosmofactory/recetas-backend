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
| CI lint gate: `gofmt -l` + `go vet` + golangci-lint (pinned, `.golangci.yml`; enforces the clean-code limits: `funlen` 30 lines, revive `argument-limit` 4 and `max-control-nesting` 2) + `go build` | `make lint` |
| Format | `make fmt` |
| All tests | `make test` (`go test -race -count=1 ./...`; integration tests skip) |
| Integration tests | `make test-integration` (compose Postgres up, creates and migrates `recetas_test`, runs all tests with `TEST_DATABASE_URL` set) |
| Single test | `go test -race -count=1 -run '^TestName$' ./db/` |
| Regenerate all generated code (CI fails if it's stale) | `make generate` (= `api-generate` + `sqlc-generate`) |
| Regenerate `internal/api/apigen` after editing `api/openapi.yaml` | `make api-generate` (oapi-codegen v2.8.0, pinned via the go.mod `tool` directive) |
| Lint the OpenAPI spec | `make api-lint` (vacuum, pinned; fails on errors, not warnings) |
| Regenerate sqlc code after editing `db/queries/<module>/*.sql` or migrations | `make sqlc-generate` (pinned sqlc via `go run`) |
| Migrations | `make migrate`, `make migrate-down`, `make migrate-status`, `make migrate-create NAME=xyz` |

The compose Postgres is published on `127.0.0.1:5435`, which is the `DATABASE_URL` in `.env.example`. Tools: `make goose-install` installs the pinned goose (v3.24.0, same as CI and the Dockerfile). oapi-codegen, sqlc and vacuum need no install: the make targets run pinned versions.

## Architecture

- **Domain modules** live in `internal/<module>/`. Today there's one, `catalog` (regions, dishes with flags, one variation per dish×region, one recipe per variation, slug keys; the model is defined by the design handoff and `../CLAUDE.md`, the schema in `docs/schema.md`). Besides its sqlc package `catalogdb` and the test-only fixture loader `catalogtest` (`Load(t, tx)` inserts one complete catalog in en/es/ru), it has these layers:
  - `catalog.go`: domain types (no JSON tags; the HTTP shapes are the generated `apigen` types) and the `Repository` interface. `errors.go`: the sentinel `ErrNotFound` and `*MissingTranslationError` (entity, key, locale).
  - `repository.go` + `mapping.go`: `NewPGRepository(catalogdb.DBTX)` on the sqlc `Queries`, mapping rows to domain types and `pgx.ErrNoRows` to `ErrNotFound`. Queries LEFT JOIN the translations (plus the `es` row a second time for `nameEs`); a NULL means a missing row, returned as `*MissingTranslationError` with no fallback (D-BE3-3). Query counts are fixed: regions 2, dishes 2 (all variations in one query), recipe 3. `ContentVersion` is 0 before the first import.
  - `service.go` + `memo.go`: reads the content version on every call and memoises the assembled region and dish lists per (locale, version); storing a newer version evicts older ones and failed loads aren't stored. `Region`/`Dish` are answered from those lists, `Recipe` goes to the repository.
  - `handler.go` + `response.go` + `etag.go`: `NewHandler(svc, logger)` implements `apigen.StrictServerInterface`. It defaults a missing `lang` to `en`, answers a key that fails the contract's slug pattern with 404 before any query, maps `ErrNotFound` to 404 `not_found` and anything else to a generic 500 `internal` (logged once at Error with `err`), and sets `ETag` (`W/"<version>-<locale>"`, recipes `W/"<version>-<locale>-<dish>-<region>"`) and `Cache-Control: public, max-age=60, stale-while-revalidate=600`.
- **API contract.** `api/openapi.yaml` (OpenAPI 3.0.3) is the source of truth for the public `/v1` API (spec-first: change the spec, then `make api-generate`). oapi-codegen writes models, the std-http router and the strict-server interface to `internal/api/apigen/`; never hand-edit it. `internal/api` adapts that interface: `Register(mux, impl, logger)` wires the catalog operations, `GET /openapi.json` (the embedded spec converted to JSON once by `SpecJSON`; `main.go` fails fast if it doesn't parse) and a `/` catch-all that returns the contract 404 (so a wrong method on a known path is 404, not 405). It validates the `lang` enum (the generated wrapper doesn't), maps parameter-binding errors to 400 `invalid parameter <name>` and handler errors to a logged 500. The catalog routes run through `httpx.ConditionalGET`, which buffers the response and turns a 200 into a bodyless 304 (keeping `ETag` and `Cache-Control`) when an `If-None-Match` value matches its ETag (weak comparison, comma lists on any number of header lines, `*`); the strict interface can't see request headers. `api.Stub` (501 for every operation) lives in a test file for the adapter's own tests. `/healthz` and `/openapi.json` are in the spec but excluded from codegen (`exclude-operation-ids`); they have their own handlers.
- **Error shape.** Every error response is `{"error":{"code":"...","message":"..."}}` (the spec's `Error` schema), written with `httpx.WriteError`. Codes: `bad_request`, `not_found`, `payload_too_large`, `internal`, `not_implemented`.
- **Wiring** happens only in `cmd/server/main.go`, which builds config → pgx pool → `catalog.NewPGRepository` → `catalog.NewService` → `catalog.NewHandler` → `api.Register`. It also handles graceful shutdown on SIGINT/SIGTERM. Modules never import each other.
- **Shared infrastructure** lives in `internal/platform/` (`database`: pgxpool bootstrap; `logging`: `slog` JSON logger; `health`: `GET /healthz`, a 1s DB ping behind a `Pinger` interface, used by the Docker/compose healthcheck, the frontend's `/readyz` and uptime checks; `httpx`: hardened `http.Server` and the middleware chain wired in `main.go`: 64 KiB body limit (outermost, so `MaxBytesReader` sees net/http's own writer and a 413 closes the connection) → access log → panic recovery → 12s handler deadline (below the 15s `WriteTimeout`, so the error response still gets written) → `Routed(mux)`, which hands the matched route back to the access log even after a middleware clones the request) and `internal/config/` (env vars parsed by the pure `FromEnv(getenv)`; `Load()` first loads `.env` via godotenv when it exists).
- **SQL-first data access.** Migrations in `db/migrations/` (goose) are the schema source of truth. `sqlc.yaml` has one `sql` entry per module: it reads the migrations plus `db/queries/<module>/*.sql` and writes `internal/<module>/<module>db/` (today only `catalog` → `catalogdb`). Never hand-edit generated code. A new module gets its own entry, queries dir and output package, so generated code stays inside its module.
- **Migrations run at container start.** `docker/entrypoint.sh` runs `goose up` and then `exec`s the server. In production, a migration is applied before the new binary takes over, while the previous container may still be serving. Keep migrations backward-compatible with the running release (expand/contract), and always write a working `-- +goose Down`.

## Conventions

- Module path: `github.com/cosmofactory/recetas-backend`. Go 1.25; dependencies are limited to pgx/v5, godotenv, and for the API contract kin-openapi (spec → JSON) and oapi-codegen/runtime (used by generated code), and goose (test-only: the migration round-trip test in `db/`). Prefer the stdlib, and justify every new dependency.
- Every I/O function takes `ctx context.Context` as its first argument, and handlers pass `r.Context()`.
- Wrap errors with `fmt.Errorf("doing x: %w", err)` at each layer boundary. Use `errors.Is` against module sentinels instead of comparing strings. Don't panic in request paths.
- Log with `log/slog` only (JSON to stdout). The logger is built in `main.go` and injected through constructors; never use the `log` package or `slog`'s default logger. Unexpected errors log at Error with an `err` attr; client errors (4xx) don't log at Error.
- Never leak internal error text to clients. 5xx responses get a generic message, and the detail goes to the log.
- Panics are recovered by `httpx.Recover` into a generic 500 (stack goes to the log). The access log records the route pattern, never the raw path, query string or body. Handlers that decode a body map `*http.MaxBytesError` to 413.
- Handlers and services never touch `*pgxpool.Pool` or a `<module>db` package directly. Everything goes through the repository.
- Integration tests use `testdb.Tx(t)` (`internal/platform/testdb`), a transaction rolled back in `t.Cleanup`; they skip without `TEST_DATABASE_URL`. They never read `DATABASE_URL`, because `make test` loads `.env`, which points it at the dev DB. Don't assume an empty table: measure deltas or look up the rows you created.
- Code must pass `make lint` (gofmt-clean) and `make test` before it's done. CI runs exactly these checks.

## CI/CD (`.github/workflows/ci.yml`)

- On every PR or push, the lint job runs gofmt, vet, golangci-lint, build, the generated-code drift check (`make generate`, then `git diff --exit-code` and no untracked files) and `make api-lint`. The test job starts `postgres:16-alpine`, applies the migrations, and runs `go test -race` with `DATABASE_URL` and `TEST_DATABASE_URL` set to it, so integration tests run there instead of skipping.
- On a push to `main` (or a tag), after lint and test pass, the image is pushed to `ghcr.io/cosmofactory/recetas-backend` with the immutable tag `sha-<7>` (plus `vX.Y.Z` for a git tag). There is no `:latest`. The binary gets the same value via `-X main.version` and logs it at startup. There is no deploy yet; INF-2 adds it.
- In CI (`CI` set) `testdb` fails instead of skipping when `TEST_DATABASE_URL` is missing, so integration tests can't silently stop running.
- Config the app reads: `APP_ENV` (required, `local|demo|prod`), `LOG_LEVEL` (`debug|info|warn|error`, default `info`), `DATABASE_URL` (required) and `HTTP_ADDR` (default `:8080`). The CI test job sets `APP_ENV=local`.
