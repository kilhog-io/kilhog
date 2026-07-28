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
│   ├── repository/      # Implémentations d'accès aux données (à venir)
│   └── model/           # Modèles et structures de données
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

Interfaces définies côté service (consommées par la logique métier à venir) :

- `NetworkRepository` — CRUD et listing des networks
- `SubnetRepository` — CRUD, listing par network ou par parent

## Dépendances

| Module | Usage |
|--------|-------|
| `github.com/google/uuid` | Identifiants `UUID` des resources |

## Routes API

| Méthode | Route      | Description              |
|---------|------------|--------------------------|
| GET     | `/healthz` | État de santé du serveur |

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

| Variable      | Défaut    | Description              |
|---------------|-----------|--------------------------|
| `KILHOG_HOST` | `0.0.0.0` | Adresse d'écoute         |
| `KILHOG_PORT` | `8080`    | Port d'écoute            |

## Format des réponses JSON

- Succès : `{"status": "success", "data": ...}`
- Erreur : `{"status": "error", "message": "...", "code": 400}`

## Build

```bash
make build
```

Compile le binaire dans `bin/kilhog` à partir de `./cmd/kilhog`.

## Run

```bash
go run ./cmd/kilhog
```
