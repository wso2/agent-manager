# Evaluation Modes

Demonstrates evaluation mode auto-detection. The `task` parameter on the evaluator function determines when an evaluator can run.

## Mode auto-detection mechanism

| Evaluator signature                                | Supported modes          |
|----------------------------------------------------|--------------------------|
| `def evaluate(trace: Trace) -> EvalResult`         | monitor + experiment     |
| `def evaluate(trace: Trace, task: Task) -> ...`    | experiment only          |
| `def evaluate(trace: Trace, task: Optional[Task] = None) -> ...` | monitor + experiment |

- **Monitor mode**: Evaluates live production traces without ground truth. Evaluators that *require* a `Task` parameter are automatically skipped.
- **Experiment mode**: Evaluates traces against a dataset with ground truth. All evaluators run, and `task` is populated from the dataset.

## What it shows

- Three `@evaluator` functions with different `task` parameter signatures
- `run_monitor.py`: Monitor skips `experiment-only`, runs the other two
- `run_experiment.py`: Experiment runs all three evaluators

## How to run

```bash
pip install amp-evaluation

# Monitor mode (experiment-only evaluator is skipped)
python run_monitor.py

# Experiment mode (all evaluators run)
python run_experiment.py
```

## Expected output

**Monitor mode:**
```
Evaluators that ran: ['monitor-friendly', 'adaptive']
Note: 'experiment-only' was skipped because Monitor mode has no tasks.
```

**Experiment mode:**
```
Evaluators that ran: ['monitor-friendly', 'experiment-only', 'adaptive']
Note: ALL evaluators ran, including 'experiment-only'.
```
