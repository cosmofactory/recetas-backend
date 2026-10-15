-- +goose Up
CREATE TABLE recipes (
    id            BIGSERIAL PRIMARY KEY,
    variation_id  BIGINT NOT NULL UNIQUE REFERENCES variations (id) ON DELETE CASCADE,
    time_minutes  INTEGER NOT NULL CHECK (time_minutes > 0),
    base_servings INTEGER NOT NULL CHECK (base_servings BETWEEN 1 AND 20)
);

-- Optional time note: no rows means no note; otherwise one per locale.
CREATE TABLE recipe_translations (
    recipe_id BIGINT NOT NULL REFERENCES recipes (id) ON DELETE CASCADE,
    locale    TEXT NOT NULL REFERENCES locales (code) ON DELETE RESTRICT,
    time_note TEXT NOT NULL,
    PRIMARY KEY (recipe_id, locale)
);

CREATE INDEX recipe_translations_locale_idx ON recipe_translations (locale);

-- unit NULL is a counted item.
CREATE TABLE recipe_ingredients (
    recipe_id BIGINT NOT NULL REFERENCES recipes (id) ON DELETE CASCADE,
    position  INTEGER NOT NULL CHECK (position > 0),
    quantity  NUMERIC(10, 3) NOT NULL CHECK (quantity > 0),
    unit      TEXT CHECK (unit IN ('g', 'ml', 'tsp')),
    PRIMARY KEY (recipe_id, position)
);

CREATE TABLE recipe_ingredient_translations (
    recipe_id BIGINT NOT NULL,
    position  INTEGER NOT NULL,
    locale    TEXT NOT NULL REFERENCES locales (code) ON DELETE RESTRICT,
    name      TEXT NOT NULL,
    PRIMARY KEY (recipe_id, position, locale),
    FOREIGN KEY (recipe_id, position) REFERENCES recipe_ingredients (recipe_id, position) ON DELETE CASCADE
);

CREATE INDEX recipe_ingredient_translations_locale_idx ON recipe_ingredient_translations (locale);

CREATE TABLE recipe_steps (
    recipe_id BIGINT NOT NULL REFERENCES recipes (id) ON DELETE CASCADE,
    position  INTEGER NOT NULL CHECK (position > 0),
    PRIMARY KEY (recipe_id, position)
);

CREATE TABLE recipe_step_translations (
    recipe_id BIGINT NOT NULL,
    position  INTEGER NOT NULL,
    locale    TEXT NOT NULL REFERENCES locales (code) ON DELETE RESTRICT,
    text      TEXT NOT NULL,
    PRIMARY KEY (recipe_id, position, locale),
    FOREIGN KEY (recipe_id, position) REFERENCES recipe_steps (recipe_id, position) ON DELETE CASCADE
);

CREATE INDEX recipe_step_translations_locale_idx ON recipe_step_translations (locale);

-- +goose Down
DROP TABLE recipe_step_translations;
DROP TABLE recipe_steps;
DROP TABLE recipe_ingredient_translations;
DROP TABLE recipe_ingredients;
DROP TABLE recipe_translations;
DROP TABLE recipes;
