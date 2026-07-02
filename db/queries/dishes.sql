-- name: CreateDish :one
INSERT INTO dishes (name, description)
VALUES ($1, $2)
RETURNING *;

-- name: ListDishes :many
SELECT * FROM dishes ORDER BY created_at DESC;

-- name: GetDish :one
SELECT * FROM dishes WHERE id = $1;

-- name: CountDishes :one
SELECT COUNT(*) FROM dishes;