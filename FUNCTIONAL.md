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

kilhog supports four authentication methods: a shared **API key**, **local users** (username and password managed in kilhog), federated **human** identity via one or more **OIDC identity pools**, and **machine identities** (named workloads that authenticate with a per-machine API key or a JWT validated through workload identity federation).

### Authentication methods

| Method | Typical use | Credential |
|--------|-------------|------------|
| **API key** (deployment-wide) | Get started, simple automation, CLI (`pogig`), SDK, scripts | Shared secret configured on the server (`KILHOG_API_KEY`) |
| **Local user** | Bootstrap / primo-admin, operators without an external IdP | Username + password stored by kilhog |
| **OIDC** | Interactive users, SSO, federated human identity | Tokens issued by a trusted OpenID Provider belonging to a configured identity pool |
| **Machine identity** | CI, Terraform, named automation | Per-machine API key **or** JWT (workload identity federation with JWKS) |

The deployment-wide API key and machine identities are **distinct**. The shared key is kept as a first-class get-started path (one secret, no extra entities). Machine identities exist when the caller must be a **named** principal (audit, rotation, several CI jobs, JWT without a kilhog secret).

#### Acceptance rule

A request to a protected operation is accepted if **at least one** enabled method successfully authenticates the caller.

| Available credentials on the server | Behavior on protected operations |
|-------------------------------------|----------------------------------|
| None (no API key, no local users, no enabled OIDC pools, no enabled machine pools) | Rejected (`403` — authentication not configured) |
| One or more methods available | Accept if **any** presented credential is valid for an available method; otherwise `401` |

### API key (deployment-wide)

The shared API key remains a **first-class** authentication method. It is the get-started path: one deployment secret, no machine pool to create, so CLI, scripts, and a first CI job stay fluid.

