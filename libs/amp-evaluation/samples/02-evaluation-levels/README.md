# Evaluation Levels

Demonstrates evaluation level auto-detection. The first parameter's type hint on the `evaluate()` method controls which level the evaluator operates at.

## Level auto-detection mechanism

| Type hint on first parameter | Evaluation level | When `evaluate()` is called |
|------------------------------|------------------|-----------------------------|
| `trace: Trace`               | trace            | Once per trace              |
| `agent: AgentTrace`          | agent            | Once per agent span         |
| `llm: LLMSpan`              | llm              | Once per LLM call           |

The framework inspects the type annotation at registration time and dispatches accordingly during evaluation. No explicit level configuration is needed.

## What it shows

- Three `BaseEvaluator` subclasses, each typed at a different level
- How `discover_evaluators()` picks up module-level instances
- That the **count** in evaluation results differs by level (trace=1 per trace, agent=N, llm=M)

## How to run

```bash
pip install amp-evaluation
python run.py
```

## Expected output

```
Loaded N traces

  trace-level-check           level=trace
  agent-level-check           level=agent
  llm-level-check             level=llm

Results by evaluator:
------------------------------------------------------------
  trace-level-check:
    level: trace
    count: N          <-- once per trace

  agent-level-check:
    level: agent
    count: X          <-- once per agent span (X >= N)

  llm-level-check:
    level: llm
    count: Y          <-- once per LLM call (Y >= X)
```
