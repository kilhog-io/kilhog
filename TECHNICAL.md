# TECHNICAL — kilhog API

## Stack

- **Langage** : Go 1.26+
- **Module** : `github.com/kilhog-io/kilhog`
- **Architecture** : API REST, découpage en couches

## Arborescence

```
kilhog/
├── cmd/
│   └── kilhog/          # Point d'entrée de l'application (main.go)
├── internal/
│   ├── handler/         # Handlers HTTP et validation des requêtes
│   ├── service/         # Logique métier et interfaces repository
│   ├── repository/      # Accès aux données, migrations, drivers SQL
│   │   ├── migration/   # Exécution des migrations versionnées (upgrade / downgrade)
│   │   ├── postgres/    # Implémentation PostgreSQL
│   │   └── sqlite/      # Implémentation SQLite
│   └── model/           # Modèles et structures de données
├── migrations/          # Scripts SQL versionnés embarqués (sqlite/ et postgres/)
├── FUNCTIONAL.md        # Règles métier (définies par l'utilisateur)
└── TECHNICAL.md         # Ce fichier
```

## Domain model (`internal/model`)

Modélisation technique de `FUNCTIONAL.md`. Voir ce fichier pour les règles métier.

| Type           | Fichier            | Description |
|----------------|--------------------|-------------|
| `Tag`          | `tag.go`           | Paire key–value de metadata |
| `AddressType`  | `address_type.go`  | Famille d'adressage : `ipv4`, `ipv6` |
| `Parent`       | `parent.go`        | Référence vers un parent `network` ou `subnet` |
| `Network`      | `network.go`       | Conteneur de tenancy |
| `Subnet`       | `subnet.go`        | Espace d'adressage IP (block ou host) |

### `Network`

| Champ         | Type        | JSON |
|---------------|-------------|------|
| `UUID`        | `uuid.UUID` | `uuid` |
| `Name`        | `string`    | `name` |
| `Description` | `string`    | `description` |
| `Tags`        | `[]Tag`     | `tags` |

### `Subnet`

| Champ         | Type          | JSON |
|---------------|---------------|------|
| `UUID`        | `uuid.UUID`   | `uuid` |
| `Name`        | `string`      | `name` |
| `Description` | `string`      | `description` |
| `Prefix`      | `int`         | `prefix` |
| `Address`     | `string`      | `address` |
| `Type`        | `AddressType` | `type` |
| `Parent`      | `Parent`      | `parent` |
| `Tags`        | `[]Tag`       | `tags` |

Méthodes :

- `CIDR() string` — retourne `{address}/{prefix}`
- `IsLeaf() bool` — `true` si prefix `/32` (IPv4) ou `/128` (IPv6)

Constantes : `IPv4HostPrefix` (32), `IPv6HostPrefix` (128).

### `Parent`

| Champ  | Type          | JSON |
|--------|---------------|------|
| `Kind` | `ParentKind`  | `kind` (`network`, `subnet`) |
| `UUID` | `uuid.UUID`   | `uuid` |

## Repository interfaces (`internal/service`)

Interfaces définies côté service (consommées par la logique métier) :

- `NetworkRepository` — CRUD et listing des networks
- `SubnetRepository` — CRUD, listing par network ou par parent

## Couche de persistance (`internal/repository`)

Backend de données **configurable et abstrait**. Deux drivers sont supportés : **SQLite** et **PostgreSQL**. Le service consomme les interfaces repository ; le choix du driver est transparent pour la logique métier.

### Abstraction

```
service (NetworkRepository, SubnetRepository)
    └── repository/
            ├── sqlite/    → implémentations SQLite
            ├── postgres/  → implémentations PostgreSQL
            └── migration/ → runner de migrations (commun aux deux drivers)
```

Chaque driver implémente les interfaces définies dans `internal/service`. Les requêtes SQL utilisent des dialectes adaptés là où nécessaire (types UUID, `TIMESTAMPTZ`, etc.).

Les implémentations concrètes des repositories se trouvent dans `internal/repository/` (`network_repository.go`, `subnet_repository.go`) et sont instanciées via `repository.Open`.

### Accès concurrents

Les appels API peuvent arriver en parallèle. La couche `db.Store` fournit les primitives suivantes :

| Mécanisme | Rôle |
|-----------|------|
| Pool `database/sql` | Connexions concurrentes (lectures parallèles) |
| `WithWriteLock` | Sur SQLite, mutex applicatif sérialisant les écritures |
| `WithTx` / `WithWriteTx` | Transactions SQL atomiques (prêtes pour la logique métier future) |
| `AcquireMigrationLock` | Verrou exclusif pendant les migrations (mutex SQLite, `pg_advisory_lock` PostgreSQL) |
| WAL + `busy_timeout` (SQLite) | Lecteurs non bloqués par les écrivains |

