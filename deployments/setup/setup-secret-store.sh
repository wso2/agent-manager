#!/bin/bash
# Creates the in-house ClusterSecretStore (amp-secret-store) for the local
# stack, where agent-manager-service runs with docker compose outside the k3d
# cluster. External Secrets Operator reads agent manager secrets through it by
# calling the service's internal API on the host (host.k3d.internal:9243).
# Helm installs create the same store from the wso2-agent-manager chart.
# The webhook provider only reads Secrets labelled external-secrets.io/type=webhook.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CERT_FILE="${SCRIPT_DIR}/../../agent-manager-service/data/certs/cert.pem"
STORE_NAME="${AMP_SECRET_STORE_NAME:-amp-secret-store}"
STORE_NAMESPACE="${AMP_SECRET_STORE_NAMESPACE:-external-secrets}"
# The same per-checkout token docker compose gives agent-manager-service.
STORE_TOKEN="${SECRET_STORE_TOKEN:-$("${SCRIPT_DIR}/secret-store-token.sh")}"
INTERNAL_URL="${AMP_INTERNAL_URL:-https://host.k3d.internal:9243}"

echo "⏳ Waiting for the agent-manager-service internal certificate..."
for _ in $(seq 1 60); do
    [ -s "$CERT_FILE" ] && break
    sleep 3
done
if [ ! -s "$CERT_FILE" ]; then
    echo "✗ $CERT_FILE not found. Start the service first (make dev-up)." >&2
    exit 1
fi
if ! openssl x509 -in "$CERT_FILE" -noout -ext subjectAltName 2>/dev/null | grep -q 'host.k3d.internal'; then
    echo "✗ $CERT_FILE does not cover host.k3d.internal. Delete agent-manager-service/data/certs" >&2
    echo "  and restart the service (make dev-restart) so it generates a new one." >&2
    exit 1
fi

kubectl create secret generic "$STORE_NAME" -n "$STORE_NAMESPACE" \
    --from-literal=secret-store-token="$STORE_TOKEN" \
    --from-file=ca.crt="$CERT_FILE" \
    --dry-run=client -o yaml \
  | kubectl label --local -f - external-secrets.io/type=webhook -o yaml \
  | kubectl apply -f -

kubectl apply -f - <<YAML
apiVersion: external-secrets.io/v1
kind: ClusterSecretStore
metadata:
  name: ${STORE_NAME}
spec:
  provider:
    webhook:
      url: '${INTERNAL_URL}/api/internal/v1/secrets/{{ .remoteRef.key }}/{{ .remoteRef.property }}'
      method: GET
      headers:
        Authorization: 'Bearer {{ index .auth "secret-store-token" }}'
      result:
        jsonPath: \$.value
      secrets:
        - name: auth
          secretRef:
            name: ${STORE_NAME}
            namespace: ${STORE_NAMESPACE}
      caProvider:
        type: Secret
        name: ${STORE_NAME}
        key: ca.crt
        namespace: ${STORE_NAMESPACE}
YAML

echo "⏳ Waiting for ClusterSecretStore ${STORE_NAME} to become ready..."
if kubectl wait --for=condition=Ready "clustersecretstore/${STORE_NAME}" --timeout=60s >/dev/null 2>&1; then
    echo "✅ ClusterSecretStore ${STORE_NAME} is ready"
else
    echo "⚠ ClusterSecretStore ${STORE_NAME} is not ready yet:" >&2
    kubectl get clustersecretstore "$STORE_NAME" -o jsonpath='{.status.conditions}' >&2 || true
    echo >&2
fi
