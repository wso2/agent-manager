#!/usr/bin/env bash
# Enables the test-only A2A_CARD_TEST_UPSTREAM_HOSTS override on the quick-start
# platform so the e2e suite can register external A2A agents hosted on the
# platform's own ingress. Hosts listed there bypass the A2A card URL guards.
#
# Usage: enable-a2a-test-upstream.sh <hostname>[,<hostname>...]
#
# The setting is patched onto the amp-api Deployment after install. A later
# `helm upgrade` reverts it, and that is deliberate: the chart has no Helm value
# for this variable and it must never become a production setting.
set -euo pipefail

if [ $# -ne 1 ] || [ -z "$1" ]; then
  echo "usage: $0 <hostname>[,<hostname>...]" >&2
  exit 1
fi

hosts="$1"
namespace="wso2-amp"
deployment="deployment/amp-api"

kubectl set env "${deployment}" -n "${namespace}" "A2A_CARD_TEST_UPSTREAM_HOSTS=${hosts}"
kubectl rollout status "${deployment}" -n "${namespace}" --timeout=300s

actual="$(kubectl exec "${deployment}" -n "${namespace}" -- printenv A2A_CARD_TEST_UPSTREAM_HOSTS || true)"
if [ "${actual}" != "${hosts}" ]; then
  echo "amp-api reports A2A_CARD_TEST_UPSTREAM_HOSTS='${actual}', expected '${hosts}'" >&2
  exit 1
fi

if [ -n "${GITHUB_ENV:-}" ]; then
  echo "A2A_CARD_TEST_UPSTREAM_HOSTS=${hosts}" >> "${GITHUB_ENV}"
fi
