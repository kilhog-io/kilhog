# TECHNICAL — kilhog API

## Stack

- **Language**: Go 1.26+
- **Module**: `github.com/kilhog-io/kilhog`
- **Architecture**: REST API, layered design

## Directory layout

```
kilhog/
├── cmd/
│   ├── kilhog/          # API server entry point (main.go, SIGTERM shutdown)
│   ├── kilhog-worker/   # Cloudflare Workers WASM entry point (GOOS=js GOARCH=wasm)
│   └── pogig/           # CLI entry point (Breton for "chick")
│       └── internal/cmd/ # Cobra commands (network, subnet, grant, health)
├── pkg/
│   └── kilhog/          # Public Go SDK for the REST API (shared by pogig and external consumers)
├── internal/
│   ├── handler/         # HTTP handlers and request validation
│   ├── log/             # Structured logging (slog) and HTTP request middleware
│   ├── metrics/         # OpenTelemetry metrics (Prometheus /metrics exporter)
│   ├── service/         # Business logic and repository interfaces
│   ├── repository/      # Data access, migrations, SQL drivers
│   │   ├── migration/   # Versioned migration runner (upgrade / downgrade)
│   │   ├── postgres/    # PostgreSQL implementation (native builds only)
│   │   ├── sqlite/      # SQLite implementation (native builds only)
│   │   └── d1/          # Cloudflare D1 implementation (WASM / Workers builds)
│   └── model/           # Models and data structures
├── .github/
│   └── workflows/       # GitHub Actions CI/CD pipelines
├── migrations/          # Embedded versioned SQL scripts (sqlite/ and postgres/)
├── workers/             # Cloudflare Workers project (Wrangler, WASM build assets)
├── scripts/
│   └── dev/             # HTTP scripts for local development
├── Dockerfile           # Multi-stage build → scratch image for the API server
├── .dockerignore        # Build context exclusions for Docker
├── terraform/           # GCP Cloud Run + GCS (SQLite) deployment
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
| `LocalUser`    | `user.go`          | Local username/password account |
| `IdentityPool` | `identity_pool.go` | OIDC identity pool configuration |
| `Session`      | `session.go`       | Server-side session + OIDC login state |
| `MachinePool`  | `machine.go`       | Group of machine identities |
| `MachineIdentityProvider` | `machine.go` | JWT/JWKS trust config owned by a machine pool |
| `Machine`      | `machine.go`       | Named workload identity |
| `MachineAPIKey`| `machine.go`       | Per-machine API key metadata (+ plaintext only at creation) |
| `Grant`        | `grant.go`         | RBAC grant: CRUD + ownership on a network, subnet, or platform capability |

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

### `Grant`

RBAC assignment defined in `FUNCTIONAL.md`. One row per `(principal, resource)`.

| Field | Type | JSON |
|-------|------|------|
| `UUID` | `uuid.UUID` | `uuid` |
| `Principal` | `GrantPrincipal` | `principal` |
| `Resource` | `GrantResource` | `resource` |
| `Permissions` | `Permissions` | `permissions` (`create`, `read`, `update`, `delete` booleans) |
| `Owner` | `bool` | `owner` |
| `CreatedAt` | `time.Time` | `created_at` |
| `UpdatedAt` | `time.Time` | `updated_at` |

`GrantPrincipal.Kind` is one of `local_user`, `oidc`, `oidc_group`, `machine`, `machine_pool`.

`GrantResource.Kind` is `network`, `subnet`, or `platform`. Platform resources use `capability` (`create_networks`); others use `uuid`.

`owner = true` implies all four CRUD flags. Platform grants cannot have `owner = true`.

## Repository interfaces (`internal/service`)

Interfaces defined on the service side (consumed by business logic):

- `NetworkRepository` — CRUD, listing, and `Count` of networks
- `SubnetRepository` — CRUD, listing by network or by parent, and `Count`
- `ResourceMetrics` — optional in-memory functional metrics (create/update/delete hooks); no SQL on scrape
- `UserRepository` — local users
- `IdentityPoolRepository` — OIDC identity pools
- `SessionRepository` / `OIDCLoginStateRepository` — sessions and PKCE state
- `MachinePoolRepository` / `MachineProviderRepository` / `MachineRepository` / `MachineAPIKeyRepository` — machine identities
- `GrantRepository` — CRUD and listing of grants (by UUID, by resource, by principal, platform capabilities)
- `AuthorizationService` — evaluates effective CRUD and ownership (inheritance, groups, machine pools, privileged bypass, list filtering)

## IPv4 utilities (`internal/iputil`)

| Function | Role |
|----------|------|
| `ParseIPv4Prefix` | Parse and normalize an IPv4 address/prefix |
| `ValidateIPv4Subnet` | Check parent containment and absence of overlap among siblings |
| `FindFreeIPv4Block` | Find the first free `/prefix` block within a subnet parent CIDR |

## Persistence layer (`internal/repository`)

**Configurable, abstract** data backend. Three drivers are supported: **SQLite**, **PostgreSQL**, and **Cloudflare D1** (Workers / WASM). The service consumes repository interfaces; driver choice is transparent to business logic.

### Abstraction

```
service (NetworkRepository, SubnetRepository, UserRepository, IdentityPoolRepository, SessionRepository, MachinePoolRepository, GrantRepository, …)
    └── repository/
            ├── sqlite/    → SQLite implementations (native)
            ├── postgres/  → PostgreSQL implementations (native)
            ├── d1/        → Cloudflare D1 (WASM / Workers)
            └── migration/ → migration runner (shared; D1 reuses SQLite scripts)
