default:
      just --list

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -o bin/nova .

test:
	go test ./...

test-all: build test

fmt:
	gofmt -s -w .
	goimports -w . 2>/dev/null || true

build-image: build
	docker compose build

up: build-image
	docker compose up -d

down:
	docker compose down

restart: build
	docker compose restart nova

logs:
	docker compose logs -f
