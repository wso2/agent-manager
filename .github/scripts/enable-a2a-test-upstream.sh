#!/usr/bin/env bash
# Enables the test-only A2A_CARD_TEST_UPSTREAM_HOSTS override on the quick-start
# platform so the e2e suite can register external A2A agents hosted on the
# platform's own ingress. On the A2A card paths, listed hosts skip the
# public-host check and may carry an upstream credential over http; scheme,
# userinfo and port checks still apply.
#
# Usage: enable-a2a-test-upstream.sh <hostname>[,<hostname>...]
#
# The setting is patched onto the amp-api Deployment after install; the chart has
# no Helm value for it and it must never become a production setting. A later
# `helm upgrade` keeps it (Helm's three-way merge leaves fields it did not set),
# so on a long-lived cluster remove it explicitly:
#   kubectl set env deployment/amp-api -n wso2-amp A2A_CARD_TEST_UPSTREAM_HOSTS-
set -euo pipefail

if [ $# -ne 1 ] || [ -z "$1" ]; then
  echo "usage: $0 <hostname>[,<hostname>...]" >&2
  exit 1
fi

hosts="$1"
namespace="wso2-amp"
deployment="deployment/amp-api"

# Exported first: if enabling fails below, the external A2A specs still run and
# fail on the rejected card URL instead of skipping.
if [ -n "${GITHUB_ENV:-}" ]; then
  echo "A2A_CARD_TEST_UPSTREAM_HOSTS=${hosts}" >> "${GITHUB_ENV}"
fi

kubectl set env "${deployment}" -n "${namespace}" "A2A_CARD_TEST_UPSTREAM_HOSTS=${hosts}"
kubectl rollout status "${deployment}" -n "${namespace}" --timeout=300s

actual="$(kubectl exec "${deployment}" -n "${namespace}" -- printenv A2A_CARD_TEST_UPSTREAM_HOSTS || true)"
if [ "${actual}" != "${hosts}" ]; then
  echo "amp-api reports A2A_CARD_TEST_UPSTREAM_HOSTS='${actual}', expected '${hosts}'" >&2
  exit 1
fi
