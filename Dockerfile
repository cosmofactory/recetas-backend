# syntax=docker/dockerfile:1.7

# ---------- Builder ----------
FROM golang:1.25-alpine AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOFLAGS=-mod=readonly

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG VERSION=dev

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/server ./cmd/server \
    && go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/import ./cmd/import

# Pinned to match CI and `make goose-install`.
ARG GOOSE_VERSION=v3.24.0
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go install github.com/pressly/goose/v3/cmd/goose@${GOOSE_VERSION}


# ---------- Runtime ----------
FROM alpine:3.20 AS runtime

LABEL org.opencontainers.image.source="https://github.com/cosmofactory/recetas-backend"

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S -G app -H -h /app app

WORKDIR /app

COPY --from=builder /out/server        /app/server
COPY --from=builder /out/import        /app/import
COPY --from=builder /go/bin/goose      /usr/local/bin/goose
COPY --chown=app:app db/migrations     /app/migrations
COPY --chown=app:app docker/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

USER app

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["/app/server"]
