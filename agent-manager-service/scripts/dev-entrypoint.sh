#!/bin/sh
set -eu

: "${IDP_CLIENT_SECRET:?Set IDP_CLIENT_SECRET to match the installed Thunder API client}"
: "${THUNDER_CLIENT_SECRET:?Set THUNDER_CLIENT_SECRET to match the installed Thunder system client}"

keys_dir="${JWT_SIGNING_KEYS_DIR:-/app/keys}"
if [ ! -w "$keys_dir" ] || { [ -f "$keys_dir/private.pem" ] && [ ! -r "$keys_dir/private.pem" ]; }; then
    echo "Cannot access the signing keys in $keys_dir. Start the stack with make dev-up, which fixes the volume ownership, or see deployments/docker-compose.yml." >&2
    exit 1
fi

bash /app/scripts/gen_keys.sh
# Air needs a writable path outside the source mount on a fresh checkout.
build_dir=/tmp/agent-manager-air
exec air -c .air.toml -tmp_dir "$build_dir" \
    -build.cmd "go build -mod=readonly -o $build_dir/agent-manager-service ." \
    -build.bin "$build_dir/agent-manager-service"
