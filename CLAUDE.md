# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project context

**recetas-backend** is the backend for a personal, non-profit website about Spanish cuisine. It is a pet project owned by a single developer.

## Learning context — important

The owner is an experienced backend developer with a **Python / SQLAlchemy / Alembic** background. This project is deliberately being used as a **sandbox to learn Go**. When assisting:

- Prefer idiomatic Go over patterns familiar from Python (no "fake classes", no over-abstraction).
- When introducing a Go-specific concept for the first time (interfaces, context.Context, error wrapping, struct tags, package layout conventions, goroutines, channels, build tags, etc.), briefly explain *what it is* and *why it's used here* — one or two sentences inline, no callout blocks. Do not re-explain the same concept later.
- Where a Go idiom differs sharply from the Python/SQLAlchemy equivalent, name the comparison once (e.g. "sqlc plays the role Alembic-generated SQLAlchemy Core code does for you").
- Favor stdlib and small focused libraries over heavyweight frameworks.

## Architecture intent

- **Modular monolith** with strict layer separation. Each domain module (e.g. `dish` now; `recipe`, `ingredient` later) lives under `internal/<module>/` and contains its own handler / service / repository layers. A `dish` is the named cultural concept (e.g. Paella); a `recipe` (future) is one specific way to prepare a dish — one-to-many.
- Shared infrastructure (db connection, config, logging) lives under `internal/platform/`.
- The `cmd/server/` entry point is the only place where modules are wired together; modules do not import each other directly.

## Stack (chosen for iteration 1)

- Go **1.22+** — uses the new `net/http.ServeMux` method+path routing; no third-party router.
- **PostgreSQL** — installed locally (no Docker for now).
- **sqlc** — SQL-first, generates type-safe Go from `.sql` files. Plays the SQLAlchemy-Core role.
- **goose** — migration tool. Plays the Alembic role: numbered up/down migrations with a version table.
- **pgx/v5** as the underlying Postgres driver (sqlc generates code against it).

## Iterative roadmap

Work proceeds in numbered iterations. Plans for each iteration live in `docs/plans/`. **Always read the relevant iteration plan before writing code** — it sets the scope and the learning goals for that iteration.

- Iteration 1: HTTP server with `POST /dishes`, `GET /dishes`, `GET /dishes/{id}`. No auth, no containerization. See `docs/plans/iteration-01.md`.

## Commands

The project is currently at the start of iteration 1 — commands will be added here as they are introduced by the plan.

## Conventions

- Module path: `github.com/cosmosfactory/recetas-backend` (adjust if a different remote is chosen).
- Errors are returned, not panicked. Wrap with `fmt.Errorf("doing X: %w", err)` to preserve the chain.
- HTTP handlers return JSON; error responses use a single consistent shape (defined in iteration 1).
- Database access goes through the module's repository interface — handlers and services never touch `*pgxpool.Pool` directly.
