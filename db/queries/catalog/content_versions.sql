-- name: LatestContentVersion :one
SELECT id, imported_at, source, checksum
FROM content_versions
ORDER BY id DESC
LIMIT 1;
