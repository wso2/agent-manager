# Class-Based Evaluators

Shows how to create custom evaluators by subclassing `BaseEvaluator`. This is the most flexible approach, giving you full control over metadata, configuration, and evaluation logic.

## What it shows

- Subclassing `BaseEvaluator` and implementing `evaluate()`
- **Trace-level** evaluator: first parameter typed as `Trace` (called once per trace)
- **Agent-level** evaluator: first parameter typed as `AgentTrace` (called once per agent span)
- **LLM-level** evaluator: first parameter typed as `LLMSpan` (called once per LLM call)
- Setting `name`, `description`, and `tags` class attributes for metadata
- Using `EvalResult` with `score`, `passed`, `explanation`, and `details`
- Using `EvalResult.skip()` to skip non-applicable evaluations
- Instantiating evaluators at module level for `discover_evaluators()` discovery

## Key concepts

### Evaluation levels (auto-detected from type hint)

| First parameter type | Level   | Called...                  |
|---------------------|---------|----------------------------|
| `Trace`             | trace   | Once per trace             |
| `AgentTrace`        | agent   | Once per agent span        |
| `LLMSpan`           | llm     | Once per LLM call          |

### EvalResult fields

| Field         | Type              | Description                          |
|--------------|-------------------|--------------------------------------|
| `score`      | `float`           | 0.0 to 1.0 (required)               |
| `passed`     | `Optional[bool]`  | Override pass/fail (default: score >= 0.5) |
| `explanation`| `str`             | Human-readable explanation           |
| `details`    | `Optional[dict]`  | Additional structured data           |

### EvalResult.skip()

Use `EvalResult.skip(reason)` when an evaluation cannot be performed (e.g., missing data, inapplicable scenario). Skipped results are tracked separately and do not affect aggregate scores.

## How to run

```bash
pip install amp-evaluation
python run.py
```

## Expected output

```text
Loaded N traces

Discovered evaluators: ['agent-error-check', 'llm-output-check', 'response-completeness']
  agent-error-check (level=agent, modes=['experiment', 'monitor'])
  llm-output-check (level=llm, modes=['experiment', 'monitor'])
  response-completeness (level=trace, modes=['experiment', 'monitor'])

Evaluation Run: run-... (EvalMode.MONITOR)
  ...
Scores:
  response-completeness:
    mean: ...
    pass_rate: ...
    count: N
  agent-error-check:
    mean: ...
    pass_rate: ...
    count: N
  llm-output-check:
    mean: ...
    pass_rate: ...
    count: N
```
