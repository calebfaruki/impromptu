.PHONY: build test fmt lint muttest check serve

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
	gremlins unleash

check: build
	./bin/impromptu check $(DIR)

serve: build
	./bin/impromptu serve --dev
