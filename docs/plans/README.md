# Plans

Step-by-step iteration plans for **recetas-backend**. Each plan is both a checklist and a teaching document — it explains *what* to do, *why* this is the Go idiom, and *what concept* is being introduced.

## How to use a plan

1. Read the whole plan before starting — the steps build on each other.
2. Work through the steps in order. Each step has a "Concept" note when something new is introduced.
3. When stuck, ask Claude — but ask with a *specific* question ("why does sqlc require a `sql.NullString` here?") rather than a vague one ("this doesn't work").
4. Mark steps as done by checking the boxes. Do not skip ahead silently — if scope changes, update the plan first.

## Iterations

| #  | Title                                          | Status     | File                          |
|----|------------------------------------------------|------------|-------------------------------|
| 01 | Minimal dish CRUD (create / list / get-by-id)  | Not started | [iteration-01.md](iteration-01.md) |

Future iterations (sketches, not committed yet): update/delete dishes, ingredients module, recipe module, validation layer, structured logging, tests, auth, containerization, deployment.