Les opérations de mutation (`Create`, `Update`, `Delete`) passent par `WithWriteTx` : verrou d'écriture SQLite + transaction SQL. Les lectures (`Get*`, `List*`) utilisent le pool directement.

Sur PostgreSQL, le verrou applicatif SQLite est désactivé : la concurrence est gérée par MVCC et les transactions SQL.

### Connexion et création de la base

Au démarrage, l'application :

1. Lit la configuration (`KILHOG_DB_DRIVER`, `KILHOG_DB_DSN`).
2. **Crée la base si elle n'existe pas** :
   - **SQLite** : crée le fichier et les répertoires parents du chemin DSN.
   - **PostgreSQL** : se connecte au catalogue `postgres`, exécute `CREATE DATABASE` si la base cible est absente, puis se reconnecte à la base cible.
3. Ouvre une connexion poolée vers la base.
4. Exécute les migrations **upgrade** en attente.
5. Injecte les repositories dans les services / handlers.

### Migrations SQL versionnées

Les scripts SQL sont embarqués via `go:embed` dans `internal/repository/migration/migrations/`, organisés par dialecte :

```
internal/repository/migration/migrations/
├── sqlite/
│   ├── 001_initial_schema.up.sql
│   └── 001_initial_schema.down.sql
└── postgres/
    ├── 001_initial_schema.up.sql
    └── 001_initial_schema.down.sql
```

| Fichier | Rôle |
|---------|------|
| `{version}_{name}.up.sql` | Applique la version N |
| `{version}_{name}.down.sql` | Annule la version N |

Convention :

- `{version}` : entier sur 3 chiffres, zero-padded (`001`, `002`, …).
- `{name}` : identifiant snake_case décrivant le changement (`initial_schema`).

Exemple (structure par dialecte) :

```
internal/repository/migration/migrations/sqlite/
├── 001_initial_schema.up.sql
└── 001_initial_schema.down.sql
```

#### Table de suivi : `schema_migrations`

| Colonne      | Type        | Description |
|--------------|-------------|-------------|
| `version`    | `INTEGER`   | Numéro de version appliquée (PK) |
| `applied_at` | `TIMESTAMPTZ` | Horodatage d'application |

#### Comportement

| Opération | Déclenchement | Comportement |
|-----------|---------------|--------------|
| **Upgrade** | Automatique au démarrage | Applique toutes les versions `> version courante`, dans l'ordre croissant |
| **Downgrade** | Explicite (CLI ou API admin) | Exécute le `.down.sql` de la version cible, supprime l'entrée dans `schema_migrations` |

Les migrations déjà appliquées ne sont jamais rejouées. En cas d'échec mid-migration, la transaction est annulée et la version n'est pas enregistrée.

### Schéma relationnel

Modélisation des entités `Network`, `Subnet` et `Tag` définies dans `FUNCTIONAL.md`.

#### Table `networks`

| Colonne       | Type          | Contraintes | Mappe |
|---------------|---------------|-------------|-------|
| `uuid`        | UUID          | PK          | `Network.UUID` |
| `name`        | TEXT          | NOT NULL, UNIQUE | `Network.Name` |
| `description` | TEXT          | NULL        | `Network.Description` |
| `created_at`  | TIMESTAMPTZ   | NOT NULL, DEFAULT now() | — |
| `updated_at`  | TIMESTAMPTZ   | NOT NULL, DEFAULT now() | — |

#### Table `subnets`

| Colonne        | Type          | Contraintes | Mappe |
|----------------|---------------|-------------|-------|
| `uuid`         | UUID          | PK          | `Subnet.UUID` |
| `network_uuid` | UUID          | NOT NULL, FK → `networks(uuid)` ON DELETE CASCADE | Tenancy (network root) |
| `name`         | TEXT          | NOT NULL, UNIQUE `(network_uuid, name)` | `Subnet.Name` |
| `description`  | TEXT          | NULL        | `Subnet.Description` |
| `prefix`       | INTEGER       | NOT NULL    | `Subnet.Prefix` |
| `address`      | TEXT          | NOT NULL    | `Subnet.Address` |
| `address_type` | TEXT          | NOT NULL, CHECK IN (`ipv4`, `ipv6`) | `Subnet.Type` |
| `parent_kind`  | TEXT          | NOT NULL, CHECK IN (`network`, `subnet`) | `Subnet.Parent.Kind` |
| `parent_uuid`  | UUID          | NOT NULL    | `Subnet.Parent.UUID` |
| `created_at`   | TIMESTAMPTZ   | NOT NULL, DEFAULT now() | — |
| `updated_at`   | TIMESTAMPTZ   | NOT NULL, DEFAULT now() | — |

Index recommandés :

- `idx_subnets_network_uuid` sur `network_uuid`
- `idx_subnets_parent` sur `(parent_kind, parent_uuid)`

