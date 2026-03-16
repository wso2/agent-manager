# Writing Custom Evaluators — AI Copilot Reference

> This guide is auto-generated from the AMP evaluation SDK.
> It contains the framework conventions, data models, templates, and rules
> needed to write custom evaluators.

## Overview

There are two types of custom evaluators:

| Type | Description |
|------|-------------|
| **Code** (`code`) | Python function that programmatically analyzes trace data |
| **LLM-Judge** (`llm_judge`) | Prompt template evaluated by an LLM |

## Evaluation Levels

| Level | Type Hint | Called |
|-------|-----------|--------|

| `trace` | `Trace` | Once per trace — end-to-end assessment of the full interaction |
| `agent` | `AgentTrace` | Once per agent span — individual agent performance in multi-agent systems |
| `llm` | `LLMSpan` | Once per LLM call — per-call quality (safety, coherence, etc.) |

## Code Evaluators

Code evaluators are Python **functions** (not classes) that receive a typed trace object and return an `EvalResult`.

### Rules

- Write a **function** (not a class)
- Type-hint the first parameter to set the evaluation level
- Define configurable parameters as function arguments with plain defaults (add them in the Config Params UI section)
- Return `EvalResult(score=0.0-1.0, explanation="...")` — higher is better
- Use `EvalResult.skip("reason")` when evaluation cannot be performed
- Score range: 0.0 (worst) to 1.0 (best)

### Code Template — trace-level (Trace)

Called: Once per trace — end-to-end assessment of the full interaction

```python
from amp_evaluation import EvalResult
from amp_evaluation.trace.models import Trace


def my_evaluator(
    trace: Trace,
    # Configurable parameters — add these in the Config Params UI section.
    # They are passed as keyword arguments at runtime.
    threshold: float = 0.5,
) -> EvalResult:
    """Evaluate a complete trace (called once per trace)."""

    user_input = trace.input or ""
    agent_output = trace.output or ""

    # Example: check that the agent produced a non-empty response
    if not agent_output.strip():
        return EvalResult.skip("No output to evaluate")

    # Your evaluation logic
    score = 1.0
    passed = score >= threshold

    return EvalResult(
        score=score,
        passed=passed,
        explanation="Evaluation explanation here",
    )
```

#### Trace Data Model

- `trace.input`: `str` — User input / query
- `trace.output`: `str` — Agent output / final response
- `trace.spans`: `List[amp_evaluation.trace.models.LLMSpan | amp_evaluation.trace.models.ToolSpan | amp_evaluation.trace.models.RetrieverSpan | amp_evaluation.trace.models.AgentSpan | amp_evaluation.trace.models.ChainSpan]` — All execution spans ordered by start time
- `trace.metrics`: `TraceMetrics` — Aggregated performance metrics
- `trace.format_evidence()`: `str` — Format tool results and retrieved documents for LLM-friendly display.
- `trace.format_spans()`: `str` — Render the full span tree using parent_span_id hierarchy.
- `trace.get_agents()`: `List[AgentSpan]` — Get all agent spans (for multi-agent systems).
- `trace.get_llm_calls()`: `List[LLMSpan]` — Get all LLM calls with enhanced filtering and deduplication.
- `trace.get_retrievals()`: `List[RetrieverSpan]` — Get all retrieval operations with agent filtering.
- `trace.get_tool_calls()`: `List[ToolSpan]` — Get all tool executions with agent filtering.

### Code Template — agent-level (AgentTrace)

Called: Once per agent span — individual agent performance in multi-agent systems

```python
from amp_evaluation import EvalResult
from amp_evaluation.trace.models import AgentTrace


def my_evaluator(
    agent_trace: AgentTrace,
    # Configurable parameters — add these in the Config Params UI section.
    # They are passed as keyword arguments at runtime.
    threshold: float = 0.5,
) -> EvalResult:
    """Evaluate an agent span (called once per agent in the trace)."""

    agent_input = agent_trace.input or ""
    agent_output = agent_trace.output or ""
    tools_used = [s.tool_name for s in agent_trace.get_tool_steps()]

    # Example: check tool usage
    if not tools_used:
        return EvalResult(score=0.5, explanation="Agent did not use any tools")

    score = 1.0
    passed = score >= threshold

    return EvalResult(
        score=score,
        passed=passed,
        explanation=f"Agent used {len(tools_used)} tool(s): {', '.join(tools_used)}",
    )
```

