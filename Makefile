.PHONY: build check compose-config fmt fmt-check lint migrate-down migrate-up race run seed test test-integration vet

build:
	go build ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

fmt-check:
	test -z "$$(gofmt -l .)"

vet:
	go vet ./...

test:
	go test ./...

test-integration:
	go test -tags=integration ./internal/postgres -count=1

race:
	go test -race ./...

lint:
	golangci-lint run ./...

compose-config:
	docker compose --env-file .env.example config --quiet

migrate-up:
	docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"' < migrations/000001_recipe_catalogue.up.sql

migrate-down:
	docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"' < migrations/000001_recipe_catalogue.down.sql

seed:
	go run ./cmd/seed -dir ./seed

check: fmt-check vet test race build lint compose-config

run:
	go run ./cmd/bot
