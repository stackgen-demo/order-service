.PHONY: build run init-db trigger-5xx tidy docker-up

build:
	go build -o bin/server ./cmd/server
	go build -o bin/initdb ./cmd/initdb

run: init-db
	go run ./cmd/server

init-db:
	go run ./cmd/initdb

trigger-5xx:
	@chmod +x scripts/trigger-5xx.sh
	@./scripts/trigger-5xx.sh

tidy:
	go mod tidy

docker-up:
	docker compose up --build
