#!/bin/sh
set -eu

if [ -z "${DATABASE_URL:-}" ]; then
    echo "entrypoint: DATABASE_URL is required" >&2
    exit 1
fi

echo "entrypoint: applying migrations"
goose -dir /app/migrations postgres "$DATABASE_URL" up

echo "entrypoint: launching $*"
exec "$@"
