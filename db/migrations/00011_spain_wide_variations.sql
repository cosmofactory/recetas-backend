-- +goose Up
-- Spanish (Spain-wide) variations (BE-9, D-ESP-1): no region row, a slug
-- instead, unique within the dish. Regional variations keep UNIQUE (dish_id,
-- region_id), which ignores NULL regions.
ALTER TABLE variations ALTER COLUMN region_id DROP NOT NULL;
ALTER TABLE variations ADD COLUMN slug TEXT CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$');
ALTER TABLE variations ADD CONSTRAINT variations_region_or_slug CHECK ((region_id IS NULL) = (slug IS NOT NULL));
CREATE UNIQUE INDEX variations_dish_slug_key ON variations (dish_id, slug) WHERE region_id IS NULL;

-- The bundle order within the dish (D-ESP-4), which orders its Spanish
-- variations; existing rows take region order. Deferrable so the import can
-- swap positions in one statement.
ALTER TABLE variations ADD COLUMN position INTEGER CHECK (position > 0);
UPDATE variations v SET position = p.position
FROM (
    SELECT v2.id, row_number() OVER (PARTITION BY v2.dish_id ORDER BY r.sort_order) AS position
    FROM variations v2
    JOIN regions r ON r.id = v2.region_id
) p
WHERE p.id = v.id;
ALTER TABLE variations ALTER COLUMN position SET NOT NULL;
ALTER TABLE variations ADD CONSTRAINT variations_dish_position_key UNIQUE (dish_id, position)
    DEFERRABLE INITIALLY IMMEDIATE;

ALTER TABLE dishes DROP CONSTRAINT dishes_course_check;
ALTER TABLE dishes ADD CONSTRAINT dishes_course_check CHECK (course IN ('tapas', 'mains', 'desserts', 'basics'));

-- +goose Down
-- Refuses rather than drop Spanish variations or basics dishes: remove them
-- with an import first.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM variations WHERE region_id IS NULL) THEN
        RAISE EXCEPTION 'Spanish (esp) variations exist; import a bundle without them before migrating down';
    END IF;
    IF EXISTS (SELECT 1 FROM dishes WHERE course = 'basics') THEN
        RAISE EXCEPTION 'basics dishes exist; import a bundle without them before migrating down';
    END IF;
END
$$;
-- +goose StatementEnd
ALTER TABLE dishes DROP CONSTRAINT dishes_course_check;
ALTER TABLE dishes ADD CONSTRAINT dishes_course_check CHECK (course IN ('tapas', 'mains', 'desserts'));
ALTER TABLE variations DROP CONSTRAINT variations_dish_position_key;
ALTER TABLE variations DROP COLUMN position;
DROP INDEX variations_dish_slug_key;
ALTER TABLE variations DROP CONSTRAINT variations_region_or_slug;
ALTER TABLE variations DROP COLUMN slug;
ALTER TABLE variations ALTER COLUMN region_id SET NOT NULL;
