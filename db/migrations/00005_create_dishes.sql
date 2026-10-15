-- +goose Up
CREATE TABLE dishes (
    id             BIGSERIAL PRIMARY KEY,
    key            TEXT NOT NULL UNIQUE CHECK (key ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    sort_order     INTEGER NOT NULL UNIQUE,
    course         TEXT NOT NULL CHECK (course IN ('tapas', 'mains', 'desserts')),
    season         TEXT NOT NULL CHECK (season IN ('summer', 'winter', 'easter', 'all')),
    is_vegetarian  BOOLEAN NOT NULL DEFAULT FALSE,
    is_seafood     BOOLEAN NOT NULL DEFAULT FALSE,
    has_pork       BOOLEAN NOT NULL DEFAULT FALSE,
    is_quick       BOOLEAN NOT NULL DEFAULT FALSE,
    is_slow_cooked BOOLEAN NOT NULL DEFAULT FALSE,
    photo_thumb    TEXT,
    photo_card     TEXT,
    photo_hero     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER dishes_set_updated_at
    BEFORE UPDATE ON dishes
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE dish_translations (
    dish_id     BIGINT NOT NULL REFERENCES dishes (id) ON DELETE CASCADE,
    locale      TEXT NOT NULL REFERENCES locales (code) ON DELETE RESTRICT,
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    PRIMARY KEY (dish_id, locale)
);

CREATE INDEX dish_translations_locale_idx ON dish_translations (locale);

-- +goose Down
DROP TABLE dish_translations;
DROP TABLE dishes;
