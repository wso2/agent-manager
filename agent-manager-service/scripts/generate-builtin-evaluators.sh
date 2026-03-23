#!/bin/bash
# ============================================================================
# Generate Builtin Evaluators Go Catalog
# ============================================================================
# This script generates catalog/builtin_evaluators.go from the amp-evaluation
# library. The generated Go file is compiled directly into the service binary —
# no database or JSON file is needed at runtime.
#
# Usage:
#   ./generate-builtin-evaluators.sh [options]
#
# Options:
#   --amp-eval-version  Version of amp-evaluation to install from PyPI (default: latest)
#   --output            Output file path (default: ./catalog/builtin_evaluators.go)
#   --dev               Use local source from libs/amp-evaluation (for development)
#
# Examples:
#   # Development (uses local source)
#   ./generate-builtin-evaluators.sh --dev
#
#   # Production (installs from PyPI)
#   ./generate-builtin-evaluators.sh --amp-eval-version 0.1.0
# ============================================================================

set -euo pipefail

# Default configuration
AMP_EVAL_VERSION="${AMP_EVAL_VERSION:-}"
OUTPUT_FILE="${OUTPUT_FILE:-./catalog/builtin_evaluators.go}"
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

log_info "Generating builtin evaluators Go catalog..."

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
        pip install --quiet -e "${LOCAL_AMP_EVAL}[litellm]"
    else
        log_error "Local amp-evaluation not found at ${LOCAL_AMP_EVAL}"
        log_error "Run from repository root or use --amp-eval-version for production"
        exit 1
    fi
elif [[ -n "${AMP_EVAL_VERSION}" ]]; then
    log_info "Installing amp-evaluation==${AMP_EVAL_VERSION} from PyPI..."
    pip install --quiet "amp-evaluation[litellm]==${AMP_EVAL_VERSION}"
else
    log_info "Installing latest amp-evaluation from PyPI..."
    pip install --quiet "amp-evaluation[litellm]"
fi

# Generate the Go source file
python3 << 'PYTHON_SCRIPT' > "${OUTPUT_FILE}"
import json
import sys
import inspect
import importlib
import re
import textwrap
from amp_evaluation.evaluators.builtin import builtin_evaluator_catalog
from amp_evaluation.evaluators.base import BaseEvaluator, LLMAsJudgeEvaluator

evaluators = builtin_evaluator_catalog(mode="monitor")

# Build a map from evaluator name to its class for source extraction
_evaluator_classes = {}
for module_name in ["standard", "llm_judge", "deepeval"]:
    try:
        mod = importlib.import_module(f"amp_evaluation.evaluators.builtin.{module_name}")
        for attr_name, obj in inspect.getmembers(mod, inspect.isclass):
            if issubclass(obj, BaseEvaluator) and obj is not BaseEvaluator:
                name = getattr(obj, "name", None)
                if name:
                    _evaluator_classes[name] = obj
    except ImportError:
        pass

