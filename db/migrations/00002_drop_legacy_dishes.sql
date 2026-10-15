-- +goose Up
-- D-BE0-1: the iteration-1 dishes table and its /dishes endpoints go in the
-- same release; the frontend never consumed them.
DROP TABLE dishes;

-- +goose Down
CREATE TABLE dishes (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER dishes_set_updated_at
    BEFORE UPDATE ON dishes
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
