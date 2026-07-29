APP_NAME := kilhog
CLI_NAME := pogig
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP_NAME)
CLI_BIN := $(BIN_DIR)/$(CLI_NAME)
DEV_DB_DSN := file:kilhog.db?_pragma=foreign_keys(ON)

.PHONY: build build-pogig build-all run-dev dev-create-networks dev-update-network-hors-prod dev-delete-network-prod dev-create-subnets dev-update-subnet-dmz dev-delete-subnet-apps

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/kilhog

build-pogig:
	mkdir -p $(BIN_DIR)
	go build -o $(CLI_BIN) ./cmd/pogig

build-all: build build-pogig

run-dev: build
	KILHOG_DB_DRIVER=sqlite KILHOG_DB_DSN='$(DEV_DB_DSN)' ./$(BIN)

dev-create-networks:
	./scripts/dev/create-networks.sh

dev-update-network-hors-prod:
	./scripts/dev/update-network-hors-prod.sh

dev-delete-network-prod:
	./scripts/dev/delete-network-prod.sh

dev-create-subnets:
	./scripts/dev/create-subnets.sh

dev-update-subnet-dmz:
	./scripts/dev/update-subnet-dmz.sh

dev-delete-subnet-apps:
	./scripts/dev/delete-subnet-apps.sh
