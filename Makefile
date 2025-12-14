# Simple build/test helpers for Raito

.PHONY: build test

build:
	cd raito && go build ./...

test:
	cd raito && go test ./...
