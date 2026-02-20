# Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.

"""
Evaluators for a customer support agent that uses tools to look up
bookings, search flights, check policies, etc.

Demonstrates all evaluator types:
  - Function-based evaluators at trace level (tool-call-relevance, response-grounding, etc.)
  - Function-based evaluators at trace level (tool-success-rate) and span level (llm-response-quality)
  - Class-based evaluator at agent level (agent-tool-efficiency)
  - Built-in evaluators registered in run_monitor.py (latency, hallucination, answer_relevancy)

Usage:
    from amp_evaluation import Monitor

    monitor = Monitor(evaluators_dir="./evaluators")
    result = monitor.run(start_time="2025-07-01T00:00:00Z", end_time="2025-07-08T00:00:00Z")
"""

import json
import os
import re
from typing import Optional, Set

from amp_evaluation import EvalResult, Task, evaluator, register_evaluator, EvaluationLevel
from amp_evaluation.evaluators import BaseEvaluator, Param
from amp_evaluation.trace import Trace, AgentTrace, LLMSpan
from amp_evaluation.aggregators import AggregationType

from dotenv import load_dotenv

load_dotenv()

# =============================================================================
# Configuration — Tool taxonomy for the customer support agent
# =============================================================================

# Group tools by domain. If a user asks about flights, we expect
# tools from the "flights" domain, not "hotels".
TOOL_DOMAINS = {
    "flights": {
        "search_flights",
        "fetch_user_flight_information",
        "cancel_ticket",
        "update_ticket_to_new_flight",
    },
    "hotels": {
        "search_hotels",
        "book_hotel",
        "update_hotel",
        "cancel_hotel",
    },
    "car_rentals": {
        "search_car_rentals",
        "book_car_rental",
        "update_car_rental",
        "cancel_car_rental",
    },
    "excursions": {
        "search_trip_recommendations",
        "book_excursion",
        "update_excursion",
        "cancel_excursion",
    },
    "policies": {
        "lookup_policy",
        "from_docs",
        "query",
    },
}

# Keywords that indicate which domain the user is asking about
DOMAIN_KEYWORDS = {
    "flights": {
        "flight",
        "fly",
        "flying",
        "airline",
        "plane",
        "airport",
        "departure",
        "arrival",
        "boarding",
        "ticket",
        "seat",
    },
    "hotels": {
        "hotel",
        "room",
        "stay",
        "accommodation",
        "resort",
        "check-in",
        "checkout",
        "lodge",
        "inn",
    },
    "car_rentals": {
        "car",
        "rental",
        "rent",
        "vehicle",
        "drive",
        "driving",
        "pickup",
        "suv",
        "sedan",
    },
    "excursions": {
        "excursion",
        "tour",
        "activity",
        "activities",
        "sightseeing",
        "attraction",
        "adventure",
        "things to do",
        "recommend",
    },
    "policies": {
        "policy",
        "rule",
        "allowed",
        "baggage",
        "luggage",
        "carry-on",
        "pet",
        "restriction",
        "fee",
        "penalty",
        "guidelines",
    },
}

# Queries that legitimately don't need tools
NO_TOOL_PATTERNS = re.compile(
    r"\b(hi|hello|hey|thanks|thank you|bye|goodbye|good morning|good evening)\b",
    re.IGNORECASE,
)


# LLM judge settings
JUDGE_MODEL = os.getenv("JUDGE_MODEL", "gpt-4o-mini")
JUDGE_API_KEY_ENV = "OPENAI_API_KEY"

# Thresholds
TOOL_RELEVANCE_THRESHOLD = 0.5
GROUNDING_THRESHOLD = 0.5
HALLUCINATION_THRESHOLD = 0.7
COMPLETENESS_MIN_LENGTH = 20

# Agent efficiency: tokens-per-tool-call above this is considered inefficient
TOKENS_PER_TOOL_THRESHOLD = 2000


# =============================================================================
# Helper Functions
# =============================================================================


