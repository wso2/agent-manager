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
Trace fetcher for loading traces from the trace service API.

This module handles fetching traces from external trace service APIs.
Uses OTEL/AMP attribute models that match the trace service API schema.
These traces can then be parsed into Trajectory objects for evaluation.

The models here are based on the OpenAPI spec (/traces/export endpoint):
- Trace (FullTrace schema)
- Span (Span schema)
- TokenUsage (TokenUsage schema)
- TraceStatus (TraceStatus schema)
"""

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import List, Optional, Dict, Any
from pathlib import Path
import json
import logging
import requests


logger = logging.getLogger(__name__)


# ============================================================================
# Helper Functions
# ============================================================================


def _parse_timestamp(raw_timestamp: Any) -> Optional[datetime]:
    """Parse timestamp from various formats."""
    if raw_timestamp is None:
        return None

    if isinstance(raw_timestamp, datetime):
        return raw_timestamp

    if isinstance(raw_timestamp, str):
        try:
            # Try ISO format
            if raw_timestamp.endswith("Z"):
                raw_timestamp = raw_timestamp[:-1] + "+00:00"
            return datetime.fromisoformat(raw_timestamp)
        except (ValueError, TypeError):
            pass

    if isinstance(raw_timestamp, (int, float)):
        try:
            # Assume milliseconds
            return datetime.fromtimestamp(raw_timestamp / 1000, tz=timezone.utc)
        except (ValueError, TypeError):
            pass

    return None


# ============================================================================
# OTEL/AMP Attribute Models (matching /traces/export API response)
# ============================================================================


@dataclass
class TokenUsage:
    """Token usage for LLM operations (from OpenAPI TokenUsage schema)."""

    inputTokens: int = 0
    outputTokens: int = 0
    totalTokens: int = 0


@dataclass
class TraceStatus:
    """Trace execution status (from OpenAPI TraceStatus schema)."""

    errorCount: int = 0


@dataclass
class Span:
    """
    A single span in the trace (from OpenAPI Span schema).
    Represents one operation from OTEL/AMP attributes.
    """

    traceId: str
    spanId: str
    name: str
    service: str
    startTime: str  # ISO 8601 format
    endTime: str  # ISO 8601 format
    durationInNanos: int
    kind: str  # CLIENT, SERVER, PRODUCER, CONSUMER, INTERNAL
    status: str  # OK, ERROR, UNSET
    parentSpanId: Optional[str] = None
    attributes: Dict[str, Any] = field(default_factory=dict)
    ampAttributes: Dict[str, Any] = field(default_factory=dict)

    @property
    def duration_ms(self) -> float:
        """Convert nanoseconds to milliseconds."""
        return self.durationInNanos / 1_000_000


@dataclass
class Trace:
    """
    Complete trace from the trace service (from OpenAPI FullTrace schema).
    This is the raw OTEL/AMP attribute model returned by /traces/export.
    Can be converted to Trajectory using parse_trace_for_evaluation().
    """

    traceId: str
    rootSpanId: str
    rootSpanName: str
    startTime: str  # ISO 8601 format
    endTime: str  # ISO 8601 format
    spans: List[Span]
    rootSpanKind: Optional[str] = None
    durationInNanos: Optional[int] = None
    spanCount: Optional[int] = None
    tokenUsage: Optional[TokenUsage] = None
    status: Optional[TraceStatus] = None
    input: Optional[Any] = None  # oneOf: string, object, array
    output: Optional[Any] = None  # oneOf: string, object, array
    taskId: Optional[str] = None  # Task ID from baggage (for evaluation experiments)
    trialId: Optional[str] = None  # Trial ID from baggage (for evaluation experiments)

    @property
    def duration_ms(self) -> float:
        """Convert nanoseconds to milliseconds."""
        if self.durationInNanos:
            return self.durationInNanos / 1_000_000
        return 0.0

    @property
    def timestamp(self) -> Optional[datetime]:
        """Parse startTime to datetime."""
        return _parse_timestamp(self.startTime)


# ============================================================================
# Helper functions to parse API responses into models
# ============================================================================


def _parse_token_usage(data: Optional[Dict[str, Any]]) -> Optional[TokenUsage]:
    """Parse TokenUsage from API response."""
    if not data:
        return None
    return TokenUsage(
        inputTokens=data.get("inputTokens", 0),
        outputTokens=data.get("outputTokens", 0),
        totalTokens=data.get("totalTokens", 0),
    )


def _parse_trace_status(data: Optional[Dict[str, Any]]) -> Optional[TraceStatus]:
    """Parse TraceStatus from API response."""
    if not data:
        return None
    return TraceStatus(errorCount=data.get("errorCount", 0))


def _parse_span(data: Dict[str, Any]) -> Span:
    """Parse Span from API response."""
    return Span(
        traceId=data["traceId"],
        spanId=data["spanId"],
        name=data["name"],
        service=data["service"],
        startTime=data["startTime"],
        endTime=data["endTime"],
        durationInNanos=data["durationInNanos"],
        kind=data["kind"],
        status=data["status"],
        parentSpanId=data.get("parentSpanId"),
        attributes=data.get("attributes", {}),
        ampAttributes=data.get("ampAttributes", {}),
    )


def _parse_trace(data: Dict[str, Any]) -> Trace:
    """Parse Trace from API response."""
    spans = [_parse_span(s) for s in data.get("spans", [])]

    return Trace(
        traceId=data["traceId"],
        rootSpanId=data["rootSpanId"],
        rootSpanName=data["rootSpanName"],
        startTime=data["startTime"],
        endTime=data["endTime"],
        spans=spans,
        rootSpanKind=data.get("rootSpanKind"),
        durationInNanos=data.get("durationInNanos"),
        spanCount=data.get("spanCount"),
        tokenUsage=_parse_token_usage(data.get("tokenUsage")),
        status=_parse_trace_status(data.get("status")),
        input=data.get("input"),
        output=data.get("output"),
        taskId=data.get("taskId"),  # Task ID from baggage
        trialId=data.get("trialId"),  # Trial ID from baggage
    )


# ============================================================================
# Trace Fetcher
# ============================================================================


@dataclass
class TraceFetchConfig:
    """Configuration for trace fetching."""

    base_url: str
    agent_uid: str
    environment_uid: str
    timeout: int = 30
    batch_size: int = 100


class TraceFetcher:
    """
    Fetches traces from the trace service API using the /traces/export endpoint.

    Returns Trace objects (OTEL/AMP attributes) that can be parsed into
    Trajectory objects using parse_trace_for_evaluation().

    Usage:
        fetcher = TraceFetcher(
            base_url="http://localhost:8001",
            agent_uid="my-agent",
            environment_uid="prod"
        )
        traces = fetcher.fetch_traces(
            start_time="2024-01-26T10:00:00Z",
            end_time="2024-01-26T12:00:00Z"
        )
    """

    def __init__(self, base_url: str, agent_uid: str, environment_uid: str, timeout: int = 30):
        """
        Initialize trace fetcher.

        Args:
            base_url: Base URL of the trace service (required)
            agent_uid: Agent unique identifier (required)
            environment_uid: Environment unique identifier (required)
            timeout: Request timeout in seconds
        """
        if not base_url:
            raise ValueError("base_url is required")
        if not agent_uid:
            raise ValueError("agent_uid is required")
        if not environment_uid:
            raise ValueError("environment_uid is required")

        self.base_url = base_url.rstrip("/")
        self.agent_uid = agent_uid
        self.environment_uid = environment_uid
        self.timeout = timeout

    def fetch_traces(self, start_time: str, end_time: str, limit: int = 100, offset: int = 0) -> List[Trace]:
        """
        Fetch traces from the trace service using /traces/export endpoint.

        Args:
            start_time: Start time in ISO 8601 format (e.g., "2025-12-16T06:58:02.433Z")
            end_time: End time in ISO 8601 format
            limit: Maximum number of traces to fetch (max 1000)
            offset: Number of traces to skip for pagination

        Returns:
            List of Trace objects with OTEL/AMP attributes
        """
        try:
            response = requests.get(
                f"{self.base_url}/api/v1/traces/export",
                params={
                    "startTime": start_time,
                    "endTime": end_time,
                    "componentUid": self.agent_uid,
                    "environmentUid": self.environment_uid,
                    "limit": str(limit),
                    "offset": str(offset),
                },
                timeout=self.timeout,
            )
            response.raise_for_status()
            data = response.json()

            # Parse TraceExportResponse
            traces_data = data.get("traces", [])
            return [_parse_trace(t) for t in traces_data]

        except requests.exceptions.RequestException as e:
            logger.error(f"Failed to fetch traces: {e}")
            return []

    def fetch_trace_by_id(self, trace_id: str) -> Optional[Trace]:
        """
        Fetch a single trace by its ID using /trace endpoint.

        Args:
            trace_id: The unique identifier of the trace

        Returns:
            Trace object or None if not found
        """
        try:
            response = requests.get(
                f"{self.base_url}/api/v1/trace",
                params={"traceId": trace_id, "componentUid": self.agent_uid, "environmentUid": self.environment_uid},
                timeout=self.timeout,
            )
            response.raise_for_status()
            data = response.json()

            # Parse TraceDetailsResponse and construct Trace
            spans_data = data.get("spans", [])
            if not spans_data:
                return None

            spans = [_parse_span(s) for s in spans_data]

            # Find root span to get trace-level info
            root_span = next((s for s in spans if s.parentSpanId is None), spans[0])

            return Trace(
                traceId=trace_id,
                rootSpanId=root_span.spanId,
                rootSpanName=root_span.name,
                startTime=root_span.startTime,
                endTime=root_span.endTime,
                spans=spans,
                durationInNanos=root_span.durationInNanos,
                spanCount=len(spans),
                input=root_span.attributes.get("input"),
                output=root_span.attributes.get("output"),
            )

        except requests.exceptions.RequestException as e:
            logger.error(f"Failed to fetch trace {trace_id}: {e}")
            return None

    def health_check(self) -> bool:
        """
        Check if the trace service is accessible.

        Returns:
            True if service is healthy, False otherwise
        """
        try:
            response = requests.get(f"{self.base_url}/health", timeout=5)
            return response.status_code == 200
        except Exception:
            return False


# ============================================================================
# Trace Loader (for loading traces from files)
# ============================================================================


class TraceLoader:
    """
    Loads traces from local JSON files.

    Usage:
        loader = TraceLoader(
            file_path="traces.json",
            agent_uid="my-agent",
            environment_uid="prod"
        )
        traces = loader.load_batch(limit=50)
    """

    def __init__(self, file_path: str, agent_uid: str, environment_uid: str):
        """
        Initialize trace loader.

        Args:
            file_path: Path to JSON file containing traces (required)
            agent_uid: Agent identifier to filter by (required)
            environment_uid: Environment identifier to filter by (required)
        """
        if not file_path:
            raise ValueError("file_path is required")
        if not agent_uid:
            raise ValueError("agent_uid is required")
        if not environment_uid:
            raise ValueError("environment_uid is required")

        self.file_path = Path(file_path)
        self.agent_uid = agent_uid
        self.environment_uid = environment_uid
        self._traces: Optional[List[Dict[str, Any]]] = None
        self._last_loaded_index = 0

    def _load_traces_from_file(self) -> List[Dict[str, Any]]:
        """Load all traces from the JSON file."""
        if not self.file_path.exists():
            logger.error(f"Trace file not found: {self.file_path}")
            return []

        try:
            with open(self.file_path, "r") as f:
                data = json.load(f)

                # Handle different JSON structures
                if isinstance(data, list):
                    return data
                elif isinstance(data, dict):
                    return data.get("traces", [])
                else:
                    logger.error(f"Unexpected JSON structure in {self.file_path}")
                    return []

        except json.JSONDecodeError as e:
            logger.error(f"Failed to parse JSON from {self.file_path}: {e}")
            return []

    def load_batch(
        self, limit: int = 100, start_time: Optional[str] = None, end_time: Optional[str] = None
    ) -> List[Trace]:
        """
        Load a batch of traces from the file.

        Args:
            limit: Maximum number of traces to load
            start_time: Optional start time filter (ISO 8601)
            end_time: Optional end time filter (ISO 8601)

        Returns:
            List of Trace objects
        """
        if self._traces is None:
            self._traces = self._load_traces_from_file()

        # Apply filters
        filtered_traces = self._traces[self._last_loaded_index :]

        if start_time or end_time:
            filtered_traces = [t for t in filtered_traces if self._matches_time_filter(t, start_time, end_time)]

        # Take batch
        batch = filtered_traces[:limit]
        self._last_loaded_index += len(batch)

        # Parse to Trace objects
        return [_parse_trace(t) for t in batch]

    def _matches_time_filter(
        self, trace_data: Dict[str, Any], start_time: Optional[str], end_time: Optional[str]
    ) -> bool:
        """Check if trace matches time filters."""
        trace_time = trace_data.get("startTime")
        if not trace_time:
            return False

        if start_time and trace_time < start_time:
            return False
        if end_time and trace_time > end_time:
            return False

        return True

    def reset_checkpoint(self):
        """Reset the loading checkpoint to start from beginning."""
        self._last_loaded_index = 0
