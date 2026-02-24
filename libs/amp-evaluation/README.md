# AMP Evaluation

A trace-based evaluation framework for AI agents. Analyze real agent executions from OpenTelemetry traces to measure quality, performance, and reliability.

## Install

```bash
pip install amp-evaluation
```

### Optional Dependencies

```bash
# For LLM-as-judge evaluators (multi-vendor LLM support)
pip install litellm

# For DeepEval evaluators
pip install deepeval>=3.8.4
```

## Quick Start

### 1. Define an Evaluator

Type hints drive everything. The first parameter type sets the evaluation level, and the `task` parameter determines mode compatibility.

```python
from amp_evaluation import evaluator, EvalResult
from amp_evaluation.trace import Trace

@evaluator("response-quality")
def response_quality(trace: Trace) -> EvalResult:
    output = trace.output or ""
    if len(output) < 20:
        return EvalResult(score=0.0, explanation="Response too short")
    return EvalResult(score=1.0, explanation="Response OK")
```

### 2. Run a Monitor

```python
from amp_evaluation import Monitor, builtin, discover_evaluators
import my_evaluators

# Discover all evaluators from a module + add built-ins
evals = discover_evaluators(my_evaluators) + [
    builtin("latency", max_latency_ms=5000),
    builtin("hallucination"),
]

monitor = Monitor(evaluators=evals, trace_service_url="http://traces:8001")
result = monitor.run(start_time="2026-01-01T00:00:00Z", end_time="2026-01-02T00:00:00Z")

print(result.summary())
```

### 3. Run an Experiment

```python
from amp_evaluation import Experiment, Dataset, Task, builtin

dataset = Dataset(tasks=[
    Task(task_id="q1", input="What is the capital of France?", expected_output="Paris"),
])

experiment = Experiment(
    evaluators=[builtin("exact_match"), builtin("latency", max_latency_ms=3000)],
    invoker=my_agent_invoker,
    dataset=dataset,
)
result = experiment.run()
```

---

## Evaluation Levels

Every evaluator operates at exactly **one** level, auto-detected from the `evaluate()` method's first parameter type hint:

| Level | Type Hint | Called | Use Case |
|-------|-----------|--------|----------|
| **trace** | `Trace` | Once per trace | Whole-request evaluation |
| **agent** | `AgentTrace` | Once per agent in the trace | Per-agent evaluation |
| **llm** | `LLMSpan` | Once per LLM call in the trace | Per-LLM-call evaluation |

```python
from amp_evaluation.trace import Trace, AgentTrace, LLMSpan

# Trace-level: called once per trace
def evaluate(self, trace: Trace) -> EvalResult: ...

# Agent-level: called N times (once per agent in the trace)
def evaluate(self, agent_trace: AgentTrace) -> EvalResult: ...

# LLM-level: called N times (once per LLM call in the trace)
def evaluate(self, llm_span: LLMSpan) -> EvalResult: ...
```

The runner's `run()` method handles iteration automatically — a trace with 3 agents calls the agent evaluator 3 times; a trace with 7 LLM calls calls the LLM evaluator 7 times.

## Evaluation Modes

Mode compatibility is auto-detected from the `task` parameter:

```python
# Both modes (monitor + experiment) — no task needed
def evaluate(self, trace: Trace) -> EvalResult: ...

# Experiment only — task is required (ground truth)
def evaluate(self, trace: Trace, task: Task) -> EvalResult: ...

# Both modes — adapts behavior based on task availability
def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult: ...
```

When running in monitor mode, evaluators that require a `task` parameter are automatically skipped with a warning.

---

## Defining Evaluators

### Function-Based (`@evaluator`)