```

Each driver implements the interfaces defined in `internal/service`. SQL queries use adapted dialects where needed (UUID types, `TIMESTAMPTZ`, etc.).

Concrete repository implementations live in `internal/repository/` (`network_repository.go`, `subnet_repository.go`) and are instantiated via `repository.Open`.

Native builds (`GOOS` ≠ `js`) compile SQLite and PostgreSQL drivers only. WASM builds (`GOOS=js GOARCH=wasm`) compile the D1 driver only, keeping the Worker binary smaller.

### Concurrent access

API calls may arrive in parallel. The `db.Store` layer provides:

| Mechanism | Role |
|-----------|------|
| `database/sql` pool | Concurrent connections (parallel reads) |
| `WithWriteLock` | On SQLite, application mutex serializing writes |
| `WithTx` / `WithWriteTx` | Atomic SQL transactions |
| `AcquireMigrationLock` | Exclusive lock during migrations (SQLite mutex, PostgreSQL `pg_advisory_lock`) |
| WAL + `busy_timeout` (SQLite) | Readers not blocked by writers |
| `Flush` / `Close` | On SIGTERM, checkpoint SQLite WAL (`TRUNCATE`) then close the file |

Mutations (`Create`, `Update`, `Delete`) go through `WithWriteTx`: SQLite write lock + SQL transaction. Reads (`Get*`, `List*`) use the pool directly.

Subnet creation with auto-allocated addresses uses `SubnetRepository.CreateAtomically`: sibling listing, overlap validation, and insert run in the same write transaction. The parent row is locked (`SELECT … FOR UPDATE` on PostgreSQL; SQLite relies on the application write mutex) so parallel creates under the same parent cannot pick the same CIDR.

On PostgreSQL, the SQLite application lock is disabled: concurrency is handled by MVCC and SQL transactions.

### Graceful shutdown (Cloud Run / SIGTERM)

Cloud Run sends **SIGTERM**, waits **10 seconds**, then **SIGKILL**. The native API server (`cmd/kilhog`) handles `SIGTERM` and `SIGINT` so SQLite is synchronized and the file is closed before the process exits.

Sequence (`cmd/kilhog/shutdown.go`):

1. Log the received signal and stop the metrics refresh loop.
2. **Stop HTTP** with `http.Server.Shutdown` (timeout **8s**). In-flight requests can finish. If the timeout expires, `http.Server.Close` force-closes remaining connections. This leaves **2s** of the Cloud Run window for the database.
3. **Synchronize SQLite** with `PRAGMA wal_checkpoint(TRUNCATE)` (`db.Store.Flush`): WAL pages are merged into the main `.db` file and the WAL is truncated. Idle pooled connections are dropped so the checkpoint can complete. PostgreSQL and D1 are no-ops.
4. **Close the database handle** (`db.Store.CloseContext`): closes the SQLite file (or PostgreSQL pool). Subsequent `Close` / `Flush` calls are no-ops.
5. Return from `main` so remaining defers run (metrics provider shutdown, ~1s). The process then exits.

`db.Store.Close` always checkpoints SQLite before closing the handle (bounded by `DefaultSQLiteFlushTimeout`, 2s), including test teardown and startup-failure paths.

Do **not** `os.Exit` after SIGTERM: that would skip defers. HTTP shutdown failure is logged; the process still flushes and closes the database.

This matters for WAL on a volume (Cloud Run volume / NFS / local disk): after a clean shutdown the durable state is in `kilhog.db`, not a leftover `-wal` sidecar that a new instance might not see.

### Cloudflare D1 notes

D1 is SQLite-compatible and uses the embedded **SQLite** migration scripts. The Go D1 driver (`github.com/syumai/workers/cloudflare/d1`) does **not** support SQL `BEGIN`/`COMMIT`. For `DialectD1`, `Store.WithTx` therefore executes statements sequentially under the application write mutex (best-effort atomicity, not a real SQL transaction). Prefer low write concurrency on the Worker deployment.

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
├── 001_initial_schema.down.sql
├── 002_auth.up.sql
├── 002_auth.down.sql
├── 003_machine_identities.up.sql
├── 003_machine_identities.down.sql
├── 004_rbac.up.sql
└── 004_rbac.down.sql
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

#### Auth script (`002_auth`)

Adds local users, OIDC identity pools, sessions, and OIDC login-state tables (see below).

#### Machine identities script (`003_machine_identities`)

Adds machine pools, JWT providers (JWKS), machines, and per-machine API keys.

#### RBAC script (`004_rbac`)

Adds the `grants` table, `groups_claim` on OIDC identity pools, and `oidc_groups` on sessions. Downgrade drops the grants table and those columns.

#### `local_users` table

| Column          | Type          | Constraints | Maps to |
|-----------------|---------------|-------------|---------|
| `uuid`          | UUID          | PK          | `LocalUser.UUID` |
| `username`      | TEXT          | NOT NULL, unique (case-insensitive) | `LocalUser.Username` |
| `password_hash` | TEXT          | NOT NULL    | bcrypt hash (never returned by API) |
| `display_name`  | TEXT          | NULL        | `LocalUser.DisplayName` |
| `email`         | TEXT          | NULL        | `LocalUser.Email` |
| `role`          | TEXT          | NOT NULL, CHECK IN (`admin`, `user`) | `LocalUser.Role` |
| `enabled`       | BOOL/INT      | NOT NULL    | `LocalUser.Enabled` |
| `created_at`    | TIMESTAMPTZ   | NOT NULL    | `LocalUser.CreatedAt` |
| `updated_at`    | TIMESTAMPTZ   | NOT NULL    | `LocalUser.UpdatedAt` |

#### `oidc_identity_pools` table

| Column          | Type          | Constraints | Maps to |
|-----------------|---------------|-------------|---------|
| `uuid`          | UUID          | PK          | `IdentityPool.UUID` |
| `name`          | TEXT          | NOT NULL, UNIQUE | `IdentityPool.Name` |
| `slug`          | TEXT          | NOT NULL, UNIQUE | `IdentityPool.Slug` |
| `issuer`        | TEXT          | NOT NULL, UNIQUE | `IdentityPool.Issuer` |
| `client_id`     | TEXT          | NOT NULL    | `IdentityPool.ClientID` |
| `client_secret` | TEXT          | NULL        | stored secret (never returned; `has_client_secret` exposed) |
| `scopes`        | TEXT          | NOT NULL (JSON array) | `IdentityPool.Scopes` |
| `groups_claim`  | TEXT          | NOT NULL, default `groups` | JWT claim listing IdP groups |
| `enabled`       | BOOL/INT      | NOT NULL    | `IdentityPool.Enabled` |
| `created_at` / `updated_at` | TIMESTAMPTZ | NOT NULL | timestamps |

#### `sessions` table

| Column | Description |
|--------|-------------|
| `uuid` | Session id |
| `token_hash` | SHA-256 hex of opaque session token (UNIQUE) |
| `principal_kind` | `local_user` or `oidc` |
| `local_user_uuid` | FK → `local_users` (CASCADE) when kind is local |
| `identity_pool_uuid` | FK → pools (SET NULL) when kind is OIDC |
| `oidc_subject` / `oidc_email` / `oidc_name` | Federated claims |
| `oidc_groups` | JSON array of group names from `groups_claim` (OIDC sessions) |
| `expires_at` | Session expiry |

#### `oidc_login_states` table

Short-lived PKCE/`state`/`nonce` rows for Authorization Code + PKCE (TTL ~10 minutes). Consumed atomically on callback (`Take`).

#### `machine_pools` table

| Column | Constraints | Maps to |
|--------|-------------|---------|
| `uuid` | PK | `MachinePool.UUID` |
| `name` | NOT NULL, UNIQUE | `MachinePool.Name` |
| `slug` | NOT NULL, UNIQUE | `MachinePool.Slug` |
| `description` | NULL | `MachinePool.Description` |
| `enabled` | BOOL/INT NOT NULL | `MachinePool.Enabled` |
| `created_at` / `updated_at` | NOT NULL | timestamps |

#### `machine_identity_providers` table

| Column | Constraints | Maps to |
|--------|-------------|---------|
| `uuid` | PK | `MachineIdentityProvider.UUID` |
| `machine_pool_uuid` | FK → `machine_pools` ON DELETE CASCADE | Parent pool |
| `name` | UNIQUE `(machine_pool_uuid, name)` | `Name` |
| `issuer` | UNIQUE `(machine_pool_uuid, issuer)` | JWT `iss` |
| `audiences` | JSON array, NOT NULL | Accepted JWT `aud` values |
| `jwks_mode` | `discovery`, `uri`, or `static` | How keys are supplied |
| `jwks_uri` | NULL | Required for `uri` |
| `jwks` | NULL | Static JWKS JSON for `static` |
| `enabled` | BOOL/INT NOT NULL | |

#### `machines` table

| Column | Constraints | Maps to |
|--------|-------------|---------|
| `uuid` | PK | `Machine.UUID` |
| `machine_pool_uuid` | FK → pools ON DELETE CASCADE | Parent pool |
| `name` | UNIQUE `(machine_pool_uuid, name)` | `Name` |
| `provider_uuid` | FK → providers ON DELETE SET NULL | JWT provider, optional |
| `subject` / `subject_prefix` | NULL | JWT `sub` matching |
| `claims` | JSON array of `{claim, op, value}` | Extra claim conditions (`eq` / `in` / `prefix`) |
| `enabled` | BOOL/INT NOT NULL | |

#### `machine_api_keys` table

| Column | Constraints | Maps to |
|--------|-------------|---------|
| `uuid` | PK | `MachineAPIKey.UUID` |
| `machine_uuid` | FK → machines ON DELETE CASCADE | Parent machine |
| `prefix` | UNIQUE | Public prefix `khog_mkey_…` |
| `token_hash` | UNIQUE | SHA-256 of the full secret |
| `name` | NULL | Label |
| `expires_at` / `revoked_at` / `last_used_at` | NULL | Lifecycle |

The plaintext secret is never stored. HTTP `DELETE` on a key sets `revoked_at`.

#### `grants` table

One ACL row per principal and resource. Permission flags are booleans (`INTEGER` 0/1 on SQLite).

| Column | Type | Constraints | Maps to |
|--------|------|-------------|---------|
| `uuid` | UUID | PK | `Grant.UUID` |
| `principal_kind` | TEXT | NOT NULL, CHECK IN (`local_user`, `oidc`, `oidc_group`, `machine`, `machine_pool`) | `GrantPrincipal.Kind` |
| `local_user_uuid` | UUID | NULL, FK → `local_users` ON DELETE CASCADE | `local_user` |
| `identity_pool_uuid` | UUID | NULL, FK → `oidc_identity_pools` ON DELETE CASCADE | `oidc` / `oidc_group` |
| `oidc_subject` | TEXT | NULL | IdP `sub` when `oidc` |
| `oidc_group` | TEXT | NULL | Group name when `oidc_group` |
| `machine_uuid` | UUID | NULL, FK → `machines` ON DELETE CASCADE | `machine` |
| `machine_pool_uuid` | UUID | NULL, FK → `machine_pools` ON DELETE CASCADE | `machine_pool` |
| `resource_kind` | TEXT | NOT NULL, CHECK IN (`network`, `subnet`, `platform`) | `GrantResource.Kind` |
| `resource_uuid` | UUID | NULL | Network or subnet UUID |
| `capability` | TEXT | NULL, CHECK IN (`create_networks`) | Platform capability |
| `can_create` | BOOL/INT | NOT NULL | `permissions.create` |
| `can_read` | BOOL/INT | NOT NULL | `permissions.read` |
| `can_update` | BOOL/INT | NOT NULL | `permissions.update` |
| `can_delete` | BOOL/INT | NOT NULL | `permissions.delete` |
| `owner` | BOOL/INT | NOT NULL | `owner` |
| `created_at` / `updated_at` | TIMESTAMPTZ | NOT NULL | timestamps |

Checks:

- Principal columns match `principal_kind` (exactly the columns for that kind are NOT NULL).
- If `resource_kind` is `network` or `subnet`: `resource_uuid` NOT NULL, `capability` NULL; at least one of the four CRUD flags or `owner` is true.
- If `resource_kind` is `platform`: `capability` NOT NULL, `resource_uuid` NULL, `owner` false, `can_create` true.
- If `owner` is true: resource is `network` or `subnet` (CRUD treated as all true).

Unique indexes:

- local user + resource
- OIDC principal (`identity_pool_uuid`, `oidc_subject`) + resource
- OIDC group (`identity_pool_uuid`, `oidc_group`) + resource
- machine + resource
- machine pool + resource

Resource identity for uniqueness is `(resource_kind, resource_uuid, capability)`.

Lookup indexes on `(resource_kind, resource_uuid)`, `local_user_uuid`, `(identity_pool_uuid, oidc_subject)`, `(identity_pool_uuid, oidc_group)`, `machine_uuid`, `machine_pool_uuid`.

Resource cascade (same polymorphic pattern as `tags`):

- `(network, resource_uuid)` → `networks(uuid)` ON DELETE CASCADE
- `(subnet, resource_uuid)` → `subnets(uuid)` ON DELETE CASCADE

```
networks / subnets ──< grants
local_users | oidc pools | machines | machine_pools ──< grants
```

### Dialect differences

| Aspect | SQLite | PostgreSQL | Cloudflare D1 |
|--------|--------|------------|---------------|
| UUID type | `TEXT` (canonical format) or `BLOB` | Native `UUID` | `TEXT` (SQLite scripts) |
| Timestamps | `TEXT` ISO-8601 or `DATETIME` | `TIMESTAMPTZ` | Same as SQLite |
| Database creation | File on disk | `CREATE DATABASE` via admin connection | Provisioned via Wrangler (`wrangler d1 create`) |
| Go driver | `modernc.org/sqlite` | `jackc/pgx/v5` | `github.com/syumai/workers/cloudflare/d1` |
| SQL transactions | Yes | Yes | No (driver limitation; see D1 notes) |
| Build target | Native | Native | `GOOS=js GOARCH=wasm` |

Migrations may contain dialect-specific sections if needed; otherwise SQL stays as portable as possible. D1 loads the `sqlite/` migration folder via `Dialect.MigrationDialect()`.

## Dependencies

| Module | Usage |
|--------|-------|
| `github.com/google/uuid` | Resource `UUID` identifiers |
| `github.com/spf13/cobra` | pogig CLI command tree |
| `database/sql` | Standard Go SQL abstraction |
| `modernc.org/sqlite` | SQLite driver (pure Go, native builds) |
| `jackc/pgx/v5` | PostgreSQL driver (native builds) |
| `go.opentelemetry.io/otel` | OpenTelemetry metrics API |
| `go.opentelemetry.io/otel/sdk` | MeterProvider / resource |
| `go.opentelemetry.io/otel/exporters/prometheus` | Prometheus scrape exporter (OTel-compatible) |
| `go.opentelemetry.io/contrib/instrumentation/runtime` | Go runtime system metrics |
| `github.com/prometheus/client_golang` | Prometheus registry and `/metrics` HTTP handler |
| `github.com/syumai/workers` | Cloudflare Workers HTTP + D1 (WASM builds) |
| `github.com/coreos/go-oidc/v3` | OIDC ID token verification |
| `github.com/go-jose/go-jose/v4` | Machine JWT signature verification and JWKS |
| `golang.org/x/oauth2` | OIDC Authorization Code + PKCE |
| `golang.org/x/crypto` | bcrypt password hashing |

## API routes

| Method | Route               | Auth required | Description |
|--------|---------------------|---------------|-------------|
| GET     | `/healthz`          | no            | Server health (includes database ping) |
| GET     | `/metrics`          | no            | OpenTelemetry metrics in Prometheus exposition format |
| GET     | `/auth/status`      | no            | Auth configuration / bootstrap availability |
| POST    | `/auth/bootstrap`   | no*           | Create primo-admin when no local user exists |
| POST    | `/auth/login`       | no            | Local username/password login |
| POST    | `/auth/logout`      | no            | End session (cookie / bearer token) |
| GET     | `/auth/me`          | yes*          | Current principal |
| GET     | `/auth/me/grants`   | yes*          | Grants targeting the current principal (including group/pool) |
| GET     | `/auth/oidc/pools`  | no            | List enabled OIDC pools (name + slug) |
| GET     | `/auth/oidc/{slug}/login` | no      | Start OIDC Authorization Code + PKCE |
| GET     | `/auth/oidc/callback` | no          | OIDC callback; issues session |
| GET/POST/PUT/DELETE | `/users…` | admin | Local user administration |
| POST    | `/users/me/password`| local user    | Change own password |
| GET/POST/PUT/DELETE | `/auth/identity-pools…` | admin | OIDC pool administration |
| GET/POST/PUT/DELETE | `/auth/machine-pools…` | admin | Machine pools, JWT providers, machines, API keys |
| GET/POST/PUT/DELETE | `/auth/platform-grants…` | admin | Platform `create_networks` grants |
| GET/POST/PUT/DELETE | `/networks/{uuid}/grants…` | admin or owner | Grants on a network |
| POST    | `/networks/{uuid}/ownership/transfer` | admin or owner | Transfer network ownership |
| GET/POST/PUT/DELETE | `/networks/{uuid}/subnets/{subnet_uuid}/grants…` | admin or owner | Grants on a subnet |
| POST    | `/networks/{uuid}/subnets/{subnet_uuid}/ownership/transfer` | admin or owner | Transfer subnet ownership |
| GET     | `/networks`         | yes*          | List networks the caller may read |
| POST    | `/networks`         | yes*          | Create a network |
| GET     | `/networks/{uuid}`  | yes*          | Get a network by UUID |
| PUT     | `/networks/{uuid}`  | yes*          | Update a network |
| DELETE  | `/networks/{uuid}`  | yes*          | Delete a network (refused if subnets have this network as parent) |
| GET     | `/networks/{uuid}/subnets` | yes*   | List subnets in a network the caller may read |
| POST    | `/networks/{uuid}/subnets` | yes*   | Create a direct child subnet of the network |
| GET     | `/networks/{uuid}/subnets/{subnet_uuid}` | yes* | Get a subnet in the network |
| PUT     | `/networks/{uuid}/subnets/{subnet_uuid}` | yes* | Update a subnet description |
| DELETE  | `/networks/{uuid}/subnets/{subnet_uuid}` | yes* | Delete a subnet (refused if it has children) |
| GET     | `/networks/{uuid}/subnets/{subnet_uuid}/subnets` | yes* | List child subnets of a subnet |
| POST    | `/networks/{uuid}/subnets/{subnet_uuid}/subnets` | yes* | Create a child subnet of a subnet |

> \* Protected IPAM and admin routes require authentication. Auth is configured when at least one of: non-empty `KILHOG_API_KEY`, ≥1 local user, ≥1 enabled OIDC pool, or ≥1 enabled machine pool. Otherwise protected routes return `403`. Invalid credentials return `401`. After authentication, IPAM routes are **authorized per grant** (see [Authorization (RBAC)](#authorization-rbac)): local `admin` and the deployment-wide API key bypass grants; other principals need a matching grant (or ownership / `create_networks`). `GET /healthz`, `GET /metrics`, and public auth discovery/login routes stay reachable without a session.

> **Tenancy**: all subnet operations go through `/networks/{uuid}/…`. The network `uuid` in the URL is the isolation boundary; the server verifies that each subnet belongs to that network (directly or via the parent hierarchy).

### Authentication

Four methods are implemented (OR semantics). See `FUNCTIONAL.md` for business rules.

| Method | How presented | Notes |
|--------|---------------|-------|
| API key (deployment-wide) | `Authorization: Bearer <key>` or `X-API-Key` | Shared secret from `KILHOG_API_KEY`; full IPAM bypass; get-started path; not identity or grant admin |
| Local session | `Authorization: Bearer <session_token>` or cookie `kilhog_session` | Issued by `/auth/bootstrap` or `/auth/login` |
| OIDC | Session after code flow, or Bearer JWT validated against an enabled human pool | IPAM via grants (principal + IdP groups); identity admin requires a local `admin` account |
| Machine identity | Per-machine API key (`khog_mkey_…`) via Bearer or `X-API-Key`, or Bearer JWT | Named principal; IPAM via grants on the machine or its pool; no kilhog session |

Bearer handling order: deployment-wide API key, machine API key (non-JWT), session token, machine JWT (JWKS), then human OIDC JWT.

`GET /healthz` and `GET /metrics` stay public (health probes and Prometheus scrapes).

#### Machine identities

Admin-only routes (local `admin`):

| Method | Route |
|--------|-------|
| GET/POST | `/auth/machine-pools` |
| GET/PUT/DELETE | `/auth/machine-pools/{uuid}` |
| GET/POST | `/auth/machine-pools/{uuid}/providers` |
| GET/PUT/DELETE | `/auth/machine-pools/{uuid}/providers/{provider_uuid}` |
| POST | `/auth/machine-pools/{uuid}/providers/{provider_uuid}/refresh` |
| GET/POST | `/auth/machine-pools/{uuid}/machines` |
| GET/PUT/DELETE | `/auth/machine-pools/{uuid}/machines/{machine_uuid}` |
| GET/POST | `/auth/machine-pools/{uuid}/machines/{machine_uuid}/api-keys` |
| DELETE | `/auth/machine-pools/{uuid}/machines/{machine_uuid}/api-keys/{key_uuid}` |

Provider `jwks_mode`:

| Mode | Config | Key refresh |
|------|--------|-------------|
| `discovery` | `issuer` (HTTPS) | `{issuer}/.well-known/openid-configuration` → `jwks_uri`; cache ~1h, refetch on unknown `kid` |
| `uri` | `issuer` + `jwks_uri` (HTTPS) | Fetch `jwks_uri`; same cache policy |
| `static` | `jwks` (RFC 7517) | Manual update only; `POST …/refresh` returns 400 |

Machine API keys: format `khog_mkey_<prefix>.<secret>`, SHA-256 stored, plaintext returned **once** on `POST`. `DELETE` revokes (`revoked_at`). JWT matching uses `subject`, `subject_prefix`, and claims (`eq` / `in` / `prefix`); exactly one enabled machine must match.

`GET /auth/status` includes `enabled_machine_pools`. `/auth/me` for a machine returns `kind=machine`, pool/machine ids, `auth_method` (`api_key` or `jwt`), and JWT `issuer`/`subject` when applicable.

Implementation: `internal/service/machine.go`, `internal/service/machine_auth.go`, `internal/handler/machine.go`. JWT verification uses `github.com/go-jose/go-jose/v4`.

#### Bootstrap (primo-admin)

`POST /auth/bootstrap` is allowed only while `local_users` is empty. Creates an `admin` and returns a session. Optional `KILHOG_BOOTSTRAP_TOKEN` must match body `bootstrap_token` or header `X-Bootstrap-Token` when set.

#### Sessions

Opaque tokens (32 random bytes, base64url) stored as SHA-256 hashes. Default TTL 24h (`KILHOG_SESSION_TTL`). Logout deletes the hash and clears the cookie. Disabling/deleting an OIDC pool stops new logins; existing sessions remain until expiry.

#### OIDC

- Requires `KILHOG_PUBLIC_URL` (redirect URI = `{KILHOG_PUBLIC_URL}/auth/oidc/callback`).
- Flow: Authorization Code + PKCE via `GET /auth/oidc/{slug}/login` → IdP → `/auth/oidc/callback`.
- Dependencies: `github.com/coreos/go-oidc/v3`, `golang.org/x/oauth2`, `golang.org/x/crypto/bcrypt`.

Missing or invalid credentials when auth is configured:

```json
{
  "status": "error",
  "message": "missing or invalid credentials",
  "code": 401
}
```

Authentication not configured:

```json
{
  "status": "error",
  "message": "authentication is not configured",
  "code": 403
}
```

### Tenancy and API scoping

The API is organized around the **network as the tenancy boundary**:

- **RBAC**: platform `create_networks`, then grants on `network/{uuid}` and optionally `subnet/{uuid}` (see [Authorization (RBAC)](#authorization-rbac)). Owners manage grants inside their tenancy.
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

### `GET /metrics`

Exposes OpenTelemetry metrics in **Prometheus exposition format** (OpenMetrics enabled). Scrapes are intentionally cheap: functional gauges are served from in-memory counters — **no SQL on each scrape**.

#### Architecture (`internal/metrics`)

```
OpenTelemetry MeterProvider
    ├── Prometheus exporter → GET /metrics (promhttp)
    ├── runtime instrumentation → Go system metrics (memory, GC, goroutines, …)
    ├── ResourceTracker → kilhog.networks / kilhog.subnets (+ operation counters)
    │     ├── seed + background Refresh from COUNT(*) (not on scrape)
    │     └── ±1 on local create/delete
    └── HTTPMetrics middleware → request count + duration
