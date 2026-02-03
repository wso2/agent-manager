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
Agent invoker abstractions for experiment evaluation.

This module provides base classes for invoking agents during experiments.
Users extend AgentInvoker to implement their specific agent invocation logic.

Two invocation patterns:

1. **HTTP Agents (most common)**:
   - Framework handles trace collection automatically via baggage propagation
   - Time-based bulk trace fetching after all invocations complete
   - Automatic trace-to-task matching using baggage attributes
   - Just return the output, framework handles the rest

2. **In-Process Agents (rare)**:
   - Agent runs in same process with direct instrumentation access
   - Can return trajectory directly from instrumentation
   - Skips trace fetching step

Example (HTTP Agent):
    invoker = HttpAgentInvoker(
        base_url="http://localhost:8000",
        endpoint="/chat",
        http_client=requests  # or httpx, etc.
    )

    # Framework automatically:
    # 1. Propagates task_id and trial_id via baggage
    # 2. Batch-fetches traces after all invocations
    # 3. Matches traces to tasks using baggage attributes

Example (In-Process Agent):
    class InProcessInvoker(AgentInvoker):
        def invoke(self, task: Task) -> InvokeResult:
            response = my_agent.run(task.input)
            trajectory = get_current_trajectory()  # From instrumentation
            return InvokeResult(output=response, trajectory=trajectory)
"""

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Optional, List, Dict, Any, Protocol, Callable
import logging

from .models import Task
from .trace import Trajectory


logger = logging.getLogger(__name__)


class HttpClient(Protocol):
    """Protocol for HTTP client (requests, httpx, etc.)."""

    def post(self, url: str, **kwargs) -> Any:
        """Make a POST request."""
        ...


@dataclass
class InvokeResult:
    """
    Result from invoking an agent during evaluation.

    For HTTP agents: Framework handles trace fetching automatically via
    time-based bulk fetching and baggage-based matching. Just return output.

    For in-process agents: Can optionally return trajectory directly to skip
    trace fetching step.

    Attributes:
        output: The agent's response (string, dict, or any serializable type)
        error: Error message if invocation failed (None if successful)
        trajectory: Direct Trajectory object (optional, for in-process agents)
        metadata: Optional metadata about the invocation
    """

    output: Any = None
    error: Optional[str] = None
    trajectory: Optional[Trajectory] = None
    metadata: Dict[str, Any] = field(default_factory=dict)

    @property
    def success(self) -> bool:
        """Check if invocation was successful."""
        return self.error is None

    @property
    def has_trajectory(self) -> bool:
        """Check if trajectory is directly available (skips trace fetching)."""
        return self.trajectory is not None


class AgentInvoker(ABC):
    """
    Abstract base class for agent invocation during experiments.

    Extend this class and implement `invoke()` to define how your agent
    is called during evaluation.

    The framework handles trace collection automatically via:
    - OpenTelemetry baggage propagation (task_id, trial_id)
    - Time-based bulk trace fetching after invocations
    - Automatic trace-to-task matching

    You only need to invoke the agent and return the output.

    Example:
        class MyHTTPAgentInvoker(AgentInvoker):
            def __init__(self, endpoint: str):
                self.endpoint = endpoint

            def invoke(self, task: Task) -> InvokeResult:
                response = requests.post(f"{self.endpoint}/chat", json=task.input)
                return InvokeResult(output=response.json())
    """

    @abstractmethod
    def invoke(self, task: Task) -> InvokeResult:
        """
        Invoke the agent with a task.

        Implement this method to define how your agent is called.

        Args:
            task: The task containing input and metadata

        Returns:
            InvokeResult with output and optionally error or trajectory
        """
        pass

    def invoke_batch(self, tasks: List[Task]) -> List[InvokeResult]:
        """
        Invoke the agent with multiple tasks.

        Default implementation calls invoke() for each task sequentially.
        Override for parallel execution or batch optimization.

        Args:
            tasks: List of tasks to invoke

        Returns:
            List of InvokeResults in same order as tasks
        """
        results = []
        for task in tasks:
            try:
                result = self.invoke(task)
                results.append(result)
            except Exception as e:
                logger.error(f"Task {task.task_id} invocation failed: {e}")
                results.append(InvokeResult(error=str(e)))
        return results


class HttpAgentInvoker(AgentInvoker):
    """
    HTTP-based agent invoker with flexible client support.

    Supports any HTTP client that follows the protocol (requests, httpx, etc.).
    The framework automatically propagates task_id and trial_id via baggage.

    Args:
        base_url: Base URL of the agent endpoint (e.g., "http://localhost:8000")
        endpoint: API endpoint path (e.g., "/chat" or "/invoke")
        http_client: HTTP client module (requests, httpx, etc.)
        timeout: Request timeout in seconds (default: 60)
        headers: Additional HTTP headers to include in all requests
        payload_builder: Optional function to build request payload from task.input
                        Default: uses task.input as-is if dict, else wraps as {"input": task.input}

    Example (with requests):
        import requests
        invoker = HttpAgentInvoker(
            base_url="http://localhost:8000",
            endpoint="/chat",
            http_client=requests
        )

    Example (with httpx):
        import httpx
        client = httpx.Client()
        invoker = HttpAgentInvoker(
            base_url="http://localhost:8000",
            endpoint="/chat",
            http_client=client
        )

    Example (with custom payload builder):
        def build_payload(task_input):
            return {
                "messages": [{"role": "user", "content": task_input}],
                "temperature": 0.7
            }

        invoker = HttpAgentInvoker(
            base_url="http://localhost:8000",
            endpoint="/chat",
            http_client=requests,
            payload_builder=build_payload
        )
    """

    def __init__(
        self,
        base_url: str,
        endpoint: str,
        http_client: Any,
        timeout: float = 60.0,
        headers: Optional[Dict[str, str]] = None,
        payload_builder: Optional[Callable[[Any], Dict[str, Any]]] = None,
    ):
        self.base_url = base_url.rstrip("/")
        self.endpoint = endpoint if endpoint.startswith("/") else f"/{endpoint}"
        self.http_client = http_client
        self.timeout = timeout
        self.headers = headers or {}
        self.payload_builder = payload_builder or self._default_payload_builder

    def _default_payload_builder(self, task_input: Any) -> Dict[str, Any]:
        """Default payload builder: use input as-is if dict, else wrap it."""
        if isinstance(task_input, dict):
            return task_input
        return {"input": task_input}

    def invoke(self, task: Task) -> InvokeResult:
        """
        Invoke HTTP agent with task.

        The framework automatically propagates task_id and trial_id via
        OpenTelemetry baggage in HTTP headers.
        """
        url = f"{self.base_url}{self.endpoint}"
        payload = self.payload_builder(task.input)

        # Merge headers (default + user headers)
        request_headers = {"Content-Type": "application/json", **self.headers}

        try:
            response = self.http_client.post(url, json=payload, headers=request_headers, timeout=self.timeout)

            # Check for HTTP errors
            if hasattr(response, "raise_for_status"):
                response.raise_for_status()

            # Parse response
            if hasattr(response, "json"):
                output = response.json()
            else:
                output = response

            return InvokeResult(output=output)

        except Exception as e:
            logger.error(f"HTTP invocation failed for task {task.task_id}: {e}")
            return InvokeResult(error=str(e))
