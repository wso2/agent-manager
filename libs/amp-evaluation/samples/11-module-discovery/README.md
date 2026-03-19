# Module Discovery

Demonstrates `discover_evaluators()`, which scans a Python module for all `BaseEvaluator` instances. This is the recommended way to collect evaluators defined across a module without manually listing them.

## What it shows

- How `discover_evaluators()` finds evaluator instances in a module
- Three types of discoverable evaluators:
  - Class-based evaluators (instantiated at module level)
  - Decorator-based evaluators (`@evaluator` returns an instance)
  - Built-in evaluators (created via `builtin()`)
- What is NOT discovered:
  - Bare classes (not instantiated)
  - Non-evaluator objects (strings, numbers, lists)
- Using discovered evaluators with `Monitor`

## How discover_evaluators() works

```python
from amp_evaluation import discover_evaluators
import my_evaluators

evaluators = discover_evaluators(my_evaluators)
```

The function iterates over all attributes of the given module using `dir()` and checks if each attribute is an instance of `BaseEvaluator`. Only instances are returned.

| Object type | Discovered? | Reason |
|---|---|---|
| `OutputLengthCheck()` | Yes | Instantiated `BaseEvaluator` subclass |
| `@evaluator("name") def fn(...)` | Yes | Decorator returns `FunctionEvaluator` instance |
| `builtin("latency")` | Yes | Factory returns `BaseEvaluator` instance |
| `class NotDiscovered(BaseEvaluator)` | No | Class, not an instance |
| `helper_string = "text"` | No | Not a `BaseEvaluator` |

## Combining with other evaluators

```python
discovered = discover_evaluators(my_evaluators)
discovered.append(builtin("latency", max_latency_ms=5000))

monitor = Monitor(evaluators=discovered)
```

## How to run

```bash
pip install amp-evaluation
python run.py
```

## Expected output

```text
Discovered 3 evaluators:

  - has-tools            level=trace  modes=['experiment', 'monitor']
  - latency              level=trace  modes=['experiment', 'monitor']
  - output-length        level=trace  modes=['experiment', 'monitor']

Not discovered (by design):
  - NotDiscovered class:  not instantiated at module level
  - helper_string:        not a BaseEvaluator instance
  - helper_number:        not a BaseEvaluator instance
  - helper_list:          not a BaseEvaluator instance

Evaluation Run: run... (EvalMode.MONITOR)
  ...
Scores:
  has-tools:
    level: trace
    count: N
    mean: ...
  latency:
    level: trace
    count: N
    mean: ...
  output-length:
    level: trace
    count: N
    mean: ...
```