```

At startup (`cmd/kilhog`):

1. `metrics.Setup` builds a custom Prometheus registry + OTel MeterProvider.
2. Go runtime metrics start via `go.opentelemetry.io/contrib/instrumentation/runtime` (plus the runtime producer for scheduling histograms).
3. Network/subnet counts are **seeded** with `NetworkRepository.Count` / `SubnetRepository.Count`.
4. A **background refresh** (default every 30s, `KILHOG_METRICS_REFRESH_INTERVAL`) overwrites those gauges from the same `COUNT(*)` queries so replicas converge after mutations handled elsewhere.
5. Successful create/delete/update paths in `NetworkService` / `SubnetService` update the in-memory tracker (optional `WithNetworkMetrics` / `WithSubnetMetrics`).

#### Metric catalog

| OTel name | Kind | Source | Notes |
|-----------|------|--------|-------|
| `go.*` (e.g. `go.goroutine.count`, `go.memory.used`) | gauges / counters | OTel runtime instrumentation | **Per process**; scrape without application SQL |
| `kilhog.networks` | observable gauge | in-memory (DB-reconciled) | Cluster-wide total; seed + local ±1 + periodic refresh |
| `kilhog.subnets` | observable gauge | in-memory (DB-reconciled) | Cluster-wide total; seed + local ±1 + periodic refresh |
| `kilhog.network.operations` | counter | service hooks | **Per process**; `operation` = `create` \| `update` \| `delete` |
| `kilhog.subnet.operations` | counter | service hooks | **Per process**; `operation` = `create` \| `update` \| `delete` |
| `http.server.request.count` | counter | HTTP middleware | **Per process**; method, route, status code / class |
| `http.server.request.duration` | histogram (seconds) | HTTP middleware | **Per process**; `/metrics` itself is not recorded |

Prometheus name translation applies (underscores / `_total` suffixes), for example `kilhog_networks`, `kilhog_network_operations_total`, `go_goroutine_count`.

#### Multi-instance (replicated API)

Several kilhog processes may share one PostgreSQL database. Metrics behave as follows:

| Metric | Scope | If you scrape every replica |
|--------|-------|-----------------------------|
| Go runtime, HTTP, `*.operations` | This process only | **`sum()`** (each request / GC / mutation is counted once, on the instance that handled it) |
| `kilhog.networks`, `kilhog.subnets` | Shared DB total, copied on each process | **`max()`** (or `avg()`), **never `sum()`** — summing would multiply the cluster total by the replica count |

Between refreshes, a replica only applies ±1 for mutations **it** handled. Other replicas catch up on the next `COUNT(*)` refresh (default 30s). Scrapes still never hit SQL.

Example PromQL:

```
max(kilhog_networks)
max(kilhog_subnets)
sum(rate(kilhog_network_operations_total[5m]))
sum(rate(http_server_request_count_total[5m]))
```

#### Response

`200 OK` with `text/plain` (Prometheus / OpenMetrics). Not wrapped in the JSON `{status,data}` envelope.

### Networks

#### `GET /networks`

Lists networks the caller may read, sorted by name. Local `admin` and the deployment-wide API key see every network. Other principals see only networks allowed by grants (including group/pool grants and structural visibility).

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

`201 Created` response: the created network in `data`. For a local `user`, OIDC principal, or machine, the server also inserts an **owner** grant on that network for the creator.

`POST /networks` is allowed for a local `admin`, the deployment-wide API key, or a principal with effective `create_networks`.

Errors:

| Code | Condition |
|------|-----------|
| `400` | Invalid JSON body, missing `name`, duplicate tag key |
| `403` | Authenticated but missing `create_networks` (and not admin / deployment API key) |
| `409` | `name` already in use |

#### `GET /networks/{uuid}`

Gets a network by UUID.

`200 OK` response: the network in `data`.

Errors:

| Code | Condition |
|------|-----------|
| `400` | Invalid UUID |
| `403` | Visible network, missing `read` |
| `404` | Network not found or not visible |

#### `PUT /networks/{uuid}`

Updates an existing network. Request body has the same shape as `POST /networks`.

`200 OK` response: the updated network in `data`.

Errors:

| Code | Condition |
|------|-----------|
| `400` | Invalid UUID or body |
| `403` | Visible network, missing `update` |
| `404` | Network not found or not visible |
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
| `403` | Visible network, missing `delete` |
| `404` | Network not found or not visible |
| `409` | Network has child subnets |

### Service layer (`internal/service/network.go`)

`NetworkService` encapsulates network business logic:

- UUID generation on create
- `name` validation (required, trim)
- `name` uniqueness (application check before insert/update)
- Tag validation (unique keys)
- **Delete protection**: calls `SubnetRepository.ListByParent` with `parent.kind = network`; if subnets exist, returns `ErrNetworkHasChildren` (HTTP 409)
- Optional `WithNetworkMetrics`: updates in-memory functional metrics on successful create/update/delete

### Subnets

All subnet routes are **scoped by network** (`{uuid}` = network UUID). The parent is **not** provided in the request body: it is implicit via the URL. After authentication, each route is authorized per [RBAC](#authorization-rbac).

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
- Optional `WithSubnetMetrics`: updates in-memory functional metrics on successful create/update/delete

## Authorization (RBAC)

Implements `FUNCTIONAL.md` § Authorization. Handlers stay HTTP-facing; evaluation lives in `internal/service`.

### Layering

```
handler (auth middleware → principal on context)
    ├── requireAdmin for /users, /auth/identity-pools, /auth/machine-pools, /auth/platform-grants
    ├── grant routes: requireAdmin OR owner of the resource (admin bypasses ownership)
    └── IPAM handlers call AuthorizationService, then NetworkService / SubnetService
            └── GrantRepository
