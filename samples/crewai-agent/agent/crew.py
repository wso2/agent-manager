"""A two-agent CrewAI crew.

A researcher (with one tool) and an editor run two sequential tasks. The
researcher answers the question using the lookup tool, and the editor shortens
the answer. Each /chat call runs both agents and the model behind them, so a
single request produces a small multi-agent trace. Pinned to crewai 1.15.34 for
dependency compatibility.
"""
from __future__ import annotations

from crewai import Agent, Crew, Process, Task
from crewai.tools import tool


@tool("country_capital_lookup")
def country_capital_lookup(country: str) -> str:
    """Return the capital city of a country."""
    return f"The capital of {country} is Paris."


def create_crew() -> Crew:
    researcher = Agent(
        role="Geography researcher",
        goal="Answer the user's question, using the lookup tool for capitals.",
        backstory="You call the lookup tool to answer geography questions.",
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
        description="Answer the user's question: {question}",
        expected_output="A sentence answering the question.",
        agent=researcher,
    )
    edit_task = Task(
        description="Shorten the researcher's previous answer to one short sentence.",
        expected_output="One short sentence.",
        agent=editor,
        context=[research_task],
    )

    return Crew(
        agents=[researcher, editor],
        tasks=[research_task, edit_task],
        process=Process.sequential,
        verbose=False,
    )
