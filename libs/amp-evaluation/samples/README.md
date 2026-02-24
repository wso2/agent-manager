# AMP Evaluation Samples

Self-contained examples demonstrating all capabilities of the `amp-evaluation` framework.
Each sample focuses on one concept and can be understood independently.

## Quick Start

```bash
# From the amp-evaluation root directory
pip install -e .

# Run any sample
cd samples/01-quickstart
pip install -r requirements.txt
python run.py
```

## Samples

| # | Sample | What It Demonstrates | Runs Offline? |
|---|--------|---------------------|---------------|
| 01 | [quickstart](01-quickstart/) | Minimal evaluator + monitor with file-based traces | Yes |
| 02 | [evaluation-levels](02-evaluation-levels/) | Auto-detection: Trace vs Agent vs LLM level from type hints | Yes |
| 03 | [evaluation-modes](03-evaluation-modes/) | Auto-detection: experiment-only vs monitor vs both from task param | Yes |
| 04 | [param-config](04-param-config/) | Configurable evaluators with `Param` descriptors and `with_config()` | Yes |
| 05 | [class-based-evaluator](05-class-based-evaluator/) | `BaseEvaluator` subclassing pattern | Yes |
| 06 | [decorator-evaluator](06-decorator-evaluator/) | `@evaluator` decorator pattern | Yes |
| 07 | [llm-as-judge](07-llm-as-judge/) | LLM-based evaluation with `LLMAsJudgeEvaluator` and `@llm_judge` | Needs LLM API key |
| 08 | [builtin-evaluators](08-builtin-evaluators/) | All 13+ built-in evaluators and catalog API | Yes |
| 09 | [deepeval-evaluators](09-deepeval-evaluators/) | DeepEval agent evaluation metrics | Needs deepeval + API key |
| 10 | [experiment-with-dataset](10-experiment-with-dataset/) | Experiment runner with JSON/CSV datasets | Yes (pre-loaded traces) |
| 11 | [module-discovery](11-module-discovery/) | `discover_evaluators()` automatic module scanning | Yes |
| 12 | [api-monitoring](12-api-monitoring/) | Production monitoring with `TraceFetcher` API | Needs trace service |

## Data Files

Shared sample data in [`data/`](data/):

| File | Description |
|------|-------------|
| `sample_traces.json` | Real OTEL trace data from a LangGraph agent workflow (19 spans) |
| `sample_dataset.json` | Customer support dataset with 5 tasks, ground truth, and expected trajectories |
| `simple_dataset.csv` | Simple CSV dataset with 5 rows |
