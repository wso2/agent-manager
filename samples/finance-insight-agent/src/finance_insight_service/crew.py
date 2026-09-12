from crewai import Agent, Crew, Process, Task
from crewai.project import CrewBase, agent, crew, task
from crewai_tools import ScrapeWebsiteTool

from finance_insight_service.tools.company_fundamentals_fetch import (
    CompanyFundamentalsFetchTool,
)
from finance_insight_service.tools.safe_python_exec import SafePythonExecTool
from finance_insight_service.tools.price_history_fetch import PriceHistoryFetchTool
from finance_insight_service.tools.serpapi_news_search import SerpApiNewsSearchTool


@CrewBase
class FinanceInsightCrew:
    """Research + quant crew for finance insight service."""

    agents_config = "config/agents.yaml"
    tasks_config = "config/tasks.yaml"

    def __init__(self, job_id: str | None = None) -> None:
        """Initialize the crew with an optional job identifier."""
        self.job_id = job_id

    @agent
    def researcher(self) -> Agent:
        """Create the research agent."""
        return Agent(
            config=self.agents_config["researcher"],
            tools=[SerpApiNewsSearchTool(), ScrapeWebsiteTool()],
            verbose=True,
            allow_delegation=False,
        )

    @agent
    def quant(self) -> Agent:
        """Create the quantitative analysis agent."""
        return Agent(
            config=self.agents_config["quant"],
            tools=[
                PriceHistoryFetchTool(),
                CompanyFundamentalsFetchTool(),
                SafePythonExecTool(),
            ],
            verbose=True,
            allow_delegation=False,
        )

    @agent
    def auditor(self) -> Agent:
        """Create the audit agent."""
        return Agent(
            config=self.agents_config["auditor"],
            verbose=True,
            allow_delegation=False,
        )

    @agent
    def reporter(self) -> Agent:
        """Create the report-writing agent."""
        return Agent(
            config=self.agents_config["reporter"],
            verbose=True,
            allow_delegation=False,
        )

    @task
    def research_task(self) -> Task:
        """Build the research task definition."""
        return Task(
            config=self.tasks_config["research_task"],
            agent=self.researcher(),
            name="research_task",
        )

    @task
    def quant_task(self) -> Task:
        """Build the quantitative analysis task definition."""
        return Task(
            config=self.tasks_config["quant_task"],
            agent=self.quant(),
            name="quant_task",
        )

    @task
    def audit_task(self) -> Task:
        """Build the audit task definition."""
        return Task(
            config=self.tasks_config["audit_task"],
            agent=self.auditor(),
            name="audit_task",
        )

    @task
    def report_task(self) -> Task:
        """Build the report task definition."""
        return Task(
            config=self.tasks_config["report_task"],
            agent=self.reporter(),
            name="report_task",
        )

    def build_crew(
        self, task_names: list[str] | None = None, include_all_agents: bool = True
    ) -> Crew:
        """Build a crew with selected tasks and agents."""
        research_task = self.research_task()
        quant_task = self.quant_task()
        audit_task = self.audit_task()
        report_task = self.report_task()

        full_order = [research_task, quant_task, audit_task, report_task]

        task_map = {
            "research": research_task,
            "quant": quant_task,
            "audit": audit_task,
            "report": report_task,
        }
        if task_names:
            unknown = [name for name in task_names if name not in task_map]
            if unknown:
                raise ValueError(f"Unknown task names: {', '.join(unknown)}")
            selected_tasks = [task_map[name] for name in task_names]
        else:
            selected_tasks = full_order

        if set(selected_tasks) == set(full_order):
            quant_task.context = [research_task]
            audit_task.context = [research_task, quant_task]
            report_task.context = [research_task, quant_task, audit_task]

        if include_all_agents:
            agents = [
                self.researcher(),
                self.quant(),
                self.auditor(),
                self.reporter(),
            ]
        else:
            selected_names = set(task_names or task_map.keys())
            agents = []
            if "research" in selected_names:
                agents.append(self.researcher())
            if "quant" in selected_names:
                agents.append(self.quant())
            if "audit" in selected_names:
                agents.append(self.auditor())
            if "report" in selected_names:
                agents.append(self.reporter())

        crew_name = (
            f"finance_insight_crew_{self.job_id}"
            if self.job_id
            else "finance_insight_crew"
        )

        return Crew(
            name=crew_name,
            agents=agents,
            tasks=selected_tasks,
            process=Process.sequential,
            verbose=True,
            tracing=True,
        )

    @crew
    def crew(self) -> Crew:
        """Creates the Finance Insight crew."""
        return self.build_crew()
