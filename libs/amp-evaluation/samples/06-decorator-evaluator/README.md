# Decorator-Based Evaluators

Shows how to create evaluators using the `@evaluator` decorator. This is a concise alternative to subclassing `BaseEvaluator`, ideal for simple evaluation logic.

## What it shows

- Creating evaluators with `@evaluator(name, description=..., tags=[...])`
- The decorated variable becomes a `FunctionEvaluator` instance (not a function)
- Evaluation level auto-detected from the first parameter type hint (`Trace`)
- Returning `EvalResult` with score, explanation, and details
- Automatic discovery via `discover_evaluators()` works identically to class-based evaluators

## Decorator syntax

```python
from amp_evaluation import evaluator, EvalResult
from amp_evaluation.trace import Trace

@evaluator("my-evaluator", description="...", tags=["..."])
def my_evaluator(trace: Trace) -> EvalResult:
    return EvalResult(score=1.0, explanation="...")
```

## Comparison with class-based

| Feature                | `@evaluator` decorator       | `BaseEvaluator` subclass     |
|-----------------------|------------------------------|------------------------------|
| Lines of code         | Fewer                        | More                         |
| Config via `Param()`  | Supported (as default args)  | Supported (as class attrs)   |
| Custom `run()` logic  | Not supported                | Supported                    |
| Best for              | Simple evaluations           | Complex logic, custom config |

Both approaches produce `BaseEvaluator` instances that are fully interchangeable in `Monitor`, `Experiment`, and `discover_evaluators()`.

## How to run

```bash
pip install amp-evaluation
python run.py
```

## Expected output

```
Discovered evaluators: ['error-free', 'has-output', 'tool-usage']
  error-free: Checks trace has no errors (level=trace, tags=['reliability'])
  has-output: Checks if agent produced output (level=trace, tags=['basic'])
  tool-usage: Checks if tools were called (level=trace, tags=['tool-use'])

Evaluation Run: run... (EvalMode.MONITOR)
  ...
Scores:
  error-free:
    level: trace
    count: N
    mean: ...
  has-output:
    level: trace
    count: N
    mean: ...
  tool-usage:
    level: trace
    count: N
    mean: ...
```