def _extract_prompt_template(cls):
    """Extract the prompt template from an LLM judge's build_prompt method.

    Returns just the prompt text with f-string placeholders, stripping all
    Python method scaffolding (def, return, conditional pre-prompt logic).
    Conditional sections are inlined as always-present placeholders.
    """
    import ast, re

    try:
        raw = inspect.getsource(cls.build_prompt)
        # inspect.getsource keeps the class-level indentation; dedent it
        source = textwrap.dedent(raw)
    except (OSError, TypeError):
        return ""

    # If still indented (e.g. method inside a class), strip common leading whitespace
    lines = source.splitlines(True)
    if lines and lines[0][0] == ' ':
        import re as _re
        leading = _re.match(r'^(\s+)', lines[0])
        if leading:
            prefix = leading.group(1)
            source = ''.join(
                line[len(prefix):] if line.startswith(prefix) else line
                for line in lines
            )

    tree = ast.parse(source)
    func = tree.body[0]  # the def build_prompt(...) node

    # Collect all f-string nodes from return statements.
    # Also track variable assignments to handle `prompt = f"""..."""; return prompt` patterns.
    fstrings = []
    assigned_fstrings = {}  # var_name -> JoinedStr node
    for node in ast.walk(func):
        if isinstance(node, ast.Assign):
            if (
                isinstance(node.value, ast.JoinedStr)
                and node.targets
                and isinstance(node.targets[0], ast.Name)
            ):
                assigned_fstrings[node.targets[0].id] = node.value
        elif isinstance(node, ast.Return) and node.value is not None:
            if isinstance(node.value, ast.JoinedStr):
                fstrings.append(node.value)
            elif isinstance(node.value, ast.Name):
                # `return prompt` — look up the assigned f-string
                if node.value.id in assigned_fstrings:
                    fstrings.append(assigned_fstrings[node.value.id])

    if not fstrings:
        # Fallback: return empty if we can't find the f-string
        return ""

    # We need to reconstruct the template from the original source lines.
    # Strategy: find the triple-quoted f-string in the raw source and extract it.

    # Find all triple-quoted f-strings in the source
    # Pattern: f"""..."""  (triple double quotes)
    fstring_pattern = re.compile(r'f"""(.*?)"""', re.DOTALL)
    matches = list(fstring_pattern.finditer(source))

    if not matches:
        return ""

    # Use the first/main f-string (the prompt template)
    template = matches[0].group(1)

    # Now handle conditional sections that prepend/append to the prompt.
    # Look for patterns like:
    #   variable = f"..." if condition else ""
    # followed by {variable} in the template.

    def _decode_py_escapes(s):
        """Convert Python string escape sequences (e.g. \\n) to real characters."""
        return s.replace('\\n', '\n').replace('\\t', '\t').replace('\\r', '\r')

    # Find conditional assignments: var = f"..." if ... else ""
    cond_pattern = re.compile(
        r'(\w+)\s*=\s*f"([^"]*?)"\s+if\s+.*?\s+else\s+""\s*$',
        re.MULTILINE,
    )
    for m in cond_pattern.finditer(source):
        var_name = m.group(1)
        raw_value = m.group(2)
        # Skip conditionals whose value is entirely self.* driven — these are
        # config-level params already captured in ConfigSchema; inlining them
        # would produce an empty or misleading placeholder.
        if re.search(r'\{self\.', raw_value) and not re.search(r'\{task\.', raw_value):
            template = template.replace("{" + var_name + "}", "")
            continue
        value = _decode_py_escapes(raw_value)
        # Guard task-dependent expressions so they degrade gracefully at runtime
        if 'task.' in value:
            guarded = re.sub(
                r'\{(task\.\w+)\}',
                lambda inner: '{' + inner.group(1) + ' if task and ' + inner.group(1) + ' else ""}',
                value,
            )
            template = template.replace("{" + var_name + "}", guarded)
        else:
            template = template.replace("{" + var_name + "}", value)

    # For error_recovery: handle multi-line pre-prompt code that builds variables
    # used as {error_summary} etc. — keep the placeholder as-is (it's a runtime
    # placeholder like {trace.input}).
    # No action needed — these are already {error_summary} in the template.

    # Strip remaining {self.*} expressions — these reference the evaluator class
    # instance and are not available in the custom template context.
    template = re.sub(r'\{self\.[^}]+\}', '', template)

    return template


def _validate_template_placeholders(template, evaluator_name):
    """Validate that all {…} placeholders in a template are valid Python expressions.

    Logs warnings for any that fail parsing — catches regressions when builtin
    evaluator prompts use patterns that the template engine cannot evaluate.
    """
    import ast as _ast
    placeholder_re = re.compile(r'\{([^}]+)\}')
    for match in placeholder_re.finditer(template):
        expr = match.group(1).strip()
        try:
            _ast.parse(expr, mode='eval')
        except SyntaxError:
            print(
                f"WARNING: [{evaluator_name}] placeholder is not a valid Python expression: {{{expr}}}",
                file=sys.stderr,
            )


def get_evaluator_type_and_source(ev_name):
    """Extract type ('code' or 'llm_judge') and prompt template for a builtin evaluator."""
    cls = _evaluator_classes.get(ev_name)
    if cls is None:
        return "", ""
    if issubclass(cls, LLMAsJudgeEvaluator):
        ev_type = "llm_judge"
        source = _extract_prompt_template(cls)
        _validate_template_placeholders(source, ev_name)
    else:
        ev_type = "code"
        try:
            source = textwrap.dedent(inspect.getsource(cls.evaluate))
        except (OSError, TypeError):
            source = ""
    return ev_type, source

def to_display_name(identifier):
    """Derive a human-readable display name from an evaluator identifier."""
    # Take the part after the last "/" for namespaced identifiers
    base = identifier.split("/")[-1]
    return base.replace("_", " ").replace("-", " ").title()

def go_value(v):
    """Format a Python value as a Go literal for the interface{} Default field."""
    if v is None:
        return "nil"
    elif isinstance(v, bool):
        return "true" if v else "false"
    elif isinstance(v, int):
        return f"float64({v})"
    elif isinstance(v, float):
        # Format without trailing zeros but keep precision
        formatted = f"{v}"
        return f"float64({formatted})"
    elif isinstance(v, str):
        return json.dumps(v)  # adds quotes and escapes special chars
    elif isinstance(v, list):
        items = ", ".join(json.dumps(item) if isinstance(item, str) else go_value(item) for item in v)
        return f"[]interface{{}}{{{items}}}"
    else:
        return "nil"

