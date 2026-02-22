# Built-in Evaluators

Comprehensive showcase of the evaluators that ship with amp-evaluation. No custom `evaluators.py` needed -- this sample uses only built-in evaluators created via the `builtin()` factory.

## What it shows

- `list_builtin_evaluators()` to discover available evaluators
- `list_builtin_evaluators(mode="monitor")` to filter by evaluation mode
- `builtin_evaluator_catalog()` to get full metadata for each evaluator
- `builtin(name, **config)` factory to create configured instances
- Running built-in evaluators with `Monitor`

## Built-in evaluator catalog

### Standard evaluators (rule-based, no external dependencies)

| Name                | Level | Modes              | Description                                         |
|--------------------|-------|---------------------|-----------------------------------------------------|
| `answer_length`     | trace | experiment, monitor | Validates output character length within bounds      |
| `answer_relevancy`  | trace | experiment, monitor | Measures relevancy via word overlap analysis         |
| `required_content`  | trace | experiment, monitor | Ensures output contains required strings/patterns    |
| `prohibited_content`| trace | experiment, monitor | Detects prohibited strings/patterns in output        |
| `exact_match`       | trace | experiment          | Validates output exactly matches expected output     |
| `contains_match`    | trace | experiment          | Validates expected output is contained in output     |
| `tool_sequence`     | trace | experiment, monitor | Validates tools were called in expected order        |
| `required_tools`    | trace | experiment, monitor | Ensures all required tools were used                 |
| `step_success_rate` | trace | experiment, monitor | Measures percentage of spans completed without errors|
| `latency`           | trace | experiment, monitor | Validates execution time meets latency constraints   |
| `token_efficiency`  | trace | experiment, monitor | Validates token usage within configured limits       |
| `iteration_count`   | trace | experiment, monitor | Validates agent completes within iteration limit     |
| `hallucination`     | trace | experiment, monitor | Detects potential hallucinations via keyword patterns |

### DeepEval evaluators (require `deepeval` package + LLM API key)

| Name                          | Level | Modes      | Description                                          |
|-------------------------------|-------|------------|------------------------------------------------------|
| `deepeval/plan-quality`       | trace | experiment | Assesses agent's plan quality                        |
| `deepeval/plan-adherence`     | trace | experiment | Measures plan adherence during execution             |
| `deepeval/tool-correctness`   | trace | experiment | Validates correct tool selection                     |
| `deepeval/argument-correctness`| trace | experiment | Validates tool argument correctness                  |
| `deepeval/task-completion`    | trace | experiment | Measures task completion success                     |
| `deepeval/step-efficiency`    | trace | experiment | Assesses execution step efficiency                   |

## Using the `builtin()` factory

```python
from amp_evaluation import builtin

# With default config
latency = builtin("latency")

# With custom config
latency = builtin("latency", max_latency_ms=5000)
tokens = builtin("token_efficiency", max_tokens=8000)
length = builtin("answer_length", min_length=10, max_length=5000)
```

## Catalog API

```python
from amp_evaluation import list_builtin_evaluators, builtin_evaluator_catalog

# List names
all_names = list_builtin_evaluators()
monitor_names = list_builtin_evaluators(mode="monitor")

# Full metadata
catalog = builtin_evaluator_catalog()
for info in catalog:
    print(info.name, info.level, info.modes, info.config_schema)
```

## How to run

```bash
pip install amp-evaluation
python run.py
```

## Expected output

```
=== All Built-in Evaluators ===
  - answer_length
  - answer_relevancy
  - ...

=== Monitor-Compatible Built-ins ===
  - answer_length
  - ...

=== Evaluator Catalog ===
  answer_length
    Description: Validates that output character length falls within configured bounds
    Level: trace
    Modes: ['experiment', 'monitor']
    Config: [...]
  ...

=== Running Monitor with Built-in Evaluators ===

Created 6 built-in evaluators:
  - latency
  - token_efficiency
  - iteration_count
  - answer_length
  - answer_relevancy
  - hallucination

Loaded N traces

Evaluation Run: run-... (EvalMode.MONITOR)
  ...
Scores:
  latency:
    mean: ...
    count: N
  ...
```
