# Content import

This is the interface between the content pipeline and the backend. The pipeline builds a **bundle**, one JSON document holding the full desired state of the catalog, and hands it to the `/app/import` CLI in the backend image. The CLI validates the bundle and replaces the stored catalog with it in one transaction. The pipeline never writes to Postgres directly, and the public API stays read-only.

## The bundle

The schema is [`api/content-bundle.schema.json`](../api/content-bundle.schema.json) (JSON Schema 2020-12). The shape follows the design's `content.json`:

```jsonc
{
  "schemaVersion": 1,
  "regions": [            // editorial (map) order
    { "key": "gal", "tileColor": "#9A9A5E",
      "name": {"en": "…", "es": "…", "ru": "…"}, "mapLabel": {…}, "description": {…},
      "regionalDishes": ["Pulpo á feira"] }   // Spanish names, not translated
  ],
  "dishes": [             // editorial order
    { "key": "tortilla", "name": {…}, "description": {…},
      "flags": {"vegetarian": true, "seafood": false, "pork": false, "quick": true, "slowCooked": false},
      "course": "tapas", "season": "all",
      "photos": {"thumb": "dishes/tortilla/thumb-1a2b3c4d", "card": null, "hero": null},
      "variations": [
        { "regionKey": "gal", "recipeKey": "tortilla-gal", "name": {…}, "description": {…}, "photos": null }
      ] }
  ],
  "recipes": [
    { "recipeKey": "tortilla-gal", "timeMinutes": 25, "timeNote": null,   // or {"en": "+ soaking", "es": "+ remojo", "ru": "+ замачивание"}
      "baseServings": 2,
      "ingredients": [{"quantity": 400, "unit": "g", "name": {…}}, {"quantity": 4, "unit": null, "name": {…}}],
      "steps": [{"en": "…", "es": "…", "ru": "…"}] }
  ]
}
```

- Every human-readable text is an object with `en`, `es` and `ru`. All three are required, and there is no fallback to another locale.
- `photos` (on dishes and variations), `timeNote` and an ingredient's `unit` are optional and may be `null`. A missing or `null` photo role means there's no photo yet, and the frontend shows its placeholder. A `null` unit is a counted item.
- Variations of a dish are listed in region order whatever their order in the bundle. All variations are equal: there is no rank and no primary variation.
- `testdata/content.design.json` is a complete example: the design's `content.json` converted by `make design-bundle`, with placeholder recipes for the 50 variations the design has no recipe for.

## Validation rules

The CLI rejects a bundle before it touches the database when any of these fail. It reports every problem it finds, each with an RFC 6901 JSON pointer into the bundle (an empty pointer, shown as `(root)`, means the whole document).

Checked by the schema:
- The input is a JSON object of at most 16 MiB, with no unknown fields at any level and every required field present with the right type.
- `course` is `tapas|mains|desserts`, `season` is `summer|winter|easter|all`, `unit` is `g|ml|tsp` or `null`.
- Lists are bounded: 1–100 regions, 1–500 dishes, 1–100 variations per dish, 1–10000 recipes, 1–100 ingredients and 1–100 steps per recipe, and at most 20 regional dishes per region.
- Texts are 1–4000 characters, regional dish names 1–200, photo keys at most 256.

`schemaVersion` is checked first: a bundle whose version isn't `1` gets that one problem (`/schemaVersion: unsupported schemaVersion 2; this importer supports 1`), not a list of the fields this importer doesn't know.

Checked after the schema (semantic rules):
- Keys (`regions[].key`, `dishes[].key`, `recipeKey`) are ASCII slugs `^[a-z0-9]+(-[a-z0-9]+)*$` of at most 64 characters, and unique within their kind.
- `tileColor` is `#RRGGBB`.
- Every `variation.regionKey` is a region in the bundle, and a dish has at most one variation per region.
- Every variation's `recipeKey` names exactly one recipe, and every recipe is used by exactly one variation.
- `timeMinutes` is 1–10080, `baseServings` is 1–20, and `quantity`, after rounding to 3 decimal places, is above 0 and at most 9999999 (so `1e-10` is rejected).
- No text in any locale is blank (empty or whitespace only), and neither is a regional dish name.
- Photo keys follow the media convention (stack.md X3, INF-4): `dishes/<dishKey>/<role>-<hash>` for a dish and `dishes/<dishKey>/<regionKey>/<role>-<hash>` for a variation, where `<role>` is the field's own role (`thumb`, `card` or `hero`) and `<hash>` is 8–64 lowercase hex digits. There's no leading slash and no file extension, because widths and formats are implied by X3.

## Import semantics

The bundle is a **full replacement** of the previous state. It is declarative, not a patch:

- Rows are matched by natural key: regions and dishes by `key`, variations and recipes by `recipeKey`. A matched row is updated in place, so its id stays the same. A key the database doesn't have yet is created.
- Anything stored that the bundle doesn't mention is **deleted**, including its children: a region or dish that's missing loses its variations and their recipes. An empty list is rejected by the schema, so a truncated bundle can't wipe the catalog.
- A variation whose `recipeKey` now sits under another dish or region is deleted and created again, along with its recipe.
- Order comes from position: a region's or dish's place in its list is its sort order. Removing or inserting a region or dish shifts the position of every later one, and each shifted row counts as updated.
- A variation moved to another dish or region, or one whose `recipeKey` was renamed, counts as one variation and one recipe deleted and one of each created.
- Quantities are rounded to 3 decimal places when the bundle is parsed, which is what the database stores, so float noise like `0.30000000000000004` neither changes the checksum nor counts as an update.
- Everything runs in one transaction, and a failure at any point leaves the database as it was. Imports are serialised by an advisory lock, so two imports never interleave.
- On success a `content_versions` row records `--source` and the bundle checksum. The checksum is the SHA-256 of the canonical JSON of the parsed bundle, so key order and whitespace in the input don't matter. The new version changes every API ETag, so caches revalidate.
- Every import compares the bundle with the rows in the database, never only with the last checksum, so re-importing a bundle repairs any drift (a row edited by hand, say).
- **Idempotent:** a bundle that changes nothing makes no changes, adds no version and prints `no changes`.

