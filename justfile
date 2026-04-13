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
