BIN_DIR ?= ./bin
BIN := $(BIN_DIR)/spktool

.PHONY: all build test

all: build

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/spktool

test:
	go test ./...
