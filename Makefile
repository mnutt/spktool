BIN_DIR ?= ./bin
BIN := $(BIN_DIR)/spktool

.PHONY: all build

all: build

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/spktool
