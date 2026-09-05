.PHONY: build test vet fmt clean

build:
	go build -o bin/flaretunnel-manager ./cmd/manager

test:
	go test ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

clean:
	rm -rf bin
