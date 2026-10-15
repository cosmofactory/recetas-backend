#!/bin/sh
set -eu

if [ -z "${DATABASE_URL:-}" ]; then
    echo "entrypoint: DATABASE_URL is required" >&2
    exit 1
fi

echo "entrypoint: applying migrations" >&2
goose -dir /app/migrations postgres "$DATABASE_URL" up

echo "entrypoint: launching $*" >&2
exec "$@"
