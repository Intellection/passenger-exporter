BIN_DIR := ./bin

.PHONY: all
all: dependencies build

.PHONY: build
build:
	promu build --prefix=$(BIN_DIR)

.PHONY: test
test:
	go test -v ./...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: dependencies
dependencies:
	go mod vendor

.PHONY: clean
clean:
	rm -rf ${BIN_DIR}
