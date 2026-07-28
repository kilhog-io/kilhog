#!/usr/bin/env bash
# Delete the "prod" network.

set -euo pipefail

export LANG="${LANG:-en_US.UTF-8}"
export LC_ALL="${LC_ALL:-en_US.UTF-8}"

base_url="${KILHOG_BASE_URL:-http://localhost:8080}"

uuid=$(curl -sS "${base_url}/networks" \
  | jq -er '.data[] | select(.name == "prod") | .uuid')

echo "Deleting network prod (${uuid})..."
curl -sSf -X DELETE "${base_url}/networks/${uuid}"
echo

echo "Done."
