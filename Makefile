BINARY := bin/server
MAIN := ./cmd/server

.PHONY: build run test lint migrate swagger dev

build:
	go build -o $(BINARY) $(MAIN)

run: build
	./$(BINARY)

test:
	go test -race ./...

lint:
	golangci-lint run ./...

DATABASE_URL ?= $(shell grep 'url:' config/application.yaml | head -1 | awk '{print $$2}' | tr -d '"')

migrate:
	goose -dir migrations postgres "$(DATABASE_URL)" up

swagger:
	docker run --rm -p 18081:8080 \
		-e SWAGGER_JSON=/docs/swagger.yaml \
		-v $(PWD)/docs:/docs \
		swaggerapi/swagger-ui

dev: build
	@docker run --rm -d -p 18081:8080 \
		-e SWAGGER_JSON=/docs/swagger.yaml \
		-v $(PWD)/docs:/docs \
		--name mynance-swagger \
		swaggerapi/swagger-ui
	@echo "Swagger UI: http://localhost:18081"
	@echo "API Server: http://localhost:18080"
	@./$(BINARY); docker stop mynance-swagger 2>/dev/null || true
