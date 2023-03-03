.PHONY: build
build:
	promu build

.PHONY: test
test:
	go test -v

.PHONY: lint
lint:
	golangci-lint run

.PHONY: deps
deps:
	go mod vendor
