# Experiment with Dataset

Demonstrates the `Experiment` runner, which evaluates agent traces against a ground-truth dataset. Shows two approaches: pre-loaded traces (offline) and real agent invocation (online).

## What it shows

- Creating evaluators that use ground truth (`task: Task` parameter)
- Creating adaptive evaluators that work in both modes (`task: Optional[Task] = None`)
- Loading datasets from JSON and CSV formats
- Running experiments with pre-loaded traces (no running agent needed)
- Running experiments with a real agent via `HttpAgentInvoker`
- Task-trace matching via `trace_id` / `task_id`

## Evaluator modes

The `task` parameter in the evaluator function signature controls mode compatibility:

| Signature | Modes | Description |
|---|---|---|
| `def evaluate(trace: Trace, task: Task)` | experiment only | Requires ground truth |
| `def evaluate(trace: Trace, task: Optional[Task] = None)` | experiment + monitor | Uses ground truth when available |
| `def evaluate(trace: Trace)` | experiment + monitor | No ground truth needed |

## Dataset formats

The framework supports two dataset formats:

**JSON** (`load_dataset_from_json`): Full-featured format with task metadata, expected outputs, success criteria, constraints, and expected trajectories.

**CSV** (`load_dataset_from_csv`): Simple tabular format. Requires `id` and `input` columns. Optional: `name`, `description`, `expected_output`, `success_criteria`.

## How to run

### Pre-loaded traces (no agent needed)

```bash
pip install amp-evaluation
python run_with_traces.py
```

### With a real agent

```bash
pip install amp-evaluation
# Start your agent on port 8000
python run_with_agent.py
```

## Task-trace matching

In the pre-loaded traces approach, you manually assign `trace.trace_id = task.task_id` to match traces to tasks.

In the real agent approach, the framework automatically propagates `task_id` and `trial_id` via OTEL baggage headers during agent invocation. The trace service captures these baggage values and returns them with the trace, enabling automatic matching.

## Expected output (run_with_traces.py)

```text
Loaded dataset: ... (N tasks)
  Task task_1: ...

Loaded N traces

Discovered evaluators: ['output-match', 'criteria-check', 'adaptive-check']
  output-match         modes=['experiment']
  criteria-check       modes=['experiment']
  adaptive-check       modes=['experiment', 'monitor']

Evaluation Run: run... (EvalMode.EXPERIMENT)
  ...
Scores:
  output-match:
    mean: ...
    count: N
  criteria-check:
    mean: ...
    count: N
  adaptive-check:
    mean: ...
    count: N

============================================================
CSV Dataset Loading
============================================================
Loaded CSV dataset: CSV Dataset (N tasks)
  Task 1: ...
```
