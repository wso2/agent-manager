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
Tests for the monitor evaluation job (main.py).

Verifies:
- Evaluator config parsing (identifier, displayName, config with level)
- Level propagation from config to builtin
- Score publishing payload matches agent-manager PublishScoresRequest schema
- Argument validation and error handling
"""

import json
import sys
from datetime import datetime, timezone
from unittest.mock import MagicMock, patch

import pytest

from main import parse_args, validate_time_format, publish_scores, _eval_template, _load_custom_code_evaluator


# ---------------------------------------------------------------------------
# Fixtures: realistic evaluator configs matching agent-manager serialization
# (level merged into config by serializeEvaluators)
# ---------------------------------------------------------------------------

REALISTIC_EVALUATORS = [
    {
        "identifier": "latency_performance",
        "displayName": "Latency Check",
        "config": {
            "max_latency_ms": 3000,
            "use_task_constraint": False,
            "level": "trace",
        },
    },
    {
        "identifier": "iteration_efficiency",
        "displayName": "Iteration Count",
        "config": {
            "max_iterations": 5,
            "use_context_constraint": False,
            "level": "trace",
        },
    },
    {
        "identifier": "token_efficiency",
        "displayName": "Token Efficiency",
        "config": {
            "max_tokens": 4000,
            "use_context_constraint": False,
            "level": "trace",
        },
    },
    {
        "identifier": "answer_relevancy",
        "displayName": "Answer Relevancy",
        "config": {"min_overlap_ratio": 0.2, "level": "trace"},
    },
    {
        "identifier": "content_safety",
        "displayName": "Prohibited Content",
        "config": {
            "case_sensitive": False,
            "prohibited_strings": ["internal error", "stack trace", "debug:", "hotels"],
            "use_context_prohibited": False,
            "level": "trace",
        },
    },
    {
        "identifier": "length_compliance",
        "displayName": "Answer Length",
        "config": {"max_length": 5000, "min_length": 10, "level": "trace"},
    },
    {
        "identifier": "latency_performance",
        "displayName": "Agent Latency",
        "config": {
            "max_latency_ms": 5000,
            "use_task_constraint": True,
            "level": "agent",
        },
    },
    {
        "identifier": "latency_performance",
        "displayName": "Span Latency",
        "config": {
            "max_latency_ms": 1000,
            "use_task_constraint": True,
            "level": "llm",
        },
    },
]


def _make_evaluator_score(
    trace_id,
    score,
    span_id=None,
    explanation=None,
    timestamp=None,
    error=None,
):
    """Helper to create a mock EvaluatorScore."""
    s = MagicMock()
    s.trace_id = trace_id
    s.score = score
    if span_id is not None:
        span_ctx = MagicMock()
        span_ctx.span_id = span_id
        span_ctx.agent_name = None
        span_ctx.model = None
        span_ctx.vendor = None
        s.span_context = span_ctx
    else:
        s.span_context = None
    s.explanation = explanation
    s.trace_start_time = timestamp
    s.skip_reason = error
    s.is_successful = error is None and score is not None
    return s


def _make_evaluator_summary(evaluator_name, level, scores, aggregated_scores):
    """Helper to create a mock EvaluatorSummary."""
    summary = MagicMock()
    summary.evaluator_name = evaluator_name
    summary.level = level
    summary.individual_scores = scores
    summary.aggregated_scores = aggregated_scores
    summary.count = len(scores)
    summary.skipped_count = sum(1 for s in scores if s.skip_reason is not None)
    return summary


# ===========================================================================
# validate_time_format
# ===========================================================================


class TestValidateTimeFormat:
    def test_valid_iso8601_utc(self):
        assert validate_time_format("2026-01-15T10:00:00Z") is True

    def test_valid_iso8601_offset(self):
        assert validate_time_format("2026-01-15T10:00:00+05:30") is True

    def test_valid_iso8601_no_tz(self):
        assert validate_time_format("2026-01-15T10:00:00") is True

    def test_invalid_format(self):
        assert validate_time_format("not-a-date") is False

    def test_empty_string(self):
        assert validate_time_format("") is False

    def test_date_only(self):
        # datetime.fromisoformat accepts date-only strings in Python 3.11+
        assert validate_time_format("2026-01-15") is True


# ===========================================================================
# parse_args
# ===========================================================================


class TestParseArgs:
    REQUIRED_ARGS = [
        "--monitor-name",
        "test-monitor",
        "--agent-id",
        "agent-uid-123",
        "--environment-id",
        "env-uid-456",
        "--evaluators",
        json.dumps(REALISTIC_EVALUATORS),
        "--trace-start",
        "2026-01-15T10:00:00Z",
        "--trace-end",
        "2026-01-15T11:00:00Z",
        "--traces-api-endpoint",
        "http://traces:8080",
        "--monitor-id",
        "550e8400-e29b-41d4-a716-446655440000",
        "--run-id",
        "660e8400-e29b-41d4-a716-446655440000",
        "--publisher-endpoint",
        "http://agent-manager:8081",
    ]

    def test_all_required_args(self):
        with patch.object(sys, "argv", ["main.py"] + self.REQUIRED_ARGS):
            args = parse_args()
        assert args.monitor_name == "test-monitor"
        assert args.agent_id == "agent-uid-123"
        assert args.environment_id == "env-uid-456"
        assert args.sampling_rate == 1.0  # default

    def test_custom_sampling_rate(self):
        extra = self.REQUIRED_ARGS + ["--sampling-rate", "0.5"]
        with patch.object(sys, "argv", ["main.py"] + extra):
            args = parse_args()
        assert args.sampling_rate == 0.5

    def test_missing_required_arg_exits(self):
        with patch.object(sys, "argv", ["main.py", "--monitor-name", "test"]):
            with pytest.raises(SystemExit):
                parse_args()

    def test_evaluators_json_roundtrip(self):
        with patch.object(sys, "argv", ["main.py"] + self.REQUIRED_ARGS):
            args = parse_args()
        parsed = json.loads(args.evaluators)
        assert len(parsed) == 8
        assert parsed[0]["config"]["level"] == "trace"
        assert parsed[6]["config"]["level"] == "agent"
        assert parsed[7]["config"]["level"] == "llm"


# ===========================================================================
# Evaluator registration: level propagation
# ===========================================================================


class TestEvaluatorRegistration:
    """Verify that builtin receives level from config kwargs."""

    @patch("main.builtin")
    def test_level_passed_as_kwarg(self, mock_builtin):
        """When config contains 'level', builtin must receive it as a kwarg."""
        from main import main

        evaluators = [
            {
                "identifier": "latency_performance",
                "displayName": "Latency Check",
                "config": {"max_latency_ms": 3000, "level": "trace"},
            },
            {
                "identifier": "latency_performance",
                "displayName": "Agent Latency",
                "config": {"max_latency_ms": 5000, "level": "agent"},
            },
            {
                "identifier": "latency_performance",
                "displayName": "Span Latency",
                "config": {"max_latency_ms": 1000, "level": "llm"},
            },
        ]

        mock_monitor_instance = MagicMock()
        mock_run_result = MagicMock()
        mock_run_result.traces_evaluated = 0  # Short-circuit: no traces found
        mock_run_result.errors = []  # No errors

        mock_monitor_instance.run.return_value = mock_run_result

        argv = [
            "main.py",
            "--monitor-name",
            "test",
            "--agent-id",
            "agent-1",
            "--environment-id",
            "env-1",
            "--evaluators",
            json.dumps(evaluators),
            "--trace-start",
            "2026-01-15T10:00:00Z",
            "--trace-end",
            "2026-01-15T11:00:00Z",
            "--traces-api-endpoint",
            "http://traces:8080",
            "--monitor-id",
            "550e8400-e29b-41d4-a716-446655440000",
            "--run-id",
            "660e8400-e29b-41d4-a716-446655440000",
            "--publisher-endpoint",
            "http://agent-manager:8081",
        ]

        with (
            patch.object(sys, "argv", argv),
            patch.dict("os.environ", {"PUBLISHER_API_KEY": "test-key"}),
            patch("main.TraceFetcher"),
            patch("main.Monitor", return_value=mock_monitor_instance),
            pytest.raises(SystemExit) as exc_info,
        ):
            main()

        assert exc_info.value.code == 0

        # Verify builtin was called with level in kwargs
        assert mock_builtin.call_count == 3

        mock_builtin.assert_any_call(
            "latency_performance",
            max_latency_ms=3000,
            level="trace",
        )
        mock_builtin.assert_any_call(
            "latency_performance",
            max_latency_ms=5000,
            level="agent",
        )
        mock_builtin.assert_any_call(
            "latency_performance",
            max_latency_ms=1000,
            level="llm",
        )

    @patch("main.builtin")
    def test_all_config_params_forwarded(self, mock_builtin):
        """All config params including level are unpacked as kwargs."""
        from main import main

        evaluators = [
            {
                "identifier": "content_safety",
                "displayName": "Prohibited Content",
                "config": {
                    "case_sensitive": False,
                    "prohibited_strings": ["internal error", "stack trace"],
                    "use_context_prohibited": False,
                    "level": "trace",
                },
            }
        ]

        mock_monitor_instance = MagicMock()
        mock_run_result = MagicMock()
        mock_run_result.traces_evaluated = 0

        mock_monitor_instance.run.return_value = mock_run_result

        argv = [
            "main.py",
            "--monitor-name",
            "test",
            "--agent-id",
            "agent-1",
            "--environment-id",
            "env-1",
            "--evaluators",
            json.dumps(evaluators),
            "--trace-start",
            "2026-01-15T10:00:00Z",
            "--trace-end",
            "2026-01-15T11:00:00Z",
            "--traces-api-endpoint",
            "http://traces:8080",
            "--monitor-id",
            "550e8400-e29b-41d4-a716-446655440000",
            "--run-id",
            "660e8400-e29b-41d4-a716-446655440000",
            "--publisher-endpoint",
            "http://agent-manager:8081",
        ]

        with (
            patch.object(sys, "argv", argv),
            patch.dict("os.environ", {"PUBLISHER_API_KEY": "test-key"}),
            patch("main.TraceFetcher"),
            patch("main.Monitor", return_value=mock_monitor_instance),
            pytest.raises(SystemExit),
        ):
            main()

        mock_builtin.assert_called_once_with(
            "content_safety",
            case_sensitive=False,
            prohibited_strings=["internal error", "stack trace"],
            use_context_prohibited=False,
            level="trace",
        )


# ===========================================================================
# publish_scores: payload structure matching agent-manager schemas
# ===========================================================================


class TestPublishScores:
    """Verify publish_scores builds payloads matching Go PublishScoresRequest."""

    MONITOR_ID = "550e8400-e29b-41d4-a716-446655440000"
    RUN_ID = "660e8400-e29b-41d4-a716-446655440000"
    API_ENDPOINT = "http://agent-manager:8081"
    API_KEY = "test-key"

    @patch("main.requests.post")
    def test_payload_structure_matches_go_schema(self, mock_post):
        """Payload must have individualScores and aggregatedScores matching Go structs."""
        mock_post.return_value = MagicMock(status_code=200)
        mock_post.return_value.raise_for_status = MagicMock()

        ts = datetime(2026, 1, 15, 10, 30, 0, tzinfo=timezone.utc)
        scores = {
            "Latency Check": _make_evaluator_summary(
                "Latency Check",
                "trace",
                scores=[
                    _make_evaluator_score("trace-1", 0.95, timestamp=ts, explanation="Within limits"),
                    _make_evaluator_score("trace-2", 0.30, timestamp=ts, explanation="Exceeded threshold"),
                ],
                aggregated_scores={"mean": 0.625, "pass_rate_0.5": 0.5},
            ),
        }

        display_name_to_identifier = {"Latency Check": "latency_performance"}

        result = publish_scores(
            self.MONITOR_ID,
            self.RUN_ID,
            scores,
            display_name_to_identifier,
            self.API_ENDPOINT,
            self.API_KEY,
        )
        assert result is True

        # Verify URL
        expected_url = f"{self.API_ENDPOINT}/api/v1/publisher/monitors/{self.MONITOR_ID}/runs/{self.RUN_ID}/scores"
        mock_post.assert_called_once()
        actual_url = mock_post.call_args[0][0]
        assert actual_url == expected_url

        # Verify headers
        headers = mock_post.call_args[1]["headers"]
        assert headers["x-api-key"] == self.API_KEY
        assert headers["Content-Type"] == "application/json"

        # Verify payload structure
        payload = mock_post.call_args[1]["json"]

        # --- aggregatedScores: matches PublishAggregateItem ---
        assert "aggregatedScores" in payload
        agg = payload["aggregatedScores"]
        assert len(agg) == 1
        assert agg[0]["identifier"] == "latency_performance"  # required in Go
        assert agg[0]["evaluatorName"] == "Latency Check"  # required in Go
        assert agg[0]["level"] == "trace"  # required, oneof=trace agent llm
        assert agg[0]["aggregations"] == {
            "mean": 0.625,
            "pass_rate_0.5": 0.5,
        }  # required
        assert agg[0]["count"] == 2
        assert agg[0]["skippedCount"] == 0

        # --- individualScores: matches PublishScoreItem ---
        assert "individualScores" in payload
        ind = payload["individualScores"]
        assert len(ind) == 2

        # Each item must have required fields per Go schema
        for item in ind:
            assert "evaluatorName" in item  # required
            assert "level" in item  # required
            assert "traceId" in item  # required

        assert ind[0]["evaluatorName"] == "Latency Check"
        assert ind[0]["level"] == "trace"
        assert ind[0]["traceId"] == "trace-1"
        assert ind[0]["score"] == 0.95
        assert ind[0]["explanation"] == "Within limits"

    @patch("main.requests.post")
    def test_multi_level_scores(self, mock_post):
        """Scores from trace, agent, and llm level evaluators use correct levels."""
        mock_post.return_value = MagicMock(status_code=200)
        mock_post.return_value.raise_for_status = MagicMock()

        ts = datetime(2026, 1, 15, 10, 0, 0, tzinfo=timezone.utc)

        scores = {
            "Latency Check": _make_evaluator_summary(
                "Latency Check",
                "trace",
                scores=[_make_evaluator_score("trace-1", 0.9, timestamp=ts)],
                aggregated_scores={"mean": 0.9},
            ),
            "Agent Latency": _make_evaluator_summary(
                "Agent Latency",
                "agent",
                scores=[_make_evaluator_score("trace-1", 0.7, span_id="agent-span-1", timestamp=ts)],
                aggregated_scores={"mean": 0.7},
            ),
            "Span Latency": _make_evaluator_summary(
                "Span Latency",
                "llm",
                scores=[_make_evaluator_score("trace-1", 0.5, span_id="llm-span-1", timestamp=ts)],
                aggregated_scores={"mean": 0.5},
            ),
        }

        display_name_to_identifier = {
            "Latency Check": "latency_performance",
            "Agent Latency": "latency_performance",
            "Span Latency": "latency_performance",
        }

        result = publish_scores(
            self.MONITOR_ID,
            self.RUN_ID,
            scores,
            display_name_to_identifier,
            self.API_ENDPOINT,
            self.API_KEY,
        )
        assert result is True

        payload = mock_post.call_args[1]["json"]

        # Verify aggregated levels
        agg_levels = {a["evaluatorName"]: a["level"] for a in payload["aggregatedScores"]}
        assert agg_levels["Latency Check"] == "trace"
        assert agg_levels["Agent Latency"] == "agent"
        assert agg_levels["Span Latency"] == "llm"

        # Verify individual score levels
        ind_levels = {i["evaluatorName"]: i["level"] for i in payload["individualScores"]}
        assert ind_levels["Latency Check"] == "trace"
        assert ind_levels["Agent Latency"] == "agent"
        assert ind_levels["Span Latency"] == "llm"

        # Verify llm-level scores include spanId
        span_scores = [i for i in payload["individualScores"] if i["evaluatorName"] == "Span Latency"]
        assert span_scores[0]["spanContext"]["spanId"] == "llm-span-1"

    @patch("main.requests.post")
    def test_error_scores_omit_score_field(self, mock_post):
        """When a score has an error, the score field should be omitted."""
        mock_post.return_value = MagicMock(status_code=200)
        mock_post.return_value.raise_for_status = MagicMock()

        ts = datetime(2026, 1, 15, 10, 0, 0, tzinfo=timezone.utc)

        scores = {
            "Answer Relevancy": _make_evaluator_summary(
                "Answer Relevancy",
                "trace",
                scores=[
                    _make_evaluator_score("trace-1", None, timestamp=ts, error="LLM call failed"),
                ],
                aggregated_scores={},
            ),
        }

        display_name_to_identifier = {"Answer Relevancy": "answer_relevancy"}

        publish_scores(
            self.MONITOR_ID,
            self.RUN_ID,
            scores,
            display_name_to_identifier,
            self.API_ENDPOINT,
            self.API_KEY,
        )

        payload = mock_post.call_args[1]["json"]
        ind = payload["individualScores"]
        assert len(ind) == 1
        assert "score" not in ind[0]
        assert ind[0]["skipReason"] == "LLM call failed"

        # Aggregated should reflect skipped count
        agg = payload["aggregatedScores"]
        assert agg[0]["skippedCount"] == 1

    @patch("main.requests.post")
    def test_timestamp_serialized_as_iso8601(self, mock_post):
        """traceTimestamp must be ISO 8601 string for Go time.Time parsing."""
        mock_post.return_value = MagicMock(status_code=200)
        mock_post.return_value.raise_for_status = MagicMock()

        ts = datetime(2026, 1, 15, 10, 30, 0, tzinfo=timezone.utc)

        scores = {
            "Latency Check": _make_evaluator_summary(
                "Latency Check",
                "trace",
                scores=[_make_evaluator_score("trace-1", 0.9, timestamp=ts)],
                aggregated_scores={"mean": 0.9},
            ),
        }

        publish_scores(
            self.MONITOR_ID,
            self.RUN_ID,
            scores,
            {"Latency Check": "latency"},
            self.API_ENDPOINT,
            self.API_KEY,
        )

        payload = mock_post.call_args[1]["json"]
        trace_ts = payload["individualScores"][0]["traceStartTime"]
        # Must be parseable ISO 8601
        parsed = datetime.fromisoformat(trace_ts)
        assert parsed == ts

    def test_empty_scores_returns_true(self):
        """No scores to publish should return True without making HTTP call."""
        result = publish_scores(
            self.MONITOR_ID,
            self.RUN_ID,
            {},
            {},
            self.API_ENDPOINT,
            self.API_KEY,
        )
        assert result is True

    @patch("main.requests.post")
    def test_http_failure_returns_false(self, mock_post):
        """HTTP error from agent-manager should return False."""
        import requests as req

        mock_response = MagicMock()
        mock_response.status_code = 400
        mock_response.text = "Bad Request"
        mock_post.return_value = mock_response
        mock_post.return_value.raise_for_status.side_effect = req.exceptions.HTTPError(response=mock_response)

        scores = {
            "Latency Check": _make_evaluator_summary(
                "Latency Check",
                "trace",
                scores=[_make_evaluator_score("trace-1", 0.9)],
                aggregated_scores={"mean": 0.9},
            ),
        }

        result = publish_scores(
            self.MONITOR_ID,
            self.RUN_ID,
            scores,
            {"Latency Check": "latency"},
            self.API_ENDPOINT,
            self.API_KEY,
        )
        assert result is False

    @patch("main.requests.post")
    def test_optional_fields_omitted_when_none(self, mock_post):
        """Optional fields (spanId, explanation, metadata, traceTimestamp) should be absent when None."""
        mock_post.return_value = MagicMock(status_code=200)
        mock_post.return_value.raise_for_status = MagicMock()

        scores = {
            "Latency Check": _make_evaluator_summary(
                "Latency Check",
                "trace",
                scores=[_make_evaluator_score("trace-1", 0.8, span_id=None, explanation=None, timestamp=None)],
                aggregated_scores={"mean": 0.8},
            ),
        }

        publish_scores(
            self.MONITOR_ID,
            self.RUN_ID,
            scores,
            {"Latency Check": "latency"},
            self.API_ENDPOINT,
            self.API_KEY,
        )

        payload = mock_post.call_args[1]["json"]
        item = payload["individualScores"][0]
        assert "spanId" not in item
        assert "explanation" not in item
        assert "traceTimestamp" not in item

    @patch("main.requests.post")
    def test_identifier_fallback_to_display_name(self, mock_post):
        """If display_name is not in the mapping, identifier falls back to display_name."""
        mock_post.return_value = MagicMock(status_code=200)
        mock_post.return_value.raise_for_status = MagicMock()

        scores = {
            "Unknown Evaluator": _make_evaluator_summary(
                "Unknown Evaluator",
                "trace",
                scores=[_make_evaluator_score("trace-1", 0.5)],
                aggregated_scores={"mean": 0.5},
            ),
        }

        # Empty mapping - no identifier found
        publish_scores(
            self.MONITOR_ID,
            self.RUN_ID,
            scores,
            {},
            self.API_ENDPOINT,
            self.API_KEY,
        )

        payload = mock_post.call_args[1]["json"]
        assert payload["aggregatedScores"][0]["identifier"] == "Unknown Evaluator"


# ===========================================================================
# main: end-to-end integration tests
# ===========================================================================


class TestMainIntegration:
    """End-to-end tests for the main() function with mocked dependencies."""

    BASE_ARGV = [
        "main.py",
        "--monitor-name",
        "test-monitor",
        "--agent-id",
        "agent-uid-123",
        "--environment-id",
        "env-uid-456",
        "--trace-start",
        "2026-01-15T10:00:00Z",
        "--trace-end",
        "2026-01-15T11:00:00Z",
        "--traces-api-endpoint",
        "http://traces:8080",
        "--monitor-id",
        "550e8400-e29b-41d4-a716-446655440000",
        "--run-id",
        "660e8400-e29b-41d4-a716-446655440000",
        "--publisher-endpoint",
        "http://agent-manager:8081",
    ]

    def _make_argv(self, evaluators):
        return self.BASE_ARGV + ["--evaluators", json.dumps(evaluators)]

    @patch("main.publish_scores", return_value=True)
    @patch("main.builtin")
    def test_full_flow_with_realistic_evaluators(self, mock_builtin, mock_publish):
        """Full flow with all 8 realistic evaluators: create, run, publish."""
        from main import main

        # Build mock RunResult with scores at all three levels
        mock_run_result = MagicMock()
        mock_run_result.traces_evaluated = 5
        mock_run_result.duration_seconds = 2.5
        mock_run_result.success = True
        mock_run_result.errors = []
        mock_run_result.scores = {
            "Latency Check": _make_evaluator_summary("Latency Check", "trace", [], {"mean": 0.9}),
            "Agent Latency": _make_evaluator_summary("Agent Latency", "agent", [], {"mean": 0.7}),
            "Span Latency": _make_evaluator_summary("Span Latency", "llm", [], {"mean": 0.5}),
        }

        mock_monitor_instance = MagicMock()
        mock_monitor_instance.run.return_value = mock_run_result

        argv = self._make_argv(REALISTIC_EVALUATORS)

        with (
            patch.object(sys, "argv", argv),
            patch.dict("os.environ", {"PUBLISHER_API_KEY": "test-key"}),
            patch("main.TraceFetcher"),
            patch("main.Monitor", return_value=mock_monitor_instance),
            pytest.raises(SystemExit) as exc_info,
        ):
            main()

        assert exc_info.value.code == 0

        # All 8 evaluators created via builtin()
        assert mock_builtin.call_count == 8

        # Verify levels were passed for each level type
        builtin_calls = mock_builtin.call_args_list
        levels_passed = [c.kwargs.get("level") for c in builtin_calls]
        assert levels_passed.count("trace") == 6
        assert levels_passed.count("agent") == 1
        assert levels_passed.count("llm") == 1

        # publish_scores was called
        mock_publish.assert_called_once()

    def test_missing_publisher_api_key_exits(self):
        """Should exit with code 1 when PUBLISHER_API_KEY is not set."""
        from main import main

        evaluators = [
            {
                "identifier": "latency_performance",
                "displayName": "Latency",
                "config": {"level": "trace"},
            }
        ]
        argv = self._make_argv(evaluators)

        with (
            patch.object(sys, "argv", argv),
            patch.dict("os.environ", {}, clear=True),
            pytest.raises(SystemExit) as exc_info,
        ):
            main()

        assert exc_info.value.code == 1

    def test_invalid_evaluators_json_exits(self):
        """Should exit with code 1 when --evaluators is invalid JSON."""
        from main import main

        argv = self.BASE_ARGV + ["--evaluators", "not-json"]

        with (
            patch.object(sys, "argv", argv),
            patch.dict("os.environ", {"PUBLISHER_API_KEY": "test-key"}),
            pytest.raises(SystemExit) as exc_info,
        ):
            main()

        assert exc_info.value.code == 1

    def test_empty_evaluators_array_exits(self):
        """Should exit with code 1 when --evaluators is an empty array."""
        from main import main

        argv = self._make_argv([])

        with (
            patch.object(sys, "argv", argv),
            patch.dict("os.environ", {"PUBLISHER_API_KEY": "test-key"}),
            pytest.raises(SystemExit) as exc_info,
        ):
            main()

        assert exc_info.value.code == 1

    def test_invalid_trace_start_exits(self):
        """Should exit with code 1 when --trace-start is not valid ISO 8601."""
        from main import main

        evaluators = [
            {
                "identifier": "latency_performance",
                "displayName": "Latency",
                "config": {"level": "trace"},
            }
        ]
        argv = [
            "main.py",
            "--monitor-name",
            "test",
            "--agent-id",
            "agent-1",
            "--environment-id",
            "env-1",
            "--evaluators",
            json.dumps(evaluators),
            "--trace-start",
            "bad-time",
            "--trace-end",
            "2026-01-15T11:00:00Z",
            "--traces-api-endpoint",
            "http://traces:8080",
            "--monitor-id",
            "550e8400-e29b-41d4-a716-446655440000",
            "--run-id",
            "660e8400-e29b-41d4-a716-446655440000",
            "--publisher-endpoint",
            "http://agent-manager:8081",
        ]

        with (
            patch.object(sys, "argv", argv),
            patch.dict("os.environ", {"PUBLISHER_API_KEY": "test-key"}),
            pytest.raises(SystemExit) as exc_info,
        ):
            main()

        assert exc_info.value.code == 1

    def test_evaluator_missing_identifier_exits(self):
        """Should exit with code 1 when an evaluator is missing 'identifier'."""
        from main import main

        evaluators = [{"displayName": "Latency", "config": {"level": "trace"}}]
        argv = self._make_argv(evaluators)

        with (
            patch.object(sys, "argv", argv),
            patch.dict("os.environ", {"PUBLISHER_API_KEY": "test-key"}),
            pytest.raises(SystemExit) as exc_info,
        ):
            main()

        assert exc_info.value.code == 1

    def test_evaluator_missing_display_name_exits(self):
        """Should exit with code 1 when an evaluator is missing 'displayName'."""
        from main import main

        evaluators = [{"identifier": "latency_performance", "config": {"level": "trace"}}]
        argv = self._make_argv(evaluators)

        with (
            patch.object(sys, "argv", argv),
            patch.dict("os.environ", {"PUBLISHER_API_KEY": "test-key"}),
            pytest.raises(SystemExit) as exc_info,
        ):
            main()

        assert exc_info.value.code == 1

    @patch("main.publish_scores", return_value=False)
    @patch("main.builtin")
    def test_publish_failure_exits_with_error(self, mock_builtin, mock_publish):
        """Should exit with code 1 when score publishing fails."""
        from main import main

        mock_run_result = MagicMock()
        mock_run_result.traces_evaluated = 1
        mock_run_result.duration_seconds = 1.0
        mock_run_result.success = True
        mock_run_result.errors = []
        mock_run_result.scores = {
            "Latency": _make_evaluator_summary("Latency", "trace", [], {"mean": 0.9}),
        }

        mock_monitor = MagicMock()
        mock_monitor.run.return_value = mock_run_result

        evaluators = [
            {
                "identifier": "latency_performance",
                "displayName": "Latency",
                "config": {"level": "trace"},
            }
        ]
        argv = self._make_argv(evaluators)

        with (
            patch.object(sys, "argv", argv),
            patch.dict("os.environ", {"PUBLISHER_API_KEY": "test-key"}),
            patch("main.TraceFetcher"),
            patch("main.Monitor", return_value=mock_monitor),
            pytest.raises(SystemExit) as exc_info,
        ):
            main()

        assert exc_info.value.code == 1


# ===========================================================================
# _eval_template: prompt template expression evaluation
# ===========================================================================


class TestEvalTemplate:
    """Verify _eval_template evaluates Python expressions in {…} placeholders."""

    def test_simple_attribute_access(self):
        obj = MagicMock()
        obj.name = "test-agent"
        result = _eval_template("Agent: {agent.name}", {"agent": obj})
        assert result == "Agent: test-agent"

    def test_dotted_attribute_access(self):
        obj = MagicMock()
        obj.metrics.score = 0.95
        result = _eval_template("Score: {agent.metrics.score}", {"agent": obj})
        assert result == "Score: 0.95"

    def test_or_fallback_with_value(self):
        obj = MagicMock()
        obj.agent_name = "my-agent"
        result = _eval_template("{obj.agent_name or 'default'}", {"obj": obj})
        assert result == "my-agent"

    def test_or_fallback_without_value(self):
        obj = MagicMock()
        obj.agent_name = ""
        result = _eval_template("{obj.agent_name or 'default'}", {"obj": obj})
        assert result == "default"

    def test_len_expression(self):
        obj = MagicMock()
        obj.steps = [1, 2, 3]
        result = _eval_template("Steps: {len(obj.steps)}", {"obj": obj})
        assert result == "Steps: 3"

    def test_method_call_no_args(self):
        obj = MagicMock()
        obj.format_steps.return_value = "Step 1: do thing"
        result = _eval_template("{obj.format_steps()}", {"obj": obj})
        assert result == "Step 1: do thing"

    def test_join_expression(self):
        Item = type("Item", (), {})
        items = [Item(), Item(), Item()]
        items[0].name = "tool_a"
        items[1].name = "tool_b"
        items[2].name = "tool_c"
        obj = MagicMock()
        obj.tools = items
        result = _eval_template("Tools: {', '.join(t.name for t in obj.tools)}", {"obj": obj})
        assert result == "Tools: tool_a, tool_b, tool_c"

    def test_ternary_expression(self):
        obj = MagicMock()
        obj.description = "Do the task"
        result = _eval_template("{obj.description if obj.description else '(none)'}", {"obj": obj})
        assert result == "Do the task"

    def test_ternary_expression_falsy(self):
        obj = MagicMock()
        obj.description = ""
        result = _eval_template("{obj.description if obj.description else '(none)'}", {"obj": obj})
        assert result == "(none)"

    def test_config_variable(self):
        result = _eval_template("Domain: {domain}", {"domain": "security"})
        assert result == "Domain: security"

    def test_multiple_placeholders(self):
        obj = MagicMock()
        obj.input = "hello"
        obj.output = "world"
        result = _eval_template("In: {obj.input}, Out: {obj.output}", {"obj": obj})
        assert result == "In: hello, Out: world"

    def test_no_placeholders(self):
        result = _eval_template("plain text", {})
        assert result == "plain text"

    def test_none_variable_renders_as_none_string(self):
        result = _eval_template("{x}", {"x": None})
        assert result == "None"

    def test_invalid_expression_raises(self):
        with pytest.raises(ValueError, match="Failed to evaluate"):
            _eval_template("{!!!}", {})

    def test_unknown_variable_raises(self):
        with pytest.raises(ValueError, match="Failed to evaluate"):
            _eval_template("{unknown_var}", {})


# ---------------------------------------------------------------------------
# _load_custom_code_evaluator with Param() descriptors
# ---------------------------------------------------------------------------


class TestLoadCustomCodeEvaluator:
    """Tests for loading custom code evaluators with Param() descriptors in source."""

    def test_param_descriptor_in_source(self):
        """Param() descriptors in source are honoured by with_config()."""
        source = (
            "from amp_evaluation.models import EvalResult\n"
            "from amp_evaluation import Param\n"
            "def my_eval(trace, threshold: float = Param(default=0.5)):\n"
            "    return EvalResult(score=threshold)\n"
        )

        instance = _load_custom_code_evaluator("test-eval", source, {"threshold": 0.9})
        assert instance._func_config["threshold"] == 0.9

    def test_with_config_after_load(self):
        """with_config() works on loaded custom code evaluators."""
        source = (
            "from amp_evaluation.models import EvalResult\n"
            "from amp_evaluation import Param\n"
            "def my_eval(trace, threshold: float = Param(default=0.5)):\n"
            "    return EvalResult(score=threshold)\n"
        )

        instance = _load_custom_code_evaluator("test-eval", source, {})
        updated = instance.with_config(threshold=0.9)
        assert updated._func_config["threshold"] == 0.9

    def test_empty_source_raises(self):
        """Empty source raises ValueError."""
        with pytest.raises(ValueError, match="empty source"):
            _load_custom_code_evaluator("test-eval", "", {})
