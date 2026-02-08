.PHONY: build dev test test-integration test-e2e test-all lint clean ui release docker help

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS  = -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the ayb binary
	go build $(LDFLAGS) -o ayb ./cmd/ayb

dev: ## Build and run with a test database URL (set DATABASE_URL)
	go run $(LDFLAGS) ./cmd/ayb start --database-url "$(DATABASE_URL)"

test: ## Run unit tests (no DB, fast)
	go test -count=1 ./...

test-integration: ## Run integration tests (one shared Postgres container)
	@cid=$$(docker run -d --rm \
		-e POSTGRES_USER=test -e POSTGRES_PASSWORD=test -e POSTGRES_DB=testdb \
		-p 0:5432 postgres:16-alpine) && \
	trap "docker stop $$cid >/dev/null 2>&1" EXIT && \
	port=$$(docker port $$cid 5432/tcp | cut -d: -f2) && \
	echo "Waiting for Postgres..." && \
	until docker exec $$cid pg_isready -U test -q 2>/dev/null; do sleep 0.1; done && \
	echo "Postgres ready on port $$port — running integration tests..." && \
	TEST_DATABASE_URL="postgresql://test:test@localhost:$$port/testdb?sslmode=disable" \
		go test -tags=integration -count=1 ./...

test-e2e: build ## Run Playwright e2e tests (starts ayb automatically)
	@./ayb start > /tmp/ayb-e2e.log 2>&1 & AYB_PID=$$!; \
	trap "kill $$AYB_PID 2>/dev/null" EXIT; \
	until curl -s http://localhost:8090/health > /dev/null 2>&1; do sleep 0.5; done; \
	cd ui && npx playwright test; \

test-all: test test-integration ## Run unit + integration tests

lint: ## Run linters (requires golangci-lint)
	golangci-lint run ./...

ui: ## Build the admin dashboard SPA
	cd ui && pnpm install && pnpm build

docker: ## Build Docker image locally
	docker build -t allyourbase/ayb:latest -t allyourbase/ayb:$(VERSION) .

clean: ## Remove build artifacts
	rm -f ayb
	rm -rf dist/

release: ## Build release binaries via goreleaser (dry run)
	goreleaser release --snapshot --clean

vet: ## Run go vet
	go vet ./...

fmt: ## Check formatting
	@test -z "$$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)
