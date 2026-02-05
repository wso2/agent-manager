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
Evaluation framework for AI agents supporting:
- Tasks and Datasets
- Multiple evaluator types (trace, trajectory)
- Aggregation and benchmarking
- Local and cloud execution
"""

__version__ = "0.0.0-dev"

# Core models
from .models import (
    # Exceptions
    DataNotAvailableError,
    # Observation
    Observation,
    # Task & Dataset
    Task,
    Dataset,
    # Results
    EvalResult,
    EvaluatorScore,
    EvaluatorSummary,
    # Agent (minimal - from config)
    Agent,
    # Utilities
    generate_id,
)

# Dataset schema (JSON loading) - Constraints is the single source of truth
from .dataset_schema import DatasetSchema, Constraints


# Evaluator base classes
from .evaluators.base import BaseEvaluator, LLMAsJudgeEvaluator, FunctionEvaluator

# Registry
from .registry import (
    evaluator,
    get_evaluator,
    list_evaluators,
    get_evaluator_metadata,
    list_by_tag,
    list_by_type,
    get_registry,
    EvaluatorRegistry,
)

# Aggregation system
from .aggregators.base import (
    Aggregation,
    AggregationType,
    aggregator,
    register_aggregator,
    list_aggregators,
    normalize_aggregations,
    DEFAULT_AGGREGATIONS,
)

# Evaluation runners
from .runner import BaseRunner, Experiment, Monitor, RunResult, RunType

# Agent invocation
from .invokers import AgentInvoker, InvokeResult, HttpAgentInvoker

# Loaders
from .loaders import (
    DatasetLoader,
)

# Trace module - evaluation-optimized trace structures
from .trace import (
    # Core trace
    Trajectory,
    # Span types
    LLMSpan,
    ToolSpan,
    RetrieverSpan,
    AgentSpan,
    # Metrics
    TraceMetrics,
    SpanMetrics,
    LLMMetrics,
    ToolMetrics,
    RetrieverMetrics,
    AgentMetrics,
    TokenUsage,
    # Supporting types
    Message,
    ToolCall as EvalToolCall,  # Renamed to avoid conflict with models.ToolCall
    RetrievedDoc,
    # Parser functions
    parse_trace_for_evaluation,
    parse_traces_for_evaluation,
    # Fetcher
    TraceFetcher,
    TraceFetchConfig,
    TraceLoader,
)

# Configuration
from .config import Config, AgentConfig, PlatformConfig, get_config, reload_config

# Built-in evaluators (auto-register on import)
from .evaluators import builtin  # noqa: F401


__all__ = [
    # Version
    "__version__",
    # Core models
    "Observation",
    "Task",
    "Constraints",
    "Dataset",
    "DatasetSchema",
    "Agent",
    "generate_id",
    "DataNotAvailableError",
    # Results & Context
    "EvalResult",
    "EvaluatorScore",
    "EvaluatorSummary",
    "Observation",
    "Task",
    # Evaluators
    "BaseEvaluator",
    "LLMAsJudgeEvaluator",
    "FunctionEvaluator",
    # Registry
    "evaluator",
    "get_evaluator",
    "list_evaluators",
    "get_evaluator_metadata",
    "list_by_tag",
    "list_by_type",
    "get_registry",
    "EvaluatorRegistry",
    # Aggregation system
    "Aggregation",
    "AggregationType",
    "aggregator",
    "register_aggregator",
    "list_aggregators",
    "normalize_aggregations",
    "DEFAULT_AGGREGATIONS",
    # Runners
    "BaseRunner",
    "Experiment",
    "Monitor",
    "RunResult",
    "RunType",
    # Agent invocation
    "AgentInvoker",
    "InvokeResult",
    "HttpAgentInvoker",
    # Loaders
    "DatasetLoader",
    # Trace fetching
    "TraceFetcher",
    "TraceFetchConfig",
    "TraceLoader",
    # Trace module - evaluation-optimized structures
    "Trajectory",
    "LLMSpan",
    "ToolSpan",
    "RetrieverSpan",
    "AgentSpan",
    # Metrics
    "TraceMetrics",
    "SpanMetrics",
    "LLMMetrics",
    "ToolMetrics",
    "RetrieverMetrics",
    "AgentMetrics",
    "TokenUsage",
    # Supporting types
    "Message",
    "EvalToolCall",
    "RetrievedDoc",
    # Parser functions
    "parse_trace_for_evaluation",
    "parse_traces_for_evaluation",
    # Configuration
    "Config",
    "AgentConfig",
    "PlatformConfig",
    "get_config",
    "reload_config",
]
