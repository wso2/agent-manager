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
- HttpAgentInvoker with requests library
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
from amp_evaluation.trace import Trace


class TestInvokeResult:
    """Test InvokeResult dataclass."""

    def test_default_initialization(self):
        """Test InvokeResult with default values."""
        result = InvokeResult()

        assert result.input is None
        assert result.output is None
        assert result.error is None
        assert result.trajectory is None
        assert result.metadata == {}
        assert result.success is True  # No error = success
        assert result.has_trajectory is False

    def test_with_output_only(self):
        """Test InvokeResult with just output."""
        result = InvokeResult(input="Hello?", output="Hello, world!")

        assert result.input == "Hello?"
        assert result.output == "Hello, world!"
        assert result.error is None
        assert result.success is True
        assert result.has_trajectory is False

    def test_with_dict_output(self):
        """Test InvokeResult with dict output."""
        input_data = {"query": "Hello"}
        output_data = {"response": "Hello", "metadata": {"model": "gpt-4"}}
        result = InvokeResult(input=input_data, output=output_data)

        assert result.input == input_data
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
        trajectory = Trace(trace_id="trace-123")

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
            def invoke(self, task_input) -> InvokeResult:
                return InvokeResult(input=task_input, output="test")

        invoker = ValidInvoker()
        result = invoker.invoke("test input")

        assert isinstance(result, InvokeResult)
        assert result.input == "test input"
        assert result.output == "test"

    def test_invoke_batch_default_implementation(self):
        """Test default invoke_batch implementation."""

        class SimpleInvoker(AgentInvoker):
            def __init__(self):
                self.call_count = 0

            def invoke(self, task_input) -> InvokeResult:
                self.call_count += 1
                return InvokeResult(input=task_input, output=f"Response {self.call_count}")

        invoker = SimpleInvoker()
        inputs = ["input 1", "input 2", "input 3"]

        results = invoker.invoke_batch(inputs)

        assert len(results) == 3
        assert results[0].output == "Response 1"
        assert results[1].output == "Response 2"
        assert results[2].output == "Response 3"
        assert invoker.call_count == 3

    def test_invoke_batch_handles_errors(self):
        """Test that invoke_batch handles errors in individual invocations."""

        class ErrorProneInvoker(AgentInvoker):
            def invoke(self, task_input) -> InvokeResult:
                if "error" in task_input:
                    raise ValueError("Simulated error")
                return InvokeResult(input=task_input, output="success")

        invoker = ErrorProneInvoker()
        inputs = ["normal", "error case", "normal"]

        results = invoker.invoke_batch(inputs)

        assert len(results) == 3
        assert results[0].success is True
        assert results[0].output == "success"
        assert results[1].success is False
        assert "Simulated error" in results[1].error
        assert results[2].success is True


class MockSession:
    """Mock requests.Session for testing HttpAgentInvoker."""

    def __init__(self, response_data=None, status_code=200, raise_error=None):
        self.response_data = response_data or {"message": "success"}
        self.status_code = status_code
        self.raise_error = raise_error
        self.last_request = None

    def _make_response(self):
        """Create a mock response object."""
        response = Mock()
        response.status_code = self.status_code
        response.json = Mock(return_value=self.response_data)

        def raise_for_status():
            if self.status_code >= 400:
                raise Exception(f"HTTP {self.status_code}")

        response.raise_for_status = raise_for_status
        return response

    def get(self, url: str, **kwargs):
        """Mock GET request."""
        if self.raise_error:
            raise self.raise_error
        self.last_request = {"method": "GET", "url": url, "kwargs": kwargs}
        return self._make_response()

    def request(self, method: str, url: str, **kwargs):
        """Mock generic request."""
        if self.raise_error:
            raise self.raise_error
        self.last_request = {"method": method, "url": url, "kwargs": kwargs}
        return self._make_response()


