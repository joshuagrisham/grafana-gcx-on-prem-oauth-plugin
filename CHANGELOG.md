# Changelog

## 0.1.1 (2026-05-29)

Upgrade to latest Grafana Plugin Go SDK and fix various dependency and linting issues as a result.

## 0.1.0 (2026-05-29)

Initial preview release of the gcx On-Prem OAuth Grafana app plugin.

### Features

- Per-user service account token management UI under `/a/joshuagrisham-gcxonpremoauth-app/tokens`,
  allowing each Grafana user to create and revoke their own API tokens without administrator
  involvement.
- "Client setup" page that provides support for users to set up various client tools.
- OAuth-style authorization page (`/authorize`) that mints a one-shot token and redirects back to
  a loopback `callback_port`, enabling browser-based login flows for CLI tools such as `gcx`.
- Backend cleanup process that prunes expired tokens, deletes orphaned service accounts when the
  owning user is removed or disabled, and syncs role changes from the user to the service account.
- Two authentication modes for the plugin's backend to talk to the Grafana API:
  - Per-organization service account token configured via the plugin settings page
  - Basic Auth with a `GrafanaAdmin` user's credentials set in environment variables
- Configurable limits via environment variables: maximum tokens per user, maximum token TTL,
  cleanup interval, and cleanup grace period.

### Security

- Token operations are authorized server-side by verifying the target token belongs to the calling
  user's service account before forwarding the request to the Grafana API.
- Token names are validated for length and basic shape before being forwarded to Grafana.
- The per-user service account naming convention (`user:<login>`) ensures tokens cannot leak
  across users.
