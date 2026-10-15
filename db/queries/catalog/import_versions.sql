-- name: ImportLock :exec
-- Serialises imports: held until the outermost transaction ends.
SELECT pg_advisory_xact_lock(sqlc.arg(lock_key)::bigint);

-- name: ImportInsertContentVersion :one
INSERT INTO content_versions (source, checksum)
VALUES (sqlc.arg(source), sqlc.arg(checksum))
RETURNING id;
