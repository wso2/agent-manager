#!/bin/sh
# Renders AMP's ThunderID configuration into the image's install directory,
# then runs the one-time setup (--setup) or starts the server.
#
# Templates use @@NAME@@ placeholders (Thunder's own files use {{ }}, and
# its JavaScript uses ${}), filled from the environment variables below.
set -eu

THUNDER_HOME=/opt/thunderid
TEMPLATES=/amp/thunder

: "${THUNDER_HOST:?}" "${THUNDER_PORT:?}" "${THUNDER_PUBLIC_URL:?}"
: "${AMP_PUBLIC_URL:?}" "${OBSERVER_PUBLIC_URL:?}" "${CONSOLE_URL:?}"
: "${AMP_API_CLIENT_SECRET:?}" "${AMP_CONSOLE_CLIENT_SECRET:?}"
: "${AMP_PUBLISHER_CLIENT_SECRET:?}" "${AMP_SYSTEM_CLIENT_SECRET:?}"

render() {
    sed -e "s#@@THUNDER_HOST@@#${THUNDER_HOST}#g" \
        -e "s#@@THUNDER_PORT@@#${THUNDER_PORT}#g" \
        -e "s#@@THUNDER_PUBLIC_URL@@#${THUNDER_PUBLIC_URL}#g" \
        -e "s#@@AMP_PUBLIC_URL@@#${AMP_PUBLIC_URL}#g" \
        -e "s#@@OBSERVER_PUBLIC_URL@@#${OBSERVER_PUBLIC_URL}#g" \
        -e "s#@@CONSOLE_URL@@#${CONSOLE_URL}#g" \
        -e "s#@@AMP_API_CLIENT_SECRET@@#${AMP_API_CLIENT_SECRET}#g" \
        -e "s#@@AMP_CONSOLE_CLIENT_SECRET@@#${AMP_CONSOLE_CLIENT_SECRET}#g" \
        -e "s#@@AMP_PUBLISHER_CLIENT_SECRET@@#${AMP_PUBLISHER_CLIENT_SECRET}#g" \
        -e "s#@@AMP_SYSTEM_CLIENT_SECRET@@#${AMP_SYSTEM_CLIENT_SECRET}#g" \
        "$1" > "$2"
}

render "$TEMPLATES/conf/deployment.yaml" "$THUNDER_HOME/deployment.yaml"
render "$TEMPLATES/conf/gate-config.js" "$THUNDER_HOME/apps/gate/config.js"
render "$TEMPLATES/conf/console-config.js" "$THUNDER_HOME/apps/console/config.js"

cd "$THUNDER_HOME"
export BACKEND_PORT="$THUNDER_PORT"

if [ "${1:-}" = "--setup" ]; then
    marker="$THUNDER_HOME/database/.amp-bootstrapped"
    if [ -f "$marker" ]; then
        echo "ThunderID already bootstrapped; skipping setup."
        exit 0
    fi
    : "${ADMIN_PASSWORD:?ADMIN_PASSWORD must be set for the first setup}"
    for f in "$TEMPLATES"/bootstrap/*.yaml; do
        render "$f" "$THUNDER_HOME/bootstrap/$(basename "$f")"
    done
    ./setup.sh
    touch "$marker"
    exit 0
fi

exec ./start.sh
