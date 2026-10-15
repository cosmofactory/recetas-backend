-- Reads of the whole stored catalog, one row per entity with every locale, so
-- the import can compare it with a bundle. A missing translation reads as ''.

-- name: ImportLatestContentVersion :one
SELECT id, checksum
FROM content_versions
ORDER BY id DESC
LIMIT 1;

-- name: ImportListRegions :many
SELECT
    r.id,
    r.key,
    r.sort_order,
    r.tile_color,
    COALESCE(en.name, '')::text        AS name_en,
    COALESCE(es.name, '')::text        AS name_es,
    COALESCE(ru.name, '')::text        AS name_ru,
    COALESCE(en.map_label, '')::text   AS map_label_en,
    COALESCE(es.map_label, '')::text   AS map_label_es,
    COALESCE(ru.map_label, '')::text   AS map_label_ru,
    COALESCE(en.description, '')::text AS description_en,
    COALESCE(es.description, '')::text AS description_es,
    COALESCE(ru.description, '')::text AS description_ru
FROM regions r
LEFT JOIN region_translations en ON en.region_id = r.id AND en.locale = 'en'
LEFT JOIN region_translations es ON es.region_id = r.id AND es.locale = 'es'
LEFT JOIN region_translations ru ON ru.region_id = r.id AND ru.locale = 'ru'
ORDER BY r.sort_order;

-- name: ImportListRegionalDishes :many
SELECT region_id, name
FROM region_regional_dishes
ORDER BY region_id, position;

-- name: ImportListDishes :many
SELECT
    d.id,
    d.key,
    d.sort_order,
    d.course,
    d.season,
    d.is_vegetarian,
    d.is_seafood,
    d.has_pork,
    d.is_quick,
    d.is_slow_cooked,
    d.photo_thumb,
    d.photo_card,
    d.photo_hero,
    COALESCE(en.name, '')::text        AS name_en,
    COALESCE(es.name, '')::text        AS name_es,
    COALESCE(ru.name, '')::text        AS name_ru,
    COALESCE(en.description, '')::text AS description_en,
    COALESCE(es.description, '')::text AS description_es,
    COALESCE(ru.description, '')::text AS description_ru
FROM dishes d
LEFT JOIN dish_translations en ON en.dish_id = d.id AND en.locale = 'en'
LEFT JOIN dish_translations es ON es.dish_id = d.id AND es.locale = 'es'
LEFT JOIN dish_translations ru ON ru.dish_id = d.id AND ru.locale = 'ru'
ORDER BY d.sort_order;

-- name: ImportListVariations :many
SELECT
    v.id,
    d.key AS dish_key,
    r.key AS region_key,
    v.recipe_key,
    v.photo_thumb,
    v.photo_card,
    v.photo_hero,
    COALESCE(en.name, '')::text        AS name_en,
    COALESCE(es.name, '')::text        AS name_es,
    COALESCE(ru.name, '')::text        AS name_ru,
    COALESCE(en.description, '')::text AS description_en,
    COALESCE(es.description, '')::text AS description_es,
    COALESCE(ru.description, '')::text AS description_ru
FROM variations v
JOIN dishes d ON d.id = v.dish_id
JOIN regions r ON r.id = v.region_id
LEFT JOIN variation_translations en ON en.variation_id = v.id AND en.locale = 'en'
LEFT JOIN variation_translations es ON es.variation_id = v.id AND es.locale = 'es'
LEFT JOIN variation_translations ru ON ru.variation_id = v.id AND ru.locale = 'ru'
ORDER BY v.id;

-- name: ImportListRecipes :many
-- has_time_note is true when the recipe has a note in any locale.
SELECT
    rc.id,
    v.recipe_key,
    rc.time_minutes,
    rc.base_servings,
    EXISTS (SELECT 1 FROM recipe_translations n WHERE n.recipe_id = rc.id) AS has_time_note,
    COALESCE(en.time_note, '')::text AS time_note_en,
    COALESCE(es.time_note, '')::text AS time_note_es,
    COALESCE(ru.time_note, '')::text AS time_note_ru
FROM recipes rc
JOIN variations v ON v.id = rc.variation_id
LEFT JOIN recipe_translations en ON en.recipe_id = rc.id AND en.locale = 'en'
LEFT JOIN recipe_translations es ON es.recipe_id = rc.id AND es.locale = 'es'
LEFT JOIN recipe_translations ru ON ru.recipe_id = rc.id AND ru.locale = 'ru'
ORDER BY rc.id;

-- name: ImportListIngredients :many
SELECT
    i.recipe_id,
    i.quantity::float8         AS quantity,
    i.unit,
    COALESCE(en.name, '')::text AS name_en,
    COALESCE(es.name, '')::text AS name_es,
    COALESCE(ru.name, '')::text AS name_ru
FROM recipe_ingredients i
LEFT JOIN recipe_ingredient_translations en
    ON en.recipe_id = i.recipe_id AND en.position = i.position AND en.locale = 'en'
LEFT JOIN recipe_ingredient_translations es
    ON es.recipe_id = i.recipe_id AND es.position = i.position AND es.locale = 'es'
LEFT JOIN recipe_ingredient_translations ru
    ON ru.recipe_id = i.recipe_id AND ru.position = i.position AND ru.locale = 'ru'
ORDER BY i.recipe_id, i.position;

-- name: ImportListSteps :many
SELECT
    s.recipe_id,
    COALESCE(en.text, '')::text AS text_en,
    COALESCE(es.text, '')::text AS text_es,
    COALESCE(ru.text, '')::text AS text_ru
FROM recipe_steps s
LEFT JOIN recipe_step_translations en
    ON en.recipe_id = s.recipe_id AND en.position = s.position AND en.locale = 'en'
LEFT JOIN recipe_step_translations es
    ON es.recipe_id = s.recipe_id AND es.position = s.position AND es.locale = 'es'
LEFT JOIN recipe_step_translations ru
    ON ru.recipe_id = s.recipe_id AND ru.position = s.position AND ru.locale = 'ru'
ORDER BY s.recipe_id, s.position;
