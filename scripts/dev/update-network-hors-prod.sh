#!/usr/bin/env bash
# Update the "hors-prod" network using network-hors-prod-update.json.

set -euo pipefail

export LANG="${LANG:-en_US.UTF-8}"
export LC_ALL="${LC_ALL:-en_US.UTF-8}"

base_url="${KILHOG_BASE_URL:-http://localhost:8080}"
dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

uuid=$(curl -sS "${base_url}/networks" \
  | jq -er '.data[] | select(.name == "hors-prod") | .uuid')

echo "Updating network hors-prod (${uuid})..."
curl -sSf -X PUT "${base_url}/networks/${uuid}" \
  -H "Content-Type: application/json; charset=utf-8" \
  --data-binary @"${dir}/network-hors-prod-update.json"
echo

echo "Done."
