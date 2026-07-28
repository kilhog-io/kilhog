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
│   ├── service/         # Logique métier (indépendante du HTTP)
│   ├── repository/      # Accès aux données / base de données
│   └── model/           # Modèles et structures de données
├── FUNCTIONAL.md        # Règles métier (définies par l'utilisateur)
└── TECHNICAL.md         # Ce fichier
```

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
