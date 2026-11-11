# shellcheck shell=bash
# Shared installer for AMP's trace storage, sourced by deployments/setup
# (local dev) and deployments/quick-start/install.sh.
#
# Installs, from upstream charts, into OBSERVABILITY_NAMESPACE:
#   - OpenSearch                   (values/opensearch.yaml)
#   - OpenTelemetry Collector      (values/otel-collector.yaml)
# The Agent Manager Observer (wso2-amp-observability-extension) is installed
# separately, into the same namespace.
#
# TODO(amp-lite): no metrics store. The collector's span counter (metering) and
# agent CPU/memory metrics were scraped by the Prometheus that came with the
# previous observability stack; nothing scrapes them now.

OBSERVABILITY_NAMESPACE="${OBSERVABILITY_NAMESPACE:-amp-observability}"
OPENSEARCH_CHART_REPO="${OPENSEARCH_CHART_REPO:-https://opensearch-project.github.io/helm-charts/}"
OPENSEARCH_CHART_VERSION="${OPENSEARCH_CHART_VERSION:-3.6.0}"
OTEL_COLLECTOR_CHART_REPO="${OTEL_COLLECTOR_CHART_REPO:-https://open-telemetry.github.io/opentelemetry-helm-charts}"
OTEL_COLLECTOR_CHART_VERSION="${OTEL_COLLECTOR_CHART_VERSION:-0.178.2}"
OPENSEARCH_USERNAME="${OPENSEARCH_USERNAME:-admin}"
# OpenSearch rejects weak initial admin passwords; override for anything
# beyond a local install.
OPENSEARCH_PASSWORD="${OPENSEARCH_PASSWORD:-ThisIsTheOpenSearchPassword1!}"

# install_observability_stack <deployments-dir>
install_observability_stack() {
    local deployments_dir="$1"

    kubectl create namespace "${OBSERVABILITY_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f - >/dev/null || return 1

    # Read by OpenSearch (initial admin password), the collector and the observer.
    kubectl create secret generic opensearch-admin-credentials \
        -n "${OBSERVABILITY_NAMESPACE}" \
        --from-literal=username="${OPENSEARCH_USERNAME}" \
        --from-literal=password="${OPENSEARCH_PASSWORD}" \
        --dry-run=client -o yaml | kubectl apply -f - >/dev/null || return 1

    echo "📦 Installing OpenSearch (${OPENSEARCH_CHART_VERSION})..."
    helm upgrade --install opensearch opensearch \
        --repo "${OPENSEARCH_CHART_REPO}" \
        --version "${OPENSEARCH_CHART_VERSION}" \
        --namespace "${OBSERVABILITY_NAMESPACE}" \
        --values "${deployments_dir}/values/opensearch.yaml" \
        --wait --timeout 15m || return 1

    echo "📦 Installing OpenTelemetry Collector (${OTEL_COLLECTOR_CHART_VERSION})..."
    helm upgrade --install opentelemetry-collector opentelemetry-collector \
        --repo "${OTEL_COLLECTOR_CHART_REPO}" \
        --version "${OTEL_COLLECTOR_CHART_VERSION}" \
        --namespace "${OBSERVABILITY_NAMESPACE}" \
        --values "${deployments_dir}/values/otel-collector.yaml" \
        --wait --timeout 5m || return 1

    echo "✅ Trace storage ready in ${OBSERVABILITY_NAMESPACE}"
}
