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
      "course": "tapas", "season": "all",   // course: tapas|mains|desserts|basics
      "photos": {"thumb": "dishes/tortilla/thumb-1a2b3c4d", "card": null, "hero": null},
      "variations": [
        { "regionKey": "gal", "recipeKey": "tortilla-gal", "name": {…}, "description": {…}, "photos": null,
          "score": 12.5 },  // optional hidden importance score, default 0
        { "regionKey": "esp", "variationSlug": "tortilla-de-diario",   // a Spanish (Spain-wide) variation
          "recipeKey": "tortilla-esp-tortilla-de-diario", "name": {…}, "description": {…} }
      ] }
  ],
  "recipes": [
    { "recipeKey": "tortilla-gal", "timeMinutes": 25, "timeNote": null,   // or {"en": "+ soaking", "es": "+ remojo", "ru": "+ замачивание"}
      "baseServings": 2,
      "ingredients": [{"quantity": 400, "unit": "g", "name": {…}}, {"quantity": 4, "unit": null, "name": {…}}],
      "steps": [{"en": "…", "es": "…", "ru": "…"}],
      "tips": [{"en": "…", "es": "…", "ru": "…"}] }   // optional, 0–3
  ]
}
```

- Every human-readable text is an object with `en`, `es` and `ru`. All three are required, and there is no fallback to another locale.
- `photos` (on dishes and variations), `timeNote` and an ingredient's `unit` are optional and may be `null`. A missing or `null` photo role means there's no photo yet, and the frontend shows its placeholder. A `null` unit is a counted item.
- `tips` on a recipe is optional: at most 3 texts, shown in order after the steps. A missing list and `[]` are the same content and checksum (no tips); `null` is rejected. Length and wording rules for tips are the content pipeline's (D-CNT2-3); the importer only requires every locale to be non-blank.
- Variations of a dish are listed in region order whatever their order in the bundle, then its Spanish variations in bundle order. All variations are equal: there is no primary variation.
- **Spanish (Spain-wide) variations** (BE-9) belong to no region: `regionKey` is the reserved key `esp`, which is never a region. A dish may have any number of them, beside regional ones or on their own, each with its own recipe.
  - `variationSlug` is **required** on an `esp` variation and **forbidden** on a regional one. It identifies the variation within its dish and in its URL (`/:lang/dishes/:dishKey/esp/:variationSlug`), so it is unique within the dish (two dishes may share one).
  - Whoever writes the bundle derives it **once** from the Spanish variation name: lowercase, accents stripped, every run of other characters a single `-` ("Allioli de morter" → `allioli-de-morter`). The importer only validates it and never derives it, so a later name fix doesn't change a URL. Keep it when the name changes.
  - The recipe key is `<dishKey>-esp-<slug>` (regional ones stay `<dishKey>-<regionKey>`). Keys are at most 64 characters, so the deriver shortens the slug, at a `-` where possible, until the recipe key fits; the importer rejects a longer key.
  - The API sends a Spanish variation with `regionKey: "esp"`, `regionName: null`, `regionRank: null` and its `variationSlug`; the frontend shows its own Spanish term. Region lists and their counts leave Spanish variations out. A dish's score still sums all its variations, Spanish ones included.
  - `course: "basics"` is meant for stocks and sauces, but any dish may use it.
- `score` on a variation is optional: a number from 0 to 1000000, default 0 (missing and `0` are the same content and checksum), rounded to 3 decimal places when parsed. It is the content pipeline's importance score (stack.md X15) and is **never shown**: the API returns positions only. It orders two lists:
  - the dish list (`GET /v1/dishes`, the Popular list and search) by dish score, the sum of its variations' scores, descending, then bundle order; `Dish.order` is the resulting 1-based position;
  - a region's list (`GET /v1/regions/{key}`) by variation score descending, then dish display order; `VariationSummary.regionRank` is the resulting position.

  A dish's own variations stay in region order, then Spanish in bundle order. A bundle without scores orders exactly as bundle order.
- `testdata/content.design.json` is a complete example: the design's `content.json` converted by `make design-bundle`, with placeholder recipes for the 50 variations the design has no recipe for. It includes the Spanish samples Allioli (two Spanish variations, course `basics`) and Arroz blanco con tomate (one); the converter derives their slugs from the Spanish names and drops the design-only `spainWide` block.

## Validation rules

The CLI rejects a bundle before it touches the database when any of these fail. It reports every problem it finds, each with an RFC 6901 JSON pointer into the bundle (an empty pointer, shown as `(root)`, means the whole document).

Checked by the schema:
- The input is a JSON object of at most 16 MiB, with no unknown fields at any level and every required field present with the right type.
- `course` is `tapas|mains|desserts|basics`, `season` is `summer|winter|easter|all`, `unit` is `g|ml|tsp` or `null`.
- Lists are bounded: 1–100 regions, 1–500 dishes, 1–100 variations per dish, 1–10000 recipes, 1–100 ingredients, 1–100 steps and 0–3 tips per recipe, and at most 20 regional dishes per region.
- Texts are 1–4000 characters, regional dish names 1–200, photo keys at most 256.
- A variation's `variationSlug`, when present, is a slug of at most 64 characters.
- A variation's `score`, when present, is a number from 0 to 1000000 (`null` is rejected).
- A recipe's `tips`, when present, is an array (`null` is rejected).

`schemaVersion` is checked first: a bundle whose version isn't `1` gets that one problem (`/schemaVersion: unsupported schemaVersion 2; this importer supports 1`), not a list of the fields this importer doesn't know.

Checked after the schema (semantic rules):
- Keys (`regions[].key`, `dishes[].key`, `recipeKey`) are ASCII slugs `^[a-z0-9]+(-[a-z0-9]+)*$` of at most 64 characters, and unique within their kind.
- `tileColor` is `#RRGGBB`.
- Every `variation.regionKey` is a region in the bundle or `esp`, and a dish has at most one variation per region.
- No region is keyed `esp`.
- An `esp` variation has a `variationSlug` (a slug of at most 64 characters, unique among the dish's Spanish variations); a regional variation has none.
- Every variation's `recipeKey` names exactly one recipe, and every recipe is used by exactly one variation.
- `timeMinutes` is 1–10080, `baseServings` is 1–20, and `quantity`, after rounding to 3 decimal places, is above 0 and at most 9999999 (so `1e-10` is rejected).
- No text in any locale is blank (empty or whitespace only), and neither is a regional dish name.
- Photo keys follow the media convention (stack.md X3, INF-4): `dishes/<dishKey>/<role>-<hash>` for a dish and `dishes/<dishKey>/<regionKey>/<role>-<hash>` for a regional variation and `dishes/<dishKey>/esp-<variationSlug>/<role>-<hash>` for a Spanish one (D-ESP-7), where `<role>` is the field's own role (`thumb`, `card` or `hero`) and `<hash>` is 8–64 lowercase hex digits. There's no leading slash and no file extension, because widths and formats are implied by X3.

## Import semantics

The bundle is a **full replacement** of the previous state. It is declarative, not a patch:

- Rows are matched by natural key: regions and dishes by `key`, variations and recipes by `recipeKey`. A matched row is updated in place, so its id stays the same. A key the database doesn't have yet is created.
- Anything stored that the bundle doesn't mention is **deleted**, including its children: a region or dish that's missing loses its variations and their recipes. An empty list is rejected by the schema, so a truncated bundle can't wipe the catalog.
- A variation whose `recipeKey` now sits under another dish or region, or under another `variationSlug`, is deleted and created again, along with its recipe. A variation is thus identified by (dish, region), or (dish, `esp`, slug) for a Spanish one, as well as by its `recipeKey`.
- A variation's place in its dish's list is stored as its position. A Spanish variation whose position changes counts as updated. A regional variation is listed by region order, so its position changing is not a change: a bundle that only reorders regional variations prints `no changes` and writes nothing. Positions are written whenever the plan has changes, so the stored ones catch up with the next import that changes anything.
- Order comes from position: a region's or dish's place in its list is its sort order. Removing or inserting a region or dish shifts the position of every later one, and each shifted row counts as updated.
- A variation moved to another dish or region, one whose `variationSlug` changed, or one whose `recipeKey` was renamed, counts as one variation and one recipe deleted and one of each created. A slug change is therefore refused by the guarded demo reseed, which refuses deletions.
- Quantities and scores are rounded to 3 decimal places when the bundle is parsed, which is what the database stores, so float noise like `0.30000000000000004` neither changes the checksum nor counts as an update.
- A changed `score` is one variation updated; it can reorder the dish and region lists without any other row changing.
- Everything runs in one transaction, and a failure at any point leaves the database as it was. Imports are serialised by an advisory lock, so two imports never interleave.
- On success a `content_versions` row records `--source` and the bundle checksum. The checksum is the SHA-256 of the canonical JSON of the parsed bundle, so key order and whitespace in the input don't matter. The new version changes every API ETag, so caches revalidate.
- Every import compares the bundle with the rows in the database, never only with the last checksum, so re-importing a bundle repairs any drift (a row edited by hand, say).
- **Idempotent:** a bundle that changes nothing makes no changes, adds no version and prints `no changes`.
- **Removals need an allowance** (INF-7, D-INF7-3). See [Removals](#removals).

The summary lists the rows created, updated and deleted for each entity:

```
imported content version 2 (source "pipeline 2026-10-15"):
  regions     created   0  updated   1  deleted   0
  dishes      created   0  updated   0  deleted   0
  variations  created   0  updated   0  deleted   1
  recipes     created   0  updated   0  deleted   1
  removed /dishes/tortilla/gal
```

After the counts, one `removed <path>` line names each public path the import removes, in a dry run too.

### Removals

A removal is a public path, without the locale prefix, that the database serves and the bundle no longer does:

| Path | Removed when |
|------|--------------|
| `/regions/<region>` | the region is missing or its `key` changed |
| `/dishes/<dish>` | the dish is missing or its `key` changed |
| `/dishes/<dish>/<region>` | the regional variation is missing, moved to another dish or region, or made Spanish; also when its dish or region is removed |
| `/dishes/<dish>/esp/<slug>` | the Spanish variation is missing, moved, or its `variationSlug` changed; also when its dish is removed |

A recipe goes with its variation and has no path of its own. A variation recreated at the same path (its `recipeKey` renamed at the same dish and region) is not a removal. The first import into an empty database has none.

Every removal must be listed in the `--allow-removals` file, by exact path: removing a dish or region needs the dish's or region's own path **and** each of its variations' paths. Without the flag nothing is allowed. Any unlisted removal refuses the whole import, the dry run included: exit 1, stdout empty, and stderr names only the paths still missing from the file. Nothing is written. Listing a path that isn't removed is fine, so the file can outlive the release it was written for. Run a dry run without the file to see every removal the bundle makes, then list the ones you mean.

The file has one path per line. Surrounding whitespace (CRLF included) is ignored, and so are blank lines and lines starting with `#`:

```
# 2026-11-24: codony allioli merged into the morter one
/dishes/allioli/esp/allioli-de-codony
```

Redirects for removed URLs are not built yet (INF-7 follow-up).

## Invocation

```
/app/import --source <label> [--file bundle.json] [--dry-run] [--allow-removals paths.txt]
```

| Flag | Meaning |
|------|---------|
| `--source` | Required. A free-text label (at most 200 bytes) stored with the content version, e.g. the pipeline run id. |
| `--file` | Reads the bundle from this file. Without it, the bundle is read from stdin. |
| `--dry-run` | Validates, applies the changes inside the transaction, prints the summary and rolls everything back. It catches database-level failures too, and writes nothing. |
| `--allow-removals` | Reads the public paths the import may remove from this file (see [Removals](#removals)). A missing or unreadable file exits 1 before the database is contacted. Without it, no removal is allowed. |

The CLI reads only `DATABASE_URL`, the same DSN the server uses. It also loads `.env` when one is present. It doesn't need `APP_ENV`. The image's entrypoint applies pending migrations before it starts the import, as it does for the server, even for a dry run or a bundle that turns out to be invalid. `docker compose run --rm -T --entrypoint /app/import backend --source …` skips that step, and fails if the schema is behind the binary. The entrypoint and goose log to stderr, so stdout carries only the summary.

### Demo and prod

The bundle goes over SSH on stdin, so nothing is copied to the VM. `-T` disables the TTY so stdin passes through. Each environment is the compose project `recetas-<env>` in `/srv/recetas/<env>` (demo: INF-2; prod as planned in INF-5). Always pass `-p recetas-<env>`: a bare `docker compose` there takes the directory name as the project and would import into a different, empty database. The image tags live in a second env file, demo's `tags.env` (written only by `deploy-service.sh`) and prod's `versions.env` (as planned in INF-5), so name both env files; without it the image tag is missing and compose refuses to start.

The owner logs in as `admin` (`ssh <vps>`, the owner's unrestricted key) and runs the import as `deploy`, which owns `/srv/recetas/*`: `ssh <vps> "sudo -u deploy bash -c '<command>'" < bundle.json`. stdin passes through sudo to the import. This repo's `demo` deploy key is restricted on the VM to `deploy-service.sh demo backend sha-<7>` (recetas-infra's `ssh-gate.sh`), so it can't run an import or `goose`.

```bash
# demo
ssh <vps> "sudo -u deploy bash -c 'cd /srv/recetas/demo && docker compose -p recetas-demo --env-file .env --env-file tags.env run --rm -T backend /app/import --source pipeline-<run-id> --dry-run'" < bundle.json
ssh <vps> "sudo -u deploy bash -c 'cd /srv/recetas/demo && docker compose -p recetas-demo --env-file .env --env-file tags.env run --rm -T backend /app/import --source pipeline-<run-id>'" < bundle.json
# prod (as planned in INF-5)
ssh <vps> "sudo -u deploy bash -c 'cd /srv/recetas/prod && docker compose -p recetas-prod --env-file .env --env-file versions.env run --rm -T backend /app/import --source pipeline-<run-id> --dry-run'" < bundle.json
ssh <vps> "sudo -u deploy bash -c 'cd /srv/recetas/prod && docker compose -p recetas-prod --env-file .env --env-file versions.env run --rm -T backend /app/import --source pipeline-<run-id>'" < bundle.json
```

Quote the command as above: double quotes around the whole remote command, single quotes around what `bash -c` runs as `deploy`. ssh joins its arguments into one string and the remote shell splits it again, so local quotes around an argument are lost: `--source "pipeline $RUN_ID"` arrives as two words and the import fails with a usage error. Keep the label a single token without spaces or shell metacharacters, such as `pipeline-<run-id>`. The `<` stays outside the quotes, because the bundle is read locally.

The allowance file has to be inside the container. Mount it read-only, since stdin already carries the bundle:

```bash
scp allow-removals.txt <vps>:/tmp/allow-removals.txt
ssh <vps> "sudo -u deploy bash -c 'cd /srv/recetas/prod && docker compose -p recetas-prod --env-file .env --env-file versions.env run --rm -T -v /tmp/allow-removals.txt:/tmp/allow-removals.txt:ro backend /app/import --source pipeline-<run-id> --allow-removals /tmp/allow-removals.txt --dry-run'" < bundle.json
```

To seed demo with the design content, use recetas-infra's `/srv/recetas/scripts/reseed-demo.sh` (dry run first, `--source design-seed`; its RUNBOOK.md, Demo → Reseed), with `testdata/content.design.json` from this repo at the deployed tag on stdin.

### Dry-run workflow

1. Run with `--dry-run` and read the summary. The deleted counts and `removed` paths deserve the most attention, because anything missing from the bundle gets deleted. An exit 1 naming paths means removals that aren't allowed yet: list the intended ones in the `--allow-removals` file, or fix the bundle.
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
| 1 | Usage error (missing `--source`, unknown flag, unreadable bundle or `--allow-removals` file) | `import: <message>` on stderr, e.g. `import: read --allow-removals: open …`. The database isn't contacted. |
| 1 | Invalid bundle | `import: invalid bundle, N problem(s):` followed by one `<pointer>: <message>` line per problem, on stderr. The database isn't contacted. |
| 1 | Unlisted removal | `import: import would remove N public path(s) not listed as allowed removals: <paths>` on stderr, nothing on stdout. The transaction was rolled back. |
| 2 | Database error (unreachable, constraint failure, commit failure) | The error on stderr. The transaction was rolled back. |

## Schema versions

`schemaVersion` is the bundle format version. This backend accepts exactly `1` (`importer.SchemaVersion`) and rejects anything else with a single problem at `/schemaVersion` naming the supported version, before checking the rest of the bundle.

Any change to the format takes a version bump, including an added optional field, because unknown fields are rejected. The exceptions so far are the optional variation `score` (BE-7), recipe `tips` (BE-8), and `esp` variations with `variationSlug` and the course `basics` (BE-9), added under version 1 because an older bundle stays valid and means the same thing. **Deploy the backend before importing a bundle with `score`, `tips`, `variationSlug` or `basics`:** an older importer rejects the unknown field (exit 1, nothing changed). The steps for a version bump:

1. The backend PR updates `api/content-bundle.schema.json` and the importer, bumps `importer.SchemaVersion`, regenerates `testdata/content.design.json`, and updates this document.
2. The owner prepares the pipeline change to emit the new version.
3. Deploy the backend to an environment, then switch that environment's pipeline. In between, imports exit 1 at `/schemaVersion` and change nothing, so the site keeps serving the last imported content. Don't run imports during that window.