```python
from amp_evaluation import evaluator, EvalResult, Param
from amp_evaluation.trace import Trace, AgentTrace, LLMSpan
from amp_evaluation.aggregators import AggregationType

@evaluator(
    name="tool-call-relevance",
    description="Are the right tools being called?",
    tags=["tool-use", "quality"],
    aggregations=[AggregationType.MEAN, AggregationType.PASS_RATE],
)
def tool_call_relevance(trace: Trace) -> EvalResult:
    tools = trace.get_tool_calls()
    if not tools:
        return EvalResult(score=0.5, explanation="No tools called")
    return EvalResult(score=1.0, explanation=f"Called {len(tools)} tools")

# LLM-level evaluator
@evaluator("llm-response-quality")
def llm_response_quality(llm_span: LLMSpan) -> EvalResult:
    if not llm_span.response and not llm_span.tool_calls:
        return EvalResult(score=0.0, explanation="Empty LLM response")
    return EvalResult(score=1.0, explanation="LLM response OK")
```

### Class-Based (`BaseEvaluator`)

```python
from amp_evaluation import BaseEvaluator, Param
from amp_evaluation.trace import AgentTrace

class AgentTokenEfficiency(BaseEvaluator):
    name = "agent-token-efficiency"
    description = "Check token usage per agent"
    tags = ["agent", "efficiency"]

    max_tokens: int = Param(default=5000, description="Max expected tokens", min=1)

    def evaluate(self, agent_trace: AgentTrace) -> EvalResult:
        tokens = agent_trace.metrics.token_usage.total_tokens
        score = min(1.0, self.max_tokens / max(tokens, 1))
        return EvalResult(
            score=score,
            explanation=f"{tokens} tokens (max: {self.max_tokens})",
        )

# Instantiate with config
efficient = AgentTokenEfficiency(max_tokens=3000)
```

---

## Configuration with `Param`

`Param` is a descriptor for evaluator parameters. Type is inferred from the annotation — never passed as an argument.

### Class-Based

```python
from amp_evaluation import BaseEvaluator, Param

class MyEvaluator(BaseEvaluator):
    name = "my-eval"
    threshold: float = Param(default=0.7, description="Pass threshold", min=0, max=1)
    model: str = Param(default="gpt-4o-mini", description="LLM model")

    def evaluate(self, trace: Trace) -> EvalResult:
        # Access config via self
        score = compute_score(trace)
        return EvalResult(score=score, passed=score >= self.threshold)

# Override at instantiation
strict = MyEvaluator(threshold=0.9, model="gpt-4o")
```

### Function-Based

```python
@evaluator("latency-check")
def latency_check(
    trace: Trace,
    max_latency_ms: float = Param(default=5000, description="Max latency in ms", min=0),
    threshold: float = Param(default=0.8, description="Pass threshold", min=0, max=1),
) -> EvalResult:
    passed = trace.metrics.total_duration_ms <= max_latency_ms
    score = 1.0 if passed else 0.0
    return EvalResult(score=score, passed=score >= threshold)

# Create configured copy (original unchanged)
strict = latency_check.with_config(max_latency_ms=1000, threshold=0.9)
```

### Schema Extraction

The platform can extract configuration schemas from any evaluator:

```python
info = my_evaluator.info
print(info.config_schema)
# [
#   {"key": "threshold", "type": "float", "default": 0.7, "min": 0, "max": 1, ...},
#   {"key": "model", "type": "string", "default": "gpt-4o-mini", ...},
# ]
```

---

## LLM-as-Judge Evaluators

Use an LLM to evaluate agent outputs for subjective criteria. Write a `build_prompt()` method — the framework handles LLM calling, output validation, and retry.

### How It Works

