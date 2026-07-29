# TECHNICAL — kilhog API

## Stack

- **Language**: Go 1.26+
- **Module**: `github.com/kilhog-io/kilhog`
- **Architecture**: REST API, layered design

## Directory layout

```
kilhog/
├── cmd/
│   └── kilhog/          # Application entry point (main.go)
├── internal/
│   ├── handler/         # HTTP handlers and request validation
│   ├── service/         # Business logic and repository interfaces
│   ├── repository/      # Data access, migrations, SQL drivers
│   │   ├── migration/   # Versioned migration runner (upgrade / downgrade)
│   │   ├── postgres/    # PostgreSQL implementation
│   │   └── sqlite/      # SQLite implementation
│   └── model/           # Models and data structures
├── migrations/          # Embedded versioned SQL scripts (sqlite/ and postgres/)
├── scripts/
│   └── dev/             # HTTP scripts for local development
├── FUNCTIONAL.md        # Business rules (defined by the user)
└── TECHNICAL.md         # This file
```

## Domain model (`internal/model`)

Technical modeling of `FUNCTIONAL.md`. See that file for business rules.

| Type           | File               | Description |
|----------------|--------------------|-------------|
| `Tag`          | `tag.go`           | Key–value metadata pair |
| `AddressType`  | `address_type.go`  | Address family: `ipv4`, `ipv6` |
| `Parent`       | `parent.go`        | Reference to a `network` or `subnet` parent |
| `Network`      | `network.go`       | Tenancy container |
| `Subnet`       | `subnet.go`        | IP address space (block or host) |

### `Network`

| Field         | Type        | JSON |
|---------------|-------------|------|
| `UUID`        | `uuid.UUID` | `uuid` |
| `Name`        | `string`    | `name` |
| `Description` | `string`    | `description` |
| `Tags`        | `[]Tag`     | `tags` |

### `Subnet`

| Field         | Type          | JSON |
|---------------|---------------|------|
| `UUID`        | `uuid.UUID`   | `uuid` |
| `Name`        | `string`      | `name` |
| `Description` | `string`      | `description` |
| `Prefix`      | `int`         | `prefix` |
| `Address`     | `string`      | `address` |
| `Type`        | `AddressType` | `type` |
| `Parent`      | `Parent`      | `parent` |
| `Tags`        | `[]Tag`       | `tags` |

Methods:

- `CIDR() string` — returns `{address}/{prefix}`
- `IsLeaf() bool` — `true` if prefix is `/32` (IPv4) or `/128` (IPv6)

Constants: `IPv4HostPrefix` (32), `IPv6HostPrefix` (128).

### `Parent`

| Field  | Type          | JSON |
|--------|---------------|------|
| `Kind` | `ParentKind`  | `kind` (`network`, `subnet`) |
| `UUID` | `uuid.UUID`   | `uuid` |

## Repository interfaces (`internal/service`)

Interfaces defined on the service side (consumed by business logic):

- `NetworkRepository` — CRUD and listing of networks
- `SubnetRepository` — CRUD, listing by network or by parent

## IPv4 utilities (`internal/iputil`)

| Function | Role |
|----------|------|
| `ParseIPv4Prefix` | Parse and normalize an IPv4 address/prefix |
| `ValidateIPv4Subnet` | Check parent containment and absence of overlap among siblings |
| `FindFreeIPv4Block` | Find the first free `/prefix` block within a subnet parent CIDR |

## Persistence layer (`internal/repository`)

**Configurable, abstract** data backend. Two drivers are supported: **SQLite** and **PostgreSQL**. The service consumes repository interfaces; driver choice is transparent to business logic.

### Abstraction

```
service (NetworkRepository, SubnetRepository)
    └── repository/
            ├── sqlite/    → SQLite implementations
            ├── postgres/  → PostgreSQL implementations
            └── migration/ → migration runner (shared by both drivers)
```

Each driver implements the interfaces defined in `internal/service`. SQL queries use adapted dialects where needed (UUID types, `TIMESTAMPTZ`, etc.).

