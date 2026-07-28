APP_NAME := kilhog
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP_NAME)
DEV_DB_DSN := file:kilhog.db?_pragma=foreign_keys(ON)

.PHONY: build run-dev

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/kilhog

run-dev: build
	KILHOG_DB_DRIVER=sqlite KILHOG_DB_DSN='$(DEV_DB_DSN)' ./$(BIN)
