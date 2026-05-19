# Iteration 01 — Minimal dish CRUD

## Goal

Stand up a Go HTTP server with three endpoints backed by Postgres:

- `POST /dishes` — create a dish
- `GET  /dishes` — list dishes
- `GET  /dishes/{id}` — get one dish by id

**Out of scope** for this iteration: auth, validation library, structured logging, tests, Docker, update/delete, pagination, search.

## Learning goals

By the end of this iteration you should be comfortable with:

- Go module layout (`cmd/`, `internal/`, why `internal/` is special).
- Packages, exported vs unexported identifiers (capitalization rule).
- Interfaces as **structural** contracts (not declared like `implements` in other languages).
- `context.Context` as the first argument of any function that does I/O.
- Error handling with `errors.Is` / `errors.As` and `%w` wrapping.
- `net/http.ServeMux` with method+path patterns (Go 1.22).
- `pgx/v5` connection pool basics.
- `sqlc` workflow: write SQL → generate Go → call generated functions.
- `goose` workflow: write migration → run `goose up` → version table.
- The repository pattern in idiomatic Go (interface in the consumer package).

A "Concept" note inside a step introduces a Go-specific idea the first time it appears. If you've already met it, you can skim.

---

## Prerequisites

- [ ] Go 1.22 or newer installed (`go version`).
- [ ] PostgreSQL installed locally and running (`psql --version`, `pg_isready`).
- [ ] A Postgres role and database you can connect to. Suggested:
      `createuser -s recetas && createdb -O recetas recetas_dev`.
- [ ] `goose` installed: `go install github.com/pressly/goose/v3/cmd/goose@latest`.
- [ ] `sqlc` installed: `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`.
- [ ] `$GOPATH/bin` (or `$HOME/go/bin`) on your `PATH` so the tools are runnable.

---

## Step 1 — Initialize the Go module

