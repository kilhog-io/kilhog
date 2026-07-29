#!/usr/bin/env bash
# Delete the "prod" network.

set -euo pipefail

export LANG="${LANG:-en_US.UTF-8}"
export LC_ALL="${LC_ALL:-en_US.UTF-8}"

base_url="${KILHOG_BASE_URL:-http://localhost:8080}"
auth_header=()
if [[ -n "${KILHOG_API_KEY:-}" ]]; then
  auth_header=(-H "Authorization: Bearer ${KILHOG_API_KEY}")
fi

uuid=$(curl -sS "${auth_header[@]}" "${base_url}/networks" \
  | jq -er '.data[] | select(.name == "prod") | .uuid')

echo "Deleting network prod (${uuid})..."
curl -sSf -X DELETE "${base_url}/networks/${uuid}" \
  "${auth_header[@]}"
echo

echo "Done."
