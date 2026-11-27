.PHONY: help test lint frontend build run docker up down

help: ## List the targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

test: ## Backend tests and frontend typecheck
	cd backend && go vet ./... && go test ./...
	cd frontend && npm run typecheck

frontend: ## Build the frontend into the backend's embed folder
	cd frontend && npm ci --no-audit --no-fund && npm run build
	rm -rf backend/internal/adapters/httpapi/dist/assets backend/internal/adapters/httpapi/dist/*.html backend/internal/adapters/httpapi/dist/*.svg
	cp -r frontend/dist/. backend/internal/adapters/httpapi/dist/

build: frontend ## Build bin/vartija (Linux, static)
	cd backend && CGO_ENABLED=0 go build -trimpath -o ../bin/vartija ./cmd/vartija

docker: ## Build the image vartija:local
	docker build -t vartija:local .

up: ## Start PostgreSQL and Vartija with docker compose (needs .env)
	docker compose up -d --build

down: ## Stop docker compose
	docker compose down
