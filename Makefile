.PHONY: build test run sandbox-runner-build sandbox-runner-run crossborder-build crossborder-test

build:
	@mkdir -p bin
	go build -o bin/server ./cmd/server

test:
	go test ./...

run:
	go run ./cmd/server

sandbox-runner-build:
	@mkdir -p bin
	go build -o bin/sandbox-runner ./cmd/sandbox-runner

sandbox-runner-run:
	go run ./cmd/sandbox-runner

crossborder-build:
	$(MAKE) -C projects/crossborder build

crossborder-test:
	$(MAKE) -C projects/crossborder test