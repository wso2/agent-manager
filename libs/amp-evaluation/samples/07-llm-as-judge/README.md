# LLM-as-Judge Evaluators

Shows how to use LLMs to evaluate agent outputs. Covers both class-based (`LLMAsJudgeEvaluator`) and decorator-based (`@llm_judge`) approaches, at multiple evaluation levels.

## What it shows

- **Class-based** `LLMAsJudgeEvaluator` with `build_prompt()` method
- **Trace-level** judge: `build_prompt(self, trace: Trace, task=None) -> str`
- **Agent-level** judge: `build_prompt(self, agent: AgentTrace) -> str`
- **`@llm_judge` decorator** (simple): just return a prompt string
- **`@llm_judge` decorator** with config: `@llm_judge(model="gpt-4o", criteria="...")`
- **Custom LLM client**: override `_call_llm_with_retry()` to use your own client
- Level and mode are auto-detected from `build_prompt()` type hints (same mechanism as `evaluate()`)

## How it works

1. You write `build_prompt()` -- return the evaluation prompt as a string
2. The framework appends output format instructions automatically (JSON with `score` and `explanation`)
3. The LLM is called via LiteLLM with JSON mode enabled
4. The response is validated with a Pydantic `JudgeOutput` model (`score: float`, `explanation: str`)
5. On invalid output, the framework retries with the Pydantic error as context (like Instructor)
6. The validated output becomes an `EvalResult`

## Prerequisites

```bash
pip install amp-evaluation litellm
export OPENAI_API_KEY=sk-...  # or another LLM provider key
```

LiteLLM supports many providers (OpenAI, Anthropic, Azure, etc.). Set the model identifier accordingly:
- `gpt-4o`, `gpt-4o-mini` (OpenAI)
- `anthropic/claude-sonnet-4-20250514` (Anthropic)
- `azure/gpt-4o` (Azure OpenAI)

## Patterns

### Class-based (recommended for complex judges)

```python
class MyJudge(LLMAsJudgeEvaluator):
    name = "my-judge"
    model = "gpt-4o-mini"
    criteria = "quality and accuracy"

    def build_prompt(self, trace: Trace, task: Optional[Task] = None) -> str:
        return f"Evaluate: {trace.input} -> {trace.output}"
```

### Decorator-based (recommended for simple judges)

```python
@llm_judge
def my_judge(trace: Trace) -> str:
    return f"Evaluate: {trace.input} -> {trace.output}"

@llm_judge(model="gpt-4o", criteria="accuracy")
def my_configured_judge(trace: Trace) -> str:
    return f"Evaluate: {trace.output}"
```

### Custom LLM client

```python
class MyJudge(LLMAsJudgeEvaluator):
    name = "my-judge"

    def build_prompt(self, trace: Trace) -> str:
        return f"Evaluate: {trace.output}"

    def _call_llm_with_retry(self, prompt: str) -> EvalResult:
        # Use your own client (OpenAI, Anthropic, etc.)
        response = your_llm_client.complete(prompt)  # replace with actual call
        llm_response_text = response.choices[0].message.content
        result, error = self._parse_and_validate(llm_response_text)
        return result or EvalResult(score=0.0, explanation=error)
```

## How to run

```bash
pip install amp-evaluation litellm
export OPENAI_API_KEY=sk-...
python run.py
```

## Expected output

```text
Loaded N traces

Discovered 5 evaluators:
  accuracy-judge (level=trace, modes=['experiment', 'monitor'])
  agent-efficiency-judge (level=agent, modes=['experiment', 'monitor'])
  custom-client-judge (level=trace, modes=['experiment', 'monitor'])
  grounding-judge (level=trace, modes=['experiment', 'monitor'])
  response-quality-judge (level=trace, modes=['experiment', 'monitor'])

Evaluation Run: run-... (EvalMode.MONITOR)
  ...
Scores:
  response-quality-judge:
    mean: ...
    count: N
  ...
```