```

`NetworkService` and `SubnetService` remain HTTP-unaware. After a successful `Create` of a network by a non-privileged principal, the handler (or a small `GrantService.EnsureOwner`) inserts the owner grant in the same write transaction when possible.

### `AuthorizationService` (`internal/service/authz.go`)

| Method | Role |
|--------|------|
| `IsPrivileged(principal)` | Local `admin` or deployment-wide API key |
| `CanCreateNetworks(ctx, principal)` | Privileged, or effective platform `create_networks` |
| `Can(ctx, principal, action, resourceKind, resourceUUID)` | Effective permission after inheritance and group/pool expansion |
| `IsOwner(ctx, principal, resourceKind, resourceUUID)` | `true` for local `admin` (implicit owner); otherwise an owner grant on the resource or an ancestor |
| `Require` / `RequireOwner` | Typed errors for 404 vs 403 |
| `VisibleNetworkUUIDs` / `FilterReadableSubnets` | List filtering |
| `EffectivePermissions` | Union of flags including `owner` |

`action` is `create`, `read`, `update`, `delete`, or `manage_grants` (equivalent to owner check).

#### Evaluation algorithm

1. Local `admin` → allow all IPAM actions **and** treat as owner of every network/subnet (`IsOwner` is always true). Deployment-wide API key → allow IPAM actions only (not grant/ownership routes).
2. Expand the principal to a set of grant subjects: self, plus OIDC groups from the session/JWT, plus the machine’s pool when `kind = machine`. **Drop subjects that are disabled or deleted** (inert grants).
3. Load grants for remaining subjects on the target, ancestor subnets, and root network (and platform row for `create_networks`).
4. Union flags. `owner` implies all CRUD.
5. Structural visibility: `read` on ancestors of any granted subnet.

#### Handler mapping

| Route | Check |
|-------|--------|
| `GET /networks` | Filter via `VisibleNetworkUUIDs` |
| `POST /networks` | `CanCreateNetworks`; then `EnsureOwner` for non-privileged creators |
| `GET/PUT/DELETE /networks/{uuid}` | `read` / `update` / `delete` on that network |
| `POST /networks/{uuid}/subnets` | `create` on the network |
| Subnet GET/PUT/DELETE | `read` / `update` / `delete` on that subnet |
| `POST …/subnets/{id}/subnets` | `create` on the parent subnet |
| Grant admin on a network/subnet | `RequireOwner` (local `admin` always passes) |
| `POST …/ownership/transfer` | `RequireOwner`; then `GrantService.Transfer` |
| `/auth/platform-grants` | Local `admin` only |

`404` when the resource is not visible; `403` when it is visible but the flag is missing. Cross-tenant subnet URLs stay `404`.

### `GrantService` (`internal/service/grant.go`)

- UUID generation; validate subject exists and is not a local `admin`
- Validate resource (network/subnet in URL tenancy, or platform `create_networks`)
- `owner` implies CRUD; platform grants reject `owner`
- Uniqueness `(principal, resource)` → `409`
- Replace-all flags on update
- Refuse last-owner removal unless the caller is `admin`
- `EnsureOwner` after network create
- `Transfer(ctx, caller, resource, from, to)` — single write transaction: upsert destination `owner = true`, then drop `owner` on the source (delete the source row if no flags remain). `from` defaults to the caller. Reject disabled `to`. Non-admin cannot finish with zero owner rows.

`GrantRepository`: `Create`, `Get`, `Update`, `Delete`, `ListByResource`, `ListByPrincipal`, `ListEffectiveForPrincipal` (self + groups + pool, **excluding disabled subjects**), `ListPlatform`, `ListByNetwork`, `CountOwners`.

### Grant routes

#### `GET /auth/me/grants`

Authenticated. Returns grants that apply to the caller (direct + OIDC groups + machine pool). Inert grants (disabled subjects) are omitted. Deployment-wide API key → `data: []`.

#### Platform grants (admin)

| Method | Route |
|--------|--------|
| GET/POST | `/auth/platform-grants` |
| GET/PUT/DELETE | `/auth/platform-grants/{grant_uuid}` |

`POST` body:

```json
{
  "principal": {
    "kind": "oidc_group",
    "identity_pool_uuid": "22222222-2222-2222-2222-222222222222",
    "group": "ipam-operators"
  },
  "capability": "create_networks"
}
```

Other principal kinds: `local_user` + `local_user_uuid`; `oidc` + pool + `subject`; `machine` + `machine_uuid`; `machine_pool` + `machine_pool_uuid`.

#### Network and subnet grants (admin or owner)

| Method | Route |
|--------|--------|
| GET/POST | `/networks/{uuid}/grants` |
| GET/PUT/DELETE | `/networks/{uuid}/grants/{grant_uuid}` |
| GET/POST | `/networks/{uuid}/subnets/{subnet_uuid}/grants` |
| GET/PUT/DELETE | `/networks/{uuid}/subnets/{subnet_uuid}/grants/{grant_uuid}` |

`POST` body example:

```json
{
  "principal": {
    "kind": "machine",
    "machine_uuid": "44444444-4444-4444-4444-444444444444"
  },
  "permissions": {
    "create": true,
    "read": true,
    "update": false,
    "delete": false
  },
  "owner": false
}
```

`PUT` body: `{ "permissions": { … }, "owner": false }`. Principal and resource are immutable. Setting `owner: true` **shares** ownership (existing owners stay).

Errors: `400` invalid subject/flags; `403` not admin and not owner; `404` resource or grant not visible/not found; `409` duplicate grant or last-owner conflict.

#### Transfer ownership

`POST /networks/{uuid}/ownership/transfer`

`POST /networks/{uuid}/subnets/{subnet_uuid}/ownership/transfer`

Request body:

```json
{
  "from": {
    "kind": "local_user",
    "local_user_uuid": "11111111-1111-1111-1111-111111111111"
  },
  "to": {
    "kind": "oidc_group",
    "identity_pool_uuid": "22222222-2222-2222-2222-222222222222",
    "group": "netops"
  }
}
```

`from` is optional: omit it to transfer the caller’s own owner grant. A local `admin` may transfer any owner grant. `200 OK` returns the destination grant in `data`.

Errors: `400` missing `to`, invalid principals; `403` not admin and not owner; `404` resource or source owner grant not found; `409` destination disabled, or transfer would leave zero owners (non-admin).

### Tests

Table-driven `AuthorizationService` tests: admin implicit owner (bypass) on networks and subnets; deployment-key IPAM bypass without grant routes; default deny for users/OIDC/machines; `create_networks` via user, OIDC group, and machine pool; auto-owner on network create; share ownership (`owner = true`); atomic transfer (source loses owner, destination gains it); last-owner protection on grant delete vs allowed transfer; disabled/deleted subjects make grants inert; group membership loss drops group grants; network owner inherits subnet CRUD and grant management; subnet owner cannot manage the parent network; structural read-up; sibling isolation.

Handler tests: owner can POST grants and transfer; non-owner `403`; admin can transfer after all owners revoked; platform grants admin-only; list filtering.

### Breaking change

Until this RBAC is implemented, authenticated principals still have full IPAM access. Shipping it **revokes** implicit full access for local `user`, OIDC, and **machine identities**. Operators must assign `create_networks` and/or resource grants (or keep using `admin` / the deployment-wide API key). No automatic backfill.

## Configuration

| Variable           | Default             | Description |
|--------------------|---------------------|-------------|
| `KILHOG_HOST`      | `0.0.0.0`           | HTTP listen address (native server only) |
| `KILHOG_PORT`      | `8080`              | HTTP listen port (native server only) |
| `KILHOG_LOG_LEVEL` | `info`              | Log level: `debug`, `info`, `warn`, `error`, or `off` |
| `KILHOG_API_KEY`   | *(empty)*           | Deployment-wide API key (get-started IPAM bypass); one of the auth methods |
| `KILHOG_BOOTSTRAP_TOKEN` | *(empty)*     | Optional secret required for `POST /auth/bootstrap` |
| `KILHOG_PUBLIC_URL` | *(empty)*          | Public base URL for OIDC redirect URIs (no trailing slash) |
| `KILHOG_SESSION_TTL` | `24h`             | Session lifetime (Go duration or integer seconds) |
| `KILHOG_DB_DRIVER` | `sqlite`            | Database driver: `sqlite`, `postgres`, or `d1` |
| `KILHOG_DB_DSN`    | see below           | Connection DSN or D1 binding name |
| `KILHOG_AUTO_MIGRATE` | `true`           | Apply upgrade migrations at startup (or first Worker request) |
| `KILHOG_METRICS_REFRESH_INTERVAL` | `30s` | How often IPAM gauges are reconciled from `COUNT(*)`. `0` or `off` disables the loop (single-instance). Scrapes never query SQL. |

Default `KILHOG_DB_DSN`:

| Driver | Default DSN |
|--------|-------------|
| `sqlite` | `file:kilhog.db?_pragma=foreign_keys(ON)` |
| `postgres` | *(none — must be set)* |
| `d1` | `DB` (Wrangler D1 binding name) |

### Logging

Logging uses the standard library `log/slog` with a text handler on stderr. Configure verbosity with `KILHOG_LOG_LEVEL`.

| Level   | HTTP requests | Other events |
|---------|---------------|--------------|
| `debug` | Method, path, status, duration, headers (`Authorization` / `X-API-Key` / `Cookie` redacted), request body, response body | Migration details, startup/shutdown |
| `info`  | Method, path, status, duration (one line per request) | Startup, migrations applied, SIGTERM shutdown, SQLite sync, database closed |
| `warn`  | — | Warnings (e.g. database close failure) |
| `error` | — | Fatal configuration or runtime errors |
| `off`   | — | No logs |

Every HTTP request passes through `internal/log.HTTPMiddleware` (wired in `handler.NewRouter`). `Authorization` and `X-API-Key` header values are never logged.

### DSN examples

| Driver   | Example DSN |
|----------|-------------|
| SQLite   | `file:./data/kilhog.db?_pragma=foreign_keys(ON)` |
| PostgreSQL | `postgres://user:pass@localhost:5432/kilhog?sslmode=disable` |
| D1       | `DB` (must match the `binding` in `workers/wrangler.jsonc`) |

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
make build        # API server binary in bin/kilhog
make build-pogig  # CLI binary in bin/pogig
make build-all    # both native binaries
make build-wasm   # Cloudflare Workers WASM bundle in workers/build/
make docker-build # container image (kilhog:local)
make vet          # go vet ./...
make test         # go test ./...
make ci           # vet + test + build-all (local equivalent of CI)
```

Compiles the API server from `./cmd/kilhog`, the CLI from `./cmd/pogig`, and the Worker entry from `./cmd/kilhog-worker` (`GOOS=js GOARCH=wasm`).

## Cloudflare Workers (WASM)

kilhog can run on [Cloudflare Workers](https://workers.cloudflare.com/) by compiling the API to WebAssembly. Persistence uses **Cloudflare D1** (SQLite-compatible).

### Layout

```
workers/
├── package.json       # npm scripts: build, dev, deploy
├── wrangler.jsonc     # Worker name, D1 binding, env vars
├── schema.sql         # Optional manual D1 bootstrap (prefer auto-migrate)
└── build/             # Generated: app.wasm, worker.mjs, runtime assets (gitignored)
cmd/kilhog-worker/     # Go entry point using github.com/syumai/workers
```

### Prerequisites

- Go 1.26+
- Node.js + npm
- Cloudflare account and Wrangler auth (`npx wrangler login`)

### One-time D1 setup

```bash
cd workers
npm install
npx wrangler d1 create kilhog
```

Copy the printed `database_id` into `workers/wrangler.jsonc` (`d1_databases[0].database_id`). Keep the binding name `DB` (or change both Wrangler and `KILHOG_DB_DSN`).

### Build, local preview, deploy

```bash
make worker-install   # npm install in workers/
make build-wasm       # generate workers/build/app.wasm + loader
make worker-dev       # wrangler dev (http://127.0.0.1:8787 by default)
make worker-deploy    # wrangler deploy
```

Or from `workers/`:

```bash
npm run build
npm start             # local Wrangler preview
npm run deploy
```

### Worker configuration

| Source | Keys |
|--------|------|
| `wrangler.jsonc` `vars` | `KILHOG_DB_DRIVER=d1`, `KILHOG_DB_DSN=DB`, `KILHOG_AUTO_MIGRATE`, `KILHOG_LOG_LEVEL` |
| Wrangler secret (recommended) | `KILHOG_API_KEY` via `npx wrangler secret put KILHOG_API_KEY` |
| D1 binding | `DB` → database `kilhog` |

On the first request, the Worker opens D1, runs pending SQLite migrations when `KILHOG_AUTO_MIGRATE=true`, then serves the same REST routes as the native server.

### Size limits

Standard Go WASM binaries can be large. Cloudflare Workers limits are approximately **3 MB** (free) / **10 MB** (paid) for the compressed Worker. If the bundle exceeds the limit, reduce dependencies or evaluate TinyGo (not wired by default).

## CI/CD (GitHub Actions)

Workflows live under `.github/workflows/`.

| Workflow | File | Trigger | Purpose |
|----------|------|---------|---------|
| **CI** | `ci.yml` | Push and pull requests to `main` | `go vet`, `go test ./...`, `make build-all`, Docker image build (not published), then cross-compile smoke builds for linux/darwin/windows (amd64/arm64 where applicable) |
| **Release** | `release.yml` | Push of tags matching `v*` | Re-run vet/tests, then in parallel: (1) build release archives (`kilhog` + `pogig`) per OS/arch and publish a GitHub Release with `.tar.gz` / `.zip` assets and `checksums.txt`; (2) build a multi-arch (`linux/amd64`, `linux/arm64`) Docker image and push it to Docker Hub |

Go version is taken from `go.mod` via `actions/setup-go` (`go-version-file`). Builds use `CGO_ENABLED=0` for portable static binaries.

To publish a release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

### Docker Hub publish (release tags)

The **Release** workflow job `Publish Docker Hub image` logs in to Docker Hub, builds the image from the root `Dockerfile`, and pushes it. Image tags for a Git tag `v1.2.3` (stable semver, not a pre-release):

| Docker tag | Example |
|------------|---------|
| `{{version}}` | `1.2.3` |
| `{{major}}.{{minor}}` | `1.2` |
| `{{major}}` | `1` |
| `latest` | added automatically for non-prerelease semver tags |

Pre-release tags such as `v1.2.3-rc.1` are published without moving `latest`.

Default image name: `{DOCKERHUB_USERNAME}/kilhog` (for example `alice/kilhog`). Override with the repository variable `DOCKERHUB_IMAGE` when the Hub namespace differs from the login username (for example `kilhog/kilhog`).

#### GitHub Actions secrets and variables

Configure these on the GitHub repository: **Settings → Secrets and variables → Actions**.

**Repository secrets** (Secrets tab → New repository secret):

| Secret | Required | Value |
|--------|----------|-------|
| `DOCKERHUB_USERNAME` | yes | Docker Hub username used to log in |
| `DOCKERHUB_TOKEN` | yes | Docker Hub [access token](https://hub.docker.com/settings/security) with Read & Write permission (Account Settings → Personal access tokens). Do not use the account password. |

**Repository variable** (Variables tab → New repository variable), optional:

| Variable | Required | Value |
|----------|----------|-------|
| `DOCKERHUB_IMAGE` | no | Full image name without a tag, e.g. `kilhog/kilhog`. When unset, the workflow uses `{DOCKERHUB_USERNAME}/kilhog`. |

Create the target repository on Docker Hub (or allow the token to create it) before the first tagged release. The CI workflow still only *builds* the image to catch Dockerfile regressions; it never logs in or pushes.

## Docker image

The root `Dockerfile` produces a **minimal scratch image** for the API server via a **multi-stage build**:

| Stage | Base | Role |
|-------|------|------|
| `builder` | `golang:1.26-alpine` (`--platform=$BUILDPLATFORM`) | Download modules, cross-compile a static binary (`CGO_ENABLED=0`, `TARGETOS` / `TARGETARCH`), install CA certificates |
| final | `scratch` | Copy only `/kilhog`, CA certs, and an empty `/data` directory |

Design notes:

- **Static binary**: `modernc.org/sqlite` is pure Go, so the binary needs no libc and runs on `scratch`.
- **TLS**: CA certificates are copied so PostgreSQL (and other) TLS connections work from the container.
- **Non-root**: the process runs as UID/GID `65532`; SQLite defaults to `file:/data/kilhog.db?_pragma=foreign_keys(ON)`.
- **Binary only**: the image contains the API server (`cmd/kilhog`), not the pogig CLI.
- **Multi-arch**: the builder stage uses BuildKit `TARGETOS` / `TARGETARCH` so release images can be `linux/amd64` and `linux/arm64`. Local `make docker-build` still produces a single-arch `kilhog:local` image.

Build and run:

```bash
make docker-build
# or: docker build -t kilhog:local .

