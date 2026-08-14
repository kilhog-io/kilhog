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

## Authentication

Access to **functional operations** (create, read, update, delete of networks and subnets) requires **authentication**. Unauthenticated callers must be rejected.

`GET /healthz` (or equivalent health probe) remains **public**: no credentials are required so load balancers and orchestrators can check availability.

kilhog supports three authentication methods: a shared **API key**, **local users** (username and password managed in kilhog), and federated identity via one or more **OIDC identity pools**.

### Authentication methods

| Method | Typical use | Credential |
|--------|-------------|------------|
| **API key** | Automation, CLI (`pogig`), SDK, scripts | Shared secret configured on the server |
| **Local user** | Bootstrap / primo-admin, operators without an external IdP | Username + password stored by kilhog |
| **OIDC** | Interactive users, SSO, federated identity | Tokens issued by a trusted OpenID Provider belonging to a configured identity pool |

#### Acceptance rule

A request to a protected operation is accepted if **at least one** enabled method successfully authenticates the caller.

| Available credentials on the server | Behavior on protected operations |
|-------------------------------------|----------------------------------|
| None (no API key, no local users, no enabled OIDC pools) | Rejected (`403` — authentication not configured) |
| One or more methods available | Accept if **any** presented credential is valid for an available method; otherwise `401` |

### API key (existing)

