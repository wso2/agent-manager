#!/bin/sh
# Installs the ThunderID configuration that amp-init generated
# (agent-manager-service -init-thunder) into the image, then either runs
# ThunderID's setup (--setup) or starts the server.
#
# Setup runs whenever the generated bootstrap resources differ from the ones
# the last successful setup applied. Re-running it is additive: it creates or
# updates AMP's resources and keeps everything else in the database. It also
# sets the admin password to ADMIN_PASSWORD again. Additional users (users/)
# are imported only once each: ThunderID refuses to write a credential twice.
set -eu

THUNDER_HOME=/opt/thunderid
CONF=/amp/thunder-config
STAMP="$THUNDER_HOME/database/.amp-bootstrap.sha256"
CREATED_USERS="$THUNDER_HOME/database/.amp-users"

: "${THUNDER_PORT:?}"

# Rewritten in place: the image's deployment.yaml cannot be replaced, only written.
cat "$CONF/deployment.yaml" > "$THUNDER_HOME/deployment.yaml"
cat "$CONF/gate-config.js" > "$THUNDER_HOME/apps/gate/config.js"
cat "$CONF/console-config.js" > "$THUNDER_HOME/apps/console/config.js"

cd "$THUNDER_HOME"
export BACKEND_PORT="$THUNDER_PORT"

if [ "${1:-}" = "--setup" ]; then
    if [ -f "$STAMP" ] && cmp -s "$STAMP" "$CONF/bootstrap.sha256"; then
        echo "ThunderID bootstrap is up to date; skipping setup."
        exit 0
    fi
    : "${ADMIN_PASSWORD:?ADMIN_PASSWORD must be set to run the setup}"
    # A restarted container still holds the files of its last run; keep only
    # ThunderID's own (0*), so a user imported then is not imported again.
    find "$THUNDER_HOME/bootstrap" -type f ! -name '0*' -delete
    cp "$CONF"/bootstrap/*.yaml "$THUNDER_HOME/bootstrap/"
    mkdir -p "$CREATED_USERS"
    new_users=""
    for f in "$CONF"/users/*.yaml; do
        [ -e "$f" ] || continue
        name=$(basename "$f")
        [ -e "$CREATED_USERS/$name" ] && continue
        cp "$f" "$THUNDER_HOME/bootstrap/"
        new_users="$new_users $name"
    done
    ./setup.sh
    for name in $new_users; do
        touch "$CREATED_USERS/$name"
    done
    cp "$CONF/bootstrap.sha256" "$STAMP"
    exit 0
fi

exec ./start.sh
