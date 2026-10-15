-- +goose Up
-- One row per content import; the latest feeds the API ETag.
CREATE TABLE content_versions (
    id          BIGSERIAL PRIMARY KEY,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source      TEXT NOT NULL,
    checksum    TEXT NOT NULL
);

-- +goose Down
DROP TABLE content_versions;
