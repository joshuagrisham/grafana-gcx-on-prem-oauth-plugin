#!/usr/bin/env bash
# setup-test-orgs.sh -- provision a multi-org test environment for the
# gcx On-Prem OAuth plugin.
#
# Run this against a freshly-started Grafana instance (e.g. after
# `npm run server`). It uses the admin/admin Basic Auth credentials to:
#
#   - Create orgs: "Org Two" (orgId 2), "Org Three" (orgId 3).
#   - Create a non-admin test user `testuser` (password `testuser`).
#   - Assign that user different roles in each org:
#        org 1 -> Viewer
#        org 2 -> Editor
#        org 3 -> Admin
#   - Create an Organization-scoped service-account token in org 2 and
#     print it so you can paste it into provisioning/plugins/apps.yaml.
#
# Usage:
#   GRAFANA_URL=http://localhost:3000 ./scripts/setup-test-orgs.sh
#
# Idempotent: safe to re-run; existing orgs/users/tokens are reused.

set -euo pipefail

GRAFANA_URL="${GRAFANA_URL:-http://localhost:3000}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin}"
TEST_USER_LOGIN="${TEST_USER_LOGIN:-testuser}"
TEST_USER_PASS="${TEST_USER_PASS:-testuser}"

curl_admin() {
  curl -sS --fail-with-body -u "${ADMIN_USER}:${ADMIN_PASS}" \
    -H 'Content-Type: application/json' "$@"
}

# get_or_create_org NAME -> orgId on stdout
get_or_create_org() {
  local name="$1"
  local existing
  existing="$(curl_admin "${GRAFANA_URL}/api/orgs/name/${name// /%20}" 2>/dev/null || true)"
  if [[ -n "${existing}" && "${existing}" == *'"id"'* ]]; then
    echo "${existing}" | grep -o '"id":[0-9]*' | head -n1 | cut -d: -f2
    return
  fi
  curl_admin -X POST "${GRAFANA_URL}/api/orgs" -d "{\"name\":\"${name}\"}" \
    | grep -o '"orgId":[0-9]*' | cut -d: -f2
}

# ensure_user_with_role LOGIN PASS ORG_ID ROLE
ensure_user_with_role() {
  local login="$1" pass="$2" org_id="$3" role="$4"

  # Switch admin into the target org so the create-user endpoint scopes correctly.
  curl_admin -X POST "${GRAFANA_URL}/api/user/using/${org_id}" >/dev/null

  # Create user (ignore 412 if it already exists globally).
  curl_admin -X POST "${GRAFANA_URL}/api/admin/users" \
    -d "{\"name\":\"${login}\",\"login\":\"${login}\",\"email\":\"${login}@example.com\",\"password\":\"${pass}\"}" \
    >/dev/null 2>&1 || true

  # Add to org with the desired role (or update role if already a member).
  local resp
  resp="$(curl_admin -X POST "${GRAFANA_URL}/api/orgs/${org_id}/users" \
    -d "{\"loginOrEmail\":\"${login}\",\"role\":\"${role}\"}" 2>&1 || true)"
  if [[ "${resp}" == *'already added'* ]]; then
    local uid
    uid="$(curl_admin "${GRAFANA_URL}/api/users/lookup?loginOrEmail=${login}" | grep -o '"id":[0-9]*' | head -n1 | cut -d: -f2)"
    curl_admin -X PATCH "${GRAFANA_URL}/api/orgs/${org_id}/users/${uid}" \
      -d "{\"role\":\"${role}\"}" >/dev/null
  fi
}

# create_org_sa_token ORG_ID NAME ROLE -> prints the token key
create_org_sa_token() {
  local org_id="$1" name="$2" role="$3"
  curl_admin -X POST "${GRAFANA_URL}/api/user/using/${org_id}" >/dev/null

  local sa_id
  sa_id="$(curl_admin "${GRAFANA_URL}/api/serviceaccounts/search?query=${name}" \
    | grep -oE '"id":[0-9]+,"name":"'"${name}"'"' | head -n1 | grep -o '[0-9]*' | head -n1 || true)"
  if [[ -z "${sa_id}" ]]; then
    sa_id="$(curl_admin -X POST "${GRAFANA_URL}/api/serviceaccounts" \
      -d "{\"name\":\"${name}\",\"role\":\"${role}\"}" | grep -o '"id":[0-9]*' | head -n1 | cut -d: -f2)"
  fi
  curl_admin -X POST "${GRAFANA_URL}/api/serviceaccounts/${sa_id}/tokens" \
    -d "{\"name\":\"provisioned-$(date +%s)\"}" \
    | grep -o '"key":"[^"]*"' | cut -d'"' -f4
}

echo "Using Grafana at ${GRAFANA_URL}"

org2_id="$(get_or_create_org 'Org Two')"
org3_id="$(get_or_create_org 'Org Three')"
echo "Orgs: 1 (Main), ${org2_id} (Org Two), ${org3_id} (Org Three)"

ensure_user_with_role "${TEST_USER_LOGIN}" "${TEST_USER_PASS}" 1        "Viewer"
ensure_user_with_role "${TEST_USER_LOGIN}" "${TEST_USER_PASS}" "${org2_id}" "Editor"
ensure_user_with_role "${TEST_USER_LOGIN}" "${TEST_USER_PASS}" "${org3_id}" "Admin"
echo "Test user '${TEST_USER_LOGIN}' (password '${TEST_USER_PASS}'): Viewer/Editor/Admin across orgs."

org2_token="$(create_org_sa_token "${org2_id}" 'gcx-onprem-plugin-sa' 'Admin')"
echo
echo "Org-scoped service-account token for org ${org2_id} (paste into provisioning/plugins/apps.yaml):"
echo "  ${org2_token}"
echo
echo "Plugin instance for org ${org3_id} will fall back to Basic Auth -- make sure"
echo "GF_PLUGIN_JOSHUAGRISHAM_GCXONPREMOAUTH_APP_BACKEND_USERNAME / _PASSWORD are set"
echo "to a GrafanaAdmin user on the Grafana server process."
