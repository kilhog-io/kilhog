APP_NAME := kilhog
CLI_NAME := pogig
WORKER_NAME := kilhog-worker
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP_NAME)
CLI_BIN := $(BIN_DIR)/$(CLI_NAME)
DEV_DB_DSN := file:kilhog.db?_pragma=foreign_keys(ON)
WORKERS_DIR := workers

# Local development API credentials (override on the command line).
# Disable auth: make run-dev KILHOG_API_KEY=
KILHOG_BASE_URL ?= http://localhost:8080
KILHOG_API_KEY ?= dev-secret

KILHOG_CLIENT_ENV = KILHOG_BASE_URL='$(KILHOG_BASE_URL)' KILHOG_API_KEY='$(KILHOG_API_KEY)'

DOCKER_IMAGE ?= kilhog:local

.PHONY: build build-pogig build-all build-wasm docker-build worker-dev worker-deploy worker-install \
	test vet ci run-dev \
	dev-create-networks dev-update-network-hors-prod dev-delete-network-prod \
	dev-create-subnets dev-update-subnet-dmz dev-delete-subnet-apps

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/kilhog

build-pogig:
	mkdir -p $(BIN_DIR)
	go build -o $(CLI_BIN) ./cmd/pogig

build-all: build build-pogig

docker-build:
	docker build -t $(DOCKER_IMAGE) .

# Compile the API as WebAssembly for Cloudflare Workers (assets under workers/build/).
build-wasm:
	cd $(WORKERS_DIR) && npm run build

worker-install:
	cd $(WORKERS_DIR) && npm install

worker-dev: worker-install
	cd $(WORKERS_DIR) && npm run dev

worker-deploy: worker-install
	cd $(WORKERS_DIR) && npm run deploy

vet:
	go vet ./...

test:
	go test ./...

ci: vet test build-all

run-dev: build
	KILHOG_DB_DRIVER=sqlite KILHOG_DB_DSN='$(DEV_DB_DSN)' KILHOG_API_KEY='$(KILHOG_API_KEY)' ./$(BIN)

dev-create-networks:
	$(KILHOG_CLIENT_ENV) ./scripts/dev/create-networks.sh

dev-update-network-hors-prod:
	$(KILHOG_CLIENT_ENV) ./scripts/dev/update-network-hors-prod.sh

dev-delete-network-prod:
	$(KILHOG_CLIENT_ENV) ./scripts/dev/delete-network-prod.sh

dev-create-subnets:
	$(KILHOG_CLIENT_ENV) ./scripts/dev/create-subnets.sh

dev-update-subnet-dmz:
	$(KILHOG_CLIENT_ENV) ./scripts/dev/update-subnet-dmz.sh

dev-delete-subnet-apps:
	$(KILHOG_CLIENT_ENV) ./scripts/dev/delete-subnet-apps.sh
