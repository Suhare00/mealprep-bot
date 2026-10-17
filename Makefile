.PHONY: build check compose-config fmt fmt-check lint run test vet

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

lint:
	golangci-lint run ./...

compose-config:
	docker compose --env-file .env.example config --quiet

check: fmt-check vet test build lint compose-config

run:
	go run ./cmd/bot
