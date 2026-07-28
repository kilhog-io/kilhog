APP_NAME := kilhog
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP_NAME)
DEV_DB_DSN := file:kilhog.db?_pragma=foreign_keys(ON)

.PHONY: build run-dev dev-create-networks dev-update-network-hors-prod dev-delete-network-prod

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/kilhog

run-dev: build
	KILHOG_DB_DRIVER=sqlite KILHOG_DB_DSN='$(DEV_DB_DSN)' ./$(BIN)

dev-create-networks:
	./scripts/dev/create-networks.sh

dev-update-network-hors-prod:
	./scripts/dev/update-network-hors-prod.sh

dev-delete-network-prod:
	./scripts/dev/delete-network-prod.sh