#### AgentTrace Data Model

- `agent_trace.input`: `str` — Agent input
- `agent_trace.output`: `str` — Agent output
- `agent_trace.steps`: `List[UserInputStep | LLMReasoningStep | ToolExecutionStep]` — Execution steps: UserInputStep, LLMReasoningStep, or ToolExecutionStep
- `agent_trace.agent_name`: `str` — Name of the agent
- `agent_trace.model`: `str` — LLM model used by the agent
- `agent_trace.system_prompt`: `str` — System prompt / instructions
- `agent_trace.available_tools`: `List[ToolDefinition]` — Tools available to the agent
- `agent_trace.metrics`: `TraceMetrics` — Aggregated performance metrics
- `agent_trace.format_steps()`: `str` — Format execution steps as a numbered list for LLM-friendly display.
- `agent_trace.get_error_steps()`: `List[ToolExecutionStep]` — Get tool steps that produced errors.
- `agent_trace.get_llm_steps()`: `List[LLMReasoningStep]` — Get all LLM output steps (both intermediate reasoning and final response).
- `agent_trace.get_sub_agents()`: `List[AgentTrace]` — Get all sub-agent traces from nested tool executions.
- `agent_trace.get_tool_steps()`: `List[ToolExecutionStep]` — Get all tool execution steps.

### Code Template — llm-level (LLMSpan)

Called: Once per LLM call — per-call quality (safety, coherence, etc.)

```python
from amp_evaluation import EvalResult
from amp_evaluation.trace.models import LLMSpan


def my_evaluator(
    llm_span: LLMSpan,
    # Configurable parameters — add these in the Config Params UI section.
    # They are passed as keyword arguments at runtime.
    threshold: float = 0.5,
) -> EvalResult:
    """Evaluate an LLM call (called once per LLM invocation)."""

    output = llm_span.output or ""
    model = llm_span.model or ""

    # Example: check output is non-empty
    if not output.strip():
        return EvalResult.skip("Empty LLM output")

    score = 1.0
    passed = score >= threshold

    return EvalResult(
        score=score,
        passed=passed,
        explanation=f"LLM ({model}) produced a valid response",
    )
```

#### LLMSpan Data Model

- `llm_span.input`: `List[SystemMessage | UserMessage | AssistantMessage | ToolMessage]` — Conversation messages sent to the LLM
- `llm_span.output`: `str` — LLM response text
- `llm_span.available_tools`: `List[ToolDefinition]` — Tools available to the LLM for this call
- `llm_span.model`: `str` — Model name (e.g. gpt-4o)
- `llm_span.vendor`: `str` — Model vendor (e.g. openai)
- `llm_span.temperature`: `float | None` — LLM temperature setting
- `llm_span.metrics`: `LLMMetrics` — LLM-specific performance metrics
- `llm_span.format_messages()`: `str` — Format conversation messages for LLM-friendly display.
- `llm_span.get_assistant_messages()`: `List[AssistantMessage]` — Get assistant messages only.
- `llm_span.get_system_messages()`: `List[SystemMessage]` — Get system messages only.
- `llm_span.get_tool_messages()`: `List[ToolMessage]` — Get tool result messages only.
- `llm_span.get_user_messages()`: `List[UserMessage]` — Get user messages only.

## LLM-Judge Evaluators

LLM-judge evaluators are **prompt template strings** (not Python code). Use `{expression}` syntax to access trace data. Python expressions like loops and joins are supported inside `{ }`.

The framework auto-appends JSON scoring instructions — **do NOT include scoring/output format instructions in your prompt**.

### Rules

- Write a **prompt template** (not a Python class or function)
- Use `{variable.field}` to access trace data (Python f-string syntax)
- Python expressions are supported: `{len(trace.spans)}`, `{', '.join(s.tool_name for s in agent_trace.get_tool_steps())}`
- Include a **scoring rubric** (0.0 to 1.0 scale) to guide consistent scoring
- Do NOT include output format instructions — the framework appends them
- Avoid imports or side effects in expressions

### LLM-Judge Template — trace-level (Trace)

