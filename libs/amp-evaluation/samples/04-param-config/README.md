# Param Config

Demonstrates the `Param` descriptor for creating configurable evaluators with type validation, defaults, and runtime-inspectable schemas.

## Key concepts

### Param descriptor

`Param` is a Python descriptor that provides:
- Default values
- Type validation (inferred from class annotations or function type hints)
- Constraint validation (`min`, `max`, `enum`)
- Runtime introspection via `.info.config_schema`

### Two usage patterns

**Class-based** (on `BaseEvaluator` subclasses):
```python
class MyEvaluator(BaseEvaluator):
    threshold: float = Param(default=0.7, description="Pass threshold", min=0, max=1)
```

**Decorator-based** (on `@evaluator` functions):
```python
@evaluator("my-eval")
def my_eval(trace: Trace, threshold: float = Param(default=0.7)) -> EvalResult:
    ...
```

### with_config()

Create configured copies of decorator-based evaluators without redefining the function:
```python
strict_eval = my_eval.with_config(threshold=0.9)
```

### .info property

Every evaluator exposes its metadata and config schema via `.info`:
```python
info = my_eval.info
print(info.config_schema)
# [{'key': 'threshold', 'type': 'float', 'default': 0.7, ...}]
```

## How to run

```bash
pip install amp-evaluation
python run.py
```

## Expected output

Results show different scores for default vs. strict configurations on the same traces, demonstrating how Param values affect evaluation behavior.
