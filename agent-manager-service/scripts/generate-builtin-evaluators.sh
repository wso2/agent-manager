#!/bin/bash
# ============================================================================
# Generate Builtin Evaluators JSON
# ============================================================================
# This script generates builtin_evaluators.json from the amp-evaluation library.
# The JSON file will be used by the agent-manager-service during DB migration.
#
# Usage:
#   ./generate-builtin-evaluators.sh [options]
#
# Options:
#   --amp-eval-version  Version of amp-evaluation to install from PyPI (default: latest)
#   --output            Output file path (default: ./data/builtin_evaluators.json)
#   --dev               Use local source from libs/amp-evaluation (for development)
#
# Examples:
#   # Development (uses local source)
#   ./generate-builtin-evaluators.sh --dev --output ./data/builtin_evaluators.json
#
#   # Production (installs from PyPI)
#   ./generate-builtin-evaluators.sh --amp-eval-version 0.1.0 --output ./data/builtin_evaluators.json
# ============================================================================

set -euo pipefail

# Default configuration
AMP_EVAL_VERSION="${AMP_EVAL_VERSION:-}"
OUTPUT_FILE="${OUTPUT_FILE:-./data/builtin_evaluators.json}"
DEV_MODE=false

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --amp-eval-version)
            AMP_EVAL_VERSION="$2"
            shift 2
            ;;
        --output)
            OUTPUT_FILE="$2"
            shift 2
            ;;
        --dev)
            DEV_MODE=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${NC}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

log_error() {
    echo -e "${RED}✗ $1${NC}"
}

# Check for Python
if ! command -v python3 &> /dev/null; then
    log_error "python3 is required but not installed"
    exit 1
fi

log_info "Generating builtin evaluators JSON..."

# Create output directory if needed
OUTPUT_DIR=$(dirname "${OUTPUT_FILE}")
mkdir -p "${OUTPUT_DIR}"

# Create temporary virtual environment
VENV_DIR=$(mktemp -d)/venv
trap 'deactivate 2>/dev/null || true; rm -rf "$(dirname "${VENV_DIR}")"' EXIT

python3 -m venv "${VENV_DIR}"
source "${VENV_DIR}/bin/activate"

pip install --quiet --upgrade pip

if [[ "${DEV_MODE}" == "true" ]]; then
    # Development mode: use local source
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    LOCAL_AMP_EVAL="${SCRIPT_DIR}/../../libs/amp-evaluation"
    
    if [[ -d "${LOCAL_AMP_EVAL}" ]]; then
        log_info "Installing amp-evaluation from local source (dev mode)..."
        pip install --quiet -e "${LOCAL_AMP_EVAL}"
    else
        log_error "Local amp-evaluation not found at ${LOCAL_AMP_EVAL}"
        log_error "Run from repository root or use --amp-eval-version for production"
        exit 1
    fi
elif [[ -n "${AMP_EVAL_VERSION}" ]]; then
    log_info "Installing amp-evaluation==${AMP_EVAL_VERSION} from PyPI..."
    pip install --quiet "amp-evaluation==${AMP_EVAL_VERSION}"
else
    log_info "Installing latest amp-evaluation from PyPI..."
    pip install --quiet amp-evaluation
fi

# Generate the JSON file
python3 << 'PYTHON_SCRIPT' > "${OUTPUT_FILE}"
import json
from amp_evaluation.evaluators.builtin import list_builtin_evaluators

evaluators = list_builtin_evaluators()
print(json.dumps(evaluators, indent=2, default=str))
PYTHON_SCRIPT

EVALUATOR_COUNT=$(python3 -c "import json; print(len(json.load(open('${OUTPUT_FILE}'))))")
log_success "Generated ${EVALUATOR_COUNT} evaluators to ${OUTPUT_FILE}"
