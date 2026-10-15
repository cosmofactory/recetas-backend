-- +goose Up
CREATE TABLE regions (
    id         BIGSERIAL PRIMARY KEY,
    key        TEXT NOT NULL UNIQUE CHECK (key ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    sort_order INTEGER NOT NULL UNIQUE,
    tile_color TEXT NOT NULL CHECK (tile_color ~ '^#[0-9A-Fa-f]{6}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER regions_set_updated_at
    BEFORE UPDATE ON regions
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE region_translations (
    region_id   BIGINT NOT NULL REFERENCES regions (id) ON DELETE CASCADE,
    locale      TEXT NOT NULL REFERENCES locales (code) ON DELETE RESTRICT,
    name        TEXT NOT NULL,
    map_label   TEXT NOT NULL,
    description TEXT NOT NULL,
    PRIMARY KEY (region_id, locale)
);

CREATE INDEX region_translations_locale_idx ON region_translations (locale);

-- Spanish proper names, stored once and not translated.
CREATE TABLE region_regional_dishes (
    region_id BIGINT NOT NULL REFERENCES regions (id) ON DELETE CASCADE,
    position  INTEGER NOT NULL CHECK (position > 0),
    name      TEXT NOT NULL,
    PRIMARY KEY (region_id, position)
);

-- +goose Down
DROP TABLE region_regional_dishes;
DROP TABLE region_translations;
DROP TABLE regions;