def detect_query_domains(query: str) -> Set[str]:
    """Detect which domains the user's query relates to."""
    query_lower = query.lower()
    query_words = set(re.findall(r"\b[a-z]{3,}\b", query_lower))
    matched_domains = set()

    for domain, keywords in DOMAIN_KEYWORDS.items():
        # Single-word, alpha-only keywords: fast set intersection
        simple_keywords = {kw for kw in keywords if kw.isalpha()}
        if simple_keywords & query_words:
            matched_domains.add(domain)
        # Multi-word or hyphenated keywords: substring match
        for kw in keywords - simple_keywords:
            if kw in query_lower:
                matched_domains.add(domain)
    return matched_domains


def get_tool_domain(tool_name: str) -> str | None:
    """Look up which domain a tool belongs to."""
    for domain, tools in TOOL_DOMAINS.items():
        if tool_name in tools:
            return domain
    return None


def extract_query_intents(query: str) -> Set[str]:
    """Extract keywords from the user's query, normalized to lowercase."""
    # Split on whitespace and punctuation, keep words with 3+ characters
    words = set(re.findall(r"\b[a-z]{3,}\b", query.lower()))
    return words


def collect_tool_outputs(trace: Trace) -> str:
    """Collect all tool outputs as a single lowercase string for matching."""
    parts = []
    for span in trace.get_tool_calls():
        result = span.result
        if result is None:
            continue
        if isinstance(result, (dict, list)):
            parts.append(json.dumps(result))
        else:
            parts.append(str(result))
    return " ".join(parts).lower()


def extract_factual_claims(text: str) -> list:
    """
    Extract specific factual claims from text — numbers, dates, codes, prices.

    These are the kinds of things an agent should NOT make up.
    """
    patterns = {
        "price": r"\$[\d,]+(?:\.\d{2})?",
        "date": r"\b\d{1,2}[/-]\d{1,2}[/-]\d{2,4}\b",
        "time": r"\b\d{1,2}:\d{2}\s*(?:AM|PM|am|pm)?\b",
        "flight_code": r"\b[A-Z]{2}\d{3,4}\b",
        "confirmation": r"\b[A-Z0-9]{6,}\b",  # Booking codes like "ABC123"
        "duration": r"\b\d+\s*(?:hours?|minutes?|days?|nights?)\b",
        "percentage": r"\b\d+(?:\.\d+)?%",
        "phone": r"\b\d{3}[-.]?\d{3}[-.]?\d{4}\b",
    }

    claims = []
    for claim_type, pattern in patterns.items():
        matches = re.findall(pattern, text)
        for match in matches:
            claims.append({"value": match, "type": claim_type})

    # Deduplicate by value
    seen = set()
    unique_claims = []
    for claim in claims:
        if claim["value"].lower() not in seen:
            seen.add(claim["value"].lower())
            unique_claims.append(claim)

    return unique_claims


# =============================================================================
# TRACE-LEVEL EVALUATORS (function-based)
# =============================================================================


# --- Evaluator 1: Tool Call Relevance (trace-level, function-based) ---


@evaluator(
    name="tool-call-relevance",
    description="Are the right tools being called for the user's request?",
    tags=["tool-use", "quality"],
    aggregations=[AggregationType.MEAN, AggregationType.PASS_RATE],
)
def tool_call_relevance(trace: Trace, task: Optional[Task] = None) -> EvalResult:
    """
    Checks whether the tools the agent called belong to the right domain
    for the user's query.

    If a user asks about flights, we expect flight-related tools — not
    hotel or car rental tools. This catches routing errors where the agent
    misunderstands what the user wants.

    Scoring:
      - 1.0 = all tools are from relevant domains
      - 0.0 = all tools are from wrong domains
    """
    query = trace.input or ""
    called_tools = [span.name for span in trace.get_tool_calls()]

    if not called_tools:
        if NO_TOOL_PATTERNS.search(query):
            return EvalResult(score=1.0, passed=True, explanation="Greeting — no tools needed")
        return EvalResult(
            score=0.5,
            passed=True,
            explanation="No tools called — cannot assess relevance",
            details={"query": query[:200]},
        )

    expected_domains = detect_query_domains(query)

    # If we can't determine the domain from the query, we can't judge
    if not expected_domains:
        return EvalResult(
            score=1.0,
            passed=True,
            explanation="Could not determine expected domain from query — skipping",
            details={"query": query[:200], "tools_called": called_tools},
        )

    # Policy tools are always acceptable — the agent might check policies
    # regardless of the primary domain
    allowed_domains = expected_domains | {"policies"}

    relevant = []
    wrong_domain = []

    for tool in called_tools:
        tool_domain = get_tool_domain(tool)
        if tool_domain is None:
            # Unknown tool — don't penalize, might be a new tool
            relevant.append({"tool": tool, "domain": "unknown"})
        elif tool_domain in allowed_domains:
            relevant.append({"tool": tool, "domain": tool_domain})
        else:
            wrong_domain.append({"tool": tool, "domain": tool_domain})

    score = len(relevant) / len(called_tools)

    return EvalResult(
        score=score,
        passed=score >= TOOL_RELEVANCE_THRESHOLD,
        explanation=f"{len(relevant)}/{len(called_tools)} tools from expected domains"
        + (
            f" — wrong domain: {', '.join(t['tool'] + ' (' + t['domain'] + ')' for t in wrong_domain)}"
            if wrong_domain
            else ""
        ),
        details={
            "expected_domains": sorted(expected_domains),
            "relevant_tools": relevant,
            "wrong_domain_tools": wrong_domain,
        },
    )