Concrete repository implementations live in `internal/repository/` (`network_repository.go`, `subnet_repository.go`) and are instantiated via `repository.Open`.

### Concurrent access

API calls may arrive in parallel. The `db.Store` layer provides:

| Mechanism | Role |
|-----------|------|
| `database/sql` pool | Concurrent connections (parallel reads) |
| `WithWriteLock` | On SQLite, application mutex serializing writes |
| `WithTx` / `WithWriteTx` | Atomic SQL transactions |
| `AcquireMigrationLock` | Exclusive lock during migrations (SQLite mutex, PostgreSQL `pg_advisory_lock`) |
| WAL + `busy_timeout` (SQLite) | Readers not blocked by writers |

Mutations (`Create`, `Update`, `Delete`) go through `WithWriteTx`: SQLite write lock + SQL transaction. Reads (`Get*`, `List*`) use the pool directly.

On PostgreSQL, the SQLite application lock is disabled: concurrency is handled by MVCC and SQL transactions.

### Connection and database creation

At startup, the application:

1. Reads configuration (`KILHOG_DB_DRIVER`, `KILHOG_DB_DSN`).
2. **Creates the database if it does not exist**:
   - **SQLite**: creates the file and parent directories from the DSN path.
   - **PostgreSQL**: connects to the `postgres` catalog, runs `CREATE DATABASE` if the target database is missing, then reconnects to the target database.
3. Opens a pooled connection to the database.
4. Runs pending **upgrade** migrations.
5. Injects repositories into services / handlers.

### Versioned SQL migrations

SQL scripts are embedded via `go:embed` in `internal/repository/migration/migrations/`, organized by dialect:

```
internal/repository/migration/migrations/
├── sqlite/
│   ├── 001_initial_schema.up.sql
│   └── 001_initial_schema.down.sql
└── postgres/
    ├── 001_initial_schema.up.sql
    └── 001_initial_schema.down.sql
```

| File | Role |
|------|------|
| `{version}_{name}.up.sql` | Applies version N |
| `{version}_{name}.down.sql` | Reverts version N |

Convention:

- `{version}`: zero-padded 3-digit integer (`001`, `002`, …).
- `{name}`: snake_case identifier describing the change (`initial_schema`).

Example (per-dialect structure):

```
internal/repository/migration/migrations/sqlite/
├── 001_initial_schema.up.sql
└── 001_initial_schema.down.sql
```

#### Tracking table: `schema_migrations`

| Column      | Type          | Description |
|-------------|---------------|-------------|
| `version`   | `INTEGER`     | Applied version number (PK) |
| `applied_at`| `TIMESTAMPTZ` | Application timestamp |

#### Behavior

| Operation | Trigger | Behavior |
|-----------|---------|----------|
| **Upgrade** | Automatic at startup | Applies all versions `> current version`, in ascending order |
| **Downgrade** | Explicit (CLI or admin API) | Runs the target version's `.down.sql`, removes the entry from `schema_migrations` |

Already applied migrations are never replayed. On mid-migration failure, the transaction is rolled back and the version is not recorded.

### Relational schema

Modeling of `Network`, `Subnet`, and `Tag` entities defined in `FUNCTIONAL.md`.

#### `networks` table

| Column       | Type          | Constraints | Maps to |
|--------------|---------------|-------------|---------|
| `uuid`       | UUID          | PK          | `Network.UUID` |
| `name`       | TEXT          | NOT NULL, UNIQUE | `Network.Name` |
| `description`| TEXT          | NULL        | `Network.Description` |
| `created_at` | TIMESTAMPTZ   | NOT NULL, DEFAULT now() | — |
| `updated_at` | TIMESTAMPTZ   | NOT NULL, DEFAULT now() | — |

#### `subnets` table

