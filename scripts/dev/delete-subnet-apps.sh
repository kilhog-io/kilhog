#!/usr/bin/env bash
# Delete the "apps" subnet created by create-subnets.sh.

set -euo pipefail

export LANG="${LANG:-en_US.UTF-8}"
export LC_ALL="${LC_ALL:-en_US.UTF-8}"

base_url="${KILHOG_BASE_URL:-http://localhost:8080}"
dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

network_uuid="${NETWORK_UUID:-}"
if [[ -z "${network_uuid}" && -f "${dir}/network-hors-prod.uuid" ]]; then
  network_uuid="$(cat "${dir}/network-hors-prod.uuid")"
fi
if [[ -z "${network_uuid}" ]]; then
  echo "Network uuid not found. Run make dev-create-subnets first or set NETWORK_UUID." >&2
  exit 1
fi

subnet_uuid="${SUBNET_UUID:-}"
if [[ -z "${subnet_uuid}" && -f "${dir}/subnet-apps.uuid" ]]; then
  subnet_uuid="$(cat "${dir}/subnet-apps.uuid")"
fi
if [[ -z "${subnet_uuid}" ]]; then
  echo "Subnet apps uuid not found. Run make dev-create-subnets first or set SUBNET_UUID." >&2
  exit 1
fi

echo "Deleting subnet apps (${subnet_uuid})..."
curl -sSf -X DELETE "${base_url}/networks/${network_uuid}/subnets/${subnet_uuid}"
echo
echo "Done."