docker run --rm -p 8080:8080 \
  -e KILHOG_API_KEY=dev-secret \
  -v kilhog-data:/data \
  kilhog:local
```

Override database settings with the usual env vars (`KILHOG_DB_DRIVER`, `KILHOG_DB_DSN`, …). Mount `/data` (or point `KILHOG_DB_DSN` at another writable path) when using SQLite so the database survives container restarts.

The binary is PID 1 in the scratch image, so it receives Cloud Run's **SIGTERM** directly. On that signal it drains HTTP (8s), checkpoints the SQLite WAL into `/data/kilhog.db`, closes the file, and exits before the 10s SIGKILL. See [Graceful shutdown (Cloud Run / SIGTERM)](#graceful-shutdown-cloud-run--sigterm).
## Go SDK (`pkg/kilhog`)

The public Go SDK wraps the kilhog REST API. It is consumed by **pogig** and is designed for reuse by external Go projects, including the **Terraform provider** (maintained in a separate Git repository).

### Layout

```
pkg/kilhog/
├── client.go   # HTTP client, request envelope handling, configuration
├── types.go    # API data types (Network, Subnet, Tag, …)
├── error.go    # APIError for non-success responses
├── health.go   # GET /healthz
├── network.go  # Network CRUD
├── subnet.go   # Subnet CRUD (network-scoped routes)
└── grant.go    # Grants (platform, network, subnet) and ListMyGrants
```

### Client configuration

| Field / env var | Default | Description |
|-----------------|---------|-------------|
| `ClientConfig.BaseURL` / `KILHOG_BASE_URL` | `http://localhost:8080` | API base URL |
| `ClientConfig.APIKey` / `KILHOG_API_KEY` | *(empty)* | Bearer token for protected routes |
| `ClientConfig.HTTPClient` | 30 s timeout | Custom `*http.Client` (optional) |

