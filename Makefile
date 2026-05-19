BINARY := kubectl-podres

.PHONY: build run test lint clean

## build: compile the binary for your current platform
build:
	go build -o $(BINARY) ./main.go

## run: build and run (add ARGS="--help" etc. to pass flags)
run: build
	./$(BINARY) $(ARGS)

## test: run all tests with race detector
test:
	go test -race ./...

## lint: vet + staticcheck (install: go install honnef.co/go/tools/cmd/staticcheck@latest)
lint:
	go vet ./...
	staticcheck ./... 2>/dev/null || echo "staticcheck not installed — skipping (run: go install honnef.co/go/tools/cmd/staticcheck@latest)"

## clean: remove compiled binary
clean:
	rm -f $(BINARY)

## help: list available targets
help:
	@grep -E '^##' Makefile | sed 's/## //'
