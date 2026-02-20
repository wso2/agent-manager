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

Example (HTTP Agent - simple):
    invoker = HttpAgentInvoker(
        base_url="http://localhost:8000",
        endpoint="/chat",
    )

Example (HTTP Agent - with options):
    invoker = HttpAgentInvoker(
        base_url="http://localhost:8000",
        endpoint="/chat",
        method="POST",
        timeout=120,
        headers={"Authorization": "Bearer xxx"},
    )

Example (In-Process Agent):
    class InProcessInvoker(AgentInvoker):
        def invoke(self, input: Any) -> InvokeResult:
            response = my_agent.run(input)
            trajectory = Trace(traceId="...", rootSpanId="...", rootSpanName="...", startTime="...", endTime="...", spans=[])  # From instrumentation
            return InvokeResult(input=input, output=response, trajectory=trajectory)
"""

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Optional, List, Dict, Any, Callable
import logging

from .trace import Trace


logger = logging.getLogger(__name__)


@dataclass
class InvokeResult:
    """
    Result from invoking an agent during evaluation.

    For HTTP agents: Framework handles trace fetching automatically via
    time-based bulk fetching and baggage-based matching. Just return output.

    For in-process agents: Can optionally return trajectory directly to skip
    trace fetching step.

    Attributes:
        input: The input that was sent to the agent
        output: The agent's response (string, dict, or any serializable type)
        error: Error message if invocation failed (None if successful)
        trajectory: Direct Trace object (optional, for in-process agents)
        metadata: Optional metadata about the invocation
    """

    input: Any = None
    output: Any = None
    error: Optional[str] = None
    trajectory: Optional[Trace] = None
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

    The invoke method receives only the task input (not the full Task object)
    to prevent data leakage of ground truth (expected_output, success_criteria, etc.)
    into the agent.

    The framework handles trace collection automatically via:
    - OpenTelemetry baggage propagation (task_id, trial_id)
    - Time-based bulk trace fetching after invocations
    - Automatic trace-to-task matching

    You only need to invoke the agent and return the output.

    Example:
        class MyHTTPAgentInvoker(AgentInvoker):
            def __init__(self, endpoint: str):
                self.endpoint = endpoint

            def invoke(self, input: Any) -> InvokeResult:
                response = requests.post(f"{self.endpoint}/chat", json=input)
                return InvokeResult(input=input, output=response.json())
    """

    @abstractmethod
    def invoke(self, input: Any) -> InvokeResult:
        """
        Invoke the agent with the given input.

        Only the task input is passed to prevent data leakage of ground truth
        (expected_output, expected_trajectory, success_criteria, etc.).

        Args:
            input: The task input (string, dict, or any serializable type)

        Returns:
            InvokeResult with input, output, and optionally error or trajectory
        """
        pass

    def invoke_batch(self, inputs: List[Any]) -> List[InvokeResult]:
        """
        Invoke the agent with multiple inputs.

        Default implementation calls invoke() for each input sequentially.
        Override for parallel execution or batch optimization.

        Args:
            inputs: List of inputs to invoke with

        Returns:
            List of InvokeResults in same order as inputs
        """
        results = []
        for input in inputs:
            try:
                result = self.invoke(input)
                results.append(result)
            except Exception as e:
                logger.error(f"Invocation failed: {e}")
                results.append(InvokeResult(input=input, error=str(e)))
        return results


class HttpAgentInvoker(AgentInvoker):
    """
    HTTP-based agent invoker using the requests library.

    Simple and clean interface for invoking HTTP agents. Uses the `requests`
    library internally - no need to pass an HTTP client.

    Args:
        base_url: Base URL of the agent endpoint (e.g., "http://localhost:8000")
        endpoint: API endpoint path (default: "/chat")
        method: HTTP method (default: "POST"). Supports "GET", "POST", "PUT", "PATCH"
        timeout: Request timeout in seconds (default: 60)
        headers: Additional HTTP headers to include in all requests
        payload_builder: Optional function to build request payload from task.input
                        Default: uses task.input as-is if dict, else wraps as {"input": task.input}

    Example (simple):
        invoker = HttpAgentInvoker(
            base_url="http://localhost:8000",
            endpoint="/chat",
        )

    Example (with options):
        invoker = HttpAgentInvoker(
            base_url="http://localhost:8000",
            endpoint="/invoke",
            method="POST",
            timeout=120,
            headers={"Authorization": "Bearer xxx"},
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
            payload_builder=build_payload,
        )
    """

    def __init__(
        self,
        base_url: str,
        endpoint: str = "/chat",
        method: str = "POST",
        timeout: float = 60.0,
        headers: Optional[Dict[str, str]] = None,
        payload_builder: Optional[Callable[[Any], Dict[str, Any]]] = None,
    ):
        self.base_url = base_url.rstrip("/")
        self.endpoint = endpoint if endpoint.startswith("/") else f"/{endpoint}"
        self.method = method.upper()
        self.timeout = timeout
        self.headers = headers or {}
        self.payload_builder = payload_builder or self._default_payload_builder

        # Import requests here to fail fast if not installed
        try:
            import requests

            self._session = requests.Session()
        except ImportError:
            raise ImportError(
                "The 'requests' library is required for HttpAgentInvoker. Install it with: pip install requests"
            )

    def _default_payload_builder(self, task_input: Any) -> Dict[str, Any]:
        """Default payload builder: use input as-is if dict, else wrap it."""
        if isinstance(task_input, dict):
            return task_input
        return {"input": task_input}

    def invoke(self, input: Any) -> InvokeResult:
        """
        Invoke HTTP agent with input.

        The framework automatically propagates task_id and trial_id via
        OpenTelemetry baggage in HTTP headers.
        """
        url = f"{self.base_url}{self.endpoint}"
        payload = self.payload_builder(input)

        # Merge headers (default + user headers)
        request_headers = dict(self.headers)

        try:
            # Make request based on method
            if self.method == "GET":
                response = self._session.get(url, params=payload, headers=request_headers, timeout=self.timeout)
            elif self.method in ("POST", "PUT", "PATCH"):
                request_headers["Content-Type"] = "application/json"
                response = self._session.request(
                    self.method, url, json=payload, headers=request_headers, timeout=self.timeout
                )
            else:
                return InvokeResult(input=input, error=f"Unsupported HTTP method: {self.method}")

            # Check for HTTP errors
            response.raise_for_status()

            # Parse response
            output = response.json()

            return InvokeResult(input=input, output=output)

        except Exception as e:
            logger.error(f"HTTP invocation failed: {e}")
            return InvokeResult(input=input, error=str(e))
