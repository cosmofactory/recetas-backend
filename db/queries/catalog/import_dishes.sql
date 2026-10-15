-- name: ImportInsertDish :one
INSERT INTO dishes (
    key, sort_order, course, season,
    is_vegetarian, is_seafood, has_pork, is_quick, is_slow_cooked,
    photo_thumb, photo_card, photo_hero
) VALUES (
    sqlc.arg(key), sqlc.arg(sort_order)::bigint, sqlc.arg(course), sqlc.arg(season),
    sqlc.arg(is_vegetarian), sqlc.arg(is_seafood), sqlc.arg(has_pork), sqlc.arg(is_quick), sqlc.arg(is_slow_cooked),
    sqlc.narg(photo_thumb), sqlc.narg(photo_card), sqlc.narg(photo_hero)
)
RETURNING id;

-- name: ImportUpdateDish :exec
UPDATE dishes
SET sort_order = sqlc.arg(sort_order)::bigint,
    course = sqlc.arg(course),
    season = sqlc.arg(season),
    is_vegetarian = sqlc.arg(is_vegetarian),
    is_seafood = sqlc.arg(is_seafood),
    has_pork = sqlc.arg(has_pork),
    is_quick = sqlc.arg(is_quick),
    is_slow_cooked = sqlc.arg(is_slow_cooked),
    photo_thumb = sqlc.narg(photo_thumb),
    photo_card = sqlc.narg(photo_card),
    photo_hero = sqlc.narg(photo_hero)
WHERE id = sqlc.arg(id);

-- name: ImportParkDishOrders :exec
-- Negates the sort orders about to change, so the new ones can't collide
-- with them under the unique constraint.
UPDATE dishes SET sort_order = -sort_order WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ImportDeleteDishes :exec
DELETE FROM dishes WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ImportUpsertDishTranslations :exec
INSERT INTO dish_translations (dish_id, locale, name, description)
SELECT
    sqlc.arg(dish_id)::bigint,
    unnest(sqlc.arg(locales)::text[]),
    unnest(sqlc.arg(names)::text[]),
    unnest(sqlc.arg(descriptions)::text[])
ON CONFLICT (dish_id, locale) DO UPDATE
SET name = EXCLUDED.name, description = EXCLUDED.description;
