# FUNCTIONAL — kilhog

## Overview

**kilhog** is an **IPAM** (*IP Address Management*) application: it manages IP pools and addresses.

Its name comes from an English–French–Breton word chain:

| Language | Word   |
|----------|--------|
| English  | pool   |
| French   | poule  |
| Breton   | kilhog |

In Breton, **kilhog** means *rooster*: a rooster that manages the hens (the pools).

## Unified model: the subnet

In kilhog, **every address space or IP address is modeled as a subnet**.

- A CIDR block (e.g. `192.168.1.0/24`) is a subnet.
- An individual IP address (e.g. `192.168.1.42`) is also a subnet, with prefix **`/32`** (IPv4) or **`/128`** (IPv6).

There is no separate `IP` entity: an IP is a **leaf subnet**.

## Entity: Network

A **network** provides **tenancy** (logical isolation boundary). It is the root container within which subnets are organized.

### Attributes

| Attribute     | Required | Description |
|---------------|----------|-------------|
| `uuid`        | yes      | Unique identifier. Link key with the rest of the system. Unique across the database. |
| `name`        | yes      | Display name. Unique across the database. |
| `description` | no       | Free-form descriptive text. |
| `tags`        | no       | List of key–value pairs (`key`, `value`). |

## Entity: Subnet

A **subnet** represents an IP address space (block or single address).

### Attributes

| Attribute     | Required | Description |
|---------------|----------|-------------|
| `uuid`        | yes      | Unique identifier. Link key with the rest of the system. Unique across the database. |
| `name`        | yes      | Display name. Unique within the network (tenancy) it belongs to. |
| `description` | no       | Free-form descriptive text. |
| `prefix`      | yes      | Prefix length (e.g. `24` for a `/24`). |
| `address`     | conditional | Network or host address (e.g. `192.168.1.0`, `192.168.1.42`). **Required** when the parent is a network. Optional when the parent is a subnet (auto-generated if absent). |
| `type`        | yes      | Address family: `ipv4` or `ipv6`. |
| `parent`      | yes      | Reference to the subnet parent (see below). |
| `tags`        | no       | List of key–value pairs (`key`, `value`). |

### Parent

The `parent` field indicates the subnet's position in the hierarchy. It may reference:

1. **A network** — the subnet is a direct child of the tenancy boundary.
2. **Another subnet** — the subnet is nested within a larger address space.

A subnet always belongs, directly or indirectly, to a single root network.

### Creation

When creating a subnet:

- If the parent is a **network**, the `address` field is **required**.
- If the parent is a **subnet**, `address` is optional: if absent, an address is automatically generated within the parent CIDR, with no overlap among siblings.

### `CIDR` method

Each subnet exposes a **`CIDR`** method that returns CIDR notation by concatenating address and prefix:

```
{address}/{prefix}
```

Examples:

- `address = 192.168.1.0`, `prefix = 24` → `192.168.1.0/24`
- `address = 192.168.1.42`, `prefix = 32` → `192.168.1.42/32`
- `address = 2001:db8::1`, `prefix = 128` → `2001:db8::1/128`

## Tags

Tags are free-form metadata as **key–value** pairs attached to a network or subnet.

- A key may appear only once per resource.
- The value is a string.

## Uniqueness rules

| Resource | Field  | Uniqueness scope |
|----------|--------|------------------|
| Network  | `uuid` | Database         |
| Network  | `name` | Database         |
| Subnet   | `uuid` | Database         |
| Subnet   | `name` | Network (tenancy)|

## Persistence

Business data (networks, subnets, tags) is persisted in a **relational database**. The persistence layer is **abstracted**: the application supports multiple engines without changing the business model or the rules above.

### Supported engines

| Engine     | Typical use                     |
|------------|---------------------------------|
| SQLite     | Local development, lightweight deployment |
| PostgreSQL | Production, multi-instance      |

The engine choice is a **deployment configuration**, not a business rule. Uniqueness and hierarchy constraints apply the same way regardless of backend.

### Automatic database creation

If the target database **does not exist yet**, the application may **create it at startup** before running migrations:

- **SQLite**: creates the file and any missing parent directories.
- **PostgreSQL**: creates the database via a connection to the `postgres` catalog (or equivalent).

If the database already exists, the application connects without recreating it.

### Versioned SQL migrations

The database schema is managed by **numbered SQL migrations**. Each version has two scripts:

- **upgrade** — applies changes to the next version;
- **downgrade** — reverts those changes to the previous version.

Rules:

- Migrations run **in ascending order** of version numbers.
- An already applied version is **never replayed**.
- At startup, the application automatically applies missing **upgrade** migrations.
- **Downgrade** is available to roll back (explicit operation, not automatic at startup).

### Persisted data integrity

The following business rules are **enforced by the schema** (SQL constraints):

| Rule | Mechanism |
|------|-----------|
| Global uniqueness of a network's `uuid` and `name` | `UNIQUE` constraint |
| Global uniqueness of a subnet's `uuid` | Primary key |
| Uniqueness of a subnet's `name` within a network | `UNIQUE (network, name)` constraint |
| A subnet always belongs to a network (tenancy) | `network_uuid` foreign key |
| A tag has a single value per key and per resource | Composite primary key `(resource, key)` |
| Network deletion | Cascades to associated subnets and tags |

Table and column details are described in `TECHNICAL.md`.
