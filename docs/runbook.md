# Backend runbook

Operating recetas-backend on demo and prod. INF-6's `recetas-infra/RUNBOOK.md` links here for the backend; deploys, backups and restores live there.

Each environment is a compose project `recetas-<env>` in `/srv/recetas/<env>`, with the services `database`, `backend` and `frontend`, and `<env>` is `demo` or `prod`. Demo's stack is `recetas-infra/envs/demo/compose.yaml` (INF-2): the networks `app` (frontend and backend) and `data` (`internal: true`, backend and database only), no published ports. The VM itself doesn't exist yet (INF-0), and prod is planned in INF-5. The VM scripts live in `/srv/recetas/scripts/`. Every command runs on the VM unless it starts with `ssh <vps>`.

**Who runs what.** `ssh <vps>` is the owner logging in as `admin` with the owner's unrestricted key. `/srv/recetas/*` belongs to `deploy` (`.env` is 0600), so every command runs as `deploy`: interactively `ssh <vps>`, then `sudo -iu deploy`; as a one-liner from your machine, `ssh <vps> "sudo -u deploy bash -c '<command>'"` (stdin passes through sudo, so a bundle can follow with `< bundle.json`). This repo's `demo` deploy key is restricted on the VM to `ssh-gate.sh backend demo`: it can run `deploy-service.sh demo backend sha-<7>` and nothing else, so imports, `goose` and every other command below need the owner's login.

