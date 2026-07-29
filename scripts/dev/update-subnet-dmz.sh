#!/usr/bin/env bash
# Update the "dmz" subnet description under "hors-prod".

set -euo pipefail

export LANG="${LANG:-en_US.UTF-8}"
export LC_ALL="${LC_ALL:-en_US.UTF-8}"

base_url="${KILHOG_BASE_URL:-http://localhost:8080}"
dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
auth_header=()
if [[ -n "${KILHOG_API_KEY:-}" ]]; then
  auth_header=(-H "Authorization: Bearer ${KILHOG_API_KEY}")
fi

network_uuid="${NETWORK_UUID:-}"
if [[ -z "${network_uuid}" && -f "${dir}/network-hors-prod.uuid" ]]; then
  network_uuid="$(cat "${dir}/network-hors-prod.uuid")"
fi
if [[ -z "${network_uuid}" ]]; then
  echo "Network uuid not found. Run make dev-create-subnets first or set NETWORK_UUID." >&2
  exit 1
fi

subnet_uuid="${SUBNET_UUID:-}"
if [[ -z "${subnet_uuid}" && -f "${dir}/subnet-dmz.uuid" ]]; then
  subnet_uuid="$(cat "${dir}/subnet-dmz.uuid")"
fi
if [[ -z "${subnet_uuid}" ]]; then
  echo "Subnet dmz uuid not found. Run make dev-create-subnets first or set SUBNET_UUID." >&2
  exit 1
fi

echo "Updating subnet dmz (${subnet_uuid})..."
curl -sSf -X PUT "${base_url}/networks/${network_uuid}/subnets/${subnet_uuid}" \
  "${auth_header[@]}" \
  -H "Content-Type: application/json; charset=utf-8" \
  --data-binary @"${dir}/subnet-dmz-update.json"
echo
echo "Done."
