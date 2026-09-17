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
| Grant    | `uuid` | Database         |
| Grant    | `(principal, resource)` | Database |

## Persistence

Business data (networks, subnets, tags, identities, and grants) is persisted in a **relational database**. The persistence layer is **abstracted**: the application supports multiple engines without changing the business model or the rules above.

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
| Network deletion | Cascades to associated subnets, tags, and grants |
| One grant per principal and resource | Unique constraint on the principal + resource pair |
| Grant target must exist | Foreign keys to identities / networks / subnets |

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
- Successful authentication as the deployment-wide key **bypasses resource grants**: full CRUD on every network and subnet, and the right to create networks (no per-caller identity beyond “API key holder”). See [Authorization (RBAC)](#authorization-rbac).
- The API key does **not** grant rights to administer local users, OIDC identity pools, machine identities, or grants (see [Roles](#roles)). It does **not** become owner of resources it creates.
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
5. The primo-admin (and any later `admin`) can create additional local users, configure OIDC identity pools, configure machine identities (pools, JWT providers, machines, and per-machine API keys), assign the right to create networks, and manage grants.

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

Local user `role` controls **identity administration** (users, OIDC pools, machine identities). IPAM access for everyone else is controlled by [grants](#authorization-rbac).

At **setup**, only a local `admin` (and optionally the deployment-wide API key) can create networks, subnets, identity pools, and machines. The admin then grants `create_networks` to users and groups; those principals become **owners** of the networks they create and can grant rights on what they own.

| Principal | IPAM (networks / subnets) | Create networks | Manage grants on owned resources | Manage local users | Manage OIDC pools | Manage machine identities |
|-----------|---------------------------|-----------------|----------------------------------|--------------------|-------------------|---------------------------|
| Local `admin` | Full CRUD **and implicit owner** (bypass) | yes (implicit) | yes (all resources, bypass) | yes | yes | yes |
| Local `user` | Only through grants | Only with platform grant | If owner | no | no | no |
| OIDC principal | Only through grants | Only with platform grant | If owner | no | no | no |
| Machine identity | Only through grants | Only with platform grant | If owner | no | no | no |
| Deployment-wide API key | Full CRUD (bypass) | yes (implicit) | no | no | no | no |

Only a local **`admin`** may manage machine identities, OIDC identity pools, and local users. Machines cannot administer identities.

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
| `groups_claim`   | no       | JWT claim that lists IdP groups (default `groups`). Used as [grant subjects](#principal-grant-subject). |
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
- Keep IPAM access **authentication-gated**. After authentication, [RBAC grants](#authorization-rbac) decide which networks and subnets a principal may use. A federated login alone does **not** confer IPAM rights. IdP **groups** from `groups_claim` may be grant subjects.

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
| `groups` | no | Group names/ids from the pool’s `groups_claim` (string or array of strings) |

The pair `(issuer, subject)` — equivalently `(identity_pool, subject)` given one pool per issuer — uniquely identifies a federated principal.

Linking a federated principal to a local user account (account linking) is **optional** and not required for IPAM access. An OIDC principal receives IPAM rights through **grants** that target that principal, an **OIDC group** they belong to, or a platform `create_networks` grant. Administration of users, OIDC identity pools, and machine identities remains reserved to local `admin` users.

#### Token and session rules

- Access tokens presented to the API must be validated (signature, issuer matching an enabled pool, expiry, and audience / client constraints as configured for that pool).
- Group membership used for grants is taken from the pool’s `groups_claim` at authentication time (interactive session snapshot, or the bearer JWT’s claims).
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
- Successful authentication uses that machine as the principal. IPAM rights come from [grants](#authorization-rbac) targeting the machine or its machine pool (see [Roles](#roles)).
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
- Read the current principal and that principal’s own grants (including grants inherited from groups / machine pools)
- Admin CRUD for local users, OIDC identity pools, machine pools, providers, machines, and machine API keys
- Admin assignment of platform `create_networks` grants
- Grant CRUD and ownership share/transfer on a network or subnet by its **owners** (and by `admin`, who bypasses ownership)

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
| Authenticated caller without the required permission, resource not visible | `404` Not Found |
| Authenticated caller who can see the resource but lacks the required permission | `403` Forbidden |
| Non-owner attempts grant administration on a resource they do not own (and is not `admin`) | `403` Forbidden |
| Principal without `create_networks` (and not admin / deployment API key) calls `POST /networks` | `403` Forbidden |
| Ownership transfer with invalid `from` / `to`, or that would leave a network with no owner (non-admin) | `400` / `409` |
| Disabled or deleted identity used as a grant subject at evaluation time | Grant is ignored (no rights) |

## Authorization (RBAC)

Authentication answers **who** is calling. Authorization answers **what** that caller may do on which **network** and **subnet**.

### Lifecycle

1. **Setup.** The primo-admin configures local users, OIDC identity pools, and machine identities. Only `admin` (and optionally the deployment-wide API key) can create IPAM resources at this stage.
2. **Delegate network creation.** The admin grants the platform capability `create_networks` to users and/or groups (OIDC groups, machine pools, or individual machines).
3. **Ownership.** When such a principal creates a network, kilhog records them as **owner** of that network. Ownership includes full CRUD on the network and every subnet in it, so the owner can create subnets immediately.
4. **Delegate inside the tenancy.** Owners **share** ownership (`owner = true` to another identity or group) or **transfer** it, and grant CRUD on the network or on specific subnets.
5. **Break-glass.** A local `admin` bypasses ownership at any time: they may operate on every network and subnet and manage every grant, without being listed as owner. That power exists to recover tenancy when owners are revoked; it is not a substitute for assigning owners in normal operation.

Default is **deny**: a local `user`, OIDC principal, or machine with no applicable grant cannot list, read, create, update, or delete any network or subnet, and cannot create networks.

Privileged exceptions:

| Principal | IPAM bypass | Create networks | Ownership / grant administration |
|-----------|-------------|-----------------|----------------------------------|
| Local `admin` | yes — implicit owner of every network and subnet | yes | yes, without an owner grant |
| Deployment-wide API key | yes (IPAM only) | yes | no |

Health probes (`GET /healthz`) and metrics (`GET /metrics`) stay public and are not subject to RBAC.

### Entity: Grant

A **grant** assigns permissions to **one** subject on **one** scope: a **network**, a **subnet**, or a **platform** capability.

#### Attributes

| Attribute | Required | Description |
|-----------|----------|-------------|
| `uuid` | yes | Unique identifier. Unique across the database. |
| `principal` | yes | The subject of the grant (see below). |
| `resource` | yes | Network, subnet, or platform capability. |
| `create` | yes | Permission to create **child** resources under this scope (`create_networks` when the resource is platform). |
| `read` | yes | Permission to read this resource. Unused on platform grants. |
| `update` | yes | Permission to update this resource. Unused on platform grants. |
| `delete` | yes | Permission to delete this resource. Unused on platform grants. |
| `owner` | yes | When `true`, the subject is an **owner** of this network or subnet (see [Ownership](#ownership)). Must be `false` on platform grants. |
| `created_at` | yes | Creation timestamp. |
| `updated_at` | yes | Last modification timestamp. |

Rules:

- For a network or subnet grant, at least one of `create`, `read`, `update`, `delete`, or `owner` must be `true`.
- `owner = true` implies all four CRUD flags are `true` (stored or treated as such).
- For a platform grant, the only valid capability in this version is `create_networks`, and only `create` is meaningful (`true`).
- There is **at most one grant** per `(principal, resource)` pair. Changing rights is an update of that grant.

#### Principal (grant subject)

A grant targets exactly one of:

| Kind | Identified by | Who matches |
|------|---------------|-------------|
| Local user | `local_user` UUID | That user, if **enabled**. Disabled or deleted → no rights ([revocation](#identity-revocation)). |
| OIDC principal | identity pool UUID + `subject` | That federated user, if the pool is enabled |
| OIDC group | identity pool UUID + group name | Any OIDC principal of that pool whose token/session `groups` contain this name **now** |
| Machine | machine UUID | That machine, if the machine **and** its pool are enabled |
| Machine pool | machine pool UUID | Every **enabled** machine in that **enabled** pool |

The deployment-wide API key is **not** a grant subject. It always bypasses grants.

A grant cannot target a local `admin`: that role already bypasses IPAM RBAC. Creating such a grant is rejected.

#### Resource (grant object)

| Kind | Meaning |
|------|---------|
| **Platform** | Capability that is not tied to an existing network. This version defines `create_networks` only. |
| **Network** | Rights on that tenancy container, inherited by every subnet in the network. |
| **Subnet** | Rights on that subnet, inherited by every nested descendant subnet. |

Network and subnet resources must exist at grant creation time. A subnet grant is always scoped to the subnet’s root network.

### Platform capability: `create_networks`

`POST /networks` is allowed when the caller is:

- a local `admin`, or
- the deployment-wide API key, or
- a principal with an effective `create_networks` grant (direct, via OIDC group, or via machine pool).

Only a local `admin` may create, list all, update, or delete **platform** grants.

When a non-privileged principal (local `user`, OIDC, or machine) successfully creates a network, kilhog **creates an owner grant** on that network for the creating principal (the user or machine, not the group or pool that conferred `create_networks`).

The deployment-wide API key and local `admin` do not receive an owner row: they already bypass RBAC. The admin can still **assign** or **transfer** ownership to named principals so day-to-day operators do not depend on break-glass.

### Ownership

A local **`admin` is an implicit owner of every network and subnet**. Ownership checks do not apply to them: they may read, mutate, delete, list grants, share ownership, transfer ownership, and recover a tenancy that has no remaining effective owner. They should use that power to restore a named owner, not to run ordinary IPAM as a substitute for grants.

A named **owner** of a resource (a grant with `owner = true`) has:

- full CRUD on that resource (and, by inheritance, on descendants);
- the right to **list, create, update, and delete grants** on that resource and on its descendants;
- the right to **share** and **transfer** ownership on that resource (and descendants, for a network owner).

| Owns | May manage grants and ownership on |
|------|-------------------------------------|
| A network | That network and every subnet in it |
| A subnet | That subnet and its nested descendants (not the parent network, not siblings) |

Ownership is recorded as `owner = true` on a grant. It is inherited **down** the tree the same way CRUD is: a network owner is treated as owner of every subnet in that network for grant-management purposes.

#### Share ownership

An owner (or `admin`) may grant `owner = true` to **another identity or group** on a resource they own. Existing owners are unchanged. Several owners may coexist (a user, an OIDC group, and a machine pool at once).

Sharing is a normal grant create/update with `owner = true`.

#### Transfer ownership

**Transfer** moves ownership from one subject to another in a **single atomic operation**:

1. The destination receives `owner = true` (create or update that grant).
2. The source loses `owner` (the source grant is deleted if it would have no remaining flags; otherwise `owner` is set to `false`).

Rules:

- Caller must be a local `admin` or an owner of the resource.
- `to` may be any valid grant subject (local user, OIDC principal, OIDC group, machine, machine pool), except a local `admin`.
- `from` defaults to the caller’s own owner grant. An `admin` (or an owner transferring a **group** / **machine pool** owner grant they control) may set `from` explicitly.
- The destination must exist and be enabled (a disabled identity cannot receive ownership).
- A non-admin transfer must not leave the resource with **zero remaining owner grants**. Because the destination is written first in the same transaction, transferring the last owner grant to a new subject is allowed.
- Transfer does not copy non-owner CRUD flags from the source to the destination.

After a transfer, the previous owner has no ownership (and no implied CRUD from that owner grant). They keep other grants they still hold (for example a separate `read` grant, or rights via a group).

Owners may also:

- grant any combination of CRUD on resources they own, to any valid grant subject;
- revoke grants they can see on those resources.

Owners may **not**:

- assign platform `create_networks` grants;
- manage identity administration (users, OIDC pools, machine identities);
- grant rights on a resource they do not own;
- grant more scope than they own (a subnet owner cannot attach a grant to the parent network).

A non-admin caller cannot delete or set `owner = false` on the **last remaining owner grant** of a network (so a tenancy cannot lose every owner **through grant edits**). An `admin` can always repair or reassign ownership, including after every named owner has been revoked.

### Identity revocation

When an identity is **revoked**, its rights **disappear immediately** from authorization: they cannot authenticate, and any stored grants targeting them (or a group/pool they no longer match) are ignored.

| Event | Authentication | Effective grants |
|-------|----------------|------------------|
| Local user **disabled** | Rejected (`401`) | Grants targeting that user are **inert** until the user is enabled again |
| Local user **deleted** | Impossible | Grant rows targeting that user are **deleted** |
| Machine or machine pool **disabled** | Rejected (`401`) | Grants targeting that machine / pool are **inert** until enabled again |
| Machine **deleted** | Impossible | Grant rows targeting that machine are **deleted** |
| Machine pool **deleted** | Impossible | Grant rows targeting that pool and its machines are **deleted** |
| Machine API key **revoked** or expired | That credential is rejected (`401`) | Grants on the **machine** remain; other keys or JWT for the same machine still use them |
| OIDC identity pool **disabled** or **deleted** | New logins/tokens through that pool fail | Direct and group grants for that pool are inert (disable) or **deleted** (delete) |
| Principal **leaves an OIDC group** | Unchanged | The OIDC **group** grant no longer applies to them |
| Machine **leaves** a pool (deleted from the pool) | Follows machine delete | Machine grants deleted |

Inert grants are not applied in `Can` / `IsOwner` / list filtering. Re-enabling a disabled user, machine, or pool restores those grants without recreating them.

If revocation removes the last **effective** owner of a network (disabled last owner, deleted last owner, empty OIDC group, disabled machine pool), the network is not deleted. A local `admin` bypasses ownership and must assign or transfer ownership to a living principal.

### CRUD meaning

Permissions are independent flags. Typical combinations:

| Combination | Intended use |
|-------------|--------------|
| `read` only | Observer |
| `read` + `create` | Allocate child subnets without changing or deleting existing ones |
| `read` + `update` | Edit descriptions / tags without allocating or deleting |
| `owner` (implies full CRUD) | Owner of that scope: operate and grant |

#### On a **network** grant

| Flag | Effect on the network | Inherited effect on subnets in that network |
|------|----------------------|-----------------------------------------------|
| `create` | Create a **direct child subnet** of the network (`POST /networks/{uuid}/subnets`) | Create a child under **any** subnet in the tree |
| `read` | Get the network; include it in `GET /networks` | Get and list those subnets |
| `update` | Update the network (`name`, `description`, `tags`) | Update those subnets (description) |
| `delete` | Delete the network (still refused if it has child subnets) | Delete those subnets (still refused if they have children) |
| `owner` | All of the above, plus grant management on the network and its subnets | Same |

Creating a **new network** uses the platform `create_networks` capability, not a network grant.

#### On a **subnet** grant

| Flag | Effect on that subnet | Inherited effect on descendant subnets |
|------|----------------------|----------------------------------------|
| `create` | Create a **direct child** of that subnet | Create a child under any descendant |
| `read` | Get that subnet | Get and list descendants |
| `update` | Update that subnet’s description | Update descendants |
| `delete` | Delete that subnet (still refused if it has children) | Delete descendants (same child-protection rule) |
| `owner` | All of the above, plus grant management on the subtree | Same |

A subnet grant does **not** confer `create` / `update` / `delete` / `owner` on the parent network or on sibling subnets.

Creating a subnet does **not** automatically make the creator owner of that subnet. The network owner stays in control unless they (or an admin) grant subnet ownership explicitly.

### Inheritance and effective permissions

Effective permissions on a resource are the **union** (OR) of all applicable grants: the principal’s own grants plus grants on OIDC groups they belong to and, for a machine, grants on its machine pool. There is no deny rule.

For a **network** N, applicable resource grants are the grants on N itself.

For a **subnet** S whose root network is N, applicable grants are:

1. the grant on S (direct);
2. grants on every **ancestor subnet** of S;
3. the grant on network N.

A more specific grant **adds** rights; it never removes rights inherited from above.

**Structural visibility (read-up):** if a principal has **any** permission on a subnet, they may **read** that subnet’s ancestor subnets and the containing network, only to reconstruct the hierarchy. Structural visibility does **not** grant `create`, `update`, `delete`, or `owner` on those ancestors, and does **not** allow listing siblings or other branches that are not otherwise readable.

### Listing and tenancy

| Operation | Visible set |
|-----------|-------------|
| `GET /networks` | Networks the principal can `read` (directly, inherited, via group/pool, or via structural visibility) |
| `GET /networks/{uuid}/subnets` | Subnets in that network the principal can `read`, plus ancestors required for structural visibility |
| `GET /networks/{uuid}/subnets/{id}/subnets` | Direct children the principal can `read` |

A subnet that belongs to another network still returns `404` (tenancy mismatch).

### Missing permission vs missing resource

| Situation | HTTP |
|-----------|------|
| Resource does not exist | `404` |
| Resource exists, principal has no applicable grant and no structural visibility | `404` |
| Resource exists and is visible, but the requested flag is missing | `403` |
| `POST /networks` without `create_networks` / admin / deployment API key | `403` |

Business conflicts (name clash, CIDR overlap, delete with children) still return `409` **after** authorization succeeds.

### Grant management

| Operation | Who |
|-----------|-----|
| Platform `create_networks` grants | Local `admin` only |
| Create / update / delete grants on a network or subnet | Local `admin` (bypass), or an **owner** of that resource (or of an ancestor that confers ownership) |
| Share ownership (`owner = true` on another subject) | Same |
| Transfer ownership | Same |
| List grants on a network or subnet | Same as create/update/delete |
| List **own** grants | The authenticated principal (including grants inherited from OIDC groups or a machine pool; excluding inert grants) |

Grant updates replace the flags (including `owner`). Deleting a grant immediately removes those rights; in-flight requests already authorized are not retroactively cancelled.

### Persistence and cascade

| Event | Effect on grants |
|-------|------------------|
| Network deleted | All grants on that network **and** on every subnet in that network are deleted |
| Subnet deleted | Grants on that subnet are deleted |
| Local user **deleted** | Grants targeting that user are deleted |
| Local user **disabled** | Grant rows kept; they are **inert** (see [Identity revocation](#identity-revocation)) |
| Identity pool **deleted** | OIDC principal and OIDC group grants for that pool are deleted |
| Identity pool **disabled** | Those grants are inert |
| Machine **deleted** | Grants targeting that machine are deleted |
| Machine or pool **disabled** | Grants targeting that machine / pool are inert |
| Machine pool **deleted** | Grants targeting that pool **and** grants targeting its machines are deleted (machines cascade) |

### Out of scope (this version)

The following are **explicitly not** part of this specification:

- Deny / negative grants (only allow-lists)
- Time-limited or conditional resource grants
- Kilhog-managed groups distinct from OIDC groups and machine pools
- Per-tag or per-field permissions
- Treating the deployment-wide API key as a grant subject
- Anonymous self-registration of local users (beyond primo-admin bootstrap)
- Removing or deprecating the deployment-wide API key (it remains the get-started path and continues to bypass IPAM grants)
- OAuth 2.0 Client Credentials through a **human** OIDC identity pool
- JWKS attached to an individual machine (trust lives on the **provider**, child of the pool)
- Policy languages beyond the `eq` / `in` / `prefix` claim operations
- Granting OIDC-pool, user, or machine-identity administration to anyone other than a local `admin`

### Relationship to tenancy

Authentication establishes **who** is calling. The **network** remains the tenancy boundary: every subnet belongs to one network, and subnet URLs always include that network UUID.

Authorization binds principals (users, OIDC groups, machines, machine pools) to **networks** and optionally to **subnets**. Creating a network is a platform right; owning that network is how tenancy is delegated after setup.

Local `admin` remains an implicit owner of every tenancy so the deployment can always recover networks after owners are revoked. Named automation uses **machine identities** under the same grant model as humans. The deployment-wide API key remains a get-started IPAM bypass, not a named owner.
