build:
	promu build

test:
	go test -v

lint:
	golangci-lint run

deps:
	go mod vendor