| Column        | Type          | Constraints | Maps to |
|---------------|---------------|-------------|---------|
| `uuid`        | UUID          | PK          | `Subnet.UUID` |
| `network_uuid`| UUID          | NOT NULL, FK → `networks(uuid)` ON DELETE CASCADE | Tenancy (root network) |
| `name`        | TEXT          | NOT NULL, UNIQUE `(network_uuid, name)` | `Subnet.Name` |
| `description` | TEXT          | NULL        | `Subnet.Description` |
| `prefix`      | INTEGER       | NOT NULL    | `Subnet.Prefix` |
| `address`     | TEXT          | NOT NULL    | `Subnet.Address` |
| `address_type`| TEXT          | NOT NULL, CHECK IN (`ipv4`, `ipv6`) | `Subnet.Type` |
| `parent_kind` | TEXT          | NOT NULL, CHECK IN (`network`, `subnet`) | `Subnet.Parent.Kind` |
| `parent_uuid` | UUID          | NOT NULL    | `Subnet.Parent.UUID` |
| `created_at`  | TIMESTAMPTZ   | NOT NULL, DEFAULT now() | — |
| `updated_at`  | TIMESTAMPTZ   | NOT NULL, DEFAULT now() | — |

Recommended indexes:

- `idx_subnets_network_uuid` on `network_uuid`
- `idx_subnets_parent` on `(parent_kind, parent_uuid)`

> **Tenancy note**: `network_uuid` is denormalized on every subnet to enforce `name` uniqueness within the network and simplify listing queries, even when the direct parent is another subnet.

#### `tags` table

Polymorphic tags attached to a network or subnet.

| Column          | Type          | Constraints | Maps to |
|-----------------|---------------|-------------|---------|
| `resource_kind` | TEXT          | NOT NULL, CHECK IN (`network`, `subnet`) | Resource type |
| `resource_uuid` | UUID          | NOT NULL    | Network or subnet UUID |
| `key`           | TEXT          | NOT NULL    | `Tag.Key` |
| `value`         | TEXT          | NOT NULL    | `Tag.Value` |

Composite primary key: `(resource_kind, resource_uuid, key)`.

Cascade delete FKs:

- `(network, resource_uuid)` → `networks(uuid)` ON DELETE CASCADE
- `(subnet, resource_uuid)` → `subnets(uuid)` ON DELETE CASCADE

#### Relationship diagram

```
networks (1) ──< subnets (N)
    │                  │
    └──< tags          └──< tags
         (polymorphic, resource_kind + resource_uuid)
```

#### Initial script (`001_initial_schema`)

The `001_initial_schema.up.sql` script creates `schema_migrations`, `networks`, `subnets`, and `tags` tables with the constraints above. The `001_initial_schema.down.sql` script drops these tables in reverse dependency order.

### Dialect differences

| Aspect | SQLite | PostgreSQL |
|--------|--------|------------|
| UUID type | `TEXT` (canonical format) or `BLOB` | Native `UUID` |
| Timestamps | `TEXT` ISO-8601 or `DATETIME` | `TIMESTAMPTZ` |
| Database creation | File on disk | `CREATE DATABASE` via admin connection |
| Go driver | `modernc.org/sqlite` or `mattn/go-sqlite3` | `jackc/pgx/v5` |

Migrations may contain dialect-specific sections if needed; otherwise SQL stays as portable as possible.

## Dependencies

| Module | Usage |
|--------|-------|
| `github.com/google/uuid` | Resource `UUID` identifiers |
| `database/sql` | Standard Go SQL abstraction |
| `modernc.org/sqlite` | SQLite driver (pure Go) |
| `jackc/pgx/v5` | PostgreSQL driver |

## API routes

