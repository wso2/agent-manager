"""Multi-agent CrewAI sample for the matrix.

Two agents — a researcher with one tool, and an editor without tools — running
two sequential tasks. Exercises the agent (×2), crewaitask (×2), llm (×N) and
tool span kinds in a single trace. Deterministic: temperature is fixed by the
default crew settings + the tool returns a constant string.

Cassette: cassettes/crewai/multi_agent.yaml.
"""
from __future__ import annotations

import os


def run_scenario() -> str:
    # Disable CrewAI's hosted-tracing upload + its interactive trace-prompt so
    # recording stays non-interactive and the cassette never captures calls to
    # app.crewai.com (which would embed a one-time access code).
    os.environ.setdefault("CREWAI_TRACING_ENABLED", "false")
    os.environ.setdefault("CREWAI_DISABLE_TRACING_PROMPT", "true")
    # Use LiteLLM's bundled model cost map instead of fetching from GitHub.
    # Otherwise the cell makes an HTTP call to raw.githubusercontent.com on
    # cold start, which isn't in the cassette and breaks replay.
    os.environ.setdefault("LITELLM_LOCAL_MODEL_COST_MAP", "True")

    from crewai import Agent, Crew, Process, Task
    from crewai.tools import tool

    @tool("country_capital_lookup")
    def country_capital_lookup(country: str) -> str:
        """Return the capital city of a country."""
        return f"The capital of {country} is Paris."

    researcher = Agent(
        role="Geography researcher",
        goal="Look up the capital of a country using the lookup tool.",
        backstory="You always call the lookup tool to answer geography questions.",
        llm="gpt-4o-mini",
        tools=[country_capital_lookup],
        allow_delegation=False,
        verbose=False,
    )
    editor = Agent(
        role="Editor",
        goal="Shorten the researcher's answer to a single short sentence.",
        backstory="You compress answers to the minimum useful form.",
        llm="gpt-4o-mini",
        allow_delegation=False,
        verbose=False,
    )

    research_task = Task(
        description="Look up the capital of France using the tool.",
        expected_output="A sentence stating the capital of France.",
        agent=researcher,
    )
    edit_task = Task(
        description=(
            "Shorten the researcher's previous answer to a single short sentence."
        ),
        expected_output="One short sentence naming the capital.",
        agent=editor,
        context=[research_task],
    )

    crew = Crew(
        agents=[researcher, editor],
        tasks=[research_task, edit_task],
        process=Process.sequential,
        verbose=False,
    )
    result = crew.kickoff()
    return str(result)
