# kilhog

<p align="center">
  <img src="kilhog.png" alt="kilhog logo" width="200">
</p>

**kilhog** est une application **IPAM** (*IP Address Management*) : elle permet de gérer des pools et des adresses IP via une API REST.

Son nom reprend une traduction anglais–français–breton : *pool* → *poule* → **kilhog** (*coq* en breton) — un coq qui gère les poules, les pools d'adresses.

## Fonctionnalités

- Gestion de **networks** (périmètres de tenancy)
- Gestion de **subnets** IPv4 hiérarchiques (blocks CIDR et adresses hôtes)
- Validation CIDR : containment parent, détection d'overlap, allocation automatique d'adresses
- Persistance **SQLite** ou **PostgreSQL** avec migrations versionnées
- API REST JSON, prête pour multi-tenancy et RBAC

## Documentation

| Fichier | Contenu |
|---------|---------|
| [FUNCTIONAL.md](FUNCTIONAL.md) | Règles métier : entités, contraintes d'unicité, hiérarchie des subnets |
| [TECHNICAL.md](TECHNICAL.md) | Architecture, stack, schéma de base, routes API et exemples |

## Démarrage rapide

```bash
# Compiler et lancer le serveur (SQLite, port 8080)
make run-dev

# Dans un autre terminal : créer des networks et subnets d'exemple
make dev-create-networks
make dev-create-subnets
```

Vérifier que l'API répond :

```bash
curl http://localhost:8080/healthz
```

## Build

```bash
make build    # binaire dans bin/kilhog
go test ./... # lancer les tests
```

## Stack

- Go 1.26+
- API REST, architecture en couches (`handler` → `service` → `repository`)
- Drivers SQLite et PostgreSQL

Voir [TECHNICAL.md](TECHNICAL.md) pour le détail de la configuration, des routes et du schéma relationnel.