The summary lists the rows created, updated and deleted for each entity:

```
imported content version 2 (source "pipeline 2026-10-15"):
  regions     created   0  updated   1  deleted   0
  dishes      created   0  updated   0  deleted   0
  variations  created   0  updated   0  deleted   1
  recipes     created   0  updated   0  deleted   1
```

## Invocation

```
/app/import --source <label> [--file bundle.json] [--dry-run]
```

| Flag | Meaning |
|------|---------|
| `--source` | Required. A free-text label (at most 200 bytes) stored with the content version, e.g. the pipeline run id. |
| `--file` | Reads the bundle from this file. Without it, the bundle is read from stdin. |
| `--dry-run` | Validates, applies the changes inside the transaction, prints the summary and rolls everything back. It catches database-level failures too, and writes nothing. |

The CLI reads only `DATABASE_URL`, the same DSN the server uses. It also loads `.env` when one is present. It doesn't need `APP_ENV`. The image's entrypoint applies pending migrations before it starts the import, as it does for the server, even for a dry run or a bundle that turns out to be invalid. `docker compose run --rm -T --entrypoint /app/import backend --source …` skips that step, and fails if the schema is behind the binary. The entrypoint and goose log to stderr, so stdout carries only the summary.

### Demo and prod

The bundle goes over SSH on stdin, so nothing is copied to the VM. `-T` disables the TTY so stdin passes through. Each environment is the compose project `recetas-<env>` in `/srv/recetas/<env>` (as planned in INF-2 and INF-5). Always pass `-p recetas-<env>`: a bare `docker compose` there takes the directory name as the project and would import into a different, empty database. Prod's image tags live in `versions.env` (INF-5), so prod also names both env files; without them the image tag interpolates blank.

```bash
# demo
ssh <vps> 'cd /srv/recetas/demo && docker compose -p recetas-demo run --rm -T backend /app/import --source pipeline-<run-id> --dry-run' < bundle.json
ssh <vps> 'cd /srv/recetas/demo && docker compose -p recetas-demo run --rm -T backend /app/import --source pipeline-<run-id>' < bundle.json
# prod
ssh <vps> 'cd /srv/recetas/prod && docker compose -p recetas-prod --env-file .env --env-file versions.env run --rm -T backend /app/import --source pipeline-<run-id> --dry-run' < bundle.json
ssh <vps> 'cd /srv/recetas/prod && docker compose -p recetas-prod --env-file .env --env-file versions.env run --rm -T backend /app/import --source pipeline-<run-id>' < bundle.json
```

Quote the whole remote command once, as above. ssh joins its arguments into one string and the remote shell splits it again, so local quotes around an argument are lost: `--source "pipeline $RUN_ID"` arrives as two words and the import fails with a usage error. Keep the label a single token without spaces or shell metacharacters, such as `pipeline-<run-id>`. The `<` stays outside the quotes, because the bundle is read locally.

To seed demo with the design content, use the demo command with `--source design-seed` and `< testdata/content.design.json`.

### Dry-run workflow

1. Run with `--dry-run` and read the summary. The deleted counts deserve the most attention, because anything missing from the bundle gets deleted.
2. If it exits 0 and the summary is what you expect, run the same command without `--dry-run`.
3. Keep the bundle. Re-running it is harmless only while it is still the latest import, when it prints `no changes` (or repairs drift); importing an older bundle reverts newer content.

### Local

```bash
make docker-up
make seed                     # = go run ./cmd/import --source design-seed --file testdata/content.design.json
make seed                     # prints "no changes"
make design-bundle            # regenerate testdata/content.design.json after the design's content.json changes
```

`make seed` targets `DATABASE_URL` from `.env`, which `.env.example` points at the compose Postgres.

## Exit codes

| Code | Meaning | Output |
|------|---------|--------|
| 0 | Imported, no changes, or dry run done | Summary on stdout |
| 1 | Usage error (missing `--source`, unknown flag, unreadable file) or invalid bundle | `import: invalid bundle, N problem(s):` followed by one `<pointer>: <message>` line per problem, on stderr. The database isn't contacted. |
| 2 | Database error (unreachable, constraint failure, commit failure) | The error on stderr. The transaction was rolled back. |

## Schema versions

`schemaVersion` is the bundle format version. This backend accepts exactly `1` (`importer.SchemaVersion`) and rejects anything else with a single problem at `/schemaVersion` naming the supported version, before checking the rest of the bundle.

Any change to the format takes a version bump, including an added optional field, because unknown fields are rejected. The steps:

1. The backend PR updates `api/content-bundle.schema.json` and the importer, bumps `importer.SchemaVersion`, regenerates `testdata/content.design.json`, and updates this document.
2. The owner prepares the pipeline change to emit the new version.
3. Deploy the backend to an environment, then switch that environment's pipeline. In between, imports exit 1 at `/schemaVersion` and change nothing, so the site keeps serving the last imported content. Don't run imports during that window.
