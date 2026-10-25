# syntax=docker/dockerfile:1.7

# Base images are pinned by multi-arch index digest; the tag is for humans.
# Bump both together: docker buildx imagetools inspect <image>:<tag>. The
# builder's Go patch release must match the toolchain line in go.mod (CI
# runs govulncheck on that version), so bump them together.

# ---------- Builder ----------
FROM golang:1.27-alpine@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS builder

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
ARG GOOSE_VERSION=v3.29.0
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go install github.com/pressly/goose/v3/cmd/goose@${GOOSE_VERSION}


# ---------- Runtime ----------
FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a AS runtime

LABEL org.opencontainers.image.source="https://github.com/cosmofactory/recetas-backend"

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S -G app -H -h /app app

WORKDIR /app

COPY --from=builder /out/server        /app/server
COPY --from=builder /out/import        /app/import
COPY --from=builder /go/bin/goose      /usr/local/bin/goose
# Everything under /app stays root-owned: the runtime user only reads it.
COPY db/migrations                     /app/migrations
COPY docker/entrypoint.sh              /app/entrypoint.sh
RUN chmod 0755 /app/entrypoint.sh

USER app

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["/app/server"]