- A single shared secret may be configured for the deployment.
- Callers present the key with each request.
- Successful authentication grants access to **all** IPAM functional operations (no per-caller identity beyond “API key holder”).
- The API key does **not** grant rights to administer local users, OIDC identity pools, or machine identities (see [Roles](#roles)).
- When the key is configured, a missing or incorrect key is rejected as unauthenticated (`401`) unless another method succeeds.
- This key is **not** a machine API key: it is not bound to a machine pool, cannot be rotated per workload, and is not replaced by machine identities in this version.

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
5. The primo-admin (and any later `admin`) can create additional local users, configure OIDC identity pools, and configure machine identities (pools, JWT providers, machines, and per-machine API keys).

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

| Role | IPAM functional operations (networks / subnets) | Manage local users | Manage OIDC identity pools | Manage machine identities |
|------|--------------------------------------------------|--------------------|----------------------------|---------------------------|
| `admin` | yes | yes | yes | yes |
| `user` | yes | no | no | no |
| API key holder (deployment-wide) | yes | no | no | no |
| OIDC principal (no linked elevated role) | yes | no | no | no |
| Machine identity | yes | no | no | no |

Network-scoped permissions remain out of scope; any authenticated principal may still operate on all networks until a future RBAC model is defined. Granting machine-identity administration to non-`admin` principals (including machines themselves) is deferred to that future RBAC model. In this version, only a local **`admin`** may manage machine identities.

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

Client Credentials (machine-to-machine via a **human** OIDC identity pool) are **out of scope**. Named automation uses **machine identities** (per-machine API keys and/or JWT workload identity federation). The deployment-wide API key remains available for get-started automation.

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

Linking a federated principal to a local user account (account linking) is **optional** and not required for IPAM access in this version. Administration of users, OIDC identity pools, and machine identities remains reserved to local `admin` users unless a later rule grants equivalent rights to linked accounts.

#### Token and session rules

- Access tokens presented to the API must be validated (signature, issuer matching an enabled pool, expiry, and audience / client constraints as configured for that pool).
- Expired or otherwise invalid tokens are rejected (`401`).
- Interactive login (local or OIDC) may result in a **kilhog session** so the client does not resend IdP tokens on every call; session lifetime and logout behavior are deployment-configurable within reasonable bounds.
- **Logout** ends the kilhog session. For OIDC, optional redirection to the IdP for end-session (RP-initiated logout) may be supported when the provider exposes it; local session termination must always succeed even if the IdP logout step fails.

### Entity: Machine pool

A **machine pool** is a named group of **machine identities**. It is the operational container in which an administrator defines machines and, optionally, JWT trust (workload identity federation).

Machine pools are **not** OIDC identity pools. An OIDC identity pool registers kilhog as an OAuth/OIDC **client** for interactive human login. A machine pool treats kilhog as a **resource**: it trusts tokens issued by an external workload issuer (GitHub Actions, GitLab CI, a cloud STS, a Kubernetes cluster, …) or issues its own per-machine API keys. There is no authorization-code redirect and no client secret toward that issuer.

A pool may exist **without** any JWT provider: machines then authenticate only with per-machine API keys.

#### Attributes

| Attribute     | Required | Description |
|---------------|----------|-------------|
| `uuid`        | yes      | Unique identifier. Unique across the database. |
| `name`        | yes      | Display name. Unique across the database. |
| `slug`        | yes      | Stable URL-safe identifier. Unique across the database. |
| `description` | no       | Free-form descriptive text. |
| `enabled`     | yes      | When `false`, no machine in the pool can authenticate (neither API key nor JWT). |
| `created_at`  | yes      | Creation timestamp. |
| `updated_at`  | yes      | Last modification timestamp. |

Uniqueness:

| Field | Scope |
|-------|--------|
| `uuid` | Database |
| `name` | Database |
| `slug` | Database |

#### Pool management

- Only **`admin`** local users may create, update, enable/disable, or delete machine pools, their providers, their machines, and their API keys.
- Deleting a pool deletes its providers, machines, and machine API keys.
- Disabling a pool immediately stops authentication for all of its machines. Already-issued JWTs remain valid only until their own expiry (CI tokens are typically short-lived). There is no kilhog session for machines.

### Entity: Machine identity provider

A **machine identity provider** is a JWT trust configuration **owned by a machine pool** (a child resource). A pool may have **zero or more** providers so one operational group can trust several issuers (for example GitHub Actions and a cloud workload identity).

The provider is the place where **JWKS** is added. Machines do not carry their own JWKS.

#### Attributes

| Attribute     | Required | Description |
|---------------|----------|-------------|
| `uuid`        | yes      | Unique identifier. Unique across the database. |
| `machine_pool`| yes      | Parent machine pool. |
| `name`        | yes      | Display name. Unique within the parent pool. |
| `issuer`      | yes      | Expected JWT `iss`. Unique within the parent pool. The same issuer **may** appear in other pools (for example several pools trusting GitHub). |
| `audiences`   | yes      | Non-empty list of accepted JWT `aud` values. A token is accepted only if it is intended for kilhog as resource. |
| `jwks_mode`   | yes      | How signing keys are supplied: `discovery`, `uri`, or `static` (see [JWKS modes](#jwks-modes)). |
| `jwks_uri`    | conditional | JWKS HTTPS URL. Required when `jwks_mode` is `uri`. Unused when `jwks_mode` is `discovery` (the URI is discovered) or `static`. |
| `jwks`        | conditional | JWKS document (RFC 7517). Required when `jwks_mode` is `static`. |
| `enabled`     | yes      | When `false`, JWTs through this provider are rejected. Per-machine API keys are unaffected. |
| `created_at`  | yes      | Creation timestamp. |
| `updated_at`  | yes      | Last modification timestamp. |

Uniqueness:

| Field | Scope |
|-------|--------|
| `uuid` | Database |
| `name` | Parent machine pool |
| `issuer` | Parent machine pool — at most one provider per issuer **in that pool** |

`audiences` must contain at least one value. A typical audience is the public URL of the kilhog API. A token whose `aud` matches none of the configured audiences is rejected (`401`).

`issuer` and `jwks_uri`, when used, must be **HTTPS** URLs.

#### JWKS modes

When creating or updating a provider, the administrator chooses **exactly one** of the following modes. All three are supported.

| Mode | How the administrator adds keys | Rotation |
|------|--------------------------------|----------|
| `discovery` | Supplies `issuer`. kilhog fetches `{issuer}/.well-known/openid-configuration`, then the discovered `jwks_uri`. | Automatic: kilhog refreshes keys from the issuer (including when a token presents an unknown `kid`). |
| `uri` | Supplies `jwks_uri` (and `issuer` for claim matching). No OpenID discovery document is required. | Automatic: kilhog refreshes keys from that URL (including when a `kid` is unknown). |
| `static` | Supplies the JWKS document (`keys` array) in the configuration. | **Manual**: the administrator must update the document when the issuer rotates keys. |

Rules:

- `discovery` is the preferred mode when the issuer is a standard OpenID Provider (GitHub Actions, GitLab, Google, Entra ID, Keycloak, …).
- `uri` is for issuers that publish JWKS but not OIDC discovery.
- `static` is for air-gapped deployments or when kilhog cannot reach the issuer’s JWKS URL.
- Signing keys are not application secrets; `static` JWKS may be returned to an `admin` on read. Fetched keys need not be echoed on every API read.
- Several keys in one JWKS are valid; verification uses the token’s `kid`.
- If keys cannot be obtained or the signature cannot be verified, the JWT is rejected (`401`). Other authentication methods are unaffected.

### Entity: Machine

A **machine** is a named workload identity inside a machine pool (a CI job, a Terraform workspace, a controller). It is the principal that appears in `/auth/me` and in logs.

#### Attributes

| Attribute         | Required | Description |
|-------------------|----------|-------------|
| `uuid`            | yes      | Unique identifier. Unique across the database. |
| `machine_pool`    | yes      | Parent machine pool. |
| `name`            | yes      | Display name. Unique within the parent pool. |
| `description`     | no       | Free-form descriptive text. |
| `enabled`         | yes      | When `false`, the machine cannot authenticate. |
| `provider`        | no       | Machine identity provider used for JWT authentication. Absent means **API keys only**. |
| `subject`         | no       | Exact JWT `sub` that maps to this machine. |
| `subject_prefix`  | no       | JWT `sub` must start with this string (for example `repo:org/app:`). |
| `claims`          | no       | Additional JWT claim conditions (see [JWT matching](#jwt-matching)). |
| `created_at`      | yes      | Creation timestamp. |
| `updated_at`      | yes      | Last modification timestamp. |

Uniqueness:

| Field | Scope |
|-------|--------|
| `uuid` | Database |
| `name` | Parent machine pool |

A machine may use **API keys**, **JWT**, or **both**.

- API-key-only: `provider` is absent; `subject`, `subject_prefix`, and `claims` are ignored for authentication.
- JWT: `provider` is set. The machine **must** have at least one matching constraint among `subject`, `subject_prefix`, and `claims`, so that any valid token from the issuer cannot impersonate it.
- Both: the machine has a provider **and** one or more machine API keys.

When `provider` is set, it **must** belong to the same machine pool as the machine.

Disabling the machine rejects both credentials. Deleting the machine deletes its API keys.

### Entity: Machine API key

A **machine API key** is a secret bound to **one machine**. It is not the deployment-wide API key.

#### Attributes

| Attribute     | Required | Description |
|---------------|----------|-------------|
| `uuid`        | yes      | Unique identifier. Unique across the database. |
| `machine`     | yes      | Parent machine. |
| `name`        | no       | Human label (for example `ci-2026-09`). |
| `prefix`      | yes      | Public lookup prefix of the secret (not sufficient to authenticate). |
| `secret`      | yes      | High-entropy secret. Stored only as a one-way hash; the plaintext is returned **only at creation**. |
| `expires_at`  | no       | When set, the key is rejected after this instant. |
| `revoked_at`  | no       | When set, the key is rejected. |
| `last_used_at`| no       | Last successful authentication, when known. |
| `created_at`  | yes      | Creation timestamp. |

Rules:

- A machine may have **several** API keys at once so a key can be rotated without downtime.
- After creation, the API never returns the plaintext secret again (only metadata: `uuid`, `name`, `prefix`, expiry, revocation, timestamps).
- A key authenticates only if the parent machine and parent pool are enabled, the key is not revoked, and it is not expired.
- Successful authentication grants the same rights as the machine (IPAM operations only; see [Roles](#roles)).
- Administrators (`admin`) may list, create, and revoke keys; they cannot recover a lost plaintext secret (they issue a new key instead).

### Machine authentication

Machine callers do **not** obtain a kilhog session. Each request presents a credential.

| Credential | How presented |
|------------|----------------|
| Machine API key | Per request, same transport as the deployment-wide key (`Authorization: Bearer …` or `X-API-Key`) |
| JWT | `Authorization: Bearer <jwt>` only |

#### JWT validation

For a bearer token that is a JWT, kilhog:

1. Reads `iss` (without trusting the token yet).
2. Selects **enabled** machine identity providers whose `issuer` matches, in **enabled** pools.
3. Verifies the signature with that provider’s JWKS (current keys for `discovery` / `uri`, or the configured document for `static`).
4. Rejects expired tokens, tokens that are not yet valid, and tokens whose `aud` matches none of the provider’s `audiences`.
5. Matches the verified claims against **enabled** machines bound to that provider ([JWT matching](#jwt-matching)).

If no machine identity provider matches, kilhog may still validate the JWT against an **enabled human OIDC identity pool** (existing bearer flow). Machine federation and human OIDC are separate trust stores.

Human OIDC login (Authorization Code + PKCE) is unchanged.

#### JWT matching

All configured constraints on the machine are evaluated with **AND** semantics.

| Constraint | Rule |
|------------|------|
| `subject` | Token `sub` equals this value. |
| `subject_prefix` | Token `sub` starts with this value. |
| `claims` | Each entry is `{ "claim": "<name>", "op": "<op>", "value": … }` and must succeed. |

Claim operations (`op`):

| `op` | `value` | Success when |
|------|---------|----------------|
| `eq` | string | The claim equals this string |
| `in` | non-empty list of strings | The claim equals one of the strings |
| `prefix` | string | The claim is a string that starts with this value |

Claim names are **top-level** JWT claims (for example `repository`, `ref`, `sub`, `environment`). Nested paths and general-purpose policy languages (CEL, etc.) are out of scope.

A verified JWT authenticates a machine only when **exactly one** enabled machine in the whole system matches. Zero matches or two or more matches are rejected (`401`). An administrator who creates overlapping conditions (two machines in two pools both matching the same GitHub repository) has produced an invalid configuration for that token.

#### Principal (machine)

After successful machine authentication, kilhog recognizes a **principal** with at least:

| Attribute | Required | Description |
|-----------|----------|-------------|
| `kind` | yes | `machine` |
| `machine_pool` | yes | Pool that owns the machine |
| `machine` | yes | The machine |
| `auth_method` | yes | `api_key` or `jwt` |
| `issuer` | when `auth_method` is `jwt` | Token `iss` |
| `subject` | when `auth_method` is `jwt` | Token `sub` |

### Auth capabilities (business)

When authentication features are available, kilhog exposes capabilities such as:

- Local login and bootstrap of the primo-admin
- List enabled identity pools (for login discovery)
- Start OIDC login for a given pool (redirect to the IdP)
- Handle the IdP callback
- End session (logout)
- Read the current principal
- Admin CRUD for local users and OIDC identity pools
- Admin CRUD for machine pools, machine identity providers (JWKS), machines, and machine API keys

Exact paths and payloads are defined in `TECHNICAL.md`.

### Failure modes

| Situation | Expected outcome |
|-----------|------------------|
| No authentication method available | `403` — authentication not configured |
| Missing credentials on a protected route | `401` Unauthenticated |
| Invalid local username/password | `401` Unauthenticated |
| Invalid, expired, or wrong-audience token | `401` Unauthenticated |
| Token issuer does not match any enabled OIDC pool or machine identity provider | `401` Unauthenticated |
| JWT signature valid but no machine matches the claims | `401` Unauthenticated |
| JWT matches more than one machine | `401` Unauthenticated |
| Machine, machine pool, or (for JWT) provider disabled | `401` Unauthenticated |
| Machine API key revoked or expired | `401` Unauthenticated |
| JWKS unavailable or unusable during JWT validation (`discovery` / `uri`) | `401` Unauthenticated for that JWT; other methods unaffected |
| IdP unavailable during interactive login | Login fails with a clear error; other methods and existing sessions are unaffected |
| Callback with invalid state / PKCE verifier | Rejected — no session established |
| Non-admin attempts user, OIDC pool, or machine-identity administration | `403` Forbidden |
| Bootstrap when a local user already exists | Rejected |

### Out of scope (this version)

The following are **explicitly not** part of this specification:

- Network-scoped or resource-scoped authorization (RBAC on networks / subnets)
- Mapping IdP groups/roles to kilhog permissions
- Anonymous self-registration of local users (beyond primo-admin bootstrap)
- Removing or deprecating the deployment-wide API key (it remains the get-started M2M path)
- OAuth 2.0 Client Credentials through a **human** OIDC identity pool
- JWKS attached to an individual machine (trust lives on the **provider**, child of the pool)
- Policy languages beyond the `eq` / `in` / `prefix` claim operations
- Granting OIDC-pool, user, or machine-identity administration to anyone other than a local `admin` (future RBAC may change this)

### Relationship to tenancy

Authentication establishes **who** is calling. It does **not** by itself restrict which **network** (tenancy) a caller may access. In this version, any authenticated principal (deployment-wide API key, local user, OIDC, or machine identity) may operate on all networks. Future RBAC may bind principals to networks; that is outside this specification.
