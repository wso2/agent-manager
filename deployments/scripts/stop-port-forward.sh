#!/bin/bash
# Kills kubectl port-forward processes for AMP namespaces only.

AMP_NS="amp-thunder|openchoreo-observability-plane|amp-secrets|openbao|openchoreo-control-plane|openchoreo-data-plane"

pkill -f "port-forward.*($AMP_NS)" 2>/dev/null \
    && echo "✅ Stopped port-forward processes" \
    || echo "ℹ️  No active port-forwards found"