Called: Once per trace — end-to-end assessment of the full interaction

Variable: `trace` (Trace)

```
You are an expert evaluator. Your sole criterion is HELPFULNESS: does the response actually help the user with what they asked for?

User Query:
{trace.input}

Agent Response:
{trace.output}

Execution Summary:
- Total spans: {len(trace.spans)}
- Agents involved: {', '.join(a.agent_name or 'unnamed' for a in trace.get_agents()) or 'none'}

Evaluation Steps:
1. Identify what the user needs: what problem are they trying to solve or what information are they seeking?
2. Assess whether the response provides actionable, useful content that moves the user closer to their goal.
3. Check for empty helpfulness: does the response acknowledge the question without actually helping?
4. Assess whether the response would leave the user better off than before they asked.

Scoring Rubric:
  0.0  = Not helpful at all; ignores the user's need or answers a completely different question
  0.25 = Minimally helpful; touches on the topic but does not provide enough useful content
  0.5  = Somewhat helpful; provides some useful content but the user would still need significant additional help
  0.75 = Helpful; addresses the user's need well with only minor gaps
  1.0  = Highly helpful; directly and fully assists the user with clear, actionable, and complete content
```

#### Available Trace Fields

- `trace.input`: `str` — User input / query
- `trace.output`: `str` — Agent output / final response
- `trace.spans`: `List[amp_evaluation.trace.models.LLMSpan | amp_evaluation.trace.models.ToolSpan | amp_evaluation.trace.models.RetrieverSpan | amp_evaluation.trace.models.AgentSpan | amp_evaluation.trace.models.ChainSpan]` — All execution spans ordered by start time
- `trace.metrics`: `TraceMetrics` — Aggregated performance metrics
- `trace.format_evidence()`: `str` — Format tool results and retrieved documents for LLM-friendly display.
- `trace.format_spans()`: `str` — Render the full span tree using parent_span_id hierarchy.
- `trace.get_agents()`: `List[AgentSpan]` — Get all agent spans (for multi-agent systems).
- `trace.get_llm_calls()`: `List[LLMSpan]` — Get all LLM calls with enhanced filtering and deduplication.
- `trace.get_retrievals()`: `List[RetrieverSpan]` — Get all retrieval operations with agent filtering.
- `trace.get_tool_calls()`: `List[ToolSpan]` — Get all tool executions with agent filtering.

### LLM-Judge Template — agent-level (AgentTrace)

Called: Once per agent span — individual agent performance in multi-agent systems

Variable: `agent_trace` (AgentTrace)

```
You are an expert evaluator. Your sole criterion is TOOL USAGE: does the agent choose and use the right tools effectively to accomplish its goal?

Agent: {agent_trace.agent_name or 'agent'}
Model: {agent_trace.model}

Goal:
{agent_trace.input}

Final Response:
{agent_trace.output}

Tools Available: {', '.join(t.name for t in agent_trace.available_tools)}
Tools Used: {', '.join(s.tool_name for s in agent_trace.get_tool_steps())}
Total Steps: {len(agent_trace.steps)}

Evaluation Steps:
1. Were the right tools selected for the task? Did the agent use the most appropriate tools from what was available?
2. Were tool inputs well-formed and effective? Did the agent pass correct arguments to get useful results?
3. Were there unnecessary tool calls, redundant lookups, or tools that should have been used but weren't?
4. Did the agent use tool results effectively in its final response?

Scoring Rubric:
  0.0  = Tools used incorrectly or not at all despite being needed
  0.25 = Some tools used but with major errors in selection or usage
  0.5  = Tools used adequately but with unnecessary calls or missed opportunities
  0.75 = Good tool usage with only minor inefficiencies
  1.0  = Optimal tool usage; right tools, right inputs, no waste
```

#### Available AgentTrace Fields

