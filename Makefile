.PHONY: help fmt vet lint golangci-lint build run test tidy \
	sqlc-generate goose-install \
	migrate migrate-down migrate-status migrate-create \
	docker-build docker-up docker-down docker-logs

ifneq (,$(wildcard .env))
	include .env
	export
endif

GOOSE_VERSION ?= v3.24.0
GOLANGCI_LINT_VERSION ?= v2.12.2
GOLANGCI_LINT ?= go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
MIGRATIONS_DIR ?= db/migrations

help:
	@echo "Common targets:"
	@echo "  make fmt              - gofmt every Go file in place"
	@echo "  make vet              - go vet ./..."
	@echo "  make lint             - fmt check + vet + golangci-lint + build"
	@echo "  make build            - build the server binary into ./bin/server"
	@echo "  make run              - go run ./cmd/server (uses .env)"
	@echo "  make test             - go test -race ./..."
	@echo "  make tidy             - go mod tidy"
	@echo ""
	@echo "sqlc:"
	@echo "  make sqlc-generate    - regenerate dishdb from db/queries/*.sql"
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

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/server ./cmd/server

run:
	go run ./cmd/server

test:
	go test -race -count=1 ./...

tidy:
	go mod tidy

# --- sqlc ---

sqlc-generate:
	sqlc generate

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
	docker compose build

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f backend
