#!/usr/bin/env bash
# standalone-ci-settings.sh — print a Compose overlay that fixes the test
# stack's client secrets and admin password, which the e2e suites, samples
# and instrumentation jobs know, instead of generating them. The values are
# the e2e suites' defaults (test/e2e/framework/config.go); change both.
#
# Usage (from deployments/standalone):
#   bash ../../.github/scripts/standalone-ci-settings.sh [file|idp] > ci-settings.yml
#   COMPOSE_FILE=docker-compose.yml:ci-settings.yml docker compose up -d --wait
#
# With a sign-in mode, the overlay also sets AUTH_MODE; the environment wins
# over amp.yaml. LOG_ROUTE_HITS=true in the environment turns on amp-api's
# route-hit log, which the e2e job reads to report route coverage.
# PRIVATE_EVENTS=true lets amp-api deliver events to private addresses and
# reach the host as host.docker.internal, for the e2e webhook sink.
set -euo pipefail
AUTH_MODE="${1:-}"
cat <<YAML
services:
  amp-init:
    environment: &ci
YAML
[ -n "$AUTH_MODE" ] && echo "      AUTH_MODE: ${AUTH_MODE}"
cat <<YAML
      IDP_CLIENT_SECRET: amp-api-client-secret
      AMP_CONSOLE_CLIENT_SECRET: amp-console-client-secret
      THUNDER_CLIENT_SECRET: amp-system-client-secret
      AMP_ADMIN_PASSWORD: admin
YAML
echo "  amp-api:"
echo "    environment:"
echo "      <<: *ci"
[ "${LOG_ROUTE_HITS:-false}" = "true" ] && echo '      LOG_ROUTE_HITS: "true"'
if [ "${PRIVATE_EVENTS:-false}" = "true" ]; then
  echo '      EVENTS_ALLOW_PRIVATE_DESTINATIONS: "true"'
  echo "    extra_hosts:"
  echo '      - "host.docker.internal:host-gateway"'
fi
true
