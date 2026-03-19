# Built-in Evaluators

Showcase of the evaluators that ship with amp-evaluation. No custom `evaluators.py` needed -- this sample uses only built-in evaluators created via the `builtin()` factory.

## What it shows

- `list_builtin_evaluators()` to discover available evaluators
- `list_builtin_evaluators(mode="monitor")` to filter by evaluation mode
- `builtin_evaluator_catalog()` to get full metadata (name, level, modes, config schema)
- `builtin(name, **config)` factory to create configured instances
- Running built-in evaluators with `Monitor`

> **Note:** Run the sample or use the catalog API to see the current list of built-in evaluators.
> The catalog is the source of truth — it stays up to date as evaluators are added or removed.

## Usage

```python
from amp_evaluation import builtin, list_builtin_evaluators, builtin_evaluator_catalog

# Discover available evaluators
all_names = list_builtin_evaluators()
monitor_names = list_builtin_evaluators(mode="monitor")

# Get full metadata
for info in builtin_evaluator_catalog():
    print(info.name, info.level, info.modes, info.config_schema)

# Create instances with the builtin() factory
latency = builtin("latency", max_latency_ms=5000)
tokens = builtin("token_efficiency", max_tokens=8000)
length = builtin("answer_length", min_length=10, max_length=5000)
```

## How to run

```bash
pip install amp-evaluation
python run.py
```
