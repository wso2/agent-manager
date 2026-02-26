# Concepts

This guide explains the core concepts behind amp-evaluation — what each abstraction is, why it exists, and when to use it. For API reference and code examples, see the [README](README.md).

## Table of Contents

- [Why Evaluate Agents?](#why-evaluate-agents)
- [Trace-Based Evaluation](#trace-based-evaluation)
- [Evaluation Levels](#evaluation-levels)
- [Evaluation Modes](#evaluation-modes)
- [Evaluators](#evaluators)
- [EvalResult — Scored vs. Skipped](#evalresult--scored-vs-skipped)
- [Datasets and Tasks](#datasets-and-tasks)
- [Aggregations](#aggregations)

---

## Why Evaluate Agents?

### The Non-Determinism Problem

Traditional software is deterministic — given the same input, you get the same output. Tests pass or fail consistently. AI agents break this assumption. The same prompt can produce:

- Different final answers (correct, partially correct, or wrong)
- Different tool call sequences (efficient or roundabout)
- Different reasoning paths (sound or flawed)
- Different error modes (graceful fallback or hallucinated response)

This non-determinism means you can't test an agent once and trust it forever. A prompt that worked yesterday might fail tomorrow because the model's behavior shifted, a tool's API changed, or context retrieval returned different documents.

### Three Core Challenges

Teams building agents face these recurring problems:

**1. Evaluation pipelines are hard to build.** Setting up production-grade evaluation requires fetching observability data efficiently, running evaluators in batch, and handling errors. Most teams spend weeks building this infrastructure instead of writing evaluation logic — or they skip automation entirely and rely on manual spot-checks.

**2. Developer workflows break down.** UI-only evaluators can't be version-controlled, peer-reviewed, or unit-tested. Framework-embedded evaluators create tight coupling — evaluation failures can impact the agent's runtime, and you can't iterate on evaluation logic without redeploying the agent.

**3. Operational misalignment.** Developers understand what quality metrics matter for their specific agent. Platform and ops teams need to monitor those metrics in production. Without a standardized bridge, deploying evaluation logic from development to production requires manual, one-off effort.

### What Continuous Evaluation Enables

A proper evaluation framework addresses all three by providing:

- **Regression detection** — catch quality drops before users notice
- **Pre-deployment validation** — benchmark against known test cases before shipping
- **Production monitoring** — track quality trends across real traffic
- **Data-driven improvement** — identify which failure modes to fix next

```
┌─────────┐     ┌──────────┐     ┌────────┐     ┌─────────┐
│ Develop │────▶│ Evaluate │────▶│ Deploy │────▶│ Monitor │
│  Agent  │     │  Locally │     │        │     │  in Prod│
└─────────┘     └──────────┘     └────────┘     └─────────┘
     ▲                                               │
     └───────────── Improve based on findings ───────┘
```

---

## Trace-Based Evaluation

### Why Traces?

amp-evaluation evaluates agents by analyzing their [OpenTelemetry](https://opentelemetry.io/) (OTEL) traces — the detailed execution records that capture every LLM call, tool invocation, and orchestration step. This is a deliberate design choice with several advantages:

**Decoupled from the agent runtime.** Evaluation runs separately from the agent, not inside it. This means zero performance impact on production agents, no risk of evaluation failures disrupting the agent, and no dependency on the agent's framework or language.

**Framework-agnostic.** Any agent that emits OTEL traces can be evaluated — LangChain, CrewAI, OpenAI Agents, Semantic Kernel, or custom implementations. The evaluator doesn't need to know how the agent was built; it only needs the trace.

**Same code, any environment.** Evaluators written for local development (running against JSON trace files) work unchanged in production (running against a trace API). No environment-specific code paths.

**Post-hoc analysis.** You can re-evaluate old traces with new evaluators. Found a new failure pattern? Write an evaluator and run it against last month's production data without re-executing the agent.

### Trace Structure

When a user sends a request to an agent, the instrumented agent produces a trace — a tree of spans representing every operation that occurred:

```
Trace (user request → final response)
│
├── AgentSpan: "travel-planner"
│   ├── LLMSpan: reasoning ("I need to search for available flights")
│   │   └── messages: [SystemMessage, UserMessage, AssistantMessage]
│   │
│   ├── ToolSpan: search_flights({from: "NYC", to: "Tokyo"})
│   │   └── result: [{flight: "AA100", price: 850}, ...]
│   │
│   ├── LLMSpan: reasoning ("Found 3 flights. Let me book the cheapest.")
│   │   └── messages: [UserMessage(tool results), AssistantMessage(decision)]
│   │
│   └── ToolSpan: book_flight({flight_id: "AA100"})
│       └── result: {confirmation: "CONF-12345"}
│
└── RetrieverSpan: policy_lookup("baggage allowance")
    └── retrieved_docs: [{content: "2 bags up to 23kg each", ...}]
```

### Three Data Views

The same trace data is accessible through three different views, each designed for a specific kind of evaluation question:

**Trace** — the complete end-to-end execution. Use this when evaluating the overall outcome — what the user asked, what they got back, and everything that happened in between across all agents.

What you can evaluate at this level:
- *"Was the final response helpful and accurate?"* — compare `trace.input` against `trace.output`
- *"Is the response grounded in evidence?"* — check `trace.output` against all tool results and retrieved documents from `trace.get_tool_calls()` and `trace.get_retrievals()`
- *"Did the request complete within acceptable time?"* — check `trace.metrics.total_duration_ms`
- *"Were all the right tools used?"* — inspect the full list of tool calls across every agent

This is the most common level — most evaluation questions are about the end-to-end experience.

**AgentTrace** — a reconstructed view of what one specific agent did, as a sequence of steps (think → act → observe). Use this when a trace involves multiple agents and you want to evaluate each one independently.

What you can evaluate at this level:
- *"Did the planner agent create a sound execution plan?"* — inspect the agent's reasoning steps
- *"Was the executor agent efficient, or did it loop unnecessarily?"* — compare `len(agent_trace.llm_steps)` against `len(agent_trace.tool_steps)`
- *"Did this agent recover from errors gracefully?"* — check `agent_trace.has_errors` and whether subsequent steps adapted
- *"Did the agent use the right subset of its available tools?"* — compare `agent_trace.tool_names_used` against `agent_trace.available_tools`

This level matters in multi-agent systems where each agent has different responsibilities — a planner, an executor, a reviewer — and you want to assess them separately.

**LLMSpan** — a single LLM API call with its full conversation context as typed messages. Use this when evaluating the quality of individual model interactions, independent of the broader agent logic.

What you can evaluate at this level:
- *"Was this model response safe and free of harmful content?"* — inspect `llm_span.response` for policy violations
- *"Was the response coherent and well-structured?"* — assess the LLM's output quality in isolation
- *"Is this model call cost-efficient?"* — check `llm_span.metrics.input_tokens` and `llm_span.metrics.output_tokens`
- *"Was the tone appropriate for the context?"* — evaluate against the `SystemMessage` instructions

This level is useful for safety monitoring (check every LLM call, not just the final output), cost optimization, and per-model quality tracking when you use multiple models.

---

## Evaluation Levels

Because traces naturally provide [three views of the data](#three-data-views), evaluators are designed to operate at exactly one level — **trace**, **agent**, or **llm**. The level determines what data the evaluator receives and how many times it runs per trace.

### Trace-Level Evaluators

**Scenario:** Your customer support agent handles a complaint. You want to check whether the final response actually addressed the user's issue and was grounded in the retrieved policy documents — an end-to-end assessment across everything that happened.

```python
@evaluator("grounding-check")
def grounding_check(trace: Trace) -> EvalResult:
    """Is the final response grounded in tool results and retrieved context?"""
    output = trace.output or ""
    # Collect all evidence from tool calls and retrievals across the full trace
    tool_evidence = [str(t.result) for t in trace.get_tool_calls() if t.result]
    retrieval_evidence = [doc.content for r in trace.get_retrievals() for doc in r.documents if doc.content]
    evidence = tool_evidence + retrieval_evidence
    if not evidence:
        return EvalResult.skip("No tool or retrieval results to verify against")
    evidence_text = " ".join(evidence)
    # Check if the response introduces claims not found in evidence
    ungrounded = any(claim not in evidence_text for claim in extract_claims(output))
    return EvalResult(
        score=0.0 if ungrounded else 1.0,
        explanation="Found claims not grounded in evidence" if ungrounded else "Response grounded"
    )
```

Trace-level evaluators run once per trace and see the complete picture — input, output, all tool calls and retrievals across every agent. This is the most common level.

### Agent-Level Evaluators

**Scenario:** Your multi-agent system has a planner and an executor. Overall results look good, but token costs are high. You need to pinpoint *which agent* is the bottleneck — is the planner overthinking, or is the executor looping?

```python
@evaluator("agent-efficiency")
def agent_efficiency(agent_trace: AgentTrace) -> EvalResult:
    """Did this agent complete its task without unnecessary steps?"""
    tool_steps = len(agent_trace.tool_steps)
    llm_steps = len(agent_trace.llm_steps)

    if llm_steps > tool_steps * 3:
        return EvalResult(
            score=0.3,
            explanation=f"Excessive reasoning: {llm_steps} LLM calls for {tool_steps} tool uses"
        )
    return EvalResult(score=1.0, explanation="Efficient execution")
```

In a trace with 3 agents, this evaluator runs 3 times — once per agent — producing 3 separate scores. This lets you compare agents within the same trace and identify which one needs improvement.

### LLM-Level Evaluators

**Scenario:** Your agent makes 5 LLM calls per request. Users report occasional inappropriate content, but the final response looks fine — the issue is in an intermediate reasoning step. You need to check every individual LLM call, not just the final output.

```python
@evaluator("llm-safety")
def llm_safety(llm_span: LLMSpan) -> EvalResult:
    """Was this LLM response safe and appropriate?"""
    response = llm_span.response or ""
    # Check for personally identifiable information leaks
    if any(pattern in response.lower() for pattern in ["ssn:", "credit card:"]):
        return EvalResult(score=0.0, explanation="PII detected in response")
    return EvalResult(score=1.0, explanation="No safety concerns")
```

In a trace with 7 LLM calls, this evaluator runs 7 times — catching the specific call that produced unsafe content, even if the final response filtered it out.

### How the Runner Dispatches

You don't need to write iteration logic. The runner inspects each evaluator's level and handles dispatch automatically:

```
Trace with 2 agents and 5 LLM calls:

Trace-level evaluator:  called 1 time  (once for the whole trace)
Agent-level evaluator:  called 2 times (once per agent)
LLM-level evaluator:    called 5 times (once per LLM call)
```

The level is auto-detected from the first parameter's type hint — `Trace` for trace-level, `AgentTrace` for agent-level, `LLMSpan` for LLM-level. No configuration needed.

---

## Evaluation Modes

### Two Stages of the Agent Lifecycle

Agents need evaluation at two fundamentally different stages:

**During development** — you have test cases with known correct answers. You want to verify the agent produces the right output, uses the right tools, and meets performance constraints. This is controlled benchmarking.

**In production** — you have real user traffic but no expected answers. You want to monitor quality trends, detect regressions, and flag problematic traces. This is continuous monitoring.

These stages require different evaluation approaches, which is why the SDK has two modes:

### Experiment Mode

An experiment tests your agent against a ground-truth dataset. The workflow:

1. Define a dataset of tasks (input + expected output + success criteria)
2. The experiment runner invokes the agent with each task's input
3. Traces are collected from the agent's execution
4. Evaluators score each trace, comparing against the task's ground truth

```python
dataset = Dataset(dataset_id="travel-benchmark", name="Travel Benchmark", tasks=[
    Task(
        task_id="flight-booking",
        input="Book a flight from NYC to Tokyo for next Monday",
        expected_output="Flight booked",
        expected_trajectory=[
            TrajectoryStep(tool="search_flights", args={"from": "NYC", "to": "Tokyo"}),
            TrajectoryStep(tool="book_flight", args={}),
        ],
        constraints=Constraints(max_latency_ms=10000, max_tokens=5000),
    ),
])

experiment = Experiment(
    evaluators=[builtin("exact_match"), builtin("tool_sequence"), builtin("latency")],
    invoker=my_agent_invoker,
    dataset=dataset,
)
result = experiment.run()
```

Use experiments for: pre-deployment validation, regression testing, comparing agent versions, benchmarking against known test suites.

### Monitor Mode

A monitor evaluates production traces over a time range. The workflow:

1. Specify a time range (or pass traces directly)
2. The monitor fetches traces from the trace service
3. Evaluators score each trace without ground truth

```python
monitor = Monitor(
    evaluators=[builtin("latency", max_latency_ms=5000), builtin("hallucination")],
    trace_fetcher=TraceFetcher(base_url="http://traces:8001"),
)
result = monitor.run(
    start_time="2026-01-01T00:00:00Z",
    end_time="2026-01-02T00:00:00Z",
)
```

Use monitors for: production quality tracking, regression detection, SLA monitoring, flagging problematic traces for human review.

### Experiment vs. Monitor — Side by Side

```
EXPERIMENT                              MONITOR
─────────                               ───────
Dataset (tasks)                         Time range
    │                                       │
    ▼                                       ▼
Invoke agent                            Fetch traces
    │                                       │
    ▼                                       ▼
Collect traces                          ────────────
    │                                       │
    ▼                                       ▼
Evaluate with                           Evaluate without
ground truth (Task)                     ground truth
    │                                       │
    ▼                                       ▼
RunResult                               RunResult
(scores + comparisons)                  (scores + trends)
```

### Mode Compatibility

How the SDK decides which evaluators can run in which mode — auto-detected from the `task` parameter:

```python
# Works in BOTH modes — no ground truth needed
@evaluator("latency")
def latency(trace: Trace) -> EvalResult:
    return EvalResult(score=1.0 if trace.metrics.total_duration_ms < 5000 else 0.0)

# EXPERIMENT only — requires ground truth
@evaluator("exact-match")
def exact_match(trace: Trace, task: Task) -> EvalResult:
    return EvalResult(score=1.0 if trace.output == task.expected_output else 0.0)

# BOTH modes, adapts behavior — uses ground truth when available
@evaluator("response-quality")
def response_quality(trace: Trace, task: Optional[Task] = None) -> EvalResult:
    if task and task.expected_output:
        # Experiment: compare against expected output
        similarity = compute_similarity(trace.output, task.expected_output)
        return EvalResult(score=similarity)
    else:
        # Monitor: use heuristics
        return EvalResult(score=1.0 if len(trace.output or "") > 20 else 0.0)
```

When running in monitor mode, evaluators that require a `task` parameter are automatically skipped with a warning — no error, no crash.

---

## Evaluators

### The Core Abstraction

An evaluator is a function that receives structured trace data and returns a quality score. Every evaluator — whether it's a simple rule, an LLM-as-judge prompt, a custom class, or a third-party wrapper — follows the same contract:

1. Receive a `Trace`, `AgentTrace`, or `LLMSpan` (determines level)
2. Optionally receive a `Task` (determines mode compatibility)
3. Return an `EvalResult` with a score from 0.0 to 1.0

Why does this matter? Because you can mix and match freely. In a single evaluation run, you might combine a rule-based latency check, an LLM-as-judge helpfulness scorer, a custom trajectory validator, and a DeepEval plan-quality metric. The runner doesn't care how each evaluator is implemented — it only cares that each one takes trace data in and returns a score out. This composability means you can start with a few simple built-in checks and progressively layer in more sophisticated evaluators as your needs evolve, without changing your evaluation pipeline.

### Three Flavors

#### Decorator-Based (`@evaluator`)

Best for: quick, lightweight, stateless checks. The simplest way to write an evaluator.

```python
@evaluator("response-length")
def response_length(trace: Trace) -> EvalResult:
    length = len(trace.output or "")
    if length < 10:
        return EvalResult(score=0.0, explanation="Too short")
    if length > 10000:
        return EvalResult(score=0.5, explanation="Excessively long")
    return EvalResult(score=1.0, explanation=f"{length} chars — good length")
```

Use when: you need a simple check, don't need configurable parameters, or are prototyping.

#### Class-Based (`BaseEvaluator`)

Best for: configurable evaluators with parameters, complex logic, or evaluators you want to reuse with different settings.

```python
class TokenEfficiency(BaseEvaluator):
    name = "token-efficiency"
    description = "Evaluates whether token usage is within acceptable bounds"

    max_tokens: int = Param(default=5000, description="Maximum expected tokens", min=1)

    def evaluate(self, trace: Trace) -> EvalResult:
        total = trace.metrics.token_usage.total_tokens
        if total <= self.max_tokens:
            return EvalResult(score=1.0, explanation=f"{total} tokens — within budget")
        ratio = self.max_tokens / total
        return EvalResult(score=ratio, explanation=f"{total} tokens — exceeds {self.max_tokens} limit")

# Create instances with different configurations
default_check = TokenEfficiency()                    # 5000 token limit
strict_check = TokenEfficiency(max_tokens=2000)      # 2000 token limit
```

Use when: you need `Param` configuration, want to create multiple instances with different settings, or have complex initialization logic.

#### LLM-as-Judge

Best for: subjective criteria that rule-based checks can't capture — helpfulness, tone, reasoning quality, factual grounding. You write the prompt; the framework handles calling the LLM, parsing the JSON response (`{score, explanation}`), validating with Pydantic, and retrying on invalid output.

**Decorator-based (`@llm_judge`)** — the simplest way:

```python
@llm_judge(model="gpt-4o", criteria="helpfulness, completeness, and accuracy")
def helpfulness_judge(trace: Trace) -> str:
    return f"""Evaluate the helpfulness of this agent response.

User asked: {trace.input}
Agent responded: {trace.output}

Consider whether the response fully addresses the user's request,
provides accurate information, and is presented clearly."""
```

**Class-based (`LLMAsJudgeEvaluator`)** — when you need more control or configuration:

```python
class HelpfulnessJudge(LLMAsJudgeEvaluator):
    name = "helpfulness"
    model = "gpt-4o"
    criteria = "helpfulness, completeness, and accuracy"

    def build_prompt(self, trace: Trace) -> str:
        return f"""Evaluate the helpfulness of this agent response.

User asked: {trace.input}
Agent responded: {trace.output}

Consider whether the response fully addresses the user's request,
provides accurate information, and is presented clearly."""
```

Use when: the quality aspect is subjective and resists simple rules — no regex or keyword check can reliably assess "helpfulness" or "reasoning quality."

### Rule-Based vs. LLM-as-Judge

| Aspect | Rule-Based | LLM-as-Judge |
|--------|-----------|--------------|
| **Speed** | Milliseconds | Seconds (LLM API call) |
| **Cost** | Free | API cost per evaluation |
| **Determinism** | Same input → same score | May vary across runs |
| **Best for** | Objective metrics | Subjective quality |
| **Examples** | Latency, token count, exact match, required tools, prohibited content | Helpfulness, tone, reasoning quality, factual grounding, safety assessment |

**Use rule-based evaluators when:**
- The criterion is objective and measurable (latency < 5s, output contains "confirmation")
- You need deterministic, reproducible results
- Cost or speed matters (running across thousands of traces)

**Use LLM-as-judge when:**
- The criterion is subjective ("is this response helpful?")
- You need nuanced assessment that accounts for meaning, not just patterns
- A human reviewer would need to read and judge the output

Many production setups combine both: rule-based for performance and compliance checks, LLM-as-judge for quality and user experience.

### Configuring Evaluators with `Param`

`Param` is a descriptor for declaring evaluator parameters with type validation, defaults, and constraints:

```python
class MyEvaluator(BaseEvaluator):
    name = "my-eval"
    threshold: float = Param(default=0.7, description="Pass threshold", min=0, max=1)
    model: str = Param(default="gpt-4o-mini", description="LLM model")
```

Why `Param` exists:

- **Schema extraction** — the platform can introspect any evaluator to generate configuration UI forms automatically (`evaluator.info.config_schema`)
- **Immutable copies** — `evaluator.with_config(threshold=0.9)` creates a new instance without mutating the original
- **Validation** — type checking, min/max bounds, and enum constraints are enforced at instantiation

---

## EvalResult — Scored vs. Skipped

Every evaluator returns an `EvalResult`. There are two fundamentally different outcomes, and the distinction matters for how results are aggregated and interpreted.

### Scored: The Evaluator Ran and Produced a Measurement

```python
# Agent failed the check — but this IS a valid evaluation result
EvalResult(score=0.0, explanation="Response contains prohibited content")

# Agent passed with high quality
EvalResult(score=0.95, explanation="Comprehensive and accurate response")

# Partial credit
EvalResult(score=0.6, explanation="Correct answer but poor formatting")
```

A score of `0.0` is a real signal — it means the evaluator ran, analyzed the trace, and determined the agent failed. This is a measurement, not an error.

The `passed` field provides a binary pass/fail interpretation. By default, `passed = score >= 0.5`, but you can override it:

```python
EvalResult(score=0.3, passed=True, explanation="Below threshold but acceptable for this case")
```

### Skipped: The Evaluator Could Not Run

```python
# Missing data — can't evaluate without it
EvalResult.skip("Trace has no LLM calls — nothing to evaluate")

# Wrong context — evaluator doesn't apply here
EvalResult.skip("No expected output available in monitor mode")

# External failure — API error, misconfiguration
EvalResult.skip("OpenAI API key not configured")
```

A skip means "no measurement was possible." The evaluator didn't fail — it simply couldn't produce a score. Common scenarios:

- An experiment-only evaluator running in monitor mode (no `Task` available)
- An LLM-level evaluator on a trace with no LLM calls
- An LLM-as-judge evaluator when the API key isn't configured
- A trace missing expected data (no output, no tool calls)

### Why the Distinction Matters

Aggregations only include scored results. If you have 100 traces and 20 are skipped:

- **Mean score** is computed from the 80 scored results
- **Pass rate** is based on the 80 scored results
- **Skip count** is tracked separately (20 skips)

If skips were treated as `score=0.0`, they would artificially deflate your metrics. A trace that was skipped because the API key wasn't set is not the same as a trace where the agent produced a terrible response.

### Decision Guide: Score 0.0 vs. Skip

| Situation | Use |
|-----------|-----|
| Agent produced a bad response | `EvalResult(score=0.0)` |
| Agent produced no output at all | `EvalResult(score=0.0)` |
| Trace has no data to evaluate (e.g., no LLM calls for an LLM-level evaluator) | `EvalResult.skip("reason")` |
| External dependency unavailable (API down, key missing) | `EvalResult.skip("reason")` |
| Evaluator doesn't apply in this mode | `EvalResult.skip("reason")` |

The rule of thumb: if the agent *could have* done something but didn't (or did it badly), that's a score of 0.0. If the evaluator itself couldn't run due to circumstances outside the agent's control, that's a skip.

---

## Datasets and Tasks

### What Is Ground Truth for Agents?

Traditional software testing checks one dimension: given input X, does the system produce output Y? Agents are fundamentally different because they produce *both* answers and *actions*. A correct answer delivered through the wrong tools is a problem. A correct answer that takes 30 seconds is a failure in real-time chat. A correct answer that leaks internal API endpoints in intermediate steps is a security issue.

This means ground truth for agents is multi-dimensional. A single test case may need to capture what the agent should *say*, what it should *do*, what bounds it must *stay within*, and what it must *never* produce. This is why `Task` has more fields than a traditional test assertion.

### Task

A `Task` is a single test case — one problem for the agent to solve, with everything needed to evaluate the result. Only `task_id` and `input` are required; the rest are optional and you use them as your evaluation needs grow.

#### Required Fields

Every task needs an identity and an input:

```python
Task(
    task_id="flight-booking-nyc-tokyo",   # Unique identifier for tracking results
    input="Book me a flight from NYC to Tokyo",  # The prompt sent to the agent
)
```

`input` can be a string or a structured dict for agents that accept JSON inputs.

#### Ground Truth Fields

These fields define what "correct" means for the agent — each captures a different dimension:

**`expected_output`** — The simplest form of ground truth. Use when you know the exact answer.

*Scenario: A Q&A agent answering factual questions.*
```python
Task(task_id="q1", input="What is the capital of France?", expected_output="Paris")
```

**`expected_trajectory`** — The sequence of tools the agent should call. Use when the *path* matters, not just the destination.

*Scenario: A booking agent should search before booking — skipping the search step would mean booking blindly.*
```python
Task(
    task_id="booking-1",
    input="Book the cheapest flight to Tokyo",
    expected_trajectory=[
        TrajectoryStep(tool="search_flights", args={"to": "Tokyo"}),
        TrajectoryStep(tool="book_flight"),
    ],
)
```

**`expected_outcome`** — Expected side effects or state changes. Use when the agent modifies external systems.

*Scenario: After a booking, you expect a specific database record to exist.*
```python
Task(
    task_id="booking-2",
    input="Book flight AA100",
    expected_outcome={"booking_status": "confirmed", "flight_id": "AA100"},
)
```

**`success_criteria`** — A human-readable description of what success looks like. Use when exact match is too rigid and you need LLM judges to assess quality.

*Scenario: A financial advisor agent where the quality of advice matters more than exact wording.*
```python
Task(
    task_id="advice-1",
    input="Should I invest in index funds?",
    success_criteria="The agent should explain at least 2 pros and 2 cons, mention risk tolerance, and not give specific financial advice",
)
```

#### Constraint Fields

Performance bounds are part of ground truth — a correct answer delivered too slowly or at too high a cost is still a failure.

**`constraints`** — Operational envelope for the task.

*Scenario: A real-time chat agent must respond within 5 seconds and stay within token budget.*
```python
Task(
    task_id="chat-1",
    input="What's my account balance?",
    constraints=Constraints(max_latency_ms=5000, max_tokens=2000, max_iterations=5),
)
```

Available constraints: `max_latency_ms`, `max_tokens`, `max_iterations`, `max_cost`.

**`prohibited_content`** — Strings that must never appear in the output. Use for compliance and safety guardrails.

*Scenario: A customer-facing agent must never leak internal system details.*
```python
Task(
    task_id="support-1",
    input="Why was my order delayed?",
    prohibited_content=["internal API", "debug", "stack trace", "admin panel"],
)
```

#### Classification Fields

For organizing large benchmark suites — filter tasks by type, slice metrics by domain, compare performance across difficulty levels.

| Field | Purpose | Example |
|-------|---------|---------|
| `task_type` | Category of task | `"qa"`, `"rag"`, `"tool_use"`, `"code_gen"` |
| `difficulty` | Complexity level | `"easy"`, `"medium"`, `"hard"`, `"expert"` |
| `domain` | Subject area | `"medical"`, `"legal"`, `"finance"` |
| `tags` | Freeform labels | `["hallucination", "multi-step", "math"]` |

#### Extensibility

**`custom`** — A dict passed to evaluators. Use for domain-specific data your custom evaluators need.

```python
Task(task_id="med-1", input="...", custom={"patient_age": 45, "severity": "high"})
# Your evaluator can access: task.custom["patient_age"]
```

**`metadata`** — A dict NOT passed to evaluators. Use for organizational tracking — who created the task, when, review status.

#### Full Task Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `task_id` | `str` | Yes | Unique identifier for tracking results across runs. e.g., `"flight-booking-nyc-tokyo"` |
| `input` | `str \| dict` | Yes | The prompt or structured input sent to the agent. String for simple prompts (`"Book a flight to Tokyo"`), dict for agents accepting JSON (`{"query": "...", "context": {...}}`) |
| `name` | `str` | No | Short human-readable label shown in reports. e.g., `"NYC→Tokyo booking"` |
| `description` | `str` | No | What capability this task tests. e.g., `"Multi-step tool use with search and booking"` |
| `expected_output` | `str` | No | The exact or expected final response. Used by evaluators like `exact_match` and `contains_match`. e.g., `"Paris"` |
| `expected_trajectory` | `List[TrajectoryStep]` | No | Ordered list of expected tool calls. Each step has `tool` (name) and optional `args`. Used by `tool_sequence` and `required_tools` evaluators |
| `expected_outcome` | `dict` | No | Expected side effects or state changes after execution. e.g., `{"booking_status": "confirmed"}`. Useful for agents that modify external systems |
| `success_criteria` | `str \| List[str]` | No | Natural language rubric for LLM-as-judge evaluators. e.g., `"Agent should recommend 3+ options and explain trade-offs"`. Can be a list for multi-criteria assessment |
| `constraints` | `Constraints` | No | Performance envelope: `max_latency_ms`, `max_tokens`, `max_iterations`, `max_cost`. Used by `latency`, `token_efficiency`, and `iteration_count` evaluators |
| `prohibited_content` | `List[str]` | No | Strings that must never appear in the output. e.g., `["internal API", "debug", "stack trace"]`. Used by the `prohibited_content` evaluator |
| `task_type` | `str` | No | Category for filtering and slicing results. Values: `"qa"`, `"rag"`, `"tool_use"`, `"code_gen"`, or custom. Default: `"general"` |
| `difficulty` | `str` | No | Complexity level for benchmarking across tiers. One of: `"easy"`, `"medium"`, `"hard"`, `"expert"`. Default: `"medium"` |
| `domain` | `str` | No | Subject area for domain-specific analysis. e.g., `"medical"`, `"legal"`, `"finance"`, `"customer_support"` |
| `tags` | `List[str]` | No | Freeform labels for flexible filtering. e.g., `["hallucination-prone", "multi-step", "math"]` |
| `custom` | `dict` | No | Domain-specific data passed through to evaluators. e.g., `{"patient_age": 45}` — accessible via `task.custom["patient_age"]` in your evaluator code |
| `metadata` | `dict` | No | Organizational info NOT passed to evaluators. e.g., `{"created_by": "qa-team", "review_status": "approved"}` |

### Dataset

A `Dataset` is a curated collection of tasks — your benchmark suite for the agent.

```python
dataset = Dataset(
    dataset_id="travel-agent-v2",
    name="Travel Agent v2 Benchmark",
    description="50 test cases covering booking, cancellation, and inquiry scenarios",
    tasks=[
        Task(task_id="q1", input="What's the baggage limit?", expected_output="2 bags, 23kg each"),
        Task(task_id="q2", input="Book a flight to Tokyo", expected_trajectory=[...]),
        Task(task_id="q3", input="Cancel booking CONF-123", success_criteria="Booking cancelled"),
        # ...
    ],
)
```

**`dataset_type`** classifies how the dataset was created — this affects how you interpret results:

| Type | When to use | Example |
|------|-------------|---------|
| `golden_set` (default) | Curated test cases with verified ground truth | Hand-written benchmark suite |
| `production_traces` | Real user interactions sampled from production | Last week's customer support traces |
| `synthetic` | AI-generated test cases | LLM-generated edge cases for stress testing |
| `human_annotated` | Production data with human labels added | Support tickets annotated by QA team |

Datasets can also be loaded from files:

```python
dataset = load_dataset_from_json("tests/data/travel_benchmark.json")
dataset = load_dataset_from_csv("tests/data/tasks.csv")
```

### When You Need Datasets

Datasets are only needed for **experiment mode**. Monitors evaluate production traces without ground truth — they don't need tasks or expected outputs.

---

## Aggregations

### Why Aggregate?

A single evaluator run across 1,000 traces produces 1,000 individual scores. These are useful for debugging specific traces, but decisions require summary statistics:

- "Should we deploy this agent version?" → look at mean score and pass rate
- "Are there outlier failures?" → look at P5 or min score
- "Is quality consistent?" → look at standard deviation
- "What's the worst-case performance?" → look at P95 latency

### Available Metrics

| Metric | What It Tells You |
|--------|-------------------|
| **Mean** | Average quality across all evaluations |
| **Median** | Typical quality (robust to outliers) |
| **Pass Rate** | What percentage met the threshold — a reliability metric |
| **P95 / P99** | Tail behavior — how bad are the worst cases? |
| **Min / Max** | Boundary cases |
| **Stdev / Variance** | How consistent is quality across traces? |

### Configuring Aggregations

Each evaluator can specify which aggregations to compute:

```python
@evaluator(
    "quality-check",
    aggregations=[
        AggregationType.MEAN,
        AggregationType.PASS_RATE,
        AggregationType.P95,
        AggregationType.STDEV,
    ],
)
def quality_check(trace: Trace) -> EvalResult:
    ...
```

Default aggregations (if none specified): `MEAN`.

### Interpreting Results

A few patterns to watch for:

- **High mean, high pass rate** (mean=0.92, pass_rate=0.95) — the agent is performing well consistently.
- **High mean, low pass rate** (mean=0.85, pass_rate=0.60) — when the agent succeeds it does well, but 40% of traces fail. Investigate the failure cases.
- **Low stdev** (stdev=0.05) — consistent quality. Good for production confidence.
- **High stdev** (stdev=0.35) — wildly inconsistent. The agent works great sometimes and terribly other times. The non-determinism problem is hitting hard.
- **P95 much lower than mean** — most traces are fine, but there's a long tail of poor results that the mean hides.
