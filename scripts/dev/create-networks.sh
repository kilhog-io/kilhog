#!/usr/bin/env bash
# Create sample networks for local development: "prod" and "hors-prod".

set -euo pipefail

export LANG="${LANG:-en_US.UTF-8}"
export LC_ALL="${LC_ALL:-en_US.UTF-8}"

base_url="${KILHOG_BASE_URL:-http://localhost:8080}"
dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Creating network prod..."
curl -sSf -X POST "${base_url}/networks" \
  -H "Content-Type: application/json; charset=utf-8" \
  --data-binary @"${dir}/network-prod.json"
echo

echo "Creating network hors-prod..."
curl -sSf -X POST "${base_url}/networks" \
  -H "Content-Type: application/json; charset=utf-8" \
  --data-binary @"${dir}/network-hors-prod.json"
echo

echo "Done."
