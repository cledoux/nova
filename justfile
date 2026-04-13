build:
	mkdir -p bin
	CGO_ENABLED=0 go build -o bin/nova .

test:
	go test ./...

test-all: build test

fmt:
	gofmt -s -w .
	goimports -w . 2>/dev/null || true

restart:
	systemctl --user restart nova

logs:
	journalctl --user -u nova -f

# Docker compose targets (container survives machine restarts)

build-image: build
	docker compose build

up: build-image
	docker compose up -d

down:
	docker compose down

logs-docker:
	docker compose logs -f

restart-docker:
	docker compose restart nova
