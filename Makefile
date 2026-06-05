.PHONY: build test vet sqlc run db-up db-down web-dev web-build

# --- Go API ---
build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

sqlc:
	sqlc generate

run:
	go run ./cmd/api

# --- local Postgres (throwaway) ---
db-up:
	docker run -d --name citeloop-pg -e POSTGRES_PASSWORD=citeloop -e POSTGRES_DB=citeloop -p 5432:5432 postgres:16

db-down:
	docker rm -f citeloop-pg

# --- web ---
web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build
