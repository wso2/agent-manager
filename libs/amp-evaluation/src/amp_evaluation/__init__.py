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
- Tasks, Trials, and Trajectories
- Multiple evaluator types (trace, trajectory, outcome)
- Aggregation and benchmarking
- Local and cloud execution
"""

__version__ = "0.0.0-dev"

# Core models
from .models import (
    # Exceptions
    DataNotAvailableError,
    # Trace & Trajectory
    Trace,
    Span,
    Trajectory,
    TrajectoryStep,
    SpanStatus,
    LLMTokenUsage,
    ToolCall,
    PromptMessage,
    RetrievedDocument,
    # Task & Trial
    Task,
    TaskInput,
    TaskSuccessCriteria,
    Trial,
    TrialStatus,
    Outcome,
    # Metrics & Results
    Metrics,
    EvalResult,
    EvalContext,
    CompositeScore,
    Constraints,
    # Dataset & Benchmark
    Dataset,
    DatasetSchema,
    Benchmark,
    BenchmarkEnvironment,
    # Agent (minimal - from config)
    Agent,
    # Utilities
    generate_id,
)


# Evaluator base classes
from .evaluators.base import BaseEvaluator, LLMAsJudgeEvaluator, CompositeEvaluator, FunctionEvaluator

# Registry
from .registry import (
    register,
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
    register_aggregator,
    list_aggregators,
    normalize_aggregations,
    DEFAULT_AGGREGATIONS,
)

from .aggregators.aggregation import ResultAggregator, AggregatedResults

# Evaluation runners
from .runner import BaseRunner, BenchmarkRunner, LiveRunner, RunResult, RunType, evaluate

# Loaders
from .loaders import (
    TraceLoader as LegacyTraceLoader,
    JSONFileTraceLoader,
    ProductionTraceLoader,
    DatasetLoader,
    DatasetMatcher,
)

# Trace module - evaluation-optimized trace structures
from .trace import (
    # Core trace
    EvalTrace,
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
    "Trace",
    "Span",
    "Trajectory",
    "TrajectoryStep",
    "SpanStatus",
    "LLMTokenUsage",
    "ToolCall",
    "PromptMessage",
    "RetrievedDocument",
    "Task",
    "TaskInput",
    "TaskSuccessCriteria",
    "Trial",
    "TrialStatus",
    "Outcome",
    "Metrics",
    "EvalResult",
    "CompositeScore",
    "Constraints",
    "Dataset",
    "DatasetSchema",
    "Benchmark",
    "BenchmarkEnvironment",
    "Agent",
    "generate_id",
    "DataNotAvailableError",
    "EvalContext",
    # Evaluators
    "BaseEvaluator",
    "LLMAsJudgeEvaluator",
    "CompositeEvaluator",
    "FunctionEvaluator",
    # Registry
    "register",
    "get_evaluator",
    "list_evaluators",
    "get_evaluator_metadata",
    "list_by_tag",
    "list_by_type",
    "get_registry",
    "EvaluatorRegistry",
    # Aggregation system
    "ResultAggregator",
    "AggregatedResults",
    "Aggregation",
    "AggregationType",
    "register_aggregator",
    "list_aggregators",
    "normalize_aggregations",
    "DEFAULT_AGGREGATIONS",
    # Runners
    "BaseRunner",
    "BenchmarkRunner",
    "LiveRunner",
    "RunResult",
    "RunType",
    "evaluate",
    # Loaders
    "TraceLoader",
    "LegacyTraceLoader",
    "JSONFileTraceLoader",
    "ProductionTraceLoader",
    "DatasetLoader",
    "DatasetMatcher",
    # Trace fetching
    "TraceFetcher",
    "TraceFetchConfig",
    # Trace module - evaluation-optimized structures
    "EvalTrace",
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