class TestHttpAgentInvoker:
    """Test HttpAgentInvoker implementation."""

    def _create_invoker_with_mock_session(self, mock_session, **kwargs):
        """Helper to create HttpAgentInvoker with a mock session."""
        invoker = HttpAgentInvoker(**kwargs)
        invoker._session = mock_session
        return invoker

    def test_basic_invocation(self):
        """Test basic HTTP invocation with default settings."""
        session = MockSession(response_data={"response": "Hello"})
        invoker = self._create_invoker_with_mock_session(session, base_url="http://localhost:8000", endpoint="/chat")

        result = invoker.invoke({"query": "Hi"})

        assert result.success is True
        assert result.input == {"query": "Hi"}
        assert result.output == {"response": "Hello"}
        assert session.last_request["url"] == "http://localhost:8000/chat"
        assert session.last_request["kwargs"]["json"] == {"query": "Hi"}

    def test_base_url_normalization(self):
        """Test that trailing slash is removed from base_url."""
        session = MockSession()
        invoker = self._create_invoker_with_mock_session(session, base_url="http://localhost:8000/", endpoint="/chat")

        invoker.invoke("test")

        assert session.last_request["url"] == "http://localhost:8000/chat"

    def test_endpoint_normalization(self):
        """Test that endpoint gets leading slash if missing."""
        session = MockSession()
        invoker = self._create_invoker_with_mock_session(session, base_url="http://localhost:8000", endpoint="chat")

        invoker.invoke("test")

        assert session.last_request["url"] == "http://localhost:8000/chat"

    def test_default_payload_builder_with_dict(self):
        """Test default payload builder with dict input."""
        session = MockSession()
        invoker = self._create_invoker_with_mock_session(session, base_url="http://localhost:8000", endpoint="/chat")

        input_dict = {"query": "Hello", "context": "test"}
        invoker.invoke(input_dict)

        # Dict input should be used as-is
        assert session.last_request["kwargs"]["json"] == input_dict

    def test_default_payload_builder_with_string(self):
        """Test default payload builder with string input."""
        session = MockSession()
        invoker = self._create_invoker_with_mock_session(session, base_url="http://localhost:8000", endpoint="/chat")

        invoker.invoke("Hello, how are you?")

        # String input should be wrapped
        assert session.last_request["kwargs"]["json"] == {"input": "Hello, how are you?"}

    def test_custom_payload_builder(self):
        """Test custom payload builder."""

        def custom_builder(task_input):
            return {"messages": [{"role": "user", "content": task_input}], "temperature": 0.7}

        session = MockSession()
        invoker = self._create_invoker_with_mock_session(
            session, base_url="http://localhost:8000", endpoint="/chat", payload_builder=custom_builder
        )

        invoker.invoke("Hello")

        expected_payload = {"messages": [{"role": "user", "content": "Hello"}], "temperature": 0.7}
        assert session.last_request["kwargs"]["json"] == expected_payload

    def test_custom_headers(self):
        """Test custom headers are included in request."""
        custom_headers = {"Authorization": "Bearer token123", "X-Custom-Header": "value"}

        session = MockSession()
        invoker = self._create_invoker_with_mock_session(
            session, base_url="http://localhost:8000", endpoint="/chat", headers=custom_headers
        )

        invoker.invoke("test")

        request_headers = session.last_request["kwargs"]["headers"]
        assert request_headers["Authorization"] == "Bearer token123"
        assert request_headers["X-Custom-Header"] == "value"
        assert request_headers["Content-Type"] == "application/json"

    def test_timeout_configuration(self):
        """Test timeout is passed to HTTP client."""
        session = MockSession()
        invoker = self._create_invoker_with_mock_session(
            session, base_url="http://localhost:8000", endpoint="/chat", timeout=30.0
        )

        invoker.invoke("test")

        assert session.last_request["kwargs"]["timeout"] == 30.0

    def test_default_timeout(self):
        """Test default timeout is 60 seconds."""
        session = MockSession()
        invoker = self._create_invoker_with_mock_session(session, base_url="http://localhost:8000", endpoint="/chat")

        invoker.invoke("test")

        assert session.last_request["kwargs"]["timeout"] == 60.0

    def test_default_method_is_post(self):
        """Test default HTTP method is POST."""
        session = MockSession()
        invoker = self._create_invoker_with_mock_session(session, base_url="http://localhost:8000", endpoint="/chat")

        invoker.invoke("test")

        assert session.last_request["method"] == "POST"

    def test_get_method(self):
        """Test GET method passes params instead of json."""
        session = MockSession()
        invoker = self._create_invoker_with_mock_session(
            session, base_url="http://localhost:8000", endpoint="/query", method="GET"
        )

        invoker.invoke({"q": "search"})

        assert session.last_request["method"] == "GET"
        assert session.last_request["kwargs"]["params"] == {"q": "search"}

    def test_put_method(self):
        """Test PUT method."""
        session = MockSession()
        invoker = self._create_invoker_with_mock_session(
            session, base_url="http://localhost:8000", endpoint="/update", method="PUT"
        )

        invoker.invoke({"data": "value"})

        assert session.last_request["method"] == "PUT"
        assert session.last_request["kwargs"]["json"] == {"data": "value"}

    def test_unsupported_method(self):
        """Test unsupported HTTP method returns error."""
        session = MockSession()
        invoker = self._create_invoker_with_mock_session(
            session, base_url="http://localhost:8000", endpoint="/delete", method="DELETE"
        )

        result = invoker.invoke("test")

        assert result.success is False
        assert "Unsupported HTTP method" in result.error

    def test_http_error_handling(self):
        """Test handling of HTTP errors."""
        session = MockSession(status_code=500)
        invoker = self._create_invoker_with_mock_session(session, base_url="http://localhost:8000", endpoint="/chat")

        result = invoker.invoke("test")

        assert result.success is False
        assert "HTTP 500" in result.error
        assert result.output is None

    def test_connection_error_handling(self):
        """Test handling of connection errors."""
        session = MockSession(raise_error=ConnectionError("Connection refused"))
        invoker = self._create_invoker_with_mock_session(session, base_url="http://localhost:8000", endpoint="/chat")

        result = invoker.invoke("test")

        assert result.success is False
        assert "Connection refused" in result.error
        assert result.output is None

    def test_timeout_error_handling(self):
        """Test handling of timeout errors."""
        session = MockSession(raise_error=TimeoutError("Request timed out"))
        invoker = self._create_invoker_with_mock_session(session, base_url="http://localhost:8000", endpoint="/chat")

        result = invoker.invoke("test")

        assert result.success is False
        assert "Request timed out" in result.error

    def test_different_response_formats(self):
        """Test handling different response formats."""
        session = MockSession(response_data={"answer": "42", "confidence": 0.95})
        invoker = self._create_invoker_with_mock_session(session, base_url="http://localhost:8000", endpoint="/chat")

        result = invoker.invoke("What is the answer?")

        assert result.output == {"answer": "42", "confidence": 0.95}