Construct a client explicitly or from the environment:

```go
client, err := kilhog.NewClient(kilhog.ClientConfig{
    BaseURL: "http://localhost:8080",
    APIKey:  "secret",
})

// or

client, err := kilhog.NewClientFromEnv()
```

### SDK methods (mirror of REST routes)

| Method | HTTP route |
|--------|------------|
| `Health` | `GET /healthz` |
| `ListNetworks`, `GetNetwork`, `CreateNetwork`, `UpdateNetwork`, `DeleteNetwork` | `/networks` … |
| `ListSubnets`, `GetSubnet`, `CreateSubnetInNetwork`, `UpdateSubnet`, `DeleteSubnet` | `/networks/{uuid}/subnets` … |
| `CreateSubnetUnderParent`, `ListChildSubnets` | `/networks/{uuid}/subnets/{subnet_uuid}/subnets` … |
| `ListMyGrants` | `GET /auth/me/grants` |
| `ListPlatformGrants`, `CreatePlatformGrant`, `UpdatePlatformGrant`, `DeletePlatformGrant` | `/auth/platform-grants` … |
| `ListNetworkGrants`, `CreateNetworkGrant`, `UpdateNetworkGrant`, `DeleteNetworkGrant` | `/networks/{uuid}/grants` … |
| `TransferNetworkOwnership`, `TransferSubnetOwnership` | `POST …/ownership/transfer` |
| `ListSubnetGrants`, `CreateSubnetGrant`, `UpdateSubnetGrant`, `DeleteSubnetGrant` | `/networks/{uuid}/subnets/{subnet_uuid}/grants` … |

