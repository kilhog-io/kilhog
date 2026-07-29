# kilhog

<p align="center">
  <img src="kilhog.png" alt="kilhog logo" width="200">
</p>

**kilhog** is an **IPAM** (*IP Address Management*) application: it manages IP pools and addresses through a REST API.

Its name comes from an English–French–Breton word chain: *pool* → *poule* → **kilhog** (*rooster* in Breton) — a rooster that manages the hens, the address pools.

## Features

- **Network** management (tenancy boundaries)
- Hierarchical IPv4 **subnet** management (CIDR blocks and host addresses)
- CIDR validation: parent containment, overlap detection, automatic address allocation
- **SQLite** or **PostgreSQL** persistence with versioned migrations
- JSON REST API with optional API key authentication
- **pogig** CLI and **Go SDK** (`pkg/kilhog`) for programmatic access and Terraform integration
- Ready for multi-tenancy and RBAC

## Documentation

| File | Content |
|------|---------|
| [FUNCTIONAL.md](FUNCTIONAL.md) | Business rules: entities, uniqueness constraints, subnet hierarchy |
| [TECHNICAL.md](TECHNICAL.md) | Architecture, stack, database schema, API routes and examples |

## Quick start

```bash
# Build and run the server (SQLite, port 8080)
make run-dev

# Build the CLI (pogig — Breton for "chick")
make build-pogig

# In another terminal: check health and create sample data
./bin/pogig health
make dev-create-networks
make dev-create-subnets
./bin/pogig network list
```

Check that the API responds:

```bash
curl http://localhost:8080/healthz
```

To enable API key protection, set `KILHOG_API_KEY` when starting the server and pass the same key on protected routes:

```bash
KILHOG_API_KEY=dev-secret make run-dev

curl -H "Authorization: Bearer dev-secret" http://localhost:8080/networks
```

`GET /healthz` stays public so load balancers and orchestrators can probe without credentials.

## Build

```bash
make build        # API server binary in bin/kilhog
make build-pogig  # CLI binary in bin/pogig
make build-all    # both binaries
go test ./...     # run tests
```

## CLI and SDK

| Component | Path | Role |
|-----------|------|------|
| **pogig** CLI | `cmd/pogig` | Command-line client for the REST API |
| **Go SDK** | `pkg/kilhog` | Shared HTTP client for pogig and external consumers (e.g. Terraform provider) |

Configure both with `KILHOG_BASE_URL` and `KILHOG_API_KEY` (or `--base-url` / `--api-key` flags on pogig).

The Terraform provider is maintained in a **separate Git repository** and should import `github.com/kilhog-io/kilhog/pkg/kilhog`. See [TECHNICAL.md](TECHNICAL.md) for SDK methods and pogig commands.

## Stack

- Go 1.26+
- REST API, layered architecture (`handler` → `service` → `repository`)
- SQLite and PostgreSQL drivers

See [TECHNICAL.md](TECHNICAL.md) for configuration details, routes, and the relational schema.