**Always name the project and both env files.** A bare `docker compose` run in `/srv/recetas/<env>` takes the directory name (`demo`, `prod`) as the project, so it talks to a different, empty stack: an import there would go into a fresh, empty database and report success. Demo's compose file sets `name: recetas-demo` as well, but the commands here don't rely on it. Demo reads two env files: `.env` (0600; the secrets, written by recetas-infra's `Demo` workflow from its `DEMO_*` repository secrets on every run, so edits on the VM are overwritten) and `tags.env` (`BACKEND_IMAGE_TAG`, `FRONTEND_IMAGE_TAG`, written only by `deploy-service.sh`). Passing `--env-file` replaces the default `.env`, so name both; without `tags.env` the image tag is missing and compose refuses to start.

**Prod** is planned to take its tags from `versions.env` (INF-5, D-INF5-1); INF-5 still has to settle between that and `tags.env`, which `deploy-service.sh` already accepts for prod. Follow whatever INF-5 settles on.

Set `dc` once per shell; every command below uses it:

```bash
ssh <vps>
sudo -iu deploy
cd /srv/recetas/<env>
dc="docker compose -p recetas-demo --env-file .env --env-file tags.env"        # demo
dc="docker compose -p recetas-prod --env-file .env --env-file versions.env"   # prod (as planned in INF-5)
$dc ps          # backend should be "Up … (healthy)"
cat tags.env    # demo: the deployed image tags
```

## Deploys

A push to `main` deploys demo: after the image job pushes `sha-<7>`, CI's `deploy-demo` job runs `/srv/recetas/scripts/deploy-service.sh demo backend sha-<7>` over ssh and then checks `https://demo.asadorov.es/` and `/readyz` (see the README, Deploy). The script waits up to 180 s for the new container to turn healthy; if it doesn't, it brings the previous tag back up and the job fails. The failed container's last log lines go only to `/srv/recetas/demo/deploy.log` (the job log is public), so read them there:

```bash
tail -n 80 /srv/recetas/demo/deploy.log
```

Don't start a failed image by hand with `$dc run`: its entrypoint applies that image's migrations. Prod deploys follow INF-5.

## Logs

The server writes one JSON object per line to stdout. `$dc logs` keeps the last ~30 MB per service (json-file rotation, 10 MB × 3); Logfire keeps them longer, with the traces.

```bash
$dc logs --tail 200 backend
$dc logs --since 15m -f backend
$dc logs --since 1h backend | grep '"level":"ERROR"'
$dc logs --since 1h backend | grep '"status":5'        # 5xx access lines
```

What the lines are:

| `msg` | When | Fields worth reading |
|-------|------|----------------------|
| `load config failed` | boot, at ERROR, then exit 1: an env var is missing or invalid | `err` names the variable |
| `telemetry setup failed` | boot, at ERROR, then exit 1: the OTLP exporters couldn't be built | `err` |
| `starting` | boot, before the DB is touched | `version` (the image's `sha-<7>`), `app_env` |
| `telemetry disabled` | boot without `LOGFIRE_TOKEN` | |
| `server starting` | listening | `addr`, `log_level` |
| `request` | every request (a 2xx `/healthz` only at `LOG_LEVEL=debug`) | `route` (the pattern, never the raw path), `status`, `duration_ms`, `request_id`, `trace_id` |
| `catalog request failed` | a 500 from a catalog handler, at ERROR | `op`, `err` (the detail the client never sees) |
| `api handler failed` | a 500 from the API adapter (an error the handler didn't map), at ERROR | `route`, `err` |
| `panic recovered` | a handler panicked; the client got a 500, at ERROR | `panic`, `stack` |
| `health check failed` | a failing `/healthz`, at WARN | `err` |
| `server exited with error` | at ERROR: startup failed (`database: …`, `server: listen …`) or the drain didn't finish in time | `err` |
| `shutting down` / `server stopped` | SIGTERM (a redeploy or `$dc stop`) | |
| `flush telemetry failed` | shutdown couldn't send the last batch | `err` |

Lines before the JSON (`entrypoint: applying migrations`, `goose: …`, `entrypoint: launching /app/server`) come from the entrypoint on stderr.

**Following one request.** Every line of a request, the access line included, carries the same `request_id` (the server's, since the frontend doesn't send `X-Request-ID`). With telemetry on, lines also carry `trace_id`, which the frontend propagates through `traceparent`, so it joins the frontend's logs, the backend's and the SQL spans.

```bash
$dc logs backend | grep '"request_id":"<id>"'
$dc logs backend | grep '"trace_id":"<trace-id>"'
```

**Logfire.** With `LOGFIRE_TOKEN` set, traces and the same log records go to Logfire (project per INF-3, D-INF3-1). Spans are named by route (`GET /v1/dishes/{dishKey}`), with `query <Name>` children named after the sqlc query; search a `trace_id` there to see the whole request. `/healthz` makes no span.

## Health

```bash
$dc exec backend wget -qO- http://127.0.0.1:8080/healthz; echo
docker inspect --format '{{.State.Health.Status}}' "$($dc ps -q backend)"
curl -fsS https://<domain>/readyz       # from anywhere: the frontend asks /healthz for you
```

`/healthz` pings Postgres with a 1s timeout and returns `Cache-Control: no-store`:

| Response | Meaning |
|----------|---------|
| 200 `{"status":"ok"}` | The process serves and the database answers. |
| 503 `{"status":"degraded","checks":{"db":"unavailable"}}` | The ping failed: Postgres down, unreachable or refusing connections. See [Database down](#database-down). |
| 503 `{"status":"degraded","checks":{"db":"timeout"}}` | No answer within 1s: Postgres overloaded, or every pool connection busy. See [Pool exhausted](#pool-exhausted). |
| no answer | The process isn't running or is wedged: `$dc ps`, then see [Backend restart-looping](#backend-restart-looping). |

The container healthcheck (`$dc ps`) is the same request; in the repo's compose file it runs every 10s and 5 failures in a row mark the container unhealthy; demo's runs every 10s with a 3s timeout, and 3 failures in a row mark it unhealthy. The frontend's `/readyz` is 200 or 503 with no body.

## Import content

The full contract is [`content-import.md`](content-import.md). Dry run first, every time, and read the `deleted` counts: anything missing from the bundle is deleted.

`docker compose run backend` goes through the image's entrypoint, which runs `goose up` first. So even a `--dry-run` import applies any pending migrations when the compose tag is ahead of the database.

Run it as `deploy` from the owner's login (see "Who runs what" at the top); the repo's deploy key can't. To reseed demo with the design content, use recetas-infra's `scripts/reseed-demo.sh` (its RUNBOOK.md, Demo → Reseed).

```bash
# From your machine; the bundle goes over stdin. Demo:
ssh <vps> "sudo -u deploy bash -c 'cd /srv/recetas/demo && docker compose -p recetas-demo --env-file .env --env-file tags.env run --rm -T backend /app/import --source pipeline-<run-id> --dry-run'" < bundle.json
# Prod (as planned in INF-5):
ssh <vps> "sudo -u deploy bash -c 'cd /srv/recetas/prod && docker compose -p recetas-prod --env-file .env --env-file versions.env run --rm -T backend /app/import --source pipeline-<run-id> --dry-run'" < bundle.json
echo "exit=$?"
```

If the summary is what you expect, run the same command without `--dry-run`. Then check the version and that the site serves it:

```bash
$dc exec database sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT id, source, imported_at FROM content_versions ORDER BY id DESC LIMIT 3"'
$dc exec backend wget -qSO /dev/null 'http://127.0.0.1:8080/v1/dishes?lang=en' 2>&1 | grep -i etag   # W/"<version>-en"
```

Pages pick up the new content within about a minute (the frontend's list cache and `max-age=60`).

## Roll back an image

> **Prod is pending INF-5.** Per D-INF5-1 (recommended, outcome TBD), prod runs the tags pinned in `recetas-infra/envs/prod/versions.env`, and a rollback is a `git revert` of the versions PR, merged and deployed by `deploy-prod.yml`. Follow INF-5.

1. Find the previous good tag: the `<old> -> <new>` line of the deploy job's log (or `deploy.log` on the VM), `docker image ls ghcr.io/cosmofactory/recetas-backend` on the VM, or for prod the `versions.env` history in recetas-infra. Images are pulled again when they've been pruned.
2. Check the migrations between the two tags (`git diff --stat <old-sha> <new-sha> -- db/migrations` in recetas-backend; the `sha-<7>` tags are image tags, not git tags). With none, roll back the image and stop there.
3. **Demo:** re-run only the `Deploy Demo` job of the older commit's CI run (Actions → that run → the `Deploy Demo` job → Re-run this job; not "Re-run all jobs", which would rebuild and re-push the same `sha-` tag), or as the owner run `/srv/recetas/scripts/deploy-service.sh demo backend sha-<old>` on the VM (as `deploy`). Either way it's the same script, with the same health wait and rollback. **Prod:** `git revert` the versions PR in recetas-infra, open a PR and merge it (needs the `prod` approval).
4. Verify:

```bash
$dc ps backend                                                # healthy
$dc logs backend | grep '"msg":"starting"' | tail -1          # "version":"sha-<old>"
curl -fsS https://<domain>/readyz && curl -fsS https://<domain>/en/dishes/tortilla >/dev/null
```

An older image starts fine on a newer schema: its entrypoint's `goose up` finds no pending migrations (`no migrations to run. current version: N`) and leaves the newer ones in place. Every migration is expand/contract-safe against the previous release (B3), so the older binary runs on it. That's why a migration is never rolled back automatically.

## Roll back a migration

Only when the schema itself is the problem; prefer a forward fix (a new migration) in every other case.

`goose down` reverts the latest applied migration and needs its file, so it runs from an image that still contains it: the **newer** image. Stop the newer server first when the down step would break it (see the table).

> **`down` destroys data. Back up first** (prod: the INF-5 backup job, run by hand).
> - Going down past **00008** drops `content_versions`: the import history and checksums.
> - Going below 00008 into **00007–00004** drops the catalog content itself.

```bash
$dc run --rm --entrypoint sh backend -c 'goose -dir /app/migrations postgres "$DATABASE_URL" status'
# After step 3 of "Roll back an image", the compose tag is the old one, so name the newer image explicitly:
BACKEND_IMAGE_TAG=sha-<new> $dc run --rm --entrypoint sh backend \
  -c 'goose -dir /app/migrations postgres "$DATABASE_URL" down'
```

`--entrypoint sh` skips the image's `goose up`, and the single quotes make `$DATABASE_URL` expand inside the container. `BACKEND_IMAGE_TAG` is the compose variable from `tags.env`; a variable set in the shell wins over the env files, and this one-off run doesn't change `tags.env`. Repeat `down` once per migration, newest first.

Before taking down 00008, save its rows. The dump includes the sequence position:

```bash
$dc exec -T database sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --data-only -t content_versions' > content_versions-$(date -u +%F).sql
```

What each Down costs (the BE-6 migration review; every Down passes the round-trip test in `db/`):

| Migration | Down | Order and aftermath |
|-----------|------|---------------------|
| `00008_create_content_versions` | **Destroys data:** drops `content_versions`, the import history and checksums. Back up and save the rows first. | **Roll the image back first.** Today's server reads the content version on every catalog request, so dropping the table under it turns every request into a 500. After rolling forward the table is empty: `ContentVersion` is 0 and every ETag becomes `W/"0-<locale>"`. The next import that changes content gets id 1 again and reissues ETags clients have already seen for other content (a client holding one gets a wrong 304). Re-importing the same bundle doesn't help: the catalog rows still match it, so the import prints `no changes (content version 0 is current)` and records nothing. **Restore the saved rows before any import** (below). |
| `00007_create_recipes` … `00004_create_regions` | **Destroys data:** drops the catalog tables and their content. Back up first. | Requires 00008 down first (row above). The release that needed them must be rolled back first. Rolling forward recreates empty tables. |
| `00003_create_locales` | drops `locales` (fixed reference rows, recreated by the Up) | As above. |
| `00002_drop_legacy_dishes` | recreates the empty iteration-1 `dishes` table | The one contract step (D-BE0-1). Nothing reads the legacy table; there's no reason to go below 00003. |
| `00001_create_dishes` | drops the legacy table | Never needed. |

**Roll forward** by deploying the newer image again: its entrypoint re-applies what's pending. Then, in this order:

1. **If you took down 00008**, restore its rows before any import, so the history comes back and the next version continues after the old highest id:

   ```bash
   $dc exec -T database sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1' < content_versions-<date>.sql
   ```

   Without the saved file, set the sequence past the old highest id (from the backup) instead: `SELECT setval('content_versions_id_seq', <old max id>)`. The history is lost, but no ETag is reissued.
2. **If you went below 00008** (00007 or lower), re-import the last bundle (on demo `testdata/content.design.json`), dry run first. It recreates the catalog and records a new version.
3. Check health and an ETag (see [Import content](#import-content)).

## Failure modes

### Backend restart-looping

- **Symptoms:** `$dc ps backend` shows `Restarting` or a climbing restart count; `/readyz` 503.
- **Check:** the last lines before each exit. `restart: unless-stopped` keeps restarting it, so look at what repeats:

```bash
$dc logs --tail 80 backend
```

| Last lines | Cause | Fix |
|------------|-------|-----|
| `entrypoint: DATABASE_URL is required` | The compose file or `.env` lost the DSN. | Fix it (demo: see [Changing configuration](#changing-configuration)). |
| `entrypoint: applying migrations`, then a `goose` error | A migration failed. goose runs each in its own transaction, so the database stays at the last good version, and the server never started. | Roll the image back (an older image starts on the current schema), then fix forward with a new migration. |
| `load config failed` with `APP_ENV "…" is invalid` or `APP_ENV is required` | `APP_ENV` isn't `local`, `demo` or `prod`. | Fix the compose file. |
| `load config failed` with `OTEL_EXPORTER_OTLP_ENDPOINT is required when LOGFIRE_TOKEN is set` (or `OTEL_EXPORTER_OTLP_ENDPOINT must be an https URL with a host`) | A token without a valid endpoint. | Set the endpoint, or blank `LOGFIRE_TOKEN`. |
| `load config failed` with `DB_MAX_CONNS "…" is invalid` (or `DB_MIN_CONNS`) | Out of range: max 1–100, min 0–max. | Fix the value or blank it for the default. |
| `telemetry setup failed` | The exporters couldn't be built from the endpoint. | Check `OTEL_EXPORTER_OTLP_ENDPOINT`. |
| `server exited with error` with `database: …` or `server: listen …` | A malformed DSN, or the port is taken. | Fix `DATABASE_URL` or `HTTP_ADDR`. |

A fix to `.env` or the compose file takes effect when the container is recreated (`$dc up -d backend`; a plain `restart` keeps the old values).

#### Changing configuration

On demo, never hand-edit `.env` or `compose.yaml` on the VM: recetas-infra's `Demo` workflow writes both on every run. Values that are secrets (`POSTGRES_*`, `LOGFIRE_TOKEN`, `OTEL_EXPORTER_OTLP_ENDPOINT`) are its `DEMO_*` repository secrets: change the secret, then run `Demo` from recetas-infra's Actions tab. Everything else (`DB_MAX_CONNS`, `DB_MIN_CONNS`, `LOG_LEVEL`, memory limits) is a recetas-infra PR to `envs/demo/compose.yaml`; demo's compose doesn't pass those today, so the server uses its defaults. The `Demo` run recreates what changed. Prod follows INF-5.

### Database down

- **Symptoms:** `/healthz` 503 `{"db":"unavailable"}`, the container turns unhealthy, `/readyz` 503; every `/v1` request is a 500 `{"error":{"code":"internal","message":"internal error"}}`, and pages show the frontend's error page.
- **Logs:** `health check failed` at WARN and `catalog request failed` at ERROR, with `err` such as `load content version: latest content version: failed to connect to …: connection refused` (or `hostname resolving error` when the `database` container is gone).
- **Check and fix:**

```bash
$dc ps database
$dc logs --tail 100 database
df -h / /var/lib/docker                       # a full disk stops Postgres; see Disk
$dc up -d database                            # restart it if it stopped
```

The backend needs no restart: the pool reconnects on the next request, and health turns green within one check interval.

### Pool exhausted

- **Symptoms:** latency climbs for everything that needs a connection while the CPU of Postgres stays low; `/healthz` 503 `{"db":"timeout"}` (the ping waits for a connection too); after the 12s handler deadline, 500s whose `err` ends in `context deadline exceeded`. The recipe endpoint suffers first: memoised dish and region lists still need one query, the recipe needs four.
- **Check:** how many connections are open and what they do:

```bash
$dc exec database sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT state, wait_event_type, count(*), max(now() - query_start) AS longest FROM pg_stat_activity WHERE datname = current_database() GROUP BY 1, 2"'
```

  Many `active` with a long `longest`: a slow query or a lock (an import holds its advisory lock and a write transaction; let it finish). Few connections in use: the bottleneck is elsewhere.
- **Fix:** raise `DB_MAX_CONNS` (default 10, 1–100) and `DB_MIN_CONNS` (default 1); on demo through recetas-infra (see [Changing configuration](#changing-configuration)), never by editing the VM's `.env`. Ten is well above what 20 rps of millisecond reads needs, so raise it only after the check above shows the server waiting on the pool and not Postgres waiting on a lock. Each environment's Postgres allows 100 connections; leave room for an import (up to 10 more) and `psql`.

### Bad import

| Exit | What happened | Database | Next |
|------|---------------|----------|------|
| 1 | Usage error, or the bundle failed validation. stderr lists every problem as `<pointer>: <message>`. | Not contacted, nothing changed (the entrypoint's `goose up` may still have applied pending migrations). | Fix the bundle or the flags, dry-run again. |
| 2 | Database error: unreachable, constraint violation or failed commit. | Rolled back, nothing changed. | Check [Database down](#database-down), then re-run the same command; an import is all-or-nothing, so a re-run is safe. |
| 0 with unexpected counts | The bundle was valid but wrong (rows missing, so deleted). | Changed: a new content version. | Re-import the previous good bundle (dry run first). Content isn't backed up separately: the pipeline's bundles are the source. |

A dry run that shows `created` for nearly everything on an environment that already holds content is pointed at the wrong project: check the `-p recetas-<env>`.

Imports take an advisory lock, so a second import waits for the first instead of interleaving. An `/schemaVersion: unsupported schemaVersion` problem means the pipeline is ahead of or behind the deployed backend; see [Schema versions](content-import.md#schema-versions).

### Telemetry export failing

Logfire unreachable or the token revoked. Requests are unaffected: export runs in background batches and never blocks a request. Logs still go to stdout as JSON. The failures show as plain-text lines on stderr from the OpenTelemetry SDK, not JSON, one per failed batch:

```
2026/10/25 12:50:19 Post "https://…/v1/logs": dial tcp …: connect: connection refused
2026/10/25 12:50:24 traces export: Post "https://…/v1/traces": dial tcp …: connect: connection refused
```

On shutdown, `flush telemetry failed` at ERROR. Batches that still fail after the exporter's own retries are dropped, so Logfire has a gap for the outage. Fix the token or `OTEL_EXPORTER_OTLP_ENDPOINT` (demo: the `DEMO_LOGFIRE_TOKEN` and `DEMO_OTEL_EXPORTER_OTLP_ENDPOINT` secrets, see [Changing configuration](#changing-configuration)). To turn telemetry off, blank `LOGFIRE_TOKEN`.

### Disk

The backend writes nothing to disk; Postgres, images and container logs do.

```bash
df -h / /var/lib/docker
docker system df
$dc exec database sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT pg_size_pretty(pg_database_size(current_database()))"'
docker image prune -af --filter "until=168h"    # the weekly prune planned in INF-5
```

`deploy-service.sh` already removes this repository's `sha-` images older than 168 h after every successful deploy, keeping the tags any env's `tags.env` names and this env's previous tag (recetas-infra RUNBOOK.md, Image cleanup). A full disk makes Postgres refuse writes or stop (an import exits 2, then see [Database down](#database-down)). Container logs are capped by the rotation, so the usual culprits are old images, the Postgres volume and dumps. The 80% disk alert is planned in INF-5.
