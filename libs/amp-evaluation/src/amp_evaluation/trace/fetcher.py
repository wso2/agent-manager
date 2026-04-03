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

The OTEL models here are based on the OpenAPI spec (/traces/export endpoint):
- OTELTrace (FullTrace schema)
- OTELSpan (Span schema)
- OTELTokenUsage (TokenUsage schema)
- OTELTraceStatus (TraceStatus schema)

Named with OTEL prefix to avoid collision with evaluation models
(Trace, TokenUsage in trace/models.py).
"""

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import List, Optional, Dict, Any, Callable

from amp_evaluation.trace.models import ToolDefinition
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

    logger.warning(f"Could not parse timestamp: {raw_timestamp!r}")
    return None


# ============================================================================
# OTEL/AMP Attribute Models (matching /traces/export API response)
# ============================================================================


@dataclass
class OTELTokenUsage:
    """Token usage for LLM operations (from OpenAPI TokenUsage schema)."""

    inputTokens: int = 0
    outputTokens: int = 0
    totalTokens: int = 0


@dataclass
class OTELTraceStatus:
    """Trace execution status (from OpenAPI TraceStatus schema)."""

    errorCount: int = 0


# ============================================================================
# Typed AMP Attribute Models
# ============================================================================


@dataclass
class AmpSpanStatus:
    """Span status extracted from ampAttributes.status."""

    error: bool = False
    error_type: Optional[str] = None
    error_message: Optional[str] = None  # from ampAttributes.error.message when ERROR


@dataclass
class AmpSpanData:
    """
    Typed span data extracted from ampAttributes.data.
    Fields are populated based on span kind (llm / tool / agent / retriever).
    """

    # LLM span fields
    model: str = ""
    vendor: str = ""
    temperature: Optional[float] = None
    token_usage: Optional[OTELTokenUsage] = None

    # Tool span fields
    name: str = ""  # also used for agent name

    # Agent span fields
    framework: str = ""
    system_prompt: str = ""
    available_tools: List[ToolDefinition] = field(default_factory=list)
    max_iter: Optional[int] = None

    # Retriever span fields
    vector_db: str = ""
    top_k: int = 0


@dataclass
class AmpAttributes:
    """
    Strongly-typed representation of a span's ampAttributes object.
    Parsed once at ingestion; never accessed as a raw dict again.
    """

    kind: str = "unknown"
    input: Optional[Any] = None
    output: Optional[Any] = None
    data: AmpSpanData = field(default_factory=AmpSpanData)
    status: AmpSpanStatus = field(default_factory=AmpSpanStatus)


@dataclass
class OTELSpan:
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
    ampAttributes: AmpAttributes = field(default_factory=AmpAttributes)

    @property
    def duration_ms(self) -> float:
        """Convert nanoseconds to milliseconds."""
        return self.durationInNanos / 1_000_000


@dataclass
class OTELTrace:
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
    spans: List[OTELSpan]
    rootSpanKind: Optional[str] = None
    durationInNanos: Optional[int] = None
    spanCount: Optional[int] = None
    tokenUsage: Optional[OTELTokenUsage] = None
    status: Optional[OTELTraceStatus] = None
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


def _parse_otel_token_usage(data: Optional[Dict[str, Any]]) -> Optional[OTELTokenUsage]:
    """Parse TokenUsage from API response."""
    if not data:
        return None
    return OTELTokenUsage(
        inputTokens=data.get("inputTokens", 0),
        outputTokens=data.get("outputTokens", 0),
        totalTokens=data.get("totalTokens", 0),
    )


def _parse_trace_status(data: Optional[Dict[str, Any]]) -> Optional[OTELTraceStatus]:
    """Parse TraceStatus from API response."""
    if not data:
        return None
    return OTELTraceStatus(errorCount=data.get("errorCount", 0))


def _parse_amp_attributes(raw: Dict[str, Any], otel_status: str) -> AmpAttributes:
    """
    Parse the raw ampAttributes dict from the API into a typed AmpAttributes struct.
    All dict access is centralised here so the rest of the codebase never touches raw dicts.
    """
    raw_data: Dict[str, Any] = raw.get("data") or {}
    raw_status: Dict[str, Any] = raw.get("status") or {}

    # Error details: prefer ampAttributes.error.message, fall back to OTEL status
    error_message: Optional[str] = None
    raw_error = raw.get("error")
    if isinstance(raw_error, dict):
        error_message = raw_error.get("message")
    has_error = otel_status == "ERROR" or bool(raw_status.get("error", False))

    status = AmpSpanStatus(
        error=has_error,
        error_type=raw_status.get("errorType"),
        error_message=error_message,
    )

    # Parse tools as ToolDefinition objects
    raw_tools = raw_data.get("tools") or []
    available_tools: List[ToolDefinition] = []
    for t in raw_tools:
        if isinstance(t, dict):
            available_tools.append(
                ToolDefinition(
                    name=t.get("name", ""),
                    description=t.get("description", ""),
                    parameters=t.get("parameters", ""),
                )
            )
        elif isinstance(t, str):
            available_tools.append(ToolDefinition(name=t))

    data = AmpSpanData(
        model=raw_data.get("model") or "",
        vendor=raw_data.get("vendor") or "",
        temperature=raw_data.get("temperature"),
        token_usage=_parse_otel_token_usage(raw_data.get("tokenUsage")),
        name=raw_data.get("name") or "",
        framework=raw_data.get("framework") or "",
        system_prompt=raw_data.get("systemPrompt") or raw_data.get("system_prompt") or "",
        available_tools=available_tools,
        max_iter=raw_data.get("maxIter") or raw_data.get("max_iterations"),
        vector_db=raw_data.get("vectorDB") or raw_data.get("vector_db") or "",
        top_k=raw_data.get("topK") or raw_data.get("top_k") or 0,
    )

    return AmpAttributes(
        kind=raw.get("kind") or "unknown",
        input=raw.get("input"),
        output=raw.get("output"),
        data=data,
        status=status,
    )


def _parse_span(data: Dict[str, Any]) -> OTELSpan:
    """Parse Span from API response."""
    otel_status: str = data.get("status", "UNSET")
    raw_amp: Dict[str, Any] = data.get("ampAttributes") or {}
    return OTELSpan(
        traceId=data["traceId"],
        spanId=data["spanId"],
        name=data["name"],
        service=data["service"],
        startTime=data["startTime"],
        endTime=data["endTime"],
        durationInNanos=data["durationInNanos"],
        kind=data["kind"],
        status=otel_status,
        parentSpanId=data.get("parentSpanId"),
        attributes=data.get("attributes", {}),
        ampAttributes=_parse_amp_attributes(raw_amp, otel_status),
    )


def _parse_trace(data: Dict[str, Any]) -> OTELTrace:
    """Parse Trace from API response."""
    spans = [_parse_span(s) for s in data.get("spans", [])]

    return OTELTrace(
        traceId=data["traceId"],
        rootSpanId=data["rootSpanId"],
        rootSpanName=data["rootSpanName"],
        startTime=data["startTime"],
        endTime=data["endTime"],
        spans=spans,
        rootSpanKind=data.get("rootSpanKind"),
        durationInNanos=data.get("durationInNanos"),
        spanCount=data.get("spanCount"),
        tokenUsage=_parse_otel_token_usage(data.get("tokenUsage")),
        status=_parse_trace_status(data.get("status")),
        input=data.get("input"),
        output=data.get("output"),
        taskId=data.get("taskId"),  # Task ID from baggage
        trialId=data.get("trialId"),  # Trial ID from baggage
    )


# ============================================================================
# Trace Fetcher
# ============================================================================


class TraceFetcher:
    """
    Fetches traces from the trace service API using the /traces/export endpoint.

    Returns Trace objects (OTEL/AMP attributes) that can be parsed into
    Trajectory objects using parse_trace_for_evaluation().

    Usage:
        fetcher = TraceFetcher(
            base_url="http://localhost:8001",
            agent_uid="my-agent",
            environment_uid="prod",
            token_provider=token_manager.get_token,
        )
        traces = fetcher.fetch_traces(
            start_time="2024-01-26T10:00:00Z",
            end_time="2024-01-26T12:00:00Z"
        )
    """

    def __init__(
        self,
        base_url: str,
        agent_uid: str,
        environment_uid: str,
        token_provider: Optional[Callable[[], str]] = None,
        timeout: int = 30,
    ):
        """
        Initialize trace fetcher.

        Args:
            base_url: Base URL of the trace service (required)
            agent_uid: Agent unique identifier (required)
            environment_uid: Environment unique identifier (required)
            token_provider: Callable that returns a JWT token for authentication (required)
            timeout: Request timeout in seconds
        """
        if not base_url:
            raise ValueError("base_url is required")
        if not agent_uid:
            raise ValueError("agent_uid is required")
        if not environment_uid:
            raise ValueError("environment_uid is required")
        if not token_provider:
            raise ValueError("token_provider is required")

        self.base_url = base_url.rstrip("/")
        self.agent_uid = agent_uid
        self.environment_uid = environment_uid
        self.token_provider = token_provider
        self.timeout = timeout

    def _get_auth_headers(self) -> Dict[str, str]:
        """Get authorization headers with a fresh JWT token."""
        token = self.token_provider()
        return {"Authorization": f"Bearer {token}"}

    def fetch_traces(self, start_time: str, end_time: str) -> List[OTELTrace]:
        """
        Fetch traces from the trace service using /traces/export endpoint.

        Args:
            start_time: Start time in ISO 8601 format (e.g., "2025-12-16T06:58:02.433Z")
            end_time: End time in ISO 8601 format

        Returns:
            List of Trace objects with OTEL/AMP attributes
        """
        headers = self._get_auth_headers()
        try:
            response = requests.get(
                f"{self.base_url}/api/v1/traces/export",
                params={
                    "startTime": start_time,
                    "endTime": end_time,
                    "componentUid": self.agent_uid,
                    "environmentUid": self.environment_uid,
                },
                headers=headers,
                timeout=self.timeout,
            )
            response.raise_for_status()
            data = response.json()

            # Parse TraceExportResponse
            traces_data = data.get("traces", [])
            return [_parse_trace(t) for t in traces_data]

        except requests.exceptions.RequestException as e:
            logger.error(f"Failed to fetch traces: {e}")
            raise

    def fetch_trace_by_id(self, trace_id: str) -> Optional[OTELTrace]:
        """
        Fetch a single trace by its ID using /trace endpoint.

        Args:
            trace_id: The unique identifier of the trace

        Returns:
            Trace object or None if not found
        """
        headers = self._get_auth_headers()
        try:
            response = requests.get(
                f"{self.base_url}/api/v1/trace",
                params={"traceId": trace_id, "componentUid": self.agent_uid, "environmentUid": self.environment_uid},
                headers=headers,
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

            return OTELTrace(
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
        loader = TraceLoader(file_path="traces.json")
        traces = loader.load_traces()
    """

    def __init__(self, file_path: str):
        """
        Initialize trace loader.

        Args:
            file_path: Path to JSON file containing traces (required)
        """
        if not file_path:
            raise ValueError("file_path is required")

        self.file_path = Path(file_path)
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

    def load_traces(self, start_time: Optional[str] = None, end_time: Optional[str] = None) -> List[OTELTrace]:
        """
        Load traces from the file.

        Args:
            start_time: Optional start time filter (ISO 8601)
            end_time: Optional end time filter (ISO 8601)

        Returns:
            List of Trace objects
        """
        if self._traces is None:
            self._traces = self._load_traces_from_file()

        # Apply filters
        remaining = self._traces[self._last_loaded_index :]

        if start_time or end_time:
            batch = [t for t in remaining if self._matches_time_filter(t, start_time, end_time)]
        else:
            batch = remaining
        # Advance past ALL examined traces (not just those that passed the filter)
        # so the next call doesn't re-scan or re-return already-seen entries.
        if batch:
            last_returned = batch[-1]
            raw_index = self._traces.index(last_returned, self._last_loaded_index)
            self._last_loaded_index = raw_index + 1

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
