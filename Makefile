.PHONY: build test fmt lint muttest check

build:
	go build -o bin/impromptu ./cmd/...

test:
	go test ./...

fmt:
	gofmt -w .
	goimports -w .

lint:
	go vet ./...

muttest:
	go-mutesting ./internal/contentcheck/...

check: build
	./bin/impromptu check $(DIR)
