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
| `address`     | yes      | Network or host address (ex. `192.168.1.0`, `192.168.1.42`). |
| `type`        | yes      | Address family : `ipv4` ou `ipv6`. |
| `parent`      | yes      | Reference to the subnet parent (voir ci-dessous). |
| `tags`        | no       | Liste de paires key–value (`key`, `value`). |

### Parent

Le champ `parent` indique la position du subnet dans la hiérarchie. Il peut référencer :

1. **Un network** — le subnet est un enfant direct du périmètre de tenancy.
2. **Un autre subnet** — le subnet est imbriqué dans un address space plus large.

Un subnet appartient toujours, directement ou indirectement, à un unique root network.

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