# --- Evaluator 2: Response Grounding (trace-level, function-based) ---


@evaluator(
    name="response-grounding",
    description="Is the response based on actual tool results, or did the agent make things up?",
    tags=["hallucination", "grounding", "quality"],
    aggregations=[AggregationType.MEAN, AggregationType.MIN, AggregationType.PASS_RATE],
)
def response_grounding(trace: Trace, task: Optional[Task] = None) -> EvalResult:
    """
    Extracts specific claims from the response (prices, dates, flight numbers, etc.)
    and checks whether each claim appears in the tool outputs.

    This catches the most dangerous type of hallucination: fabricated specifics.
    A response saying "$450 on flight AA123 departing 3:30 PM" should have all
    of those values present in the tool results.

    Scoring:
      - 1.0 = all specific claims found in tool outputs
      - 0.0 = no claims found in tool outputs (likely hallucinated)
    """
    response = trace.output or ""

    if not trace.get_tool_calls():
        return EvalResult.skip(
            "No tools called — skipping grounding check",
            details={"response": response[:200]},
        )

    tool_outputs = collect_tool_outputs(trace)
    claims = extract_factual_claims(response)

    if not claims:
        return EvalResult(
            score=1.0,
            passed=True,
            explanation="No specific factual claims to verify",
        )

    grounded = []
    ungrounded = []

    for claim in claims:
        if claim["value"].lower() in tool_outputs:
            grounded.append(claim)
        else:
            ungrounded.append(claim)

    total = len(grounded) + len(ungrounded)
    score = len(grounded) / total

    return EvalResult(
        score=score,
        passed=score >= GROUNDING_THRESHOLD,
        explanation=f"{len(grounded)}/{total} claims grounded in tool outputs"
        + (f" — potentially fabricated: {', '.join(c['value'] for c in ungrounded[:3])}" if ungrounded else ""),
        details={
            "grounded": [c["value"] for c in grounded[:10]],
            "ungrounded": [{"value": c["value"], "type": c["type"]} for c in ungrounded[:10]],
            "tools_used": [span.name for span in trace.get_tool_calls()],
        },
    )


# --- Evaluator 3: Response Completeness (trace-level, function-based) ---