class TestHttpAgentInvokerBatch:
    """Test batch invocation with HttpAgentInvoker."""

    def test_batch_invocation_sequential(self):
        """Test that batch invocation calls invoke for each task."""
        session = MockSession()
        invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat")
        invoker._session = session

        inputs = ["Question 1", "Question 2", "Question 3"]

        results = invoker.invoke_batch(inputs)

        assert len(results) == 3
        for result in results:
            assert result.success is True
            assert result.output == {"message": "success"}


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

        session = MockSession(response_data=response_data)
        invoker = HttpAgentInvoker(
            base_url="http://localhost:8123",
            endpoint="/chat",
            timeout=60.0,
            payload_builder=build_langgraph_payload,
        )
        invoker._session = session

        result = invoker.invoke("What is the status of flight AA123?")

        assert result.success is True
        assert result.output == response_data

        # Verify payload was built correctly
        sent_payload = session.last_request["kwargs"]["json"]
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

        session = MockSession(response_data=response_data)
        invoker = HttpAgentInvoker(
            base_url="https://api.openai.com/v1",
            endpoint="/chat/completions",
            headers={"Authorization": "Bearer sk-..."},
            payload_builder=build_openai_payload,
        )
        invoker._session = session

        result = invoker.invoke("What is the meaning of life?")

        assert result.success is True
        assert result.output["choices"][0]["message"]["content"] == "The answer is 42."


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