- A single shared secret may be configured for the deployment.
- Callers present the key with each request.
- Successful authentication grants access to **all** IPAM functional operations (no per-caller identity beyond “API key holder”).
- The API key does **not** grant rights to administer local users or identity pools (see [Roles](#roles)).
- When the key is configured, a missing or incorrect key is rejected as unauthenticated (`401`) unless another method succeeds.

### Entity: Local user

A **local user** is an account whose credentials are managed by kilhog (not by an external IdP).

#### Attributes

| Attribute     | Required | Description |
|---------------|----------|-------------|
| `uuid`        | yes      | Unique identifier. Unique across the database. |
| `username`    | yes      | Login name. Unique across the database (case-insensitive). |
| `password`    | yes      | Secret used for local login. Stored only as a one-way hash; never returned by the API. |
| `display_name`| no       | Human-readable name. |
| `email`       | no       | Contact email. |
| `role`        | yes      | Either `admin` or `user` (see [Roles](#roles)). |
| `enabled`     | yes      | When `false`, the account cannot authenticate. |
| `created_at`  | yes      | Creation timestamp. |
| `updated_at`  | yes      | Last modification timestamp. |

#### Local login

- A local user authenticates with `username` + `password`.
- Successful login establishes a **kilhog session** (or equivalent server-issued credential), same session model as interactive OIDC.
- Disabled users and invalid passwords are rejected (`401`) without revealing which field failed.
- Local users may change their own password when authenticated (subject to current-password confirmation).

#### Bootstrap: primo-admin

On a fresh deployment, an administrator must be able to take control **before** any OIDC pool exists.

Rules:

1. While **no local user** exists, kilhog allows a one-time **bootstrap** that creates the first local user.
2. That first user is always created with role **`admin`** (the **primo-admin**).
3. Bootstrap is **unavailable** once at least one local user exists.
4. Bootstrap must be protected against abuse (e.g. only when the user store is empty; optional deployment secret or local-only exposure may be defined in `TECHNICAL.md`).
5. The primo-admin (and any later `admin`) can create additional local users and configure OIDC identity pools.

Self-registration of local users by anonymous callers is **not** allowed outside this bootstrap path.

#### Local user management

| Operation | Who |
|-----------|-----|
| Bootstrap first admin | Anonymous, only when no local user exists |
| List / create / update / disable / delete local users | `admin` only |
| Change own password | The authenticated local user (own account) |
| Change another user’s password or role | `admin` only |

An administrator **must not** be able to delete or disable the **last remaining enabled `admin`** local user (so the system cannot lock itself out of local administration unless another admin already exists).

### Roles

This version introduces a minimal authorization model for **identity administration** only (not network-scoped RBAC).

| Role | IPAM functional operations (networks / subnets) | Manage local users | Manage OIDC identity pools |
|------|--------------------------------------------------|--------------------|----------------------------|
| `admin` | yes | yes | yes |
| `user` | yes | no | no |
| API key holder | yes | no | no |
| OIDC principal (no linked elevated role) | yes | no | no |

Network-scoped permissions remain out of scope; any authenticated principal may still operate on all networks until a future RBAC model is defined.

### Entity: OIDC identity pool

An **OIDC identity pool** is a named configuration that trusts a given OpenID Provider. kilhog supports **several** pools so an administrator can connect multiple IdPs (e.g. corporate SSO and a partner IdP).

#### Attributes

| Attribute        | Required | Description |
|------------------|----------|-------------|
| `uuid`           | yes      | Unique identifier. Unique across the database. |
| `name`           | yes      | Display name. Unique across the database. |
| `slug`           | yes      | Stable URL-safe identifier used in login routes. Unique across the database. |
| `issuer`         | yes      | OIDC issuer URL. |
| `client_id`      | yes      | OAuth / OIDC client identifier at the IdP. |
| `client_secret`  | conditional | Confidential client secret when required by the IdP. Stored securely; never returned in full by the API after creation. |
| `scopes`         | no       | Extra scopes beyond the OpenID baseline (`openid`, and typically `profile` / `email`). |
| `enabled`        | yes      | When `false`, the pool cannot be used for login or token acceptance. |
| `created_at`     | yes      | Creation timestamp. |
| `updated_at`     | yes      | Last modification timestamp. |

Uniqueness:

| Field | Scope |
|-------|--------|
| `uuid` | Database |
| `name` | Database |
| `slug` | Database |
| `issuer` | Database — at most one pool per issuer |

#### Pool management

- Only **`admin`** local users may create, update, enable/disable, or delete identity pools.
- Creating or updating a pool may use OpenID Connect discovery (`/.well-known/openid-configuration`) when available.
- Deleting or disabling a pool immediately stops new authentications through that pool; existing kilhog sessions already issued may remain valid until they expire or are revoked (policy detail in `TECHNICAL.md`).

### OIDC (OpenID Connect)

OIDC allows kilhog to trust external **OpenID Providers** (IdPs) such as Keycloak, Authentik, Okta, Azure AD / Entra ID, or Google, without storing those users’ passwords in kilhog.

#### Goals

- Authenticate human users via one or more standards-based IdPs configured as identity pools.
- Let a local **primo-admin** (then other admins) configure those pools after bootstrap.
- Keep IPAM access **authentication-gated but not network-scoped**: any successfully authenticated principal has full access to networks and subnets. **Fine-grained authorization (RBAC per network)** remains out of scope for this specification.

#### Flows

| Flow | Audience | Purpose |
|------|----------|---------|
| **Authorization Code + PKCE** | Interactive clients (browser / future UI) | User picks (or is directed to) an identity pool, signs in at that IdP; kilhog completes the callback and establishes a session |
| **Bearer access token** | API clients that already hold an IdP access token | Client sends `Authorization: Bearer <access_token>`; kilhog validates it against an **enabled** pool whose issuer matches the token |

Client Credentials (machine-to-machine via an IdP) may be accepted later if the access token is valid for kilhog as audience; until then, automation should use the **API key**.

#### Principal (authenticated identity)

After successful OIDC authentication, kilhog recognizes a **principal** with at least:

| Attribute | Required | Description |
|-----------|----------|-------------|
| `identity_pool` | yes | Reference to the pool that validated the identity |
| `issuer` | yes | OIDC issuer URL (`iss`) |
| `subject` | yes | Stable user identifier at the IdP (`sub`) |
| `email` | no | Email claim when provided by the IdP |
| `name` | no | Display name when provided by the IdP |

The pair `(issuer, subject)` — equivalently `(identity_pool, subject)` given one pool per issuer — uniquely identifies a federated principal.

Linking a federated principal to a local user account (account linking) is **optional** and not required for IPAM access in this version. Administration of users and pools remains reserved to local `admin` users unless a later rule grants equivalent rights to linked accounts.

#### Token and session rules

- Access tokens presented to the API must be validated (signature, issuer matching an enabled pool, expiry, and audience / client constraints as configured for that pool).
- Expired or otherwise invalid tokens are rejected (`401`).
- Interactive login (local or OIDC) may result in a **kilhog session** so the client does not resend IdP tokens on every call; session lifetime and logout behavior are deployment-configurable within reasonable bounds.
- **Logout** ends the kilhog session. For OIDC, optional redirection to the IdP for end-session (RP-initiated logout) may be supported when the provider exposes it; local session termination must always succeed even if the IdP logout step fails.

#### Auth capabilities (business)

When authentication features are available, kilhog exposes capabilities such as:

- Local login and bootstrap of the primo-admin
- List enabled identity pools (for login discovery)
- Start OIDC login for a given pool (redirect to the IdP)
- Handle the IdP callback
- End session (logout)
- Read the current principal
- Admin CRUD for local users and identity pools

Exact paths and payloads are defined in `TECHNICAL.md`.

#### Failure modes

| Situation | Expected outcome |
|-----------|------------------|
| No authentication method available | `403` — authentication not configured |
| Missing credentials on a protected route | `401` Unauthenticated |
| Invalid local username/password | `401` Unauthenticated |
| Invalid, expired, or wrong-audience token | `401` Unauthenticated |
| Token issuer does not match any enabled pool | `401` Unauthenticated |
| IdP unavailable during interactive login | Login fails with a clear error; other methods and existing sessions are unaffected |
| Callback with invalid state / PKCE verifier | Rejected — no session established |
| Non-admin attempts user or pool administration | `403` Forbidden |
| Bootstrap when a local user already exists | Rejected |

### Out of scope (this version)

The following are **explicitly not** part of this specification:

- Network-scoped or resource-scoped authorization (RBAC on networks / subnets)
- Mapping IdP groups/roles to kilhog permissions
- Anonymous self-registration of local users (beyond primo-admin bootstrap)
- Replacing the API key for machine automation (API key remains the supported M2M path)
- Granting pool/user administration rights to pure OIDC principals without a local `admin` account

### Relationship to tenancy

Authentication establishes **who** is calling. It does **not** by itself restrict which **network** (tenancy) a caller may access. In this version, any authenticated principal (API key, local user, or OIDC) may operate on all networks. Future RBAC may bind principals to networks; that is outside this specification.
