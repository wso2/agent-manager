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
Unit tests for agent invokers.

Tests:
- InvokeResult properties and behavior
- AgentInvoker abstract base class
- HttpAgentInvoker with different HTTP clients
- HttpAgentInvoker payload building
- HttpAgentInvoker error handling
- invoke_batch default implementation
"""

import pytest
import sys
from pathlib import Path
from unittest.mock import Mock

# Add src to path
sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from amp_evaluation.invokers import InvokeResult, AgentInvoker, HttpAgentInvoker
from amp_evaluation.models import Task
from amp_evaluation.trace import Trajectory


class TestInvokeResult:
    """Test InvokeResult dataclass."""

    def test_default_initialization(self):
        """Test InvokeResult with default values."""
        result = InvokeResult()

        assert result.output is None
        assert result.error is None
        assert result.trajectory is None
        assert result.metadata == {}
        assert result.success is True  # No error = success
        assert result.has_trajectory is False

    def test_with_output_only(self):
        """Test InvokeResult with just output."""
        result = InvokeResult(output="Hello, world!")

        assert result.output == "Hello, world!"
        assert result.error is None
        assert result.success is True
        assert result.has_trajectory is False

    def test_with_dict_output(self):
        """Test InvokeResult with dict output."""
        output_data = {"response": "Hello", "metadata": {"model": "gpt-4"}}
        result = InvokeResult(output=output_data)

        assert result.output == output_data
        assert result.success is True

    def test_with_error(self):
        """Test InvokeResult with error."""
        result = InvokeResult(error="Connection timeout")

        assert result.output is None
        assert result.error == "Connection timeout"
        assert result.success is False
        assert result.has_trajectory is False

    def test_with_trajectory(self):
        """Test InvokeResult with direct trajectory."""
        trajectory = Trajectory(trace_id="trace-123")

        result = InvokeResult(output="response", trajectory=trajectory)

        assert result.output == "response"
        assert result.trajectory == trajectory
        assert result.has_trajectory is True
        assert result.success is True

    def test_with_metadata(self):
        """Test InvokeResult with metadata."""
        metadata = {"duration": 1.5, "model": "gpt-4"}
        result = InvokeResult(output="response", metadata=metadata)

        assert result.metadata == metadata
        assert result.success is True

    def test_output_and_error(self):
        """Test InvokeResult with both output and error."""
        result = InvokeResult(output="Partial response", error="Request timed out")

        assert result.output == "Partial response"
        assert result.error == "Request timed out"
        assert result.success is False  # Error takes precedence


class TestAgentInvoker:
    """Test AgentInvoker abstract base class."""

    def test_cannot_instantiate_abstract_class(self):
        """Test that AgentInvoker cannot be instantiated directly."""
        with pytest.raises(TypeError, match="Can't instantiate abstract class"):
            AgentInvoker()

    def test_must_implement_invoke(self):
        """Test that subclasses must implement invoke()."""

        class IncompleteInvoker(AgentInvoker):
            pass

        with pytest.raises(TypeError, match="Can't instantiate abstract class"):
            IncompleteInvoker()

    def test_valid_subclass(self):
        """Test creating a valid AgentInvoker subclass."""

        class ValidInvoker(AgentInvoker):
            def invoke(self, task: Task) -> InvokeResult:
                return InvokeResult(output="test")

        invoker = ValidInvoker()
        task = Task(task_id="task-1", name="Test Task", description="Test description", input="test input")
        result = invoker.invoke(task)

        assert isinstance(result, InvokeResult)
        assert result.output == "test"

    def test_invoke_batch_default_implementation(self):
        """Test default invoke_batch implementation."""

        class SimpleInvoker(AgentInvoker):
            def __init__(self):
                self.call_count = 0

            def invoke(self, task: Task) -> InvokeResult:
                self.call_count += 1
                return InvokeResult(output=f"Response {self.call_count}")

        invoker = SimpleInvoker()
        tasks = [
            Task(task_id="task-1", name="Test Task", description="Test description", input="input 1"),
            Task(task_id="task-2", name="Test Task", description="Test description", input="input 2"),
            Task(task_id="task-3", name="Test Task", description="Test description", input="input 3"),
        ]

        results = invoker.invoke_batch(tasks)

        assert len(results) == 3
        assert results[0].output == "Response 1"
        assert results[1].output == "Response 2"
        assert results[2].output == "Response 3"
        assert invoker.call_count == 3

    def test_invoke_batch_handles_errors(self):
        """Test that invoke_batch handles errors in individual invocations."""

        class ErrorProneInvoker(AgentInvoker):
            def invoke(self, task: Task) -> InvokeResult:
                if "error" in task.input:
                    raise ValueError("Simulated error")
                return InvokeResult(output="success")

        invoker = ErrorProneInvoker()
        tasks = [
            Task(task_id="task-1", name="Test Task", description="Test description", input="normal"),
            Task(task_id="task-2", name="Test Task", description="Test description", input="error case"),
            Task(task_id="task-3", name="Test Task", description="Test description", input="normal"),
        ]

        results = invoker.invoke_batch(tasks)

        assert len(results) == 3
        assert results[0].success is True
        assert results[0].output == "success"
        assert results[1].success is False
        assert "Simulated error" in results[1].error
        assert results[2].success is True


class MockHttpClient:
    """Mock HTTP client for testing."""

    def __init__(self, response_data=None, status_code=200, raise_error=None):
        self.response_data = response_data or {"message": "success"}
        self.status_code = status_code
        self.raise_error = raise_error
        self.last_request = None

    def post(self, url: str, **kwargs):
        """Mock POST request."""
        if self.raise_error:
            raise self.raise_error

        # Store request details
        self.last_request = {"url": url, "kwargs": kwargs}

        # Create mock response
        response = Mock()
        response.status_code = self.status_code
        response.json = Mock(return_value=self.response_data)

        # Add raise_for_status method
        def raise_for_status():
            if self.status_code >= 400:
                raise Exception(f"HTTP {self.status_code}")

        response.raise_for_status = raise_for_status

        return response


class TestHttpAgentInvoker:
    """Test HttpAgentInvoker implementation."""

    def test_basic_invocation(self):
        """Test basic HTTP invocation with default settings."""
        client = MockHttpClient(response_data={"response": "Hello"})
        invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat", http_client=client)

        task = Task(task_id="task-1", name="Test Task", description="Test description", input={"query": "Hi"})
        result = invoker.invoke(task)

        assert result.success is True
        assert result.output == {"response": "Hello"}
        assert client.last_request["url"] == "http://localhost:8000/chat"
        assert client.last_request["kwargs"]["json"] == {"query": "Hi"}

    def test_base_url_normalization(self):
        """Test that trailing slash is removed from base_url."""
        client = MockHttpClient()
        invoker = HttpAgentInvoker(base_url="http://localhost:8000/", endpoint="/chat", http_client=client)

        task = Task(task_id="task-1", name="Test Task", description="Test description", input="test")
        invoker.invoke(task)

        assert client.last_request["url"] == "http://localhost:8000/chat"

    def test_endpoint_normalization(self):
        """Test that endpoint gets leading slash if missing."""
        client = MockHttpClient()
        invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="chat", http_client=client)

        task = Task(task_id="task-1", name="Test Task", description="Test description", input="test")
        invoker.invoke(task)

        assert client.last_request["url"] == "http://localhost:8000/chat"

    def test_default_payload_builder_with_dict(self):
        """Test default payload builder with dict input."""
        client = MockHttpClient()
        invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat", http_client=client)

        input_dict = {"query": "Hello", "context": "test"}
        task = Task(task_id="task-1", name="Test Task", description="Test description", input=input_dict)
        invoker.invoke(task)

        # Dict input should be used as-is
        assert client.last_request["kwargs"]["json"] == input_dict

    def test_default_payload_builder_with_string(self):
        """Test default payload builder with string input."""
        client = MockHttpClient()
        invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat", http_client=client)

        task = Task(task_id="task-1", name="Test Task", description="Test description", input="Hello, how are you?")
        invoker.invoke(task)

        # String input should be wrapped
        assert client.last_request["kwargs"]["json"] == {"input": "Hello, how are you?"}

    def test_custom_payload_builder(self):
        """Test custom payload builder."""

        def custom_builder(task_input):
            return {"messages": [{"role": "user", "content": task_input}], "temperature": 0.7}

        client = MockHttpClient()
        invoker = HttpAgentInvoker(
            base_url="http://localhost:8000", endpoint="/chat", http_client=client, payload_builder=custom_builder
        )

        task = Task(task_id="task-1", name="Test Task", description="Test description", input="Hello")
        invoker.invoke(task)

        expected_payload = {"messages": [{"role": "user", "content": "Hello"}], "temperature": 0.7}
        assert client.last_request["kwargs"]["json"] == expected_payload

    def test_custom_headers(self):
        """Test custom headers are included in request."""
        client = MockHttpClient()
        custom_headers = {"Authorization": "Bearer token123", "X-Custom-Header": "value"}

        invoker = HttpAgentInvoker(
            base_url="http://localhost:8000", endpoint="/chat", http_client=client, headers=custom_headers
        )

        task = Task(task_id="task-1", name="Test Task", description="Test description", input="test")
        invoker.invoke(task)

        request_headers = client.last_request["kwargs"]["headers"]
        assert request_headers["Authorization"] == "Bearer token123"
        assert request_headers["X-Custom-Header"] == "value"
        assert request_headers["Content-Type"] == "application/json"

    def test_timeout_configuration(self):
        """Test timeout is passed to HTTP client."""
        client = MockHttpClient()
        invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat", http_client=client, timeout=30.0)

        task = Task(task_id="task-1", name="Test Task", description="Test description", input="test")
        invoker.invoke(task)

        assert client.last_request["kwargs"]["timeout"] == 30.0

    def test_default_timeout(self):
        """Test default timeout is 60 seconds."""
        client = MockHttpClient()
        invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat", http_client=client)

        task = Task(task_id="task-1", name="Test Task", description="Test description", input="test")
        invoker.invoke(task)

        assert client.last_request["kwargs"]["timeout"] == 60.0

    def test_http_error_handling(self):
        """Test handling of HTTP errors."""
        client = MockHttpClient(status_code=500)
        invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat", http_client=client)

        task = Task(task_id="task-1", name="Test Task", description="Test description", input="test")
        result = invoker.invoke(task)

        assert result.success is False
        assert "HTTP 500" in result.error
        assert result.output is None

    def test_connection_error_handling(self):
        """Test handling of connection errors."""
        client = MockHttpClient(raise_error=ConnectionError("Connection refused"))
        invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat", http_client=client)

        task = Task(task_id="task-1", name="Test Task", description="Test description", input="test")
        result = invoker.invoke(task)

        assert result.success is False
        assert "Connection refused" in result.error
        assert result.output is None

    def test_timeout_error_handling(self):
        """Test handling of timeout errors."""
        client = MockHttpClient(raise_error=TimeoutError("Request timed out"))
        invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat", http_client=client)

        task = Task(task_id="task-1", name="Test Task", description="Test description", input="test")
        result = invoker.invoke(task)

        assert result.success is False
        assert "Request timed out" in result.error

    def test_different_response_formats(self):
        """Test handling different response formats."""
        # Test with dict response
        client = MockHttpClient(response_data={"answer": "42", "confidence": 0.95})
        invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat", http_client=client)

        task = Task(task_id="task-1", name="Test Task", description="Test description", input="What is the answer?")
        result = invoker.invoke(task)

        assert result.output == {"answer": "42", "confidence": 0.95}

    def test_works_with_different_http_clients(self):
        """Test that HttpAgentInvoker works with different HTTP client libraries."""
        # Test with requests-like client
        requests_mock = MockHttpClient(response_data={"status": "ok"})
        invoker1 = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat", http_client=requests_mock)

        task = Task(task_id="task-1", name="Test Task", description="Test description", input="test")
        result1 = invoker1.invoke(task)

        assert result1.success is True
        assert result1.output == {"status": "ok"}

        # Test with httpx-like client (same interface)
        httpx_mock = MockHttpClient(response_data={"status": "ok"})
        invoker2 = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat", http_client=httpx_mock)

        result2 = invoker2.invoke(task)

        assert result2.success is True
        assert result2.output == {"status": "ok"}


class TestHttpAgentInvokerBatch:
    """Test batch invocation with HttpAgentInvoker."""

    def test_batch_invocation_sequential(self):
        """Test that batch invocation calls invoke for each task."""
        client = MockHttpClient()
        invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat", http_client=client)

        tasks = [
            Task(task_id="task-1", name="Test Task", description="Test description", input="Question 1"),
            Task(task_id="task-2", name="Test Task", description="Test description", input="Question 2"),
            Task(task_id="task-3", name="Test Task", description="Test description", input="Question 3"),
        ]

        results = invoker.invoke_batch(tasks)

        assert len(results) == 3
        for result in results:
            assert result.success is True
            assert result.output == {"message": "success"}

    def test_batch_continues_after_error(self):
        """Test that batch invocation continues after individual errors."""
        # Client that fails on second request
        call_count = 0

        def make_client():
            nonlocal call_count
            call_count += 1
            if call_count == 2:
                return MockHttpClient(raise_error=Exception("Failed"))
            return MockHttpClient(response_data={"result": f"response-{call_count}"})

        invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat", http_client=Mock())

        # Override invoke to use different clients
        original_invoke = invoker.invoke

        def custom_invoke(task):
            invoker.http_client = make_client()
            return original_invoke(task)

        invoker.invoke = custom_invoke

        tasks = [
            Task(task_id="task-1", name="Test Task", description="Test description", input="input-1"),
            Task(task_id="task-2", name="Test Task", description="Test description", input="input-2"),  # Will fail
            Task(task_id="task-3", name="Test Task", description="Test description", input="input-3"),
        ]

        results = invoker.invoke_batch(tasks)

        assert len(results) == 3
        assert results[0].success is True
        assert results[1].success is False
        assert "Failed" in results[1].error
        assert results[2].success is True


class TestHttpAgentInvokerIntegration:
    """Integration tests with realistic scenarios."""

    def test_langgraph_agent_scenario(self):
        """Test scenario similar to LangGraph customer support agent."""

        def build_langgraph_payload(task_input):
            # Simulate building payload for LangGraph agent
            if isinstance(task_input, dict):
                return task_input
            return {"thread_id": "thread-123", "passenger_id": "passenger-456", "question": task_input}

        response_data = {
            "messages": [{"role": "assistant", "content": "I can help you with that."}],
            "metadata": {"model": "gpt-4", "tokens": 150},
        }

        client = MockHttpClient(response_data=response_data)
        invoker = HttpAgentInvoker(
            base_url="http://localhost:8123",
            endpoint="/chat",
            http_client=client,
            timeout=60.0,
            payload_builder=build_langgraph_payload,
        )

        task = Task(
            task_id="eval-task-1",
            name="Flight Status",
            description="Check flight status query",
            input="What is the status of flight AA123?",
        )

        result = invoker.invoke(task)

        assert result.success is True
        assert result.output == response_data

        # Verify payload was built correctly
        sent_payload = client.last_request["kwargs"]["json"]
        assert sent_payload["thread_id"] == "thread-123"
        assert sent_payload["question"] == "What is the status of flight AA123?"

    def test_openai_compatible_api(self):
        """Test with OpenAI-compatible API format."""

        def build_openai_payload(task_input):
            return {"model": "gpt-4", "messages": [{"role": "user", "content": task_input}], "temperature": 0.7}

        response_data = {
            "choices": [{"message": {"role": "assistant", "content": "The answer is 42."}}],
            "usage": {"total_tokens": 100},
        }

        client = MockHttpClient(response_data=response_data)
        invoker = HttpAgentInvoker(
            base_url="https://api.openai.com/v1",
            endpoint="/chat/completions",
            http_client=client,
            headers={"Authorization": "Bearer sk-..."},
            payload_builder=build_openai_payload,
        )

        task = Task(
            task_id="task-1", name="Test Task", description="Test description", input="What is the meaning of life?"
        )
        result = invoker.invoke(task)

        assert result.success is True
        assert result.output["choices"][0]["message"]["content"] == "The answer is 42."


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
