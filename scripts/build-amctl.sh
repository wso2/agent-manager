#!/bin/sh
set -eu

VERSION="dev"
COMMIT=""
DATE=""
OUTPUT_DIR="dist"
SINGLE_TARGET=false

LDFLAGS_PKG="github.com/wso2/agent-manager/cli/pkg/version"

TARGETS="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64"

usage() {
    cat <<USAGE
Usage: $0 [OPTIONS]

Cross-compile, package, and checksum the amctl CLI binary.

Options:
  --version VERSION     Version string (default: dev)
  --commit SHA          Git commit (default: current HEAD short SHA)
  --date DATE           Build date (default: now in RFC3339)
  --output-dir DIR      Output directory for archives and checksums (default: dist/)
  --single-target       Build only for the current GOOS/GOARCH
  -h, --help            Show this help
USAGE
}

while [ $# -gt 0 ]; do
    case "$1" in
        --version)    VERSION="$2"; shift 2 ;;
        --commit)     COMMIT="$2"; shift 2 ;;
        --date)       DATE="$2"; shift 2 ;;
        --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
        --single-target) SINGLE_TARGET=true; shift ;;
        -h|--help)    usage; exit 0 ;;
        *)            echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
    esac
done

REPO_ROOT="$(git rev-parse --show-toplevel)"
[ -z "$COMMIT" ] && COMMIT="$(git rev-parse --short HEAD)"
[ -z "$DATE" ] && DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

if [ "$SINGLE_TARGET" = true ]; then
    TARGETS="$(go env GOOS)/$(go env GOARCH)"
fi

mkdir -p "$OUTPUT_DIR"
OUTPUT_DIR="$(cd "$OUTPUT_DIR" && pwd)"

echo "==> Building amctl v${VERSION} (commit ${COMMIT}, ${DATE})"
echo "==> Targets: ${TARGETS}"

BUILD_TMPDIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_TMPDIR"' EXIT

cd "${REPO_ROOT}/cli"
go mod tidy

for target in $TARGETS; do
    os="${target%/*}"
    arch="${target#*/}"
    echo "==> Compiling ${os}/${arch}..."

    staging="${BUILD_TMPDIR}/${os}_${arch}"
    mkdir -p "$staging"

    bin_name="amctl"
    [ "$os" = "windows" ] && bin_name="amctl.exe"

    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
        go build -o "${staging}/${bin_name}" \
        -ldflags "-s -w \
            -X ${LDFLAGS_PKG}.Version=${VERSION} \
            -X ${LDFLAGS_PKG}.Commit=${COMMIT} \
            -X ${LDFLAGS_PKG}.Date=${DATE}" \
        ./cmd/amctl

    cp "${REPO_ROOT}/LICENSE" "${staging}/LICENSE"

    archive_base="amctl_v${VERSION}_${os}_${arch}"
    if [ "$os" = "windows" ]; then
        (cd "$staging" && zip -q "${OUTPUT_DIR}/${archive_base}.zip" "$bin_name" LICENSE)
    else
        (cd "$staging" && tar -czf "${OUTPUT_DIR}/${archive_base}.tar.gz" "$bin_name" LICENSE)
    fi
done

echo "==> Generating checksums..."
if command -v sha256sum >/dev/null 2>&1; then
    (cd "$OUTPUT_DIR" && sha256sum amctl_v* > checksums.txt)
elif command -v shasum >/dev/null 2>&1; then
    (cd "$OUTPUT_DIR" && shasum -a 256 amctl_v* > checksums.txt)
else
    echo "Error: neither sha256sum nor shasum found" >&2
    exit 1
fi

count=$(find "$OUTPUT_DIR" -maxdepth 1 -name 'amctl_v*' | wc -l | tr -d ' ')
echo "==> Done: ${count} archives + checksums.txt in ${OUTPUT_DIR}/"