def go_strings(lst):
    """Format a list of strings as a Go []string literal."""
    if not lst:
        return "nil"
    items = ", ".join(json.dumps(s) for s in lst)
    return f"[]string{{{items}}}"

def go_float_ptr(v):
    """Format a float as floatPtr(v)."""
    if v is None:
        return ""
    formatted = f"{v}"
    return f"floatPtr({formatted})"

lines = [
    "// Code generated by scripts/generate-builtin-evaluators.sh; DO NOT EDIT.",
    "",
    "package catalog",
    "",
    'import "github.com/wso2/ai-agent-management-platform/agent-manager-service/models"',
    "",
    "var entries = []*Entry{",
]

for ev in evaluators:
    identifier = ev.name
    display_name = to_display_name(identifier)
    description = ev.description or ""
    version = ev.version or "1.0"
    provider = ev.module or ""
    class_name = ev.class_name or ""
    level = ev.level or "trace"
    tags = ev.tags or []
    config_schema = ev.config_schema or []
    ev_type, ev_source = get_evaluator_type_and_source(identifier)

    lines.append("\t{")
    lines.append(f"\t\tIdentifier:  {json.dumps(identifier)},")
    lines.append(f"\t\tDisplayName: {json.dumps(display_name)},")
    lines.append(f"\t\tDescription: {json.dumps(description)},")
    lines.append(f"\t\tVersion:     {json.dumps(version)},")
    lines.append(f"\t\tProvider:    {json.dumps(provider)},")
    lines.append(f"\t\tClassName:   {json.dumps(class_name)},")
    lines.append(f"\t\tLevel:       {json.dumps(level)},")
    lines.append(f"\t\tTags:        {go_strings(tags)},")
    if ev_type:
        lines.append(f"\t\tType:        {json.dumps(ev_type)},")
    if ev_source:
        lines.append(f"\t\tSource:      {json.dumps(ev_source)},")

    if config_schema:
        lines.append("\t\tConfigSchema: []models.EvaluatorConfigParam{")
        for param in config_schema:
            key = param.get("key", "")
            ptype = param.get("type", "string")
            desc = param.get("description", "")
            required = "true" if param.get("required", False) else "false"
            default_val = go_value(param.get("default", None))
            min_val = param.get("min", None)
            max_val = param.get("max", None)
            enum_values = param.get("enum_values", [])

            field_parts = [
                f"Key: {json.dumps(key)}",
                f"Type: {json.dumps(ptype)}",
                f"Description: {json.dumps(desc)}",
                f"Required: {required}",
            ]
            if default_val != "nil":
                field_parts.append(f"Default: {default_val}")
            else:
                field_parts.append("Default: nil")
            if min_val is not None:
                field_parts.append(f"Min: {go_float_ptr(min_val)}")
            if max_val is not None:
                field_parts.append(f"Max: {go_float_ptr(max_val)}")
            if enum_values:
                field_parts.append(f"EnumValues: {go_strings(enum_values)}")

            lines.append("\t\t\t{" + ", ".join(field_parts) + "},")
        lines.append("\t\t},")
    else:
        lines.append("\t\tConfigSchema: []models.EvaluatorConfigParam{},")

    lines.append("\t},")

lines.append("}")
print("\n".join(lines))
PYTHON_SCRIPT

