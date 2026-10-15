-- name: ImportInsertRecipe :one
INSERT INTO recipes (variation_id, time_minutes, base_servings)
VALUES (sqlc.arg(variation_id), sqlc.arg(time_minutes)::bigint, sqlc.arg(base_servings)::bigint)
RETURNING id;

-- name: ImportUpdateRecipe :exec
UPDATE recipes
SET time_minutes = sqlc.arg(time_minutes)::bigint, base_servings = sqlc.arg(base_servings)::bigint
WHERE id = sqlc.arg(id);

-- name: ImportDeleteTimeNotes :exec
DELETE FROM recipe_translations WHERE recipe_id = sqlc.arg(recipe_id);

-- name: ImportDeleteIngredients :exec
-- Their translations cascade.
DELETE FROM recipe_ingredients WHERE recipe_id = sqlc.arg(recipe_id);

-- name: ImportDeleteSteps :exec
-- Their translations cascade.
DELETE FROM recipe_steps WHERE recipe_id = sqlc.arg(recipe_id);

-- name: ImportInsertTimeNotes :exec
INSERT INTO recipe_translations (recipe_id, locale, time_note)
SELECT
    sqlc.arg(recipe_id)::bigint,
    unnest(sqlc.arg(locales)::text[]),
    unnest(sqlc.arg(time_notes)::text[]);

-- name: ImportInsertIngredients :exec
-- An empty unit is a counted item (NULL). Positions are 1-based.
INSERT INTO recipe_ingredients (recipe_id, position, quantity, unit)
SELECT
    sqlc.arg(recipe_id)::bigint,
    generate_series(1, cardinality(sqlc.arg(quantities)::float8[])),
    unnest(sqlc.arg(quantities)::float8[]),
    NULLIF(unnest(sqlc.arg(units)::text[]), '');

-- name: ImportInsertIngredientTranslations :exec
INSERT INTO recipe_ingredient_translations (recipe_id, position, locale, name)
SELECT
    sqlc.arg(recipe_id)::bigint,
    unnest(sqlc.arg(positions)::bigint[]),
    unnest(sqlc.arg(locales)::text[]),
    unnest(sqlc.arg(names)::text[]);

-- name: ImportInsertSteps :exec
-- Positions are 1-based.
INSERT INTO recipe_steps (recipe_id, position)
SELECT sqlc.arg(recipe_id)::bigint, generate_series(1, sqlc.arg(count)::bigint);

-- name: ImportInsertStepTranslations :exec
INSERT INTO recipe_step_translations (recipe_id, position, locale, text)
SELECT
    sqlc.arg(recipe_id)::bigint,
    unnest(sqlc.arg(positions)::bigint[]),
    unnest(sqlc.arg(locales)::text[]),
    unnest(sqlc.arg(texts)::text[]);
