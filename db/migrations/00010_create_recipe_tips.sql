-- +goose Up
-- Optional cooking tips shown after the steps: no rows means none.
CREATE TABLE recipe_tips (
    recipe_id BIGINT NOT NULL REFERENCES recipes (id) ON DELETE CASCADE,
    position  INTEGER NOT NULL CHECK (position > 0),
    PRIMARY KEY (recipe_id, position)
);

CREATE TABLE recipe_tip_translations (
    recipe_id BIGINT NOT NULL,
    position  INTEGER NOT NULL,
    locale    TEXT NOT NULL REFERENCES locales (code) ON DELETE RESTRICT,
    text      TEXT NOT NULL,
    PRIMARY KEY (recipe_id, position, locale),
    FOREIGN KEY (recipe_id, position) REFERENCES recipe_tips (recipe_id, position) ON DELETE CASCADE
);

CREATE INDEX recipe_tip_translations_locale_idx ON recipe_tip_translations (locale);

-- +goose Down
DROP TABLE recipe_tip_translations;
DROP TABLE recipe_tips;