EVALUATOR_COUNT=$(python3 -c "
import sys
sys.path.insert(0, '${VENV_DIR}/lib/python3.*/site-packages' if False else '.')
count = 0
with open('${OUTPUT_FILE}') as f:
    for line in f:
        if line.strip().startswith('Identifier:'):
            count += 1
print(count)
")

log_success "Generated ${EVALUATOR_COUNT} evaluators to ${OUTPUT_FILE}"

# Generate the LLM provider catalog Go source file
LLM_PROVIDERS_FILE="$(dirname "${OUTPUT_FILE}")/builtin_llm_providers.go"
log_info "Generating builtin LLM provider catalog..."

python3 << 'PYTHON_SCRIPT' > "${LLM_PROVIDERS_FILE}"
import json
from amp_evaluation.evaluators.builtin import get_llm_provider_catalog

providers = get_llm_provider_catalog()

lines = [
    "// Code generated by scripts/generate-builtin-evaluators.sh; DO NOT EDIT.",
    "",
    "package catalog",
    "",
    "var llmProviderEntries = []*LLMProviderEntry{",
]

for p in providers:
    lines.append("\t{")
    lines.append(f"\t\tName:        {json.dumps(p.name)},")
    lines.append(f"\t\tDisplayName: {json.dumps(p.display_name)},")
    lines.append("\t\tConfigFields: []LLMConfigField{")
    for f in p.config_fields:
        required = "true" if f.required else "false"
        lines.append(
            f"\t\t\t{{Key: {json.dumps(f.key)}, Label: {json.dumps(f.label)}, "
            f"FieldType: {json.dumps(f.field_type)}, Required: {required}, EnvVar: {json.dumps(f.env_var)}}},"
        )
    lines.append("\t\t},")
    if p.models:
        model_items = ", ".join(json.dumps(m) for m in p.models)
        lines.append(f"\t\tModels: []string{{{model_items}}},")
    else:
        lines.append("\t\tModels: nil,")
    lines.append("\t},")

lines.append("}")
print("\n".join(lines))
PYTHON_SCRIPT

PROVIDER_COUNT=$(python3 -c "
count = 0
with open('${LLM_PROVIDERS_FILE}') as f:
    for line in f:
        if line.strip().startswith('Name:'):
            count += 1
print(count)
")

log_success "Generated ${PROVIDER_COUNT} LLM providers to ${LLM_PROVIDERS_FILE}"

# Generate the LLM judge base config schema Go source file
LLM_JUDGE_BASE_FILE="$(dirname "${OUTPUT_FILE}")/../models/llm_judge_base_config.generated.go"
mkdir -p "$(dirname "${LLM_JUDGE_BASE_FILE}")"
log_info "Generating LLM judge base config schema..."

python3 << 'PYTHON_SCRIPT' > "${LLM_JUDGE_BASE_FILE}"
import json
from amp_evaluation.evaluators.base import LLMAsJudgeEvaluator

def is_param_descriptor(obj):
    """Duck-type check for _ParamDescriptor to avoid aliasing issues."""
    return (
        not callable(obj)
        and not isinstance(obj, (str, int, float, bool, type, frozenset))
        and hasattr(obj, "to_schema")
        and callable(getattr(obj, "to_schema", None))
        and hasattr(obj, "_attr_name")
    )

def go_value(v):
    if v is None:
        return "nil"
    elif isinstance(v, bool):
        return "true" if v else "false"
    elif isinstance(v, int):
        return f"float64({v})"
    elif isinstance(v, float):
        return f"float64({v})"
    elif isinstance(v, str):
        return json.dumps(v)
    return "nil"

def go_float_ptr(v):
    if v is None:
        return ""
    return f"fp({v})"

base_params = []
for attr_name, attr in vars(LLMAsJudgeEvaluator).items():
    if is_param_descriptor(attr):
        attr._attr_name = attr_name
        base_params.append(attr.to_schema())

lines = [
    "// Code generated by scripts/generate-builtin-evaluators.sh; DO NOT EDIT.",
    "",
    "package models",
    "",
    "func fp(v float64) *float64 { return &v }",
    "",
    "// LLMJudgeBaseConfigSchema contains the Param descriptors inherited by all",
    "// LLMAsJudgeEvaluator subclasses. Generated from the amp-evaluation Python class.",
    "var LLMJudgeBaseConfigSchema = []EvaluatorConfigParam{",
]

for param in base_params:
    key = param.get("key", "")
    ptype = param.get("type", "string")
    desc = param.get("description", "")
    required = "true" if param.get("required", False) else "false"
    default_val = go_value(param.get("default", None))
    min_val = param.get("min", None)
    max_val = param.get("max", None)
    enum_values = param.get("enum_values", [])

    field_parts = [
        f"Key: {json.dumps(key)}",
        f"Type: {json.dumps(ptype)}",
        f"Description: {json.dumps(desc)}",
        f"Required: {required}",
        f"Default: {default_val}",
    ]
    if min_val is not None:
        field_parts.append(f"Min: {go_float_ptr(min_val)}")
    if max_val is not None:
        field_parts.append(f"Max: {go_float_ptr(max_val)}")
    if enum_values:
        items = ", ".join(json.dumps(s) for s in enum_values)
        field_parts.append(f"EnumValues: []string{{{items}}}")

    lines.append("\t{" + ", ".join(field_parts) + "},")

lines.append("}")
print("\n".join(lines))
PYTHON_SCRIPT

log_success "Generated LLM judge base config ($(python3 -c "
count = 0
with open('${LLM_JUDGE_BASE_FILE}') as f:
    for line in f:
        if line.strip().startswith('Key:'):
            count += 1
print(count)
") params) to ${LLM_JUDGE_BASE_FILE}"