- `agent_trace.input`: `str` — Agent input
- `agent_trace.output`: `str` — Agent output
- `agent_trace.steps`: `List[UserInputStep | LLMReasoningStep | ToolExecutionStep]` — Execution steps: UserInputStep, LLMReasoningStep, or ToolExecutionStep
- `agent_trace.agent_name`: `str` — Name of the agent
- `agent_trace.model`: `str` — LLM model used by the agent
- `agent_trace.system_prompt`: `str` — System prompt / instructions
- `agent_trace.available_tools`: `List[ToolDefinition]` — Tools available to the agent
- `agent_trace.metrics`: `TraceMetrics` — Aggregated performance metrics
- `agent_trace.format_steps()`: `str` — Format execution steps as a numbered list for LLM-friendly display.
- `agent_trace.get_error_steps()`: `List[ToolExecutionStep]` — Get tool steps that produced errors.
- `agent_trace.get_llm_steps()`: `List[LLMReasoningStep]` — Get all LLM output steps (both intermediate reasoning and final response).
- `agent_trace.get_sub_agents()`: `List[AgentTrace]` — Get all sub-agent traces from nested tool executions.
- `agent_trace.get_tool_steps()`: `List[ToolExecutionStep]` — Get all tool execution steps.

### LLM-Judge Template — llm-level (LLMSpan)

Called: Once per LLM call — per-call quality (safety, coherence, etc.)

Variable: `llm_span` (LLMSpan)

```
You are an expert evaluator. Your sole criterion is COHERENCE: is this LLM response well-structured, logical, and easy to follow?

Model: {llm_span.model}
Vendor: {llm_span.vendor}
Messages in conversation: {len(llm_span.input)}

LLM Response:
{llm_span.output}

Evaluation Steps:
1. Does the response have a clear structure with logical flow from one point to the next?
2. Are ideas connected coherently, or are there abrupt jumps or contradictions?
3. Is the level of detail appropriate and consistent throughout?
4. Would a reader understand the response on first reading without confusion?

Scoring Rubric:
  0.0  = Incoherent; disorganized, contradictory, or impossible to follow
  0.25 = Poorly structured; significant logical gaps or confusing organization
  0.5  = Understandable but with structural issues or unclear passages
  0.75 = Well-structured and clear with only minor areas that could be tighter
  1.0  = Exceptionally coherent; perfectly organized, logical, and easy to follow
```

#### Available LLMSpan Fields

- `llm_span.input`: `List[SystemMessage | UserMessage | AssistantMessage | ToolMessage]` — Conversation messages sent to the LLM
- `llm_span.output`: `str` — LLM response text
- `llm_span.available_tools`: `List[ToolDefinition]` — Tools available to the LLM for this call
- `llm_span.model`: `str` — Model name (e.g. gpt-4o)
- `llm_span.vendor`: `str` — Model vendor (e.g. openai)
- `llm_span.temperature`: `float | None` — LLM temperature setting
- `llm_span.metrics`: `LLMMetrics` — LLM-specific performance metrics
- `llm_span.format_messages()`: `str` — Format conversation messages for LLM-friendly display.
- `llm_span.get_assistant_messages()`: `List[AssistantMessage]` — Get assistant messages only.
- `llm_span.get_system_messages()`: `List[SystemMessage]` — Get system messages only.
- `llm_span.get_tool_messages()`: `List[ToolMessage]` — Get tool result messages only.
- `llm_span.get_user_messages()`: `List[UserMessage]` — Get user messages only.

### EvalResult

Every evaluator must return an `EvalResult`.

```python
# Success — provide a score and explanation
EvalResult(score=0.85, explanation="Response covers 4 of 5 topics")

# With explicit pass/fail override (default: score >= 0.5 passes)
EvalResult(score=0.3, passed=False, explanation="Below threshold")

# Skip — when evaluation cannot be performed
EvalResult.skip("No output to evaluate")
```

**Rules:**
- `score`: float, 0.0 to 1.0 (mandatory). Higher is always better.
- `explanation`: str (recommended). Human-readable reason for the score.
- `passed`: bool (optional). Defaults to `score >= 0.5`.
- Use `EvalResult.skip(reason)` for missing data — do NOT return score=0.0.

## Common Mistakes

```python
# DON'T: Return score outside 0-1 range
EvalResult(score=5.0, ...)  # ValueError!

# DON'T: Return 0.0 for missing data — use skip
if not trace.output:
    return EvalResult(score=0.0, explanation="No output")  # Wrong
    return EvalResult.skip("No output to evaluate")         # Correct

# DON'T: Include scoring instructions in LLM-judge prompts
# The framework appends them automatically.
```
