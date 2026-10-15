-- +goose Up
-- One variation per (dish, region). Variations are ordered by
-- regions.sort_order; there is deliberately no rank or primary flag.
CREATE TABLE variations (
    id          BIGSERIAL PRIMARY KEY,
    dish_id     BIGINT NOT NULL REFERENCES dishes (id) ON DELETE CASCADE,
    region_id   BIGINT NOT NULL REFERENCES regions (id) ON DELETE CASCADE,
    recipe_key  TEXT NOT NULL UNIQUE CHECK (recipe_key ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    photo_thumb TEXT,
    photo_card  TEXT,
    photo_hero  TEXT,
    UNIQUE (dish_id, region_id)
);

CREATE INDEX variations_region_id_idx ON variations (region_id);

CREATE TABLE variation_translations (
    variation_id BIGINT NOT NULL REFERENCES variations (id) ON DELETE CASCADE,
    locale       TEXT NOT NULL REFERENCES locales (code) ON DELETE RESTRICT,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL,
    PRIMARY KEY (variation_id, locale)
);

CREATE INDEX variation_translations_locale_idx ON variation_translations (locale);

-- +goose Down
DROP TABLE variation_translations;
DROP TABLE variations;