1. Override `build_prompt()` with typed parameters (same as `evaluate()`)
2. **Level** auto-detected from first parameter type hint (`Trace`, `AgentTrace`, `LLMSpan`)
3. **Mode** auto-detected from task parameter (same rules as regular evaluators)
4. Framework auto-appends output format instructions to your prompt
5. LLM called via [LiteLLM](https://docs.litellm.ai/) (supports 100+ providers)
6. Response validated with Pydantic (`score: 0.0–1.0`, `explanation: str`)
7. On invalid output, retries with Pydantic error as context (like [Instructor](https://python.useinstructor.com/))

### Class-Based

```python
from amp_evaluation import LLMAsJudgeEvaluator, EvalResult
from amp_evaluation.trace import Trace, AgentTrace, LLMSpan
from amp_evaluation.dataset import Task

# Trace-level judge
class GroundingJudge(LLMAsJudgeEvaluator):
    name = "grounding-judge"
    model = "gpt-4o"
    criteria = "Is the response grounded in the tool results?"

    def build_prompt(self, trace: Trace, task: Optional[Task] = None) -> str:
        tools = trace.get_tool_calls()
        tool_info = "\n".join(f"- {t.name}: {t.result}" for t in tools) if tools else "No tools called."

        prompt = f"""Evaluate whether this response is grounded in tool results.

Input: {trace.input}
Output: {trace.output}
Tool Results:
{tool_info}"""

        if task and task.expected_output:
            prompt += f"\n\nExpected Output: {task.expected_output}"

        return prompt
        # Framework auto-appends: "Respond with JSON: {score, explanation}"
        # Framework auto-validates: score 0.0–1.0, explanation string
        # Framework auto-retries: on invalid output, sends Pydantic error to LLM

# Agent-level judge
class AgentEfficiencyJudge(LLMAsJudgeEvaluator):
    name = "agent-efficiency"
    model = "anthropic/claude-sonnet-4-20250514"

    def build_prompt(self, agent: AgentTrace) -> str:
        return f"""Evaluate this agent's tool usage efficiency.
Input: {agent.input}
Output: {agent.output}
Tools available: {', '.join(agent.available_tools)}
Steps taken: {len(agent.steps)}
Has errors: {agent.has_errors}"""

# LLM-level judge
class LLMResponseQuality(LLMAsJudgeEvaluator):
    name = "llm-response-quality"

    def build_prompt(self, llm: LLMSpan) -> str:
        return f"""Rate the quality of this LLM response.
Model: {llm.model}
Response: {llm.response}
Tool calls: {len(llm.tool_calls)}"""
```

### Decorator-Based (`@llm_judge`)

```python
from amp_evaluation import llm_judge
from amp_evaluation.trace import Trace, AgentTrace
from amp_evaluation.dataset import Task

@llm_judge
def quality_judge(trace: Trace, task: Optional[Task] = None) -> str:
    prompt = f"Evaluate: {trace.input} → {trace.output}"
    if task and task.expected_output:
        prompt += f"\nExpected: {task.expected_output}"
    return prompt

@llm_judge(model="gpt-4o", criteria="accuracy and completeness")
def grounding_judge(trace: Trace) -> str:
    tools = trace.get_tool_calls()
    return f"""Is this grounded?
Output: {trace.output}
Tools: {', '.join(t.name for t in tools)}"""

@llm_judge(model="anthropic/claude-sonnet-4-20250514")
def agent_efficiency(agent: AgentTrace) -> str:
    return f"""Evaluate agent efficiency.
Input: {agent.input}
Steps: {len(agent.steps)}
Tools: {', '.join(agent.available_tools)}"""
```

### Custom LLM Client

Override `_call_llm_with_retry()` to use any LLM client instead of LiteLLM:

```python
class CustomLLMJudge(LLMAsJudgeEvaluator):
    name = "custom-judge"

    def build_prompt(self, trace: Trace) -> str:
        return f"Evaluate: {trace.input} → {trace.output}"

    def _call_llm_with_retry(self, prompt: str) -> EvalResult:
        import openai
        client = openai.OpenAI()
        resp = client.chat.completions.create(
            model=self.model,
            messages=[{"role": "user", "content": prompt}],
            response_format={"type": "json_object"},
        )
        result, error = self._parse_and_validate(resp.choices[0].message.content)
        return result or EvalResult(score=0.0, passed=False, explanation=error)
```

### Configuration

LLM-as-judge evaluators use `Param` descriptors:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `model` | `gpt-4o-mini` | LiteLLM model identifier |
| `criteria` | `quality, accuracy, and helpfulness` | Evaluation criteria |
| `temperature` | `0.0` | LLM temperature |
| `max_tokens` | `1024` | Max tokens for response |
| `threshold` | `0.5` | Score threshold for pass/fail |
| `max_retries` | `2` | Retries on invalid output |

### LiteLLM Model Identifiers

LiteLLM uses a `provider/model` format:

| Provider | Example |
|----------|---------|
| OpenAI | `gpt-4o`, `gpt-4o-mini` |
| Anthropic | `anthropic/claude-sonnet-4-20250514`, `anthropic/claude-haiku-4-5-20251001` |
| Ollama | `ollama/llama2` |
| Azure | `azure/gpt-4o` |

API keys are set via environment variables: `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, etc.

---

## Trace Model

Three vocabulary levels, each serving a clear purpose:

| Level | Container | Contains | Vocabulary |
|-------|-----------|----------|-----------|
| Trace | `Trace` | `spans: List[Span]` | **Spans** — raw OTEL execution records |
| Agent | `AgentTrace` | `steps: List[AgentStep]` | **Steps** — reconstructed agent execution flow |
| LLM | `LLMSpan` | `messages: List[Message]` | **Messages** — conversation to/from the LLM |

### Trace (spans)

The full execution path. Holds a flat list of spans ordered by time.

```python
from amp_evaluation.trace import Trace

trace.trace_id        # Unique identifier
trace.input           # User's original request
trace.output          # Agent's final response
trace.spans           # All raw spans (LLMSpan, ToolSpan, AgentSpan, RetrieverSpan)
trace.metrics         # Aggregated metrics (tokens, duration, counts)
trace.timestamp       # When the trace occurred

# Span access methods
trace.get_llm_calls()    # List[LLMSpan]
trace.get_tool_calls()   # List[ToolSpan]
trace.get_agents()       # List[AgentSpan]
trace.get_retrievals()   # List[RetrieverSpan]
```

### AgentTrace (steps)

Reconstructed view of one agent's execution, created from `trace.create_agent_trace(agent_span_id)`. Steps represent what the agent DID.

```python
from amp_evaluation.trace import AgentTrace, UserStep, LLMStep, ToolExecutionStep

agent_trace.agent_id
agent_trace.agent_name
agent_trace.system_prompt     # Agent metadata (not a step)
agent_trace.available_tools
agent_trace.steps             # List[UserStep | LLMStep | ToolExecutionStep]

# Three step types
UserStep(content="Book me a flight")

LLMStep(
    content="I'll search for flights.",
    tool_calls=[ToolCallInfo(id="tc-1", name="search_flights", arguments={...})],
    llm_span_id="llm-1",
)
# step.is_response → True if no tool_calls (final answer)

ToolExecutionStep(
    tool_name="search_flights",
    tool_call_id="tc-1",
    tool_input={"from": "NYC", "to": "Tokyo"},
    tool_output={"flights": [...]},
    nested_traces=[...],  # LLM calls or sub-agents within this tool
)

# Convenience properties
agent_trace.tool_steps         # List[ToolExecutionStep]
agent_trace.llm_steps          # List[LLMStep]
agent_trace.response_step      # Last LLMStep where is_response=True
agent_trace.tool_names_used    # List[str]
agent_trace.has_errors         # bool
agent_trace.sub_agent_traces   # List[AgentTrace]
```

### LLMSpan (typed messages)

A single LLM API call with typed messages.

```python
from amp_evaluation.trace import (
    LLMSpan, SystemMessage, UserMessage, AssistantMessage, ToolMessage
)

llm_span.span_id
llm_span.messages       # List[SystemMessage | UserMessage | AssistantMessage | ToolMessage]
llm_span.response       # LLM's text output
llm_span.tool_calls     # Tool calls requested by the LLM
llm_span.model          # Model name

# Typed messages
SystemMessage(content="You are a helpful assistant")
UserMessage(content="Hello")
AssistantMessage(content="I'll search.", tool_calls=[ToolCall(...)])
ToolMessage(content='{"result": "ok"}', tool_call_id="tc-1")

# Convenience properties
llm_span.system_messages   # List[SystemMessage]
llm_span.user_messages     # List[UserMessage]
llm_span.tool_messages     # List[ToolMessage]
```

---

## Built-in Evaluators

Use `builtin()` to get configured built-in evaluators by name:

```python
from amp_evaluation import builtin

latency = builtin("latency", max_latency_ms=5000)
hallucination = builtin("hallucination")
answer_rel = builtin("answer_relevancy")
```

### Discovery

```python
from amp_evaluation import list_builtin_evaluators, builtin_evaluator_catalog

# List names
names = list_builtin_evaluators()                    # All
names = list_builtin_evaluators(mode="monitor")      # Monitor-compatible only

# Full catalog with metadata
catalog = builtin_evaluator_catalog()
for info in catalog:
    print(f"{info.name} [{info.level}] — {info.description}")
```

### Available Built-ins

**Output Quality** (trace-level):

| Name | Description | Key Config |
|------|-------------|------------|
| `answer_length` | Output length within bounds | `min_length`, `max_length` |
| `answer_relevancy` | Word overlap between input/output | `min_overlap_ratio` |
| `required_content` | Required strings/patterns present | `required_strings`, `required_patterns` |
| `prohibited_content` | Prohibited content absent | `prohibited_strings`, `prohibited_patterns` |
| `exact_match` | Exact match with expected output | `case_sensitive` |
| `contains_match` | Expected output in actual | `case_sensitive` |
| `hallucination` | Keyword-based hallucination detection | `hallucination_keywords` |

**Trajectory** (trace-level):

| Name | Description | Key Config |
|------|-------------|------------|
| `tool_sequence` | Tool call sequence validation | `expected_sequence`, `strict` |
| `required_tools` | Required tools were used | `required_tools` |
| `step_success_rate` | Span success rate | `min_success_rate` |

**Performance** (trace-level):

| Name | Description | Key Config |
|------|-------------|------------|
| `latency` | Execution time within bounds | `max_latency_ms` |
| `token_efficiency` | Token usage within limits | `max_tokens` |
| `iteration_count` | Iteration count within limits | `max_iterations` |

**DeepEval** (trace-level, requires `deepeval` package):

| Name | Description |
|------|-------------|
| `deepeval/plan-quality` | Agent plan quality |
| `deepeval/plan-adherence` | Agent follows its plan |
| `deepeval/tool-correctness` | Correct tool selection |
| `deepeval/argument-correctness` | Correct tool arguments |
| `deepeval/task-completion` | Task completion assessment |
| `deepeval/step-efficiency` | Agent efficiency |

---

## Runners

### Monitor

Evaluates live production traces. No ground truth needed.

```python
from amp_evaluation import Monitor, builtin

monitor = Monitor(
    evaluators=[builtin("latency", max_latency_ms=5000), builtin("hallucination")],
    trace_service_url="http://traces:8001",
)

# Fetch and evaluate
result = monitor.run(
    start_time="2026-01-01T00:00:00Z",
    end_time="2026-01-02T00:00:00Z",
)

# Or pass traces directly
result = monitor.run(traces=my_traces)
```

### Experiment

Evaluates against a ground-truth dataset. Invokes the agent, fetches traces, then evaluates.

```python
from amp_evaluation import Experiment, Dataset, Task

dataset = Dataset(tasks=[
    Task(task_id="q1", input="What is 2+2?", expected_output="4"),
])

experiment = Experiment(
    evaluators=[builtin("exact_match"), builtin("latency", max_latency_ms=3000)],
    invoker=my_invoker,
    dataset=dataset,
)
result = experiment.run()
```

### RunResult

Both runners return a `RunResult`:

```python
result.run_id               # Unique run ID
result.eval_mode            # "experiment" or "monitor"
result.traces_evaluated     # Number of traces
result.evaluators_run       # Number of evaluators
result.scores               # Dict[str, EvaluatorSummary]
result.errors               # List[str]
result.success              # True if no errors and at least one trace evaluated
result.duration_seconds     # Run duration

# Per-evaluator results
summary = result.scores["latency"]
summary.aggregated_scores   # {"mean": 0.85, "pass_rate": 0.92, ...}
summary.individual_scores   # List[EvaluatorScore]
summary.count               # Total evaluations
summary.mean                # Shortcut for aggregated_scores["mean"]
summary.pass_rate           # Shortcut for pass rate

# Print formatted summary
print(result.summary())
```

---

## EvalResult

Return type for all evaluators:

```python
from amp_evaluation import EvalResult

# Success — evaluation completed with a score
EvalResult(score=0.85, explanation="Good response")
EvalResult(score=0.0, passed=False, explanation="Failed check")

# Error — evaluation could not be performed
EvalResult.skip("Missing required data")
EvalResult.skip("API key not configured")
```

**Key distinction**: `score=0.0` means "evaluated and failed"; `skip()` means "could not evaluate at all".

```python
result = my_evaluator.evaluate(trace)
if result.is_skipped:
    print(f"Skipped: {result.skip_reason}")
else:
    print(f"Score: {result.score}, Passed: {result.passed}")
```

---

## Discovery and Module Scanning

### `discover_evaluators()`

Scans a Python module for all `BaseEvaluator` instances — both `@evaluator`-decorated functions and class instances.

```python
from amp_evaluation import discover_evaluators
import my_evaluators

evals = discover_evaluators(my_evaluators)
monitor = Monitor(evaluators=evals)
```

### Evaluator Info

Every evaluator exposes metadata via the `.info` property:

```python
info = my_evaluator.info
info.name            # "latency"
info.description     # "Validates execution time..."
info.level           # "trace", "agent", or "llm"
info.modes           # ["monitor", "experiment"]
info.tags            # ["performance", "rule-based"]
info.config_schema   # List of config parameter descriptors
```

---

## Aggregations

Configure per-evaluator aggregation of scores across traces:

```python
from amp_evaluation.aggregators import AggregationType, Aggregation

@evaluator(
    "quality-check",
    aggregations=[
        AggregationType.MEAN,
        AggregationType.MEDIAN,
        AggregationType.P95,
        AggregationType.PASS_RATE,
    ],
)
def quality_check(trace: Trace) -> EvalResult:
    return EvalResult(score=0.85)
```

Built-in aggregation types: `MEAN`, `MEDIAN`, `MIN`, `MAX`, `SUM`, `COUNT`, `STDEV`, `VARIANCE`, `P50`, `P75`, `P90`, `P95`, `P99`, `PASS_RATE`.

---

## Project Structure

```text
amp-evaluation/
├── src/amp_evaluation/
│   ├── __init__.py              # Public API (Tier 1 imports)
│   ├── models.py                # EvalResult, EvaluatorInfo, EvaluatorSummary
│   ├── registry.py              # @evaluator, @llm_judge decorators, discover_evaluators()
│   ├── runner.py                # Experiment, Monitor, RunResult
│   ├── config.py                # Config management
│   ├── invokers.py              # Agent invocation
│   ├── evaluators/
│   │   ├── __init__.py          # BaseEvaluator, LLMAsJudgeEvaluator, Param exports
│   │   ├── base.py              # BaseEvaluator, LLMAsJudgeEvaluator, FunctionEvaluator, FunctionLLMJudge
│   │   ├── params.py            # Param, EvaluationLevel, EvalMode
│   │   └── builtin/
│   │       ├── __init__.py      # builtin(), list_builtin_evaluators(), catalog
│   │       ├── standard.py      # Rule-based evaluators
│   │       └── deepeval.py      # DeepEval wrapper evaluators (optional dep)
│   ├── trace/
│   │   ├── __init__.py          # Trace, spans, messages, steps exports
│   │   ├── models.py            # Trace, AgentTrace, LLMSpan, typed steps/messages
│   │   ├── parser.py            # OTEL → Trace conversion
│   │   └── fetcher.py           # TraceFetcher, TraceLoader
│   ├── aggregators/
│   │   ├── __init__.py
│   │   ├── base.py              # AggregationType, Aggregation
│   │   └── builtin.py           # Built-in aggregation functions
│   └── dataset/
│       ├── __init__.py
│       ├── schema.py            # Task, Dataset, Constraints
│       └── loader.py            # JSON/CSV loading
├── samples/                     # 12 focused examples (see Samples below)
│   ├── data/                    # Shared sample data
│   ├── 01-quickstart/           # Minimal working example
│   ├── 02-evaluation-levels/    # Level auto-detection from type hints
│   ├── 03-evaluation-modes/     # Mode auto-detection from task parameter
│   ├── 04-param-config/         # Param descriptors and with_config()
│   ├── 05-class-based-evaluator/# BaseEvaluator subclassing
│   ├── 06-decorator-evaluator/  # @evaluator decorator
│   ├── 07-llm-as-judge/         # LLM-based evaluation
│   ├── 08-builtin-evaluators/   # Built-in evaluator factory
│   ├── 09-deepeval-evaluators/  # DeepEval integration
│   ├── 10-experiment-with-dataset/ # Full experiment workflow
│   ├── 11-module-discovery/     # discover_evaluators()
│   └── 12-api-monitoring/       # API trace fetching
├── tests/                       # Test suite
└── pyproject.toml
```

## Samples

12 focused samples, each demonstrating one concept. See [samples/README.md](samples/README.md) for full details.

| # | Sample | Demonstrates | Offline? |
|---|--------|-------------|----------|
| 01 | [Quickstart](samples/01-quickstart/) | `@evaluator` + `builtin()` + `Monitor.run()` | Yes |
| 02 | [Evaluation Levels](samples/02-evaluation-levels/) | Level auto-detection from type hints (`Trace`, `AgentTrace`, `LLMSpan`) | Yes |
| 03 | [Evaluation Modes](samples/03-evaluation-modes/) | Mode auto-detection from task parameter | Yes |
| 04 | [Param Config](samples/04-param-config/) | `Param` descriptors, `with_config()`, `.info` schema | Yes |
| 05 | [Class-Based Evaluator](samples/05-class-based-evaluator/) | `BaseEvaluator` subclassing, `EvalResult.skip()` | Yes |
| 06 | [Decorator Evaluator](samples/06-decorator-evaluator/) | `@evaluator` decorator | Yes |
| 07 | [LLM-as-Judge](samples/07-llm-as-judge/) | `LLMAsJudgeEvaluator`, `@llm_judge` | Needs API key |
| 08 | [Built-in Evaluators](samples/08-builtin-evaluators/) | All 13 standard built-ins, catalog | Yes |
| 09 | [DeepEval Evaluators](samples/09-deepeval-evaluators/) | DeepEval integration (6 evaluators) | Needs deepeval |
| 10 | [Experiment with Dataset](samples/10-experiment-with-dataset/) | `load_dataset_from_json/csv()`, `Experiment.run()` | Yes (traces mode) |
| 11 | [Module Discovery](samples/11-module-discovery/) | `discover_evaluators()` scanning | Yes |
| 12 | [API Monitoring](samples/12-api-monitoring/) | `TraceFetcher` API trace fetching | Needs trace service |

## Testing

```bash
cd libs/amp-evaluation
pip install -e .
pytest
```

## License

Apache License 2.0