| Method | Route               | Auth required | Description |
|--------|---------------------|---------------|-------------|
| GET     | `/healthz`          | no            | Server health (includes database ping) |
| GET     | `/networks`         | yes*          | List all networks |
| POST    | `/networks`         | yes*          | Create a network |
| GET     | `/networks/{uuid}`  | yes*          | Get a network by UUID |
| PUT     | `/networks/{uuid}`  | yes*          | Update a network |
| DELETE  | `/networks/{uuid}`  | yes*          | Delete a network (refused if subnets have this network as parent) |
| GET     | `/networks/{uuid}/subnets` | yes*   | List all subnets in a network |
| POST    | `/networks/{uuid}/subnets` | yes*   | Create a direct child subnet of the network |
| GET     | `/networks/{uuid}/subnets/{subnet_uuid}` | yes* | Get a subnet in the network |
| PUT     | `/networks/{uuid}/subnets/{subnet_uuid}` | yes* | Update a subnet description |
| DELETE  | `/networks/{uuid}/subnets/{subnet_uuid}` | yes* | Delete a subnet (refused if it has children) |
| GET     | `/networks/{uuid}/subnets/{subnet_uuid}/subnets` | yes* | List child subnets of a subnet |
| POST    | `/networks/{uuid}/subnets/{subnet_uuid}/subnets` | yes* | Create a child subnet of a subnet |

> \* Auth is required when `KILHOG_API_KEY` is set. When the variable is empty or unset, all routes remain open (local development default).

> **Tenancy**: all subnet operations go through `/networks/{uuid}/…`. The network `uuid` in the URL is the isolation boundary; the server verifies that each subnet belongs to that network (directly or via the parent hierarchy).

### Authentication

Minimal API key protection is enabled when the `KILHOG_API_KEY` environment variable is set to a non-empty value.

| Aspect | Behavior |
|--------|----------|
| Scope | All routes except `GET /healthz` (health probes stay public) |
| Disabled | When `KILHOG_API_KEY` is empty or unset |
| Comparison | Constant-time (`crypto/subtle`) to reduce timing leaks |

Clients must send the key in one of these headers:

| Header | Format |
|--------|--------|
| `Authorization` | `Bearer <api_key>` |
| `X-API-Key` | `<api_key>` |

Missing or invalid credentials:

```json
{
  "status": "error",
  "message": "missing or invalid API key",
  "code": 401
}
```

### Tenancy and API scoping

The API is organized around the **network as the tenancy boundary**:

- **RBAC**: permissions can be defined per `network/{uuid}` without walking the subnet tree.
- **Multi-tenancy**: every subnet request explicitly carries the target network; a subnet from another network returns `404`.
- **Merge / federation**: two instances can merge networks independently; the network UUID is the grouping key.
- **Implicit parent**: the create request body no longer contains `parent` — it is derived from the URL, avoiding URL/body inconsistencies.

```
/networks/{uuid}/subnets                              → direct children of the network
/networks/{uuid}/subnets/{subnet_uuid}                → subnet CRUD
/networks/{uuid}/subnets/{subnet_uuid}/subnets        → direct children of the subnet
```

The `parent` field remains exposed in **responses** (immutable after creation) so clients can reconstruct the hierarchy.

### `GET /healthz`

```json
{
  "status": "success",
  "data": {
    "status": "ok"
  }
}
```

### Networks

#### `GET /networks`

Lists all networks, sorted by name.

`200 OK` response:

```json
{
  "status": "success",
  "data": [
    {
      "uuid": "550e8400-e29b-41d4-a716-446655440000",
      "name": "lab",
      "description": "Lab network",
      "tags": [{"key": "env", "value": "dev"}]
    }
  ]
}
```

#### `POST /networks`

Creates a network. UUID is generated server-side.

Request body:

```json
{
  "name": "lab",
  "description": "Lab network",
  "tags": [{"key": "env", "value": "dev"}]
}
```

| Field         | Required | Description |
|---------------|----------|-------------|
| `name`        | yes      | Unique name in the database |
| `description` | no       | Free-form text |
| `tags`        | no       | Key–value pairs (unique keys per resource) |

`201 Created` response: the created network in `data`.

Errors:

| Code | Condition |
|------|-----------|
| `400` | Invalid JSON body, missing `name`, duplicate tag key |
| `409` | `name` already in use |

#### `GET /networks/{uuid}`

Gets a network by UUID.

`200 OK` response: the network in `data`.

Errors:

| Code | Condition |
|------|-----------|
| `400` | Invalid UUID |
| `404` | Network not found |

#### `PUT /networks/{uuid}`

Updates an existing network. Request body has the same shape as `POST /networks`.

`200 OK` response: the updated network in `data`.

Errors:

| Code | Condition |
|------|-----------|
| `400` | Invalid UUID or body |
| `404` | Network not found |
| `409` | `name` already used by another network |

#### `DELETE /networks/{uuid}`

Deletes a network **only if it has no child subnets** (subnets whose parent is this network). If at least one subnet references this network as parent, deletion is refused.

`200 OK` response:

```json
{
  "status": "success",
  "data": null
}
```

Errors:

| Code | Condition |
|------|-----------|
| `400` | Invalid UUID |
| `404` | Network not found |
| `409` | Network has child subnets |

### Service layer (`internal/service/network.go`)

`NetworkService` encapsulates network business logic:

- UUID generation on create
- `name` validation (required, trim)
- `name` uniqueness (application check before insert/update)
- Tag validation (unique keys)
- **Delete protection**: calls `SubnetRepository.ListByParent` with `parent.kind = network`; if subnets exist, returns `ErrNetworkHasChildren` (HTTP 409)

### Subnets

All subnet routes are **scoped by network** (`{uuid}` = network UUID). The parent is **not** provided in the request body: it is implicit via the URL.

| Route | Implicit parent |
|-------|-----------------|
| `POST /networks/{uuid}/subnets` | The network `{uuid}` |
| `POST /networks/{uuid}/subnets/{subnet_uuid}/subnets` | The subnet `{subnet_uuid}` |

The `parent` field remains in JSON **responses** (immutable after creation).

#### `GET /networks/{uuid}/subnets`

Lists all subnets belonging to the network, sorted by name.

`200 OK` response:

```json
{
  "status": "success",
  "data": [
    {
      "uuid": "660e8400-e29b-41d4-a716-446655440001",
      "name": "dmz",
      "description": "DMZ subnet",
      "prefix": 24,
      "address": "10.0.0.0",
      "type": "ipv4",
      "parent": {"kind": "network", "uuid": "550e8400-e29b-41d4-a716-446655440000"}
    }
  ]
}
```

Errors:

| Code | Condition |
|------|-----------|
| `400` | Invalid network UUID |
| `404` | Network not found |

#### `POST /networks/{uuid}/subnets`

Creates an IPv4 subnet as a **direct child of the network**. UUID is generated server-side.

Request body:

```json
{
  "name": "dmz",
  "description": "DMZ subnet",
  "prefix": 24,
  "address": "10.0.0.0",
  "type": "ipv4"
}
```

| Field         | Required | Description |
|---------------|----------|-------------|
| `name`        | yes      | Unique name within the network |
| `description` | no       | Free-form text |
| `prefix`      | yes      | IPv4 prefix length (1–32) |
| `address`     | yes      | Network or host address (required because parent = network) |
| `type`        | no       | `ipv4` (default); `ipv6` rejected for now |

Business rules:

- No overlap with other subnets sharing the same parent (the network)
- No parent CIDR constraint (a network has no address space)

`201 Created` response: the created subnet in `data`.

Errors:

| Code | Condition |
|------|-----------|
| `400` | Invalid network UUID, invalid JSON body, missing required fields, invalid address |
| `404` | Network not found |
| `409` | Name already in use, overlap with a sibling |

#### `POST /networks/{uuid}/subnets/{subnet_uuid}/subnets`

Creates an IPv4 subnet as a **child of an existing subnet** in the same network.

Request body (explicit address):

```json
{
  "name": "apps",
  "description": "Application subnet",
  "prefix": 25,
  "address": "10.0.0.0",
  "type": "ipv4"
}
```

Request body (auto-generated address — omit `address`):

```json
{
  "name": "apps",
  "prefix": 25,
  "type": "ipv4"
}
```