@evaluator(
    name="response-completeness",
    description="Is the response complete and actionable, or is it cut off / broken?",
    tags=["quality", "output"],
    aggregations=[AggregationType.PASS_RATE],
)
def response_completeness(trace: Trace, task: Optional[Task] = None) -> EvalResult:
    """
    Checks structural quality of the response. Catches truncated responses,
    empty outputs, and system error messages leaking to the user.

    Does NOT judge content quality — use llm-hallucination-judge for that.

    Scoring:
      - 1.0 = response looks structurally complete
      - 0.0 = empty or severely broken response
    """
    response = (trace.output or "").strip()
    issues = []

    # Empty response — this is always bad
    if not response:
        return EvalResult.skip(
            "Empty response — skipping completeness check",
            details={"length": 0, "issues": ["empty"]},
        )

    # Too short to be useful
    if len(response) < COMPLETENESS_MIN_LENGTH:
        issues.append(f"Very short response ({len(response)} chars)")

    # Truncation — only meaningful for longer responses
    if len(response) > 50:
        # Check for explicit truncation markers first
        if response.endswith(("...", "…", "[truncated]", "[cut")):
            issues.append("Response appears truncated")
        # Check if response ends mid-sentence (no terminal punctuation)
        last_char = response[-1]
        if last_char not in ".!?\"')]}":
            issues.append("Response appears truncated")

    # System errors leaking to the user (not legitimate refusals)
    system_error_patterns = [
        r"an?\s+error\s+occurred",
        r"something\s+went\s+wrong",
        r"internal\s+(?:server\s+)?error",
        r"please\s+try\s+again\s+later",
        r"service\s+(?:is\s+)?unavailable",
        r"connection\s+(?:timed?\s+out|refused)",
        r"unexpected\s+error",
    ]
    response_lower = response.lower()
    for pattern in system_error_patterns:
        if re.search(pattern, response_lower):
            issues.append("Contains system error message")
            break

    # Scoring
    if not issues:
        score = 1.0
    elif len(issues) == 1:
        score = 0.4
    else:
        score = 0.1

    return EvalResult(
        score=score,
        passed=len(issues) == 0,
        explanation="; ".join(issues) if issues else "Response looks complete",
        details={"length": len(response), "issues": issues},
    )


# =============================================================================
# SPAN-LEVEL EVALUATORS (function-based)
# =============================================================================


# --- Evaluator 4: Tool Success Rate (span-level, function-based) ---


@evaluator(
    name="tool-success-rate",
    description="Did each tool execute without errors?",
    tags=["tool-use", "reliability"],
    level="trace",
    aggregations=[AggregationType.MEAN, AggregationType.MIN],
)
def tool_success_rate(trace: Trace, task: Optional[Task] = None) -> EvalResult:
    """
    Evaluates all tool spans in the trace for success/failure.

    A failing tool often means the agent will hallucinate a response or
    give a generic error message. Monitors this to catch API outages,
    authentication issues, or bad input being passed to tools.

    Scoring:
      - 1.0 = all tools succeeded
      - 0.0 = one or more tools failed
    """
    tool_spans = trace.get_tool_calls()
    if not tool_spans:
        return EvalResult.skip("No tool calls found in trace")

    results = []
    for span in tool_spans:
        if span.metrics.error:
            results.append(
                EvalResult(
                    score=0.0,
                    passed=False,
                    explanation=f"Tool '{span.name}' failed: {span.metrics.error_message or 'unknown error'}",
                    details={
                        "tool": span.name,
                        "error": span.metrics.error_message,
                        "error_type": span.metrics.error_type,
                    },
                )
            )
        else:
            results.append(
                EvalResult(
                    score=1.0,
                    passed=True,
                    explanation=f"Tool '{span.name}' succeeded",
                    details={
                        "tool": span.name,
                        "duration_ms": span.metrics.duration_ms,
                        "has_result": span.result is not None,
                    },
                )
            )

    avg_score = sum(r.score for r in results) / len(results)
    all_passed = all(r.passed for r in results)
    succeeded = sum(1 for r in results if r.passed)
    return EvalResult(
        score=avg_score,
        passed=all_passed,
        explanation=f"{succeeded}/{len(results)} tools succeeded",
    )


# --- Evaluator 5: LLM Response Quality (span-level, function-based) ---


