# DeepEval Evaluators

Demonstrates all 6 DeepEval agent evaluation metrics available through the `builtin()` factory. These evaluators use LLM-as-judge under the hood and require an OpenAI API key.

## DeepEval evaluators

All DeepEval evaluators are **experiment-only** (they require task data / ground truth).

| Evaluator name | Layer | What it measures |
|---|---|---|
| `deepeval/plan-quality` | Reasoning | Is the agent's plan logical, complete, and efficient? |
| `deepeval/plan-adherence` | Reasoning | Does the agent follow its own plan during execution? |
| `deepeval/tool-correctness` | Action | Does the agent select the correct tools? |
| `deepeval/argument-correctness` | Action | Are the arguments passed to tools correct? |
| `deepeval/task-completion` | Execution | Did the agent complete the intended task? |
| `deepeval/step-efficiency` | Execution | Did the agent complete the task without unnecessary steps? |

## Configuration options

All DeepEval evaluators share these common configuration parameters:

| Parameter | Default | Description |
|---|---|---|
| `threshold` | `0.7` | Minimum score for passing (0.0-1.0) |
| `model` | `"gpt-4o"` | LLM model for evaluation |
| `include_reason` | `True` | Include reasoning in the result |
| `strict_mode` | `False` | Binary scoring (0 or 1) |

Additionally, `deepeval/tool-correctness` supports:

| Parameter | Default | Description |
|---|---|---|
| `evaluate_input` | `False` | Also check input arguments match |
| `evaluate_output` | `False` | Also check outputs match |
| `evaluate_order` | `False` | Enforce call sequence |
| `exact_match` | `False` | Require exact match of tools called vs expected |

## Prerequisites

1. Install the deepeval extra:
   ```bash
   pip install amp-evaluation[deepeval]
   ```

2. Set the OpenAI API key:
   ```bash
   export OPENAI_API_KEY=your-key-here
   ```

## How to run

```bash
python run.py
```

## Expected output

```text
Loading DeepEval evaluators...
  Loaded: deepeval/plan-quality
  Loaded: deepeval/plan-adherence
  Loaded: deepeval/tool-correctness
  Loaded: deepeval/argument-correctness
  Loaded: deepeval/task-completion
  Loaded: deepeval/step-efficiency

Loaded 6 DeepEval evaluators
Loaded dataset: ... (N tasks)
Loaded N traces

Evaluation Run: run... (EvalMode.EXPERIMENT)
  ...
Scores:
  deepeval/plan-quality:
    level: trace
    count: N
    mean: ...
  ...
```