| Field         | Required | Description |
|---------------|----------|-------------|
| `name`        | yes      | Unique name within the network |
| `description` | no       | Free-form text |
| `prefix`      | yes      | IPv4 prefix length (1–32), more specific than the parent |
| `address`     | no       | Auto-generated if absent, within the parent CIDR |
| `type`        | no       | `ipv4` (default) |

Business rules:

- The parent subnet must belong to network `{uuid}`
- The address (explicit or generated) must belong to the parent CIDR
- No overlap among siblings of the same parent subnet

Errors:

| Code | Condition |
|------|-----------|
| `400` | Invalid UUIDs, invalid address, subnet outside parent CIDR, prefix too broad |
| `404` | Network or parent subnet not found in this network |
| `409` | Name already in use, overlap, no free address |

#### `GET /networks/{uuid}/subnets/{subnet_uuid}/subnets`

Lists direct child subnets of a parent subnet.

`200 OK` response: array of subnets in `data`.

Errors:

| Code | Condition |
|------|-----------|
| `400` | Invalid UUIDs |
| `404` | Network or parent subnet not found in this network |

#### `GET /networks/{uuid}/subnets/{subnet_uuid}`

Gets a subnet by UUID, verifying it belongs to the network.

`200 OK` response: the subnet in `data`.

Errors:

| Code | Condition |
|------|-----------|
| `400` | Invalid UUIDs |
| `404` | Network not found, or subnet not found in this network |

#### `PUT /networks/{uuid}/subnets/{subnet_uuid}`

Updates **only** the `description`. Fields `name`, `prefix`, `address`, `type`, and `parent` are immutable.

Request body:

```json
{
  "description": "New description"
}
```

`200 OK` response: the updated subnet in `data`.

Errors:

| Code | Condition |
|------|-----------|
| `400` | Invalid UUIDs or body |
| `404` | Network not found, or subnet not found in this network |

#### `DELETE /networks/{uuid}/subnets/{subnet_uuid}`

Deletes a subnet **only if it has no child subnets**.

`200 OK` response:

```json
{
  "status": "success",
  "data": null
}
```

Errors:

| Code | Condition |
|------|-----------|
| `400` | Invalid UUIDs |
| `404` | Network not found, or subnet not found in this network |
| `409` | Subnet has child subnets |

### Service layer (`internal/service/subnet.go`)

`SubnetService` encapsulates IPv4 subnet business logic:

- **Tenancy scoping**: every operation receives `networkUUID` and verifies membership via `ensureInNetwork`
- UUID generation on create
- `name` validation (required, trim, unique within the network)
- Implicit parent derived from the URL (`CreateInNetwork`)
- IPv4 address normalization (e.g. `192.168.1.5/24` → `192.168.1.0/24`)
- Sibling overlap detection via `internal/iputil`
- Automatic address generation within the parent subnet CIDR (only when `address` is absent)
- **Limited updates**: only `description` is modifiable
- **Delete protection**: refused if child subnets exist (`ErrSubnetHasChildren`, HTTP 409)

## Configuration

| Variable           | Default             | Description |
|--------------------|---------------------|-------------|
| `KILHOG_HOST`      | `0.0.0.0`           | HTTP listen address |
| `KILHOG_PORT`      | `8080`              | HTTP listen port |
| `KILHOG_API_KEY`   | *(empty)*           | API key for protected routes; auth disabled when unset |
| `KILHOG_DB_DRIVER` | `sqlite`            | Database driver: `sqlite` or `postgres` |
| `KILHOG_DB_DSN`    | `file:kilhog.db`    | Connection DSN (see examples below) |
| `KILHOG_AUTO_MIGRATE` | `true`           | Apply upgrade migrations at startup |

### DSN examples

| Driver   | Example DSN |
|----------|-------------|
| SQLite   | `file:./data/kilhog.db?_pragma=foreign_keys(ON)` |
| PostgreSQL | `postgres://user:pass@localhost:5432/kilhog?sslmode=disable` |

## JSON response format

- Success: `{"status": "success", "data": ...}`
- Error: `{"status": "error", "message": "...", "code": 400}`

