# FUNCTIONAL — kilhog

## Présentation

**kilhog** est une application **IPAM** (*IP Address Management*) : elle permet de gérer des pools et des adresses IP.

Son nom reprend une traduction anglais–français–breton :

| Langue   | Mot    |
|----------|--------|
| Anglais  | pool   |
| Français | poule  |
| Breton   | kilhog |

En breton, **kilhog** signifie *coq* : un coq qui gère les poules (les pools).

## Modèle unifié : le subnet

Dans kilhog, **tout espace d'adressage ou adresse IP est modélisé comme un subnet**.

- Un CIDR block (ex. `192.168.1.0/24`) est un subnet.
- Une adresse IP individuelle (ex. `192.168.1.42`) est également un subnet, avec un prefix **`/32`** (IPv4) ou **`/128`** (IPv6).

Il n'existe pas d'entité `IP` distincte du subnet : une IP est un **leaf subnet**.

## Entité : Network

Un **network** apporte une notion de **tenancy** (périmètre d'isolation logique). C'est le conteneur racine au sein duquel s'organisent les subnets.

### Attributes

| Attribute     | Required | Description |
|---------------|----------|-------------|
| `uuid`        | yes      | Unique identifier. Clé de liaison avec le reste du système. Unique dans toute la database. |
| `name`        | yes      | Display name. Unique dans toute la database. |
| `description` | no       | Texte libre descriptif. |
| `tags`        | no       | Liste de paires key–value (`key`, `value`). |

## Entité : Subnet

Un **subnet** représente un espace d'adressage IP (block ou adresse unique).

### Attributes

| Attribute     | Required | Description |
|---------------|----------|-------------|
| `uuid`        | yes      | Unique identifier. Clé de liaison avec le reste du système. Unique dans toute la database. |
| `name`        | yes      | Display name. Unique au sein du network (tenancy) auquel il appartient. |
| `description` | no       | Texte libre descriptif. |
| `prefix`      | yes      | Prefix length (ex. `24` pour un `/24`). |
| `address`     | conditional | Network or host address (ex. `192.168.1.0`, `192.168.1.42`). **Required** when the parent is a network. Optional when the parent is a subnet (auto-generated if absent). |
| `type`        | yes      | Address family : `ipv4` ou `ipv6`. |
| `parent`      | yes      | Reference to the subnet parent (voir ci-dessous). |
| `tags`        | no       | Liste de paires key–value (`key`, `value`). |

### Parent

Le champ `parent` indique la position du subnet dans la hiérarchie. Il peut référencer :

1. **Un network** — le subnet est un enfant direct du périmètre de tenancy.
2. **Un autre subnet** — le subnet est imbriqué dans un address space plus large.

Un subnet appartient toujours, directement ou indirectement, à un unique root network.

### Création

Lors de la création d'un subnet :

- Si le parent est un **network**, le champ `address` est **obligatoire**.
- Si le parent est un **subnet**, `address` est optionnel : s'il est absent, une adresse est générée automatiquement dans le CIDR du parent, sans overlap avec les siblings.

### Méthode `CIDR`

Chaque subnet expose une méthode **`CIDR`** qui retourne la notation CIDR en concaténant l'address et le prefix :

```
{address}/{prefix}
```

Exemples :

- `address = 192.168.1.0`, `prefix = 24` → `192.168.1.0/24`
- `address = 192.168.1.42`, `prefix = 32` → `192.168.1.42/32`
- `address = 2001:db8::1`, `prefix = 128` → `2001:db8::1/128`

## Tags

Les tags sont des metadata libres sous forme de paires **key–value** attachées à un network ou à un subnet.

- Une key peut apparaître une seule fois par resource.
- La value est une chaîne de caractères.

## Uniqueness rules

| Resource | Field  | Uniqueness scope |
|----------|--------|------------------|
| Network  | `uuid` | Database         |
| Network  | `name` | Database         |
| Subnet   | `uuid` | Database         |
| Subnet   | `name` | Network (tenancy)|

## Persistance

Les données métier (networks, subnets, tags) sont persistées dans une **base de données relationnelle**. La couche de persistance est **abstraite** : l'application supporte plusieurs moteurs sans changer le modèle métier ni les règles ci-dessus.

### Moteurs supportés

| Moteur     | Usage typique                          |
|------------|----------------------------------------|
| SQLite     | Développement local, déploiement léger |
| PostgreSQL | Production, multi-instances            |

Le choix du moteur est une **configuration de déploiement**, pas une règle métier. Les contraintes d'unicité et de hiérarchie s'appliquent de la même manière quel que soit le backend.

### Création automatique de la base

Si la base de données cible **n'existe pas encore**, l'application peut la **créer au démarrage** avant d'exécuter les migrations :

- **SQLite** : création du fichier et des répertoires parents manquants.
- **PostgreSQL** : création de la base via une connexion au catalogue `postgres` (ou équivalent).

Si la base existe déjà, l'application s'y connecte sans la recréer.

### Migrations SQL versionnées

Le schéma de la base est géré par des **migrations SQL numérotées**. Chaque version possède deux scripts :

- **upgrade** — applique les changements vers la version suivante ;
- **downgrade** — annule ces changements et revient à la version précédente.

Règles :

- Les migrations s'exécutent **dans l'ordre croissant** des numéros de version.
- Une version déjà appliquée n'est **jamais rejouée**.
- Au démarrage, l'application applique automatiquement les migrations **upgrade** manquantes.
- Le **downgrade** est disponible pour revenir en arrière (opération explicite, pas automatique au démarrage).

### Intégrité des données persistées

Les règles métier suivantes sont **garanties par le schéma** (contraintes SQL) :

| Règle | Mécanisme |
|-------|-----------|
| Unicité globale du `uuid` et du `name` d'un network | Contrainte `UNIQUE` |
| Unicité globale du `uuid` d'un subnet | Clé primaire |
| Unicité du `name` d'un subnet au sein d'un network | Contrainte `UNIQUE (network, name)` |
| Un subnet appartient toujours à un network (tenancy) | Clé étrangère `network_uuid` |
| Un tag a une seule value par key et par resource | Clé primaire `(resource, key)` |
| Suppression d'un network | Supprime en cascade ses subnets et tags associés |

Le détail des tables et colonnes est décrit dans `TECHNICAL.md`.
