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
├── scripts/
│   └── dev/             # Scripts HTTP pour le développement local
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

## Utilitaires IPv4 (`internal/iputil`)

| Fonction | Rôle |
|----------|------|
| `ParseIPv4Prefix` | Parse et normalise une adresse/prefix IPv4 |
| `ValidateIPv4Subnet` | Vérifie containment parent et absence d'overlap entre siblings |
| `FindFreeIPv4Block` | Trouve la première plage `/prefix` libre dans le CIDR d'un subnet parent |

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

| Méthode | Route               | Description |
|---------|---------------------|-------------|
| GET     | `/healthz`          | État de santé du serveur (inclut un ping base de données) |
| GET     | `/networks`         | Liste tous les networks |
| POST    | `/networks`         | Crée un network |
| GET     | `/networks/{uuid}`  | Récupère un network par UUID |
| PUT     | `/networks/{uuid}`  | Met à jour un network |
| DELETE  | `/networks/{uuid}`  | Supprime un network (refusé si des subnets ont ce network comme parent) |
| GET     | `/networks/{uuid}/subnets` | Liste tous les subnets d'un network |
| POST    | `/networks/{uuid}/subnets` | Crée un subnet enfant direct du network |
| GET     | `/networks/{uuid}/subnets/{subnet_uuid}` | Récupère un subnet du network |
| PUT     | `/networks/{uuid}/subnets/{subnet_uuid}` | Met à jour la description d'un subnet |
| DELETE  | `/networks/{uuid}/subnets/{subnet_uuid}` | Supprime un subnet (refusé s'il a des enfants) |
| GET     | `/networks/{uuid}/subnets/{subnet_uuid}/subnets` | Liste les subnets enfants d'un subnet |
| POST    | `/networks/{uuid}/subnets/{subnet_uuid}/subnets` | Crée un subnet enfant d'un subnet |

> **Tenancy** : toutes les opérations sur les subnets passent par `/networks/{uuid}/…`. Le `uuid` du network dans l'URL est le périmètre d'isolation ; le serveur vérifie que chaque subnet appartient bien à ce network (directement ou via la hiérarchie de parents).

### Tenancy et scoping API

L'API est organisée autour du **network comme frontière de tenancy** :

- **RBAC** : les permissions peuvent être définies par `network/{uuid}` sans parcourir l'arbre de subnets.
- **Multi-tenancy** : chaque requête subnet porte explicitement le network cible ; un subnet d'un autre network renvoie `404`.
- **Merge / fédération** : deux instances peuvent fusionner des networks indépendamment ; l'UUID network est la clé de regroupement.
- **Parent implicite** : le corps de création ne contient plus `parent` — il est dérivé de l'URL, ce qui évite les incohérences URL/body.

```
/networks/{uuid}/subnets                              → enfants directs du network
/networks/{uuid}/subnets/{subnet_uuid}                → CRUD d'un subnet
/networks/{uuid}/subnets/{subnet_uuid}/subnets        → enfants directs du subnet
```

Le champ `parent` reste exposé dans les **réponses** (immuable après création) pour reconstruire la hiérarchie côté client.

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

Liste tous les networks, triés par nom.

Réponse `200 OK` :

```json
{
  "status": "success",
  "data": [
    {
      "uuid": "550e8400-e29b-41d4-a716-446655440000",
      "name": "lab",
      "description": "Réseau de laboratoire",
      "tags": [{"key": "env", "value": "dev"}]
    }
  ]
}
```

#### `POST /networks`

Crée un network. L'UUID est généré côté serveur.

Corps de requête :

```json
{
  "name": "lab",
  "description": "Réseau de laboratoire",
  "tags": [{"key": "env", "value": "dev"}]
}
```

| Champ         | Requis | Description |
|---------------|--------|-------------|
| `name`        | oui    | Nom unique en base |
| `description` | non    | Texte libre |
| `tags`        | non    | Paires key–value (clés uniques par resource) |

Réponse `201 Created` : le network créé dans `data`.

Erreurs :

| Code | Condition |
|------|-----------|
| `400` | Corps JSON invalide, `name` manquant, clé de tag dupliquée |
| `409` | `name` déjà utilisé |

#### `GET /networks/{uuid}`

Récupère un network par UUID.

Réponse `200 OK` : le network dans `data`.

Erreurs :

| Code | Condition |
|------|-----------|
| `400` | UUID invalide |
| `404` | Network introuvable |

#### `PUT /networks/{uuid}`

Met à jour un network existant. Le corps de requête a la même forme que `POST /networks`.

Réponse `200 OK` : le network mis à jour dans `data`.

Erreurs :

| Code | Condition |
|------|-----------|
| `400` | UUID ou corps invalide |
| `404` | Network introuvable |
| `409` | `name` déjà utilisé par un autre network |

#### `DELETE /networks/{uuid}`

Supprime un network **uniquement s'il n'a pas de subnets enfants** (subnets dont le parent est ce network). Si au moins un subnet référence ce network comme parent, la suppression est refusée.

Réponse `200 OK` :

```json
{
  "status": "success",
  "data": null
}
```

Erreurs :

| Code | Condition |
|------|-----------|
| `400` | UUID invalide |
| `404` | Network introuvable |
| `409` | Le network a des subnets enfants |

### Couche service (`internal/service/network.go`)

`NetworkService` encapsule la logique métier des networks :

- Génération de l'UUID à la création
- Validation du `name` (obligatoire, trim)
- Unicité du `name` (vérification applicative avant insert/update)
- Validation des tags (clés uniques)
- **Protection à la suppression** : appel à `SubnetRepository.ListByParent` avec `parent.kind = network` ; si des subnets existent, retourne `ErrNetworkHasChildren` (HTTP 409)

### Subnets

Toutes les routes subnets sont **scopées par network** (`{uuid}` = UUID du network). Le parent n'est **pas** fourni dans le corps de requête : il est implicite via l'URL.

| Route | Parent implicite |
|-------|------------------|
| `POST /networks/{uuid}/subnets` | Le network `{uuid}` |
| `POST /networks/{uuid}/subnets/{subnet_uuid}/subnets` | Le subnet `{subnet_uuid}` |

Le champ `parent` reste présent dans les **réponses** JSON (immuable après création).

#### `GET /networks/{uuid}/subnets`

Liste tous les subnets appartenant au network, triés par nom.

Réponse `200 OK` :

```json
{
  "status": "success",
  "data": [
    {
      "uuid": "660e8400-e29b-41d4-a716-446655440001",
      "name": "dmz",
      "description": "Subnet DMZ",
      "prefix": 24,
      "address": "10.0.0.0",
      "type": "ipv4",
      "parent": {"kind": "network", "uuid": "550e8400-e29b-41d4-a716-446655440000"}
    }
  ]
}
```

Erreurs :

| Code | Condition |
|------|-----------|
| `400` | UUID network invalide |
| `404` | Network introuvable |

#### `POST /networks/{uuid}/subnets`

Crée un subnet IPv4 **enfant direct du network**. L'UUID est généré côté serveur.

Corps de requête :

```json
{
  "name": "dmz",
  "description": "Subnet DMZ",
  "prefix": 24,
  "address": "10.0.0.0",
  "type": "ipv4"
}
```

| Champ         | Requis | Description |
|---------------|--------|-------------|
| `name`        | oui    | Nom unique au sein du network |
| `description` | non    | Texte libre |
| `prefix`      | oui    | Longueur du prefix IPv4 (1–32) |
| `address`     | oui    | Adresse réseau ou hôte (obligatoire car parent = network) |
| `type`        | non    | `ipv4` (défaut) ; `ipv6` refusé pour l'instant |

Règles métier :

- Pas d'overlap avec les autres subnets ayant le même parent (le network)
- Pas de contrainte CIDR parent (le network n'a pas d'espace d'adressage)

Réponse `201 Created` : le subnet créé dans `data`.

Erreurs :

| Code | Condition |
|------|-----------|
| `400` | UUID network invalide, corps JSON invalide, champs requis manquants, adresse invalide |
| `404` | Network introuvable |
| `409` | Nom déjà utilisé, overlap avec un sibling |

#### `POST /networks/{uuid}/subnets/{subnet_uuid}/subnets`

Crée un subnet IPv4 **enfant d'un subnet existant** dans le même network.

Corps de requête (adresse explicite) :

```json
{
  "name": "apps",
  "description": "Subnet applicatif",
  "prefix": 25,
  "address": "10.0.0.0",
  "type": "ipv4"
}
```

Corps de requête (adresse auto-générée — omettre `address`) :

```json
{
  "name": "apps",
  "prefix": 25,
  "type": "ipv4"
}
```

| Champ         | Requis | Description |
|---------------|--------|-------------|
| `name`        | oui    | Nom unique au sein du network |
| `description` | non    | Texte libre |
| `prefix`      | oui    | Longueur du prefix IPv4 (1–32), plus spécifique que le parent |
| `address`     | non    | Auto-générée si absente, dans le CIDR du parent |
| `type`        | non    | `ipv4` (défaut) |

Règles métier :

- Le subnet parent doit appartenir au network `{uuid}`
- L'adresse (explicite ou générée) doit appartenir au CIDR du parent
- Pas d'overlap entre siblings du même parent subnet

Erreurs :

| Code | Condition |
|------|-----------|
| `400` | UUID invalides, adresse invalide, subnet hors CIDR parent, prefix trop large |
| `404` | Network ou subnet parent introuvable dans ce network |
| `409` | Nom déjà utilisé, overlap, aucune adresse libre |

#### `GET /networks/{uuid}/subnets/{subnet_uuid}/subnets`

Liste les subnets enfants directs d'un subnet parent.

Réponse `200 OK` : tableau de subnets dans `data`.

Erreurs :

| Code | Condition |
|------|-----------|
| `400` | UUID invalides |
| `404` | Network ou subnet parent introuvable dans ce network |

#### `GET /networks/{uuid}/subnets/{subnet_uuid}`

Récupère un subnet par UUID, en vérifiant qu'il appartient au network.

Réponse `200 OK` : le subnet dans `data`.

Erreurs :

| Code | Condition |
|------|-----------|
| `400` | UUID invalides |
| `404` | Network introuvable, ou subnet introuvable dans ce network |

#### `PUT /networks/{uuid}/subnets/{subnet_uuid}`

Met à jour **uniquement** la `description`. Les champs `name`, `prefix`, `address`, `type` et `parent` sont immuables.

Corps de requête :

```json
{
  "description": "Nouvelle description"
}
```

Réponse `200 OK` : le subnet mis à jour dans `data`.

Erreurs :

| Code | Condition |
|------|-----------|
| `400` | UUID invalides ou corps invalide |
| `404` | Network introuvable, ou subnet introuvable dans ce network |

#### `DELETE /networks/{uuid}/subnets/{subnet_uuid}`

Supprime un subnet **uniquement s'il n'a pas de subnets enfants**.

Réponse `200 OK` :

```json
{
  "status": "success",
  "data": null
}
```

Erreurs :

| Code | Condition |
|------|-----------|
| `400` | UUID invalides |
| `404` | Network introuvable, ou subnet introuvable dans ce network |
| `409` | Le subnet a des subnets enfants |

### Couche service (`internal/service/subnet.go`)

`SubnetService` encapsule la logique métier des subnets IPv4 :

- **Scoping tenancy** : chaque opération reçoit le `networkUUID` et vérifie l'appartenance via `ensureInNetwork`
- Génération de l'UUID à la création
- Validation du `name` (obligatoire, trim, unicité au sein du network)
- Parent implicite dérivé de l'URL (`CreateInNetwork`)
- Normalisation de l'adresse IPv4 (ex. `192.168.1.5/24` → `192.168.1.0/24`)
- Détection d'overlap entre siblings via `internal/iputil`
- Génération automatique d'adresse dans le CIDR du parent subnet (uniquement si `address` est absent)
- **Mise à jour limitée** : seule la `description` est modifiable
- **Protection à la suppression** : refus si des subnets enfants existent (`ErrSubnetHasChildren`, HTTP 409)

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

Les messages d'erreur de conflit (`409`) et de validation (`400`) sont **explicites** : ils incluent les valeurs en cause (nom, CIDR, prefix, etc.) pour faciliter le diagnostic côté client.

Exemple — nom de subnet déjà utilisé :

```json
{
  "status": "error",
  "message": "subnet name \"dmz\" is already used in this network",
  "code": 409
}
```

Exemple — overlap CIDR :

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

### Scripts HTTP de développement

Le dossier `scripts/dev/` contient des scripts Bash autonomes qui appellent l'API REST sur une instance locale déjà démarrée (`make run-dev`). Chaque script lit un ou plusieurs fichiers JSON UTF-8 situés à côté de lui ; il n'y a pas de bibliothèque partagée entre scripts.

**Prérequis** : `curl`. Les scripts `update-network-hors-prod.sh` et `delete-network-prod.sh` utilisent aussi `jq` pour retrouver l'UUID d'un network par son `name`.

| Variable | Défaut | Description |
|----------|--------|-------------|
| `KILHOG_BASE_URL` | `http://localhost:8080` | URL de base de l'API |

| Script | Fichier(s) JSON | Cible Make | Action |
|--------|-----------------|------------|--------|
| `create-networks.sh` | `network-prod.json`, `network-hors-prod.json` | `make dev-create-networks` | Crée `prod` et `hors-prod` |
| `update-network-hors-prod.sh` | `network-hors-prod-update.json` | `make dev-update-network-hors-prod` | Met à jour `hors-prod` |
| `delete-network-prod.sh` | — | `make dev-delete-network-prod` | Supprime `prod` |
| `create-subnets.sh` | `subnet-dmz.json`, `subnet-apps-auto.json` | `make dev-create-subnets` | Crée `dmz` sous `hors-prod` (adresse explicite) puis `apps` sous `dmz` (adresse auto) |
| `update-subnet-dmz.sh` | `subnet-dmz-update.json` | `make dev-update-subnet-dmz` | Met à jour la description de `dmz` |
| `delete-subnet-apps.sh` | — | `make dev-delete-subnet-apps` | Supprime `apps` |

Contenu des payloads :

| Fichier | `name` | `description` |
|---------|--------|---------------|
| `network-prod.json` | `prod` | `réseau de prod` |
| `network-hors-prod.json` | `hors-prod` | *(absente)* |
| `network-hors-prod-update.json` | `hors-prod` | `réseau de hors-prod` |
| `subnet-dmz.json` | `dmz` | `Subnet DMZ avec adresse explicite 10.0.0.0/24` |
| `subnet-apps-auto.json` | `apps` | `Subnet apps sous dmz, adresse auto-générée /25` |
| `subnet-dmz-update.json` | — | `Subnet DMZ mis à jour` |

Exemple d'enchaînement :

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