Conflict (`409`) and validation (`400`) error messages are **explicit**: they include the values involved (name, CIDR, prefix, etc.) to help client-side diagnosis.

Example — subnet name already in use:

```json
{
  "status": "error",
  "message": "subnet name \"dmz\" is already used in this network",
  "code": 409
}
```

Example — CIDR overlap:

```json
{
  "status": "error",
  "message": "subnet 10.0.0.0/24 overlaps with an existing sibling under the same parent",
  "code": 409
}
```

## Build

```bash
make build
```

Compiles the binary to `bin/kilhog` from `./cmd/kilhog`.

## Run

### Local development (recommended)

```bash
make run-dev
```

Builds and runs the application with SQLite:

- **Driver**: `sqlite`
- **File**: `kilhog.db` at the project root (created automatically on first startup)
- **Migrations**: applied automatically (`KILHOG_AUTO_MIGRATE=true` by default)
- **Listen**: `http://0.0.0.0:8080`

The `kilhog.db` file (and SQLite auxiliary files `kilhog.db-wal`, `kilhog.db-shm`) is ignored by Git (see `.gitignore`).

### Direct run

```bash
go run ./cmd/kilhog
```

Uses the same defaults as `run-dev` (`sqlite`, DSN `file:kilhog.db?_pragma=foreign_keys(ON)`).

### Development HTTP scripts

The `scripts/dev/` folder contains standalone Bash scripts that call the REST API on a locally running instance (`make run-dev`). Each script reads one or more UTF-8 JSON files located next to it; there is no shared library between scripts.

**Prerequisites**: `curl`. Scripts `update-network-hors-prod.sh` and `delete-network-prod.sh` also use `jq` to look up a network UUID by `name`.

| Variable | Default | Description |
|----------|---------|-------------|
| `KILHOG_BASE_URL` | `http://localhost:8080` | API base URL |
| `KILHOG_API_KEY` | *(empty)* | API key sent as `Authorization: Bearer …` when set |

When the server runs with `KILHOG_API_KEY` set, export the same value before calling the dev scripts (or pass it inline: `KILHOG_API_KEY=secret make dev-create-networks`).

| Script | JSON file(s) | Make target | Action |
|--------|--------------|-------------|--------|
| `create-networks.sh` | `network-prod.json`, `network-hors-prod.json` | `make dev-create-networks` | Creates `prod` and `hors-prod` |
| `update-network-hors-prod.sh` | `network-hors-prod-update.json` | `make dev-update-network-hors-prod` | Updates `hors-prod` |
| `delete-network-prod.sh` | — | `make dev-delete-network-prod` | Deletes `prod` |
| `create-subnets.sh` | `subnet-dmz.json`, `subnet-apps-auto.json` | `make dev-create-subnets` | Creates `dmz` under `hors-prod` (explicit address) then `apps` under `dmz` (auto address) |
| `update-subnet-dmz.sh` | `subnet-dmz-update.json` | `make dev-update-subnet-dmz` | Updates `dmz` description |
| `delete-subnet-apps.sh` | — | `make dev-delete-subnet-apps` | Deletes `apps` |

Payload contents:

| File | `name` | `description` |
|------|--------|---------------|
| `network-prod.json` | `prod` | `production network` |
| `network-hors-prod.json` | `hors-prod` | *(absent)* |
| `network-hors-prod-update.json` | `hors-prod` | `non-production network` |
| `subnet-dmz.json` | `dmz` | `DMZ subnet with explicit address 10.0.0.0/24` |
| `subnet-apps-auto.json` | `apps` | `Apps subnet under dmz, auto-generated /25 address` |
| `subnet-dmz-update.json` | — | `Updated DMZ subnet` |

Example workflow:

```bash
# Terminal 1
make run-dev

# Terminal 2
make dev-create-networks
make dev-create-subnets
make dev-update-subnet-dmz
make dev-delete-subnet-apps
make dev-update-network-hors-prod
make dev-delete-network-prod
```
