-- +goose Up
-- Hidden importance score (stack.md X15). A dish's score is the sum of its
-- variations' scores, derived when read. The default keeps editorial order.
ALTER TABLE variations ADD COLUMN score DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (score >= 0);

-- +goose Down
ALTER TABLE variations DROP COLUMN score;