@evaluator(
    name="llm-response-quality",
    description="Is each LLM call producing a valid, non-empty response?",
    tags=["quality", "llm"],
    level="span",
    aggregations=[AggregationType.MEAN, AggregationType.MIN],
)
def llm_response_quality(span: LLMSpan, task: Optional[Task] = None) -> EvalResult:
    """
    Evaluates each LLM span individually for basic response quality.

    Checks:
      - LLM did not error
      - Response is not empty
      - Response has reasonable length (not just a single token)

    Scoring:
      - 1.0 = good response
      - 0.5 = response present but very short
      - 0.0 = error or empty response
    """
    if span.metrics.error:
        return EvalResult(
            score=0.0,
            passed=False,
            explanation=f"LLM error: {span.metrics.error_message or 'unknown error'}",
            details={
                "model": span.model,
                "error": span.metrics.error_message,
            },
        )

    if not span.response and not span.tool_calls:
        return EvalResult(
            score=0.0,
            passed=False,
            explanation="Empty LLM response (no text and no tool calls)",
            details={"model": span.model},
        )

    # Tool calls without text response is fine (agent deciding to use a tool)
    if span.tool_calls and not span.response:
        return EvalResult(
            score=1.0,
            passed=True,
            explanation=f"LLM invoked {len(span.tool_calls)} tool(s)",
            details={
                "model": span.model,
                "tool_calls": [tc.name for tc in span.tool_calls],
            },
        )

    # Very short text response
    if len(span.response) < 10:
        return EvalResult(
            score=0.5,
            passed=True,
            explanation=f"Very short LLM response ({len(span.response)} chars)",
            details={
                "model": span.model,
                "response_length": len(span.response),
            },
        )

    return EvalResult(
        score=1.0,
        passed=True,
        explanation=f"LLM response OK ({len(span.response)} chars)",
        details={
            "model": span.model,
            "response_length": len(span.response),
            "has_tool_calls": bool(span.tool_calls),
        },
    )


# =============================================================================
# AGENT-LEVEL EVALUATOR (class-based)
# =============================================================================


# --- Evaluator 6: Agent Tool Efficiency (agent-level, class-based) ---