> **Note tenancy** : `network_uuid` est dénormalisé sur chaque subnet pour garantir l'unicité du `name` au sein du network et simplifier les requêtes de listing, même lorsque le parent direct est un autre subnet.

#### Table `tags`

Tags polymorphiques attachés à un network ou un subnet.

| Colonne         | Type          | Contraintes | Mappe |
|-----------------|---------------|-------------|-------|
| `resource_kind` | TEXT          | NOT NULL, CHECK IN (`network`, `subnet`) | Type de resource |
| `resource_uuid` | UUID          | NOT NULL    | UUID du network ou subnet |
| `key`           | TEXT          | NOT NULL    | `Tag.Key` |
| `value`         | TEXT          | NOT NULL    | `Tag.Value` |

Clé primaire composite : `(resource_kind, resource_uuid, key)`.

FK avec suppression en cascade :

- `(network, resource_uuid)` → `networks(uuid)` ON DELETE CASCADE
- `(subnet, resource_uuid)` → `subnets(uuid)` ON DELETE CASCADE

#### Diagramme des relations

```
networks (1) ──< subnets (N)
    │                  │
    └──< tags          └──< tags
         (polymorphique, resource_kind + resource_uuid)
```

#### Script initial (`001_initial_schema`)

Le script `001_initial_schema.up.sql` crée les tables `schema_migrations`, `networks`, `subnets` et `tags` avec les contraintes ci-dessus. Le script `001_initial_schema.down.sql` supprime ces tables dans l'ordre inverse des dépendances.

### Différences dialectales

| Aspect | SQLite | PostgreSQL |
|--------|--------|------------|
| Type UUID | `TEXT` (format canonical) ou `BLOB` | `UUID` natif |
| Horodatage | `TEXT` ISO-8601 ou `DATETIME` | `TIMESTAMPTZ` |
| Création de base | Fichier sur disque | `CREATE DATABASE` via connexion admin |
| Driver Go | `modernc.org/sqlite` ou `mattn/go-sqlite3` | `jackc/pgx/v5` |

Les migrations peuvent contenir des sections dialect-specific si nécessaire ; à défaut, le SQL reste le plus portable possible.

## Dépendances

| Module | Usage |
|--------|-------|
| `github.com/google/uuid` | Identifiants `UUID` des resources |
| `database/sql` | Abstraction SQL standard Go |
| `modernc.org/sqlite` | Driver SQLite (pure Go) |
| `jackc/pgx/v5` | Driver PostgreSQL |

## Routes API

| Méthode | Route      | Description              |
|---------|------------|--------------------------|
| GET     | `/healthz` | État de santé du serveur (inclut un ping base de données) |

Réponse `GET /healthz` :

```json
{
  "status": "success",
  "data": {
    "status": "ok"
  }
}
```

## Configuration

| Variable           | Défaut              | Description |
|--------------------|---------------------|-------------|
| `KILHOG_HOST`      | `0.0.0.0`           | Adresse d'écoute HTTP |
| `KILHOG_PORT`      | `8080`              | Port d'écoute HTTP |
| `KILHOG_DB_DRIVER` | `sqlite`            | Driver de base : `sqlite` ou `postgres` |
| `KILHOG_DB_DSN`    | `file:kilhog.db`    | DSN de connexion (voir exemples ci-dessous) |
| `KILHOG_AUTO_MIGRATE` | `true`           | Appliquer les migrations upgrade au démarrage |

### Exemples de DSN

| Driver   | DSN exemple |
|----------|-------------|
| SQLite   | `file:./data/kilhog.db?_pragma=foreign_keys(ON)` |
| PostgreSQL | `postgres://user:pass@localhost:5432/kilhog?sslmode=disable` |

## Format des réponses JSON

- Succès : `{"status": "success", "data": ...}`
- Erreur : `{"status": "error", "message": "...", "code": 400}`

## Build

```bash
make build
```

Compile le binaire dans `bin/kilhog` à partir de `./cmd/kilhog`.

## Run

### Développement local (recommandé)

```bash
make run-dev
```

Compile l'application puis la lance avec SQLite :

- **Driver** : `sqlite`
- **Fichier** : `kilhog.db` à la racine du projet (créé automatiquement au premier démarrage)
- **Migrations** : appliquées automatiquement (`KILHOG_AUTO_MIGRATE=true` par défaut)
- **Écoute** : `http://0.0.0.0:8080`

Le fichier `kilhog.db` (et ses fichiers auxiliaires SQLite `kilhog.db-wal`, `kilhog.db-shm`) est ignoré par Git (voir `.gitignore`).

### Lancement direct

```bash
go run ./cmd/kilhog
```

Utilise les mêmes valeurs par défaut que `run-dev` (`sqlite`, DSN `file:kilhog.db?_pragma=foreign_keys(ON)`).
