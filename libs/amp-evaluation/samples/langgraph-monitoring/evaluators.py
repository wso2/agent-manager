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

These evaluators monitor whether the agent:
  - Calls the right tools for the user's request
  - Grounds its answers in actual tool results (no hallucinations)
  - Handles tool failures gracefully
  - Produces complete, useful responses
  - Doesn't fabricate specific facts (LLM-verified)

Usage:
    from amp_evaluation import Monitor

    monitor = Monitor(evaluators_dir="./evaluators")
    result = monitor.run(start_time="2025-07-01T00:00:00Z", end_time="2025-07-08T00:00:00Z")
"""

import json
import os
import re
from typing import Optional, Set

from amp_evaluation import Observation, EvalResult, Task, evaluator
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


# =============================================================================
# Helper Functions
# =============================================================================


def detect_query_domains(query: str) -> Set[str]:
    """Detect which domains the user's query relates to."""
    query_lower = query.lower()
    query_words = set(re.findall(r"\b[a-z]{3,}\b", query_lower))
    matched_domains = set()

    for domain, keywords in DOMAIN_KEYWORDS.items():
        if keywords & query_words:
            matched_domains.add(domain)
        # Also check multi-word keywords as substrings
        for kw in keywords:
            if " " in kw and kw in query_lower:
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


def collect_tool_outputs(observation: Observation) -> str:
    """Collect all tool outputs as a single lowercase string for matching."""
    parts = []
    for span in observation.tool_spans:
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
# Evaluator: Tool Call Relevance
# =============================================================================


@evaluator(
    name="tool-call-relevance",
    description="Are the right tools being called for the user's request?",
    tags=["tool-use", "quality"],
    aggregations=[AggregationType.MEAN, AggregationType.PASS_RATE],
)
def tool_call_relevance(observation: Observation, task: Optional[Task] = None) -> EvalResult:
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
    query = observation.input or ""
    called_tools = [span.name for span in observation.tool_spans]

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


# =============================================================================
# Evaluator: Response Grounding
# =============================================================================


@evaluator(
    name="response-grounding",
    description="Is the response based on actual tool results, or did the agent make things up?",
    tags=["hallucination", "grounding", "quality"],
    aggregations=[AggregationType.MEAN, AggregationType.MIN, AggregationType.PASS_RATE],
)
def response_grounding(observation: Observation, task: Optional[Task] = None) -> EvalResult:
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
    response = observation.output or ""

    if not observation.tool_spans:
        return EvalResult(
            score=1.0,
            passed=True,
            explanation="No tools called — skipping grounding check",
        )

    tool_outputs = collect_tool_outputs(observation)
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
            "tools_used": [span.name for span in observation.tool_spans],
        },
    )


# =============================================================================
# Evaluator: Tool Success Rate
# =============================================================================


@evaluator(
    name="tool-success-rate",
    description="Did the tools execute without errors?",
    tags=["tool-use", "reliability"],
    aggregations=[AggregationType.MEAN, AggregationType.MIN],
)
def tool_success_rate(observation: Observation, task: Optional[Task] = None) -> EvalResult:
    """
    Checks whether tools executed successfully. A failing tool often means
    the agent will hallucinate a response or give a generic error message.

    Monitors this to catch:
      - API outages in downstream services
      - Authentication/permission issues
      - Bad input being passed to tools

    Scoring:
      - 1.0 = all tools succeeded
      - 0.0 = all tools failed
    """
    if not observation.tool_spans:
        return EvalResult(score=1.0, passed=True, explanation="No tools called")

    failed = []
    succeeded = []

    for span in observation.tool_spans:
        if span.metrics and span.metrics.error:
            failed.append(span.name)
        else:
            succeeded.append(span.name)

    total = len(failed) + len(succeeded)
    score = len(succeeded) / total

    return EvalResult(
        score=score,
        passed=len(failed) == 0,
        explanation=f"{len(succeeded)}/{total} tools succeeded" + (f" — failed: {', '.join(failed)}" if failed else ""),
        details={"failed_tools": failed, "succeeded_tools": succeeded},
    )


# =============================================================================
# Evaluator: Response Completeness
# =============================================================================


@evaluator(
    name="response-completeness",
    description="Is the response complete and actionable, or is it cut off / broken?",
    tags=["quality", "output"],
    aggregations=[AggregationType.PASS_RATE],
)
def response_completeness(observation: Observation, task: Optional[Task] = None) -> EvalResult:
    """
    Checks structural quality of the response. Catches truncated responses,
    empty outputs, and system error messages leaking to the user.

    Does NOT judge content quality — use llm-hallucination-judge for that.

    Scoring:
      - 1.0 = response looks structurally complete
      - 0.0 = empty or severely broken response
    """
    response = (observation.output or "").strip()
    issues = []

    # Empty response — this is always bad
    if not response:
        return EvalResult(
            score=0.0,
            passed=False,
            explanation="Empty response",
            details={"length": 0, "issues": ["empty"]},
        )

    # Too short to be useful
    if len(response) < COMPLETENESS_MIN_LENGTH:
        issues.append(f"Very short response ({len(response)} chars)")

    # Truncation — only meaningful for longer responses
    if len(response) > 50:
        # Check if response ends mid-sentence (no terminal punctuation)
        last_char = response[-1]
        if last_char not in ".!?\"')]}":
            # Could be truncated, but not definitive
            # Check for explicit truncation markers
            if response.endswith(("...", "…", "[truncated]", "[cut")):
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
# Evaluator: LLM Hallucination Judge
# =============================================================================


@evaluator(
    name="llm-hallucination-judge",
    description="Uses an LLM to detect hallucinations the regex-based checks might miss",
    tags=["hallucination", "llm-judge", "quality"],
    aggregations=[AggregationType.MEAN, AggregationType.MIN, AggregationType.PASS_RATE],
)
def llm_hallucination_judge(observation: Observation, task: Optional[Task] = None) -> EvalResult:
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
    if not observation.tool_spans:
        return EvalResult(
            score=1.0,
            passed=True,
            explanation="No tools called — nothing to verify against",
        )

    api_key = os.getenv(JUDGE_API_KEY_ENV)
    if not api_key:
        return EvalResult(
            score=0.0,
            passed=False,
            explanation=f"{JUDGE_API_KEY_ENV} not set — cannot run LLM judge",
            details={"skipped": True, "reason": "missing_api_key"},
        )

    # Format tool outputs, with a size cap to control costs
    tool_sections = []
    total_chars = 0
    max_chars = 3000

    for span in observation.tool_spans:
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
{observation.input}

TOOL RESULTS:
{chr(10).join(tool_sections)}

AGENT RESPONSE:
{observation.output}

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
        return EvalResult(
            score=0.0,
            passed=False,
            explanation="openai package not installed — run: pip install openai",
            details={"skipped": True, "reason": "missing_dependency"},
        )
    except json.JSONDecodeError:
        return EvalResult(
            score=0.0,
            passed=False,
            explanation="LLM returned invalid JSON — could not parse judge response",
            details={"skipped": True, "reason": "invalid_json"},
        )
    except Exception as e:
        return EvalResult(
            score=0.0,
            passed=False,
            explanation=f"LLM judge failed: {type(e).__name__}: {e}",
            details={"skipped": True, "reason": "llm_error", "error": str(e)},
        )