- [ ] `go mod init github.com/cosmosfactory/recetas-backend` (adjust the path to your eventual remote — it's used as the import prefix).
- [ ] Commit the resulting `go.mod`.

**Concept — Go modules.** A *module* is the unit of versioning (one `go.mod` per module). The module path is also the import prefix for every package inside: a file at `internal/dish/service.go` is imported as `github.com/cosmosfactory/recetas-backend/internal/dish`. This is the rough equivalent of a Python package + `pyproject.toml`, but the module path is baked into every import.

## Step 2 — Lay out the directories

Create these (empty `.gitkeep` is fine for now):

```
cmd/server/                 # main.go — process entry point, wiring only
internal/
  config/                   # env loading
  platform/database/        # pgx pool setup
  dish/                     # the dish module: handler + service + repository
db/
  migrations/               # goose .sql migrations
  queries/                  # sqlc input .sql files
```

- [ ] Create the directory tree.
- [ ] Add a `.gitkeep` to each empty dir so git tracks them.

**Concept — `internal/`.** Any package under a directory named `internal/` can only be imported by code rooted at the parent of that `internal/`. It's the Go compiler's built-in "private to this module" mechanism — there is no `public`/`private` keyword on packages, just this directory rule. We use it so that other people (or future modules) cannot accidentally depend on our internals.

**Concept — `cmd/<name>/main.go`.** Convention: every runnable binary lives in `cmd/<binary-name>/`. The `main.go` there is the *only* place wiring (constructing the db pool, building the router, injecting deps into modules) happens. Everything else is a library package.

## Step 3 — Config from environment

- [ ] In `internal/config/config.go`, define a `Config` struct with at least `DatabaseURL string` and `HTTPAddr string` (e.g. `":8080"`).
- [ ] Write a `Load() (Config, error)` that reads from `os.Getenv` and returns an error if a required value is missing.
- [ ] Create `.env.example` at repo root documenting `DATABASE_URL` and `HTTP_ADDR`. Add `.env` to `.gitignore` if not already.

We are deliberately **not** using a config library yet. Reading env vars by hand teaches you what such libraries automate.

## Step 4 — First migration (goose)

- [ ] Write `db/migrations/00001_create_dishes.sql`:

  ```sql
  -- +goose Up
  CREATE TABLE dishes (
      id          BIGSERIAL PRIMARY KEY,
      name        TEXT NOT NULL,
      description TEXT NOT NULL DEFAULT '',
      created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );

  -- +goose Down
  DROP TABLE dishes;
  ```

- [ ] Run it: `goose -dir db/migrations postgres "$DATABASE_URL" up`.
- [ ] Verify in psql: `\d dishes` shows the table; `SELECT * FROM goose_db_version;` shows the version row.

**Concept — goose vs Alembic.** Same idea: a `goose_db_version` table records which migrations have been applied. Differences from Alembic: migrations are plain numbered SQL files (no autogenerate from models — there are no "models" yet at this stage), and there is no branching/merging. The `-- +goose Up` / `-- +goose Down` comments delimit the directions in a single file.

## Step 5 — sqlc setup and first queries

- [ ] Create `sqlc.yaml` at repo root:

  ```yaml
  version: "2"
  sql:
    - engine: postgresql
      schema: db/migrations
      queries: db/queries
      gen:
        go:
          package: dishdb
          out: internal/dish/dishdb
          sql_package: pgx/v5
          emit_json_tags: true
          emit_pointers_for_null_types: true
  ```

- [ ] Create `db/queries/dishes.sql`:

  ```sql
  -- name: CreateDish :one
  INSERT INTO dishes (name, description)
  VALUES ($1, $2)
  RETURNING *;

  -- name: ListDishes :many
  SELECT * FROM dishes ORDER BY created_at DESC;

  -- name: GetDish :one
  SELECT * FROM dishes WHERE id = $1;
  ```

- [ ] Run `sqlc generate`. This creates `internal/dish/dishdb/{db.go,models.go,dishes.sql.go}`.
- [ ] Inspect the generated files — note that `Queries` has methods named after the `-- name:` comments, and `Dish` is a generated struct.
- [ ] Commit the generated code. (Generated code in Go is conventionally committed, unlike many Python projects.)

**Concept — sqlc vs SQLAlchemy.** sqlc reads your schema (the migrations) plus your hand-written queries, and emits a typed Go function per query. There is no ORM, no session, no lazy loading — just functions that take a `context.Context` + parameters and return structs. Closer in spirit to SQLAlchemy Core than ORM, but generated rather than fluent.

**Concept — `:one` / `:many` / `:exec`.** These annotations tell sqlc the shape of the result: a single row, many rows, or no row.

## Step 6 — Database connection package

- [ ] In `internal/platform/database/postgres.go`, write a `NewPool(ctx context.Context, url string) (*pgxpool.Pool, error)` that calls `pgxpool.New` and pings.
- [ ] Add `github.com/jackc/pgx/v5` to dependencies (`go get` runs implicitly on first build, or `go mod tidy`).

**Concept — `context.Context`.** Every I/O-ish function in Go takes `ctx context.Context` as the first argument. It carries cancellation, deadlines, and request-scoped values. Think of it as an explicit "thread of work" handle that lets callers cancel downstream work when the request is dropped. There is no implicit per-request global like Flask's `g` — you pass it.

## Step 7 — The `dish` module

This module has three files. Keep them in this order — the dependencies flow handler → service → repository.

### 7a. Domain type

- [ ] `internal/dish/dish.go` — define a public `Dish` struct used by the service and handler layers (not the generated `dishdb.Dish`). It has the fields `ID int64`, `Name string`, `Description string`, `CreatedAt time.Time`. Add JSON tags.

**Why a separate struct from the generated one?** It decouples your HTTP/service layer from the database schema. If sqlc regenerates and renames a column, only the repository's mapping function changes. Don't propagate generated types across layers.

### 7b. Repository

- [ ] `internal/dish/repository.go` — define an **interface** `Repository` with `Create(ctx, name, description) (Dish, error)`, `List(ctx) ([]Dish, error)`, `Get(ctx, id) (Dish, error)`.
- [ ] In the same file, define a `pgRepository` struct that holds a `*dishdb.Queries`, and write methods that call the generated queries and convert `dishdb.Dish` → `dish.Dish`.
- [ ] Define a sentinel error: `var ErrNotFound = errors.New("dish: not found")`. In `Get`, when the generated query returns `pgx.ErrNoRows`, return `ErrNotFound` (wrap or replace).
- [ ] Export a constructor `func NewPGRepository(q *dishdb.Queries) Repository`.

**Concept — interfaces in Go.** Interfaces are *structural*: any type with the right method set satisfies the interface, no `implements` keyword. Convention: define the interface **in the package that consumes it**, not in the package that implements it. We're putting it in `dish` because the service consumes it; the implementation could later be swapped (in-memory for tests, a different DB) without changing service code.

**Concept — sentinel errors + `errors.Is`.** Instead of catching exceptions by type, callers check `if errors.Is(err, dish.ErrNotFound)`. The `%w` verb in `fmt.Errorf` wraps an error so `errors.Is` can find it through wrapping.

### 7c. Service

- [ ] `internal/dish/service.go` — define `Service` struct holding a `Repository`. Methods `CreateDish`, `ListDishes`, `GetDish` that delegate to the repository. For now there is no business logic; the layer exists so future logic (validation, events, caching) has a clear home.
- [ ] Constructor `func NewService(r Repository) *Service`.

It's fine that the service is "thin" right now. The point of layering is that *when* logic appears, you know exactly where it goes.

### 7d. HTTP handler

- [ ] `internal/dish/handler.go` — define `Handler` holding `*Service`. Constructor `func NewHandler(s *Service) *Handler`.
- [ ] Add a method `Register(mux *http.ServeMux)` that registers:
  - `mux.HandleFunc("POST /dishes", h.create)`
  - `mux.HandleFunc("GET /dishes", h.list)`
  - `mux.HandleFunc("GET /dishes/{id}", h.get)`
- [ ] Implement the three handler methods. `get` extracts the path value with `r.PathValue("id")` and parses it with `strconv.ParseInt`. `create` decodes JSON with `json.NewDecoder(r.Body).Decode(&payload)`.
- [ ] Add a small `writeJSON(w, status, v)` helper and a `writeError(w, status, msg)` helper (in this file is fine; promote to `internal/platform/httpx` later if reused).
- [ ] Map errors to status codes: `ErrNotFound` → 404, bad JSON / bad id → 400, anything else → 500.

**Concept — `net/http.ServeMux` (Go 1.22).** Before 1.22 you needed chi/gorilla for `POST /dishes`-style routing. Now the stdlib mux supports method-prefixed patterns and `{path}` parameters via `r.PathValue`. Good enough for this iteration; reach for chi only when you need middleware chains, sub-routers, or regex constraints.

**Concept — `http.HandlerFunc` vs methods.** A method like `h.create` has the right signature `func(http.ResponseWriter, *http.Request)`, so it can be passed directly to `HandleFunc`. The handler struct is how you inject dependencies (the service) — there's no DI container.

## Step 8 — Wire it all up in `cmd/server/main.go`

- [ ] In `main()`:
  1. Build a root `context.Context` with `signal.NotifyContext(ctx, os.Interrupt)` so Ctrl-C cleans up.
  2. `cfg, err := config.Load()`.
  3. `pool, err := database.NewPool(ctx, cfg.DatabaseURL)`; `defer pool.Close()`.
  4. `queries := dishdb.New(pool)`.
  5. `repo := dish.NewPGRepository(queries)`.
  6. `svc := dish.NewService(repo)`.
  7. `mux := http.NewServeMux()`; `dish.NewHandler(svc).Register(mux)`.
  8. Construct an `http.Server{Addr: cfg.HTTPAddr, Handler: mux}` and start it in a goroutine.
  9. Block on `<-ctx.Done()`, then call `srv.Shutdown` with a fresh 5s timeout context.
- [ ] Run: `go run ./cmd/server`. Server should log "listening on :8080".

**Concept — graceful shutdown.** A long-running Go server typically: starts in a goroutine, the main goroutine waits on signals via context cancellation, then calls `Shutdown` which lets in-flight requests finish. This is a recurring Go pattern — get it right once and reuse it.

**Concept — goroutines and `go func()`.** A goroutine is a lightweight thread managed by the Go runtime. We start the HTTP server in one so `main` can wait on signals in parallel. We'll meet channels and `sync` primitives in later iterations.

## Step 9 — Try it

- [ ] `curl -X POST localhost:8080/dishes -d '{"name":"Paella","description":"Valencian rice dish"}' -H 'content-type: application/json'`
- [ ] `curl localhost:8080/dishes`
- [ ] `curl localhost:8080/dishes/1`
- [ ] `curl -i localhost:8080/dishes/99999` (expect 404)

## Step 10 — Commands to record in CLAUDE.md

Once this iteration runs, append the canonical commands to the `## Commands` section of `CLAUDE.md`:

- `go run ./cmd/server` — run the server.
- `go build ./...` — compile everything.
- `go vet ./...` — static checks.
- `goose -dir db/migrations postgres "$DATABASE_URL" up` / `down` — migrate.
- `goose -dir db/migrations create <name> sql` — new migration.
- `sqlc generate` — regenerate `internal/dish/dishdb/`.

---

## Done definition

- All steps checked off.
- Three endpoints work end-to-end against a real local Postgres.
- `go vet ./...` is clean.
- `CLAUDE.md` commands section is populated.

## Stretch (optional, only if everything above is comfortable)

- Add `golangci-lint` and fix anything it complains about.
- Replace the inline `writeJSON` helper with a `internal/platform/httpx` package and reuse from the handler.
- Add a `/healthz` endpoint that pings the pool.

Leave anything bigger (tests, validation, more endpoints) for iteration 2 — scope discipline matters more than feature count on a learning project.
