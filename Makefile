BINARY := bin/server
MAIN := ./cmd/server

.PHONY: build run test lint migrate

build:
	go build -o $(BINARY) $(MAIN)

run: build
	./$(BINARY)

test:
	go test -race ./...

lint:
	golangci-lint run ./...

migrate:
	goose -dir migrations postgres "$(DATABASE_URL)" up
