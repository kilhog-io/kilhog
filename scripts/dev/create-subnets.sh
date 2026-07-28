#!/usr/bin/env bash
# Create sample subnets under the "hors-prod" network.

set -euo pipefail

export LANG="${LANG:-en_US.UTF-8}"
export LC_ALL="${LC_ALL:-en_US.UTF-8}"

base_url="${KILHOG_BASE_URL:-http://localhost:8080}"
dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

network_uuid="$(curl -sSf "${base_url}/networks" | jq -r '.data[] | select(.name == "hors-prod") | .uuid')"
if [[ -z "${network_uuid}" || "${network_uuid}" == "null" ]]; then
  echo "Network hors-prod not found. Run make dev-create-networks first." >&2
  exit 1
fi
echo "${network_uuid}" > "${dir}/network-hors-prod.uuid"

echo "Creating subnet dmz..."
dmz_response="$(curl -sSf -X POST "${base_url}/networks/${network_uuid}/subnets" \
  -H "Content-Type: application/json; charset=utf-8" \
  --data-binary @"${dir}/subnet-dmz.json")"
echo "${dmz_response}"
dmz_uuid="$(echo "${dmz_response}" | jq -r '.data.uuid')"
echo "${dmz_uuid}" > "${dir}/subnet-dmz.uuid"

echo "Creating subnet apps (auto address under dmz)..."
apps_response="$(curl -sSf -X POST "${base_url}/networks/${network_uuid}/subnets/${dmz_uuid}/subnets" \
  -H "Content-Type: application/json; charset=utf-8" \
  --data-binary @"${dir}/subnet-apps-auto.json")"
echo "${apps_response}"
apps_uuid="$(echo "${apps_response}" | jq -r '.data.uuid')"
echo "${apps_uuid}" > "${dir}/subnet-apps.uuid"

echo "Done."
