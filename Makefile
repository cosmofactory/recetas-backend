.PHONY: help fmt vet lint golangci-lint vulncheck vulncheck-image build run test test-integration tidy \
	generate api-generate api-lint sqlc-generate goose-install seed design-bundle \
	migrate migrate-down migrate-status migrate-create \
	docker-build docker-up docker-down docker-logs

ifneq (,$(wildcard .env))
	include .env
	export
endif

GOOSE_VERSION ?= v3.29.0
GOLANGCI_LINT_VERSION ?= v2.15.0
GOLANGCI_LINT ?= go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
SQLC_VERSION ?= v1.31.1
SQLC ?= go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
GOVULNCHECK_VERSION ?= v1.9.0
GOVULNCHECK ?= go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
VACUUM_VERSION ?= v0.30.6
VACUUM ?= go run github.com/daveshanley/vacuum@$(VACUUM_VERSION)
MIGRATIONS_DIR ?= db/migrations
VERSION ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
DESIGN_CONTENT ?= ../design/design_handoff_asadorov_desktop/content.json
SEED_BUNDLE ?= testdata/content.design.json
LDFLAGS = -s -w -X main.version=$(VERSION)

# Integration tests opt in through TEST_DATABASE_URL, never DATABASE_URL (which
# .env points at the dev DB). Kept out of the environment of other targets so
# `make test` always skips them; test-integration passes it explicitly.
TEST_DATABASE_URL ?= postgres://$(or $(POSTGRES_USER),recetas):$(or $(POSTGRES_PASSWORD),recetas_dev)@127.0.0.1:$(or $(DB_HOST_PORT),5435)/recetas_test?sslmode=disable
unexport TEST_DATABASE_URL

help:
	@echo "Common targets:"
	@echo "  make fmt              - gofmt every Go file in place"
	@echo "  make vet              - go vet ./..."
	@echo "  make lint             - fmt check + vet + golangci-lint + build"
	@echo "  make vulncheck        - govulncheck ./... (fails on vulnerabilities in reachable code)"
	@echo "  make vulncheck-image IMAGE_REF=img - govulncheck -mode=binary on the image's server and import"
	@echo "  make build            - build the server and import binaries into ./bin/"
	@echo "  make run              - go run ./cmd/server (uses .env)"
	@echo "  make seed             - import testdata/content.design.json into DATABASE_URL"
	@echo "  make design-bundle    - regenerate testdata/content.design.json from the design content.json"
	@echo "  make test             - go test -race ./... (integration tests skip)"
	@echo "  make test-integration - compose Postgres + migrated recetas_test DB, then go test -race ./..."
	@echo "  make tidy             - go mod tidy"
	@echo ""
	@echo "Code generation:"
	@echo "  make generate         - api-generate + sqlc-generate"
	@echo "  make api-generate     - regenerate internal/api/apigen from api/openapi.yaml"
	@echo "  make api-lint         - lint api/openapi.yaml with vacuum (fails on errors)"
	@echo "  make sqlc-generate    - regenerate <module>db packages from db/queries/<module>/*.sql"
	@echo ""
	@echo "goose (migrations):"
	@echo "  make goose-install    - go install pinned goose CLI"
	@echo "  make migrate          - apply all up migrations"
	@echo "  make migrate-down     - revert last migration"
	@echo "  make migrate-status   - list applied/pending migrations"
	@echo "  make migrate-create NAME=xyz - scaffold new migration"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-build     - docker compose build"
	@echo "  make docker-up        - docker compose up --build -d"
	@echo "  make docker-down      - docker compose down"
	@echo "  make docker-logs      - docker compose logs -f backend"

# --- Go toolchain ---

fmt:
	gofmt -w .

vet:
	go vet ./...

# Same checks as the CI lint job.
lint:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt violations:"; echo "$$unformatted"; exit 1; \
	fi
	go vet ./...
	$(GOLANGCI_LINT) run ./...
	go build ./...

golangci-lint:
	$(GOLANGCI_LINT) run ./...

# Source mode reports the stdlib of the Go toolchain running it, too.
vulncheck:
	$(GOVULNCHECK) ./...

# Binary mode checks what an image actually ships (its Go and dependencies).
vulncheck-image:
ifndef IMAGE_REF
	$(error IMAGE_REF is required, e.g. make vulncheck-image IMAGE_REF=recetas-backend:scan)
endif
	@dir=$$(mktemp -d); trap 'rm -rf "$$dir"' EXIT; \
	id=$$(docker create "$(IMAGE_REF)") || exit 1; \
	docker cp "$$id:/app/server" "$$dir/server" && docker cp "$$id:/app/import" "$$dir/import"; \
	copied=$$?; docker rm "$$id" >/dev/null; [ $$copied -eq 0 ] || exit 1; \
	$(GOVULNCHECK) -mode=binary "$$dir/server" && $(GOVULNCHECK) -mode=binary "$$dir/import"

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/server ./cmd/server
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/import ./cmd/import

run:
	go run ./cmd/server

# --- content ---

seed:
	go run ./cmd/import --source design-seed --file $(SEED_BUNDLE)

design-bundle:
	go run ./scripts/designbundle -in $(DESIGN_CONTENT) -out $(SEED_BUNDLE)

test:
	go test -race -count=1 ./...

test-integration:
	docker compose up -d --wait database
	docker compose exec -T database psql -U "$${POSTGRES_USER:-recetas}" -d postgres -tAc \
		"SELECT 1 FROM pg_database WHERE datname = 'recetas_test'" | grep -q 1 || \
	docker compose exec -T database psql -U "$${POSTGRES_USER:-recetas}" -d postgres -c \
		"CREATE DATABASE recetas_test"
	go run github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION) -dir $(MIGRATIONS_DIR) postgres "$(TEST_DATABASE_URL)" up
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -race -count=1 ./...

tidy:
	go mod tidy

# --- code generation ---

generate: api-generate sqlc-generate

api-generate:
	go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml

api-lint:
	$(VACUUM) lint --details --fail-severity error api/openapi.yaml

sqlc-generate:
	$(SQLC) generate

# --- goose ---

goose-install:
	go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)

migrate:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" up

migrate-down:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" down

migrate-status:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" status

# Usage: make migrate-create NAME=add_recipes
migrate-create:
ifndef NAME
	$(error NAME is required, e.g. make migrate-create NAME=add_recipes)
endif
	goose -dir $(MIGRATIONS_DIR) create $(NAME) sql

# --- docker ---

docker-build:
	VERSION=$(VERSION) docker compose build

docker-up:
	VERSION=$(VERSION) docker compose up --build -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f backend
