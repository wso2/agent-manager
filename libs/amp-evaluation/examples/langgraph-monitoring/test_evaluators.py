#!/usr/bin/env python3
"""
Test evaluators locally with sample trace data.

Setup:
    1. Copy .env.example to .env
    2. Add your OPENAI_API_KEY to test LLM-as-judge evaluator
    3. Run: python test_evaluators.py

Run: python test_evaluators.py
"""

# Load .env if available
try:
    from dotenv import load_dotenv

    load_dotenv()
except ImportError:
    pass

# Import evaluators to register them
import evaluators  # noqa: F401

from amp_evaluation import (
    Observation,
    Trajectory,
    ToolSpan,
    ToolMetrics,
    get_evaluator,
    list_evaluators,
)


def create_sample_trace_good() -> Trajectory:
    """Create a trace where agent behaves well - uses right tools, no hallucination."""
    return Trajectory(
        trace_id="trace-good-001",
        input="I need to search for flights from NYC to LA on March 15",
        output="I found several flights from NYC to LA on March 15. The best option is flight AA123 departing at 10:30 AM for $299. Would you like me to book it?",
        steps=[
            ToolSpan(
                span_id="tool-1",
                name="search_flights",
                arguments={"from": "NYC", "to": "LA", "date": "2024-03-15"},
                result='{"flights": [{"id": "AA123", "departure": "10:30 AM", "price": "$299"}]}',
                metrics=ToolMetrics(duration_ms=150.0, error=False),
            )
        ],
    )


def create_sample_trace_hallucinated() -> Trajectory:
    """Create a trace where agent hallucinates - invents data not from tools."""
    return Trajectory(
        trace_id="trace-hallucinated-001",
        input="What flights are available to Boston?",
        output="I found a great flight to Boston! Flight UA789 departs at 2:00 PM for only $199 with complimentary meals included.",
        steps=[
            ToolSpan(
                span_id="tool-1",
                name="search_flights",
                arguments={"to": "Boston"},
                result='{"flights": [{"id": "DL456", "departure": "9:00 AM", "price": "$350"}]}',
                metrics=ToolMetrics(duration_ms=200.0, error=False),
            )
        ],
    )


def create_sample_trace_wrong_tool() -> Trajectory:
    """Create a trace where agent uses irrelevant tools."""
    return Trajectory(
        trace_id="trace-wrong-tool-001",
        input="What is your refund policy?",
        output="Our refund policy allows cancellations up to 24 hours before departure.",
        steps=[
            ToolSpan(
                span_id="tool-1",
                name="search_hotels",  # Wrong tool for policy question!
                arguments={"query": "refund"},
                result='{"hotels": []}',
                metrics=ToolMetrics(duration_ms=100.0, error=False),
            )
        ],
    )


def test_evaluators():
    print("=" * 60)
    print("Testing Custom Evaluators")
    print("=" * 60)
    print()

    # Show registered evaluators
    print("Registered evaluators:")
    for name in list_evaluators():
        print(f"  - {name}")
    print()

    # Test cases
    test_cases = [
        ("Good trace (correct tools, grounded response)", create_sample_trace_good()),
        ("Hallucinated trace (invents data)", create_sample_trace_hallucinated()),
        ("Wrong tool trace (irrelevant tool)", create_sample_trace_wrong_tool()),
    ]

    # Evaluators to test
    evaluator_names = [
        "tool-call-relevance",
        "response-grounding",
        "tool-success-rate",
        "response-completeness",
        "llm-hallucination-judge",  # LLM-as-judge (requires OPENAI_API_KEY)
    ]

    for test_name, trace in test_cases:
        print("-" * 60)
        print(f"Test: {test_name}")
        print(f"  Input: {trace.input[:50]}...")
        print(f"  Output: {trace.output[:50]}...")
        print(f"  Tools: {[s.name for s in trace.tool_spans]}")
        print()

        observation = Observation(trajectory=trace)

        for eval_name in evaluator_names:
            try:
                evaluator = get_evaluator(eval_name)
                result = evaluator.evaluate(observation)
                status = "✓ PASS" if result.passed else "✗ FAIL"
                print(f"  {eval_name}: {result.score:.2f} {status}")
                print(f"    → {result.explanation}")
            except ValueError:
                print(f"  {eval_name}: (not registered)")
        print()

    print("=" * 60)
    print("Done!")


if __name__ == "__main__":
    test_evaluators()
