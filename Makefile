APP_NAME := kilhog
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP_NAME)

.PHONY: build

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/kilhog
