# Ibis - Starknet Event Indexer
# ============================

BINARY_NAME := ibis
BINARY_PATH := ./bin/$(BINARY_NAME)
CMD_PATH    := ./cmd/ibis
MODULE      := github.com/b-j-roberts/ibis

GO       := go
GOFLAGS  := -v
LDFLAGS  := -s -w
DOCKER   := docker
COMPOSE  := docker compose

IMAGE_NAME := ibis
IMAGE_TAG  := latest

# ---- Development ----

.PHONY: dev
dev: ## Run with hot reload (requires air: go install github.com/air-verse/air@latest)
	@command -v air >/dev/null 2>&1 || { echo "Install air: go install github.com/air-verse/air@latest"; exit 1; }
	air

.PHONY: run
run: build ## Build and run the indexer
	$(BINARY_PATH) run

# ---- Build ----

.PHONY: build
build: ## Build static binary to ./bin/ibis
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_PATH) $(CMD_PATH)

# ---- Test & Quality ----

.PHONY: test
test: ## Run all tests
	$(GO) test $(GOFLAGS) ./...

.PHONY: check
check: fmt vet lint test ## Run all checks (fmt, vet, lint, test)

.PHONY: fmt
fmt: ## Format code
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: lint
lint: ## Run golangci-lint (requires: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	@command -v golangci-lint >/dev/null 2>&1 || { echo "Install golangci-lint: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; exit 1; }
	golangci-lint run ./...

# ---- Docker ----

.PHONY: docker-build
docker-build: ## Build Docker image
	$(DOCKER) build -t $(IMAGE_NAME):$(IMAGE_TAG) .

.PHONY: docker-run
docker-run: ## Run Docker container
	$(DOCKER) run --rm \
		--env-file .env \
		-v $(PWD)/ibis.config.yaml:/app/ibis.config.yaml:ro \
		-p 8080:8080 \
		$(IMAGE_NAME):$(IMAGE_TAG)

.PHONY: docker-compose-up
docker-compose-up: ## Start all services (ibis + postgres)
	$(COMPOSE) up -d

.PHONY: docker-compose-down
docker-compose-down: ## Stop all services
	$(COMPOSE) down

# ---- Utilities ----

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin/
	$(GO) clean

.PHONY: deps
deps: ## Download and tidy dependencies
	$(GO) mod download
	$(GO) mod tidy

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