class AgentToolEfficiency(BaseEvaluator):
    """
    Checks if the agent uses tools efficiently relative to its token budget.

    An agent that burns 5000 tokens per tool call might be over-explaining,
    retrying unnecessarily, or getting stuck in loops. This evaluator flags
    agents that are token-heavy relative to their tool usage.

    Scoring:
      - 1.0 = efficient (reasonable tokens per tool call)
      - 0.0 = very inefficient (excessive tokens per tool)
    """

    name = "agent-tool-efficiency"
    description = "Does the agent use tools efficiently relative to its token budget?"
    tags = ["agent", "efficiency", "tool-use"]

    level = Param(EvaluationLevel, default=EvaluationLevel.AGENT, description="Evaluation level")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)

    def _trace_evaluation(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        tool_count = len(trace.get_tool_calls())
        total_tokens = trace.metrics.token_usage.total_tokens
        return self._score(tool_count, total_tokens, "trace")

    def _agent_evaluation(self, agent_trace: AgentTrace, task: Optional[Task] = None) -> EvalResult:
        tool_count = agent_trace.metrics.tool_call_count
        total_tokens = agent_trace.metrics.token_usage.total_tokens
        label = agent_trace.agent_name or agent_trace.agent_id
        return self._score(tool_count, total_tokens, label)

    def _score(self, tool_count: int, total_tokens: int, label: str) -> EvalResult:
        if tool_count == 0:
            return EvalResult(
                score=1.0,
                passed=True,
                explanation=f"{label}: No tools called — nothing to measure",
                details={"label": label, "tool_count": 0, "total_tokens": total_tokens},
            )

        if total_tokens == 0:
            return EvalResult(
                score=1.0,
                passed=True,
                explanation=f"{label}: No token data available",
                details={"label": label, "tool_count": tool_count, "total_tokens": 0},
            )

        tokens_per_tool = total_tokens / tool_count
        # Score: 1.0 if under threshold, decays linearly above it
        score = min(1.0, TOKENS_PER_TOOL_THRESHOLD / max(tokens_per_tool, 1))

        return EvalResult(
            score=round(score, 3),
            passed=score >= 0.5,
            explanation=f"{label}: {tokens_per_tool:.0f} tokens/tool ({tool_count} tools, {total_tokens} tokens)",
            details={
                "label": label,
                "tool_count": tool_count,
                "total_tokens": total_tokens,
                "tokens_per_tool": round(tokens_per_tool, 1),
                "threshold": TOKENS_PER_TOOL_THRESHOLD,
            },
        )


# Register the class-based evaluator to the global registry
register_evaluator(AgentToolEfficiency())


# =============================================================================
# TRACE-LEVEL LLM JUDGE (function-based)
# =============================================================================


# --- Evaluator 7: LLM Hallucination Judge (trace-level, function-based) ---


@evaluator(
    name="llm-hallucination-judge",
    description="Uses an LLM to detect hallucinations the regex-based checks might miss",
    tags=["hallucination", "llm-judge", "quality"],
    aggregations=[AggregationType.MEAN, AggregationType.MIN, AggregationType.PASS_RATE],
)
def llm_hallucination_judge(trace: Trace, task: Optional[Task] = None) -> EvalResult:
    """
    Sends the agent's response and tool outputs to an LLM to check for
    hallucinated information.

    This complements response-grounding: while that evaluator catches
    fabricated numbers and codes, this one catches semantic hallucinations
    like invented policies, wrong explanations, or made-up procedures.

    Requires OPENAI_API_KEY environment variable. Set JUDGE_MODEL above
    to change the model.

    Scoring:
      - 1.0 = fully grounded in tool results
      - 0.0 = heavily hallucinated
    """

    if not trace.get_tool_calls():
        return EvalResult(
            score=1.0,
            passed=True,
            explanation="No tools called — nothing to verify against",
        )

    api_key = os.getenv(JUDGE_API_KEY_ENV)
    if not api_key:
        return EvalResult.skip(
            f"{JUDGE_API_KEY_ENV} not set — cannot run LLM judge",
            details={"skipped": True, "reason": "missing_api_key"},
        )

    # Format tool outputs, with a size cap to control costs
    tool_sections = []
    total_chars = 0
    max_chars = 3000

    for span in trace.get_tool_calls():
        result = span.result
        if result is None:
            result_str = "(no result returned)"
        elif isinstance(result, (dict, list)):
            result_str = json.dumps(result, indent=2)
        else:
            result_str = str(result)

        if total_chars + len(result_str) > max_chars:
            result_str = result_str[: max_chars - total_chars] + "\n...[truncated]"
            tool_sections.append(f"Tool: {span.name}\nResult:\n{result_str}")
            break

        tool_sections.append(f"Tool: {span.name}\nResult:\n{result_str}")
        total_chars += len(result_str)

    prompt = f"""You are a hallucination detector for a customer support AI agent.

The agent received a user query, called some tools, got results, and produced a response.
Your job: check if the response contains claims that are NOT supported by the tool results.

Focus on:
- Specific facts: prices, dates, times, flight numbers, policies
- Procedures or steps that aren't backed by tool data
- Invented availability, status, or eligibility claims

Do NOT penalize:
- Polite language, greetings, or conversational filler
- Reasonable inferences clearly derived from tool data
- Standard disclaimers or "please contact support" suggestions

USER QUERY:
{trace.input}

TOOL RESULTS:
{chr(10).join(tool_sections)}

AGENT RESPONSE:
{trace.output}

Respond with JSON:
{{
  "score": <0.0 to 1.0, where 1.0 = fully grounded, 0.0 = heavily hallucinated>,
  "explanation": "<one sentence>",
  "hallucinated_claims": ["<specific claim not in tool results>"] or []
}}"""

    try:
        from openai import OpenAI

        client = OpenAI(api_key=api_key)
        response = client.chat.completions.create(
            model=JUDGE_MODEL,
            messages=[{"role": "user", "content": prompt}],
            temperature=0.0,
            max_tokens=300,
            response_format={"type": "json_object"},
        )

        parsed = json.loads(response.choices[0].message.content)
        score = max(0.0, min(1.0, float(parsed.get("score", 0.5))))

        return EvalResult(
            score=score,
            passed=score >= HALLUCINATION_THRESHOLD,
            explanation=parsed.get("explanation", "No explanation provided"),
            details={
                "hallucinated_claims": parsed.get("hallucinated_claims", []),
                "model": JUDGE_MODEL,
            },
        )

    except ImportError:
        return EvalResult.skip(
            "openai package not installed — run: pip install openai",
            details={"skipped": True, "reason": "missing_dependency"},
        )
    except json.JSONDecodeError:
        return EvalResult.skip(
            "LLM returned invalid JSON — could not parse judge response",
            details={"skipped": True, "reason": "invalid_json"},
        )
    except Exception as e:
        return EvalResult.skip(
            f"LLM judge failed: {type(e).__name__}: {e}",
            details={"skipped": True, "reason": "llm_error", "error": str(e)},
        )
