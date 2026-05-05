.PHONY: test test-fresh test-race fmt build check extension lint

test:
	go test ./...

test-fresh:
	go test -count=1 ./...

test-race:
	go test -race ./...

fmt:
	gofmt -w .

build:
	go build -o bin/gh-peek ./cmd/gh-peek

# extension builds the binary at the repo root with the name `gh` expects
# for `gh extension install .` to discover it as a local extension.
extension:
	go build -o gh-peek ./cmd/gh-peek
	@echo "Built ./gh-peek — install locally with: gh extension install ."

lint:
	golangci-lint run

check: fmt lint test
