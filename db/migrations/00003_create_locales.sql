-- +goose Up
CREATE TABLE locales (
    code TEXT PRIMARY KEY
);

INSERT INTO locales (code) VALUES ('en'), ('es'), ('ru');

-- +goose Down
DROP TABLE locales;
