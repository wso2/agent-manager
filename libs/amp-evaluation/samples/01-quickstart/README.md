# Quickstart

The simplest possible amp-evaluation example. Defines one custom evaluator with the `@evaluator` decorator, adds a built-in latency evaluator, and runs them in **Monitor** mode on sample traces.

## What it shows

- Creating a custom evaluator with the `@evaluator` decorator
- Using `discover_evaluators()` to auto-find evaluator instances in a module
- Adding a built-in evaluator with `builtin("latency")`
- Running `Monitor` with pre-loaded traces
- Printing the evaluation summary

## How to run

```bash
pip install amp-evaluation
python run.py
```

## Expected output

```
Loaded N traces

Discovered evaluators: ['response-check']
All evaluators: ['response-check', 'latency']

Evaluation Run: run... (EvalMode.MONITOR)
  ...
Scores:
  response-check:
    mean: ...
    pass_rate: ...
    count: N
  latency:
    mean: ...
    pass_rate: ...
    count: N
```