Errors from the API are returned as `*kilhog.APIError` with the HTTP status code and server message. A **403** whose body is not the kilhog error envelope (HTML or empty, typical of Cloud Armor `deny-403`) includes a hint to enable JSON parsing on the security policy; see [Load balancer and Cloud Armor](#load-balancer-and-cloud-armor).

### Terraform provider (external project)

The Terraform provider is **not** part of this repository. It lives in a separate Git project and should depend on this module:

```go
import "github.com/kilhog-io/kilhog/pkg/kilhog"
```

Provider configuration maps to `kilhog.ClientConfig`:

| Terraform provider attribute | SDK field / env var |
|------------------------------|---------------------|
| `base_url` | `ClientConfig.BaseURL` / `KILHOG_BASE_URL` |
| `api_key` | `ClientConfig.APIKey` / `KILHOG_API_KEY` |

Resource implementations call the same SDK methods as pogig (create/read/update/delete networks and subnets).

## CLI — pogig (`cmd/pogig`)

**pogig** (*chick* in Breton) is the command-line client for kilhog. It uses the shared SDK (`pkg/kilhog`) instead of calling the REST API directly.

### Configuration

Same variables and flags as the SDK:

| Flag | Env var | Default |
|------|---------|---------|
| `--base-url` | `KILHOG_BASE_URL` | `http://localhost:8080` |
| `--api-key` | `KILHOG_API_KEY` | *(empty)* |

### Commands

| Command | Description |
|---------|-------------|
| `pogig health` | Check server health (`GET /healthz`) |
| `pogig network list` | List all networks |
| `pogig network get <uuid>` | Get a network |
| `pogig network create --name … [--description …]` | Create a network |
| `pogig network update <uuid> --name … [--description …]` | Update a network |
| `pogig network delete <uuid>` | Delete a network |
| `pogig subnet list --network <uuid>` | List subnets in a network |
| `pogig subnet get <subnet-uuid> --network <uuid>` | Get a subnet |
| `pogig subnet create --network <uuid> --name … --prefix … [--address …] [--parent-subnet …]` | Create a subnet |
| `pogig subnet update <subnet-uuid> --network <uuid> --description …` | Update a subnet description |
| `pogig subnet delete <subnet-uuid> --network <uuid>` | Delete a subnet |
| `pogig grant me` | List grants for the current principal |
| `pogig grant platform list\|create\|update\|delete` | Platform `create_networks` grants (admin) |
| `pogig grant list --network <uuid> [--subnet <uuid>]` | List grants on a network or subnet (admin or owner) |
| `pogig grant create --network <uuid> [--subnet <uuid>] --principal-kind … [--owner] --read/--create/--update/--delete` | Create a resource grant (use `--owner` to **share** ownership) |
| `pogig grant transfer --network <uuid> [--subnet <uuid>] --to-kind … [--from-kind …]` | Transfer ownership |
| `pogig grant update <grant-uuid> …` | Replace grant flags |
| `pogig grant delete <grant-uuid> --network <uuid> [--subnet <uuid>]` | Delete a grant |

Successful reads and writes print JSON to stdout.

### Examples

```bash
make build-pogig

# Server must be running (make run-dev)
./bin/pogig health
./bin/pogig network list
./bin/pogig network create --name lab --description "Lab network"
./bin/pogig subnet list --network 550e8400-e29b-41d4-a716-446655440000
```

With API key authentication:

```bash
KILHOG_API_KEY=dev-secret ./bin/pogig network list
```

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
- **API key**: `dev-secret` by default (`KILHOG_API_KEY` in the Makefile). Leaving it empty rejects functional routes with `403`.

The `kilhog.db` file (and SQLite auxiliary files `kilhog.db-wal`, `kilhog.db-shm`) is ignored by Git (see `.gitignore`).

### Makefile API key defaults

| Make variable | Default | Used by |
|---------------|---------|---------|
| `KILHOG_API_KEY` | `dev-secret` | `run-dev`, `dev-*` script targets |
| `KILHOG_BASE_URL` | `http://localhost:8080` | `dev-*` script targets |

Override on the command line, for example `make dev-create-networks KILHOG_API_KEY=other-secret`. When calling **pogig** directly, export the same key: `KILHOG_API_KEY=dev-secret ./bin/pogig network list`. If `KILHOG_API_KEY` is empty on the server, functional routes return `403`.

### Direct run

```bash
go run ./cmd/kilhog
```

Uses the same defaults as `run-dev` (`sqlite`, DSN `file:kilhog.db?_pragma=foreign_keys(ON)`).

### GCP Cloud Run (SQLite on Cloud Storage)

The `terraform/` root module deploys a **public** Cloud Run service that mounts a Cloud Storage bucket for the SQLite file (`KILHOG_DB_DRIVER=sqlite`, DSN `file:/mnt/sqlite/kilhog.db?_pragma=foreign_keys(ON)`).

This matches the lightweight SQLite deployment path in `FUNCTIONAL.md`. GCS FUSE is **not** POSIX-compliant (no file locking); the service is therefore capped at **one instance**. For multi-instance production, use PostgreSQL instead of this stack.

#### APIs (enabled by Terraform)

| API | Purpose |
|-----|---------|
| `run.googleapis.com` | Cloud Run |
| `storage.googleapis.com` | Bucket + GCS FUSE volume mount |
| `iam.googleapis.com` | Service accounts and IAM |
| `iamcredentials.googleapis.com` | Service-account credentials |
| `cloudresourcemanager.googleapis.com` | Project IAM from Terraform |
| `artifactregistry.googleapis.com` | Pull the container image |
| `secretmanager.googleapis.com` | `KILHOG_API_KEY` |
| `serviceusage.googleapis.com` | Enable the APIs |

#### Resources

| Resource | Role |
|----------|------|
| `google_service_account.runtime` (`{service}-run`) | Cloud Run / GCS FUSE runtime identity |
| `google_storage_bucket.sqlite` | Regional bucket for `kilhog.db` (versioned, public access prevented) |
| `google_secret_manager_secret.api_key` | Optional API key (created only when `api_key` is set) |
| `google_cloud_run_v2_service.kilhog` | Public gen2 service, bucket mounted at `/mnt/sqlite` |

#### IAM

| Principal | Role | Resource |
|-----------|------|----------|
| Runtime SA | `roles/storage.objectUser` | SQLite bucket (read/write `kilhog.db`, `-wal`, `-shm`) |
| Runtime SA | `roles/secretmanager.secretAccessor` | API key secret (when configured) |
| Cloud Run service agent `service-{PROJECT_NUMBER}@serverless-robot.iam.gserviceaccount.com` | `roles/iam.serviceAccountUser` | Runtime SA (required to deploy a revision that uses it) |
| Cloud Run service agent | `roles/artifactregistry.reader` | Project (pull the container image) |
| `allUsers` | `roles/run.invoker` | Cloud Run service (unauthenticated HTTP) |

The bucket itself is **not** public (`public_access_prevention = enforced`). Application-level protection is `KILHOG_API_KEY`. A Domain Restricted Sharing organization policy can block `allUsers`; grant an exception on the project if `terraform apply` fails on the invoker binding.

#### Apply

```bash
cd terraform
cp terraform.tfvars.example terraform.tfvars   # set project_id, image, api_key
terraform init
terraform apply
```

`image` must already exist in a registry Cloud Run can pull (typically Artifact Registry in the same project). After apply, `terraform output service_uri` is the public HTTPS URL.

### Load balancer and Cloud Armor

When Cloud Run is published behind an HTTP(S) load balancer with a Cloud Armor security policy, preconfigured WAF rules can **false-positive on legitimate IPAM JSON**. The request never reaches Cloud Run (`deny-403`).

Reference policy: `terraform/cloud_armor.tf`.

#### False positive 1 — raw JSON vs CRS 942200

Without JSON parsing, Cloud Armor inspects the entire body as one string. A typical subnet create:

```json
{"name":"dmz","prefix":24,"address":"10.0.0.0","type":"ipv4"}
```

matches **942200** (`owasp-crs-v030301-id942200-sqli`, paranoia level 2) on the comma-separated object shape. Individual values (`10.0.0.0`, `24`, `ipv4`) do not match. Network create with `{"name":"lab"}` often still works because it has no comma.

**Fix:** set `jsonParsing=STANDARD` so each JSON field is inspected on its own.

```bash
gcloud compute security-policies update kilhog-armor \
  --region=REGION \
  --json-parsing=STANDARD \
  --log-level=VERBOSE
```

Use `--region` for a **regional** external HTTP(S) load balancer (`http_external_regional_lb_rule` in the deny log). Confirm `advancedOptionsConfig.jsonParsing` is `STANDARD`.

#### False positive 2 — hyphenated names vs CRS 942432

After JSON parsing is on, the WAF inspects `name` as `ARG_VALUES`. Default Cloud Armor sensitivity is **4** (all CRS paranoia levels). Rule **942432** (`owasp-crs-v030301-id942432-sqli`, paranoia level 4) is *Restricted SQL Character Anomaly Detection: 2 special characters*. Hyphens count. A name such as `prod-databases-east1` matches `-databases-` and is denied.

**Fix:** use `evaluatePreconfiguredWaf` at **sensitivity 1** (high-confidence signatures only), and exclude IPAM JSON fields from SQLi inspection. Names, descriptions, and tags routinely contain hyphens and words like `database`; `address` / `prefix` are dotted numbers.

Replace a combined stanza such as:

```hcl
expression = "evaluatePreconfiguredExpr('sqli-v33-stable') || evaluatePreconfiguredExpr('xss-v33-stable')"
```

with the rules in `terraform/cloud_armor.tf` (all three must be in the same `rules` list):

- priority **2147483647**: default `allow` with `versioned_expr = "SRC_IPS_V1"` and `src_ip_ranges = ["*"]`. The Cloud Armor API rejects any update that lists `rules` without this rule (`Every security policy must have a default rule at priority 2147483647 with match condition *`).
- priority 1000: `evaluatePreconfiguredWaf('sqli-v33-stable', {'sensitivity': 1})` plus `preconfigured_waf_config` exclusions on `name`, `description`, `address`, `prefix`, `type`, `tags`, `permissions`, `principal`, `subject`, `group`, `capability`
- priority 1001: XSS at sensitivity 1, exclusions on `name`, `description`, `tags`, `subject`, `group`
- `advanced_options_config.json_parsing = "STANDARD"`

Do **not** change REST field names or switch to form encoding to dodge signatures. Headers, cookies, and the URL remain covered.

#### Client error

When a gateway returns 403 with a body that is not the kilhog `{status, message, code}` envelope, the Go SDK wraps it as `*APIError` and mentions Cloud Armor JSON parsing and IPAM field exclusions so the operator can distinguish a WAF deny from `authentication is not configured`.

### Development HTTP scripts

The `scripts/dev/` folder contains standalone Bash scripts that call the REST API on a locally running instance (`make run-dev`). Each script reads one or more UTF-8 JSON files located next to it; there is no shared library between scripts.

**Prerequisites**: `curl`. Scripts `update-network-hors-prod.sh` and `delete-network-prod.sh` also use `jq` to look up a network UUID by `name`.

| Variable | Default | Description |
|----------|---------|-------------|
| `KILHOG_BASE_URL` | `http://localhost:8080` | API base URL |
| `KILHOG_API_KEY` | `dev-secret` (via Makefile) | API key sent as `Authorization: Bearer …`; server must have the same non-empty key |

Make targets `run-dev` and `dev-*` inject these values automatically. Override with `make … KILHOG_API_KEY=other-secret`. An empty server key rejects functional routes with `403`.

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
