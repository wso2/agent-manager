# shellcheck shell=bash
# Util: Check if a port is in use
#
# On Linux a published container port is held by a root-owned docker-proxy,
# which lsof hides from an unprivileged caller — the port reads as free and the
# stack only fails much later, at bind time. ss reads /proc/net/tcp and sees
# every listener regardless of owner. macOS ships no ss, but there the listener
# belongs to the invoking user, so lsof is accurate.
is_port_in_use() {
    local port="$1"
    if command -v ss &> /dev/null; then
        [ -n "$(ss -ltnH "sport = :$port" 2>/dev/null)" ]
        return
    fi
    lsof -i :"$port" -sTCP:LISTEN &> /dev/null
}

# Util: Name the Docker containers publishing a port, so a conflict report says
# what to stop instead of only which port is taken.
containers_publishing_port() {
    docker ps --filter "publish=$1" --format '{{.Names}}' 2>/dev/null \
        | tr '\n' ' ' \
        | sed 's/ *$//'
}

# Util: The command that shows which process owns a listening port, per platform.
port_owner_command() {
    if command -v ss &> /dev/null; then
        echo "ss -ltnp 'sport = :<port>'"
    else
        echo "lsof -i :<port>"
    fi
}

# Util: Get minimum required version for a command (bash 3.x compatible)
get_min_version() {
    case "$1" in
        docker)  echo "26" ;;
        go)      echo "1.25" ;;
        *)       echo "" ;;
    esac
}

# Util: check version is greater than or equal by comparing two version strings (returns 0 if $1 >= $2)
version_gte() {
    local ver1="$1"
    local ver2="$2"
    # Use sort -V for version comparison
    [ "$(printf '%s\n%s' "$ver2" "$ver1" | sort -V | head -n1)" = "$ver2" ]
}

# Util: Extract version number from command output
get_version() {
    "$1" version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+' | head -1
}

# Util: Suggest how to install a missing command on the current platform.
install_hint() {
    if [ "$(uname -s)" = "Darwin" ]; then
        echo "brew install $1"
    else
        echo "install '$1' with your distribution's package manager"
    fi
}

# Util: Check if a command is installed and version is compatible
check_command() {
    local cmd="$1"
    if ! command -v "$cmd" &> /dev/null; then
        echo "❌ $cmd is not installed. Please install it first:"
        echo "   $(install_hint "$cmd")"
        exit 1
    fi

    # Check version compatibility
    local min_version
    min_version=$(get_min_version "$cmd")
    if [ -n "$min_version" ]; then
        local current_version
        current_version=$(get_version "$cmd")

        if [ -n "$current_version" ]; then
            if ! version_gte "$current_version" "$min_version"; then
                echo "⚠️  Warning: $cmd version $current_version is below minimum required v$min_version+"
            fi
        fi
    fi
}
