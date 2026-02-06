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
Evaluation framework for AI agents.

Import Structure:
-----------------

Main module (amp_evaluation):
    Core models, runners, config, and convenience decorators.

    >>> from amp_evaluation import (
    ...     # Core models
    ...     Observation, Task, Dataset, EvalResult, Agent,
    ...     # Runners
    ...     Experiment, Monitor,
    ...     # Config
    ...     Config,
    ...     # Convenience decorators (allowed here for ergonomics)
    ...     evaluator, aggregator,
    ...     # Registry functions
    ...     register_builtin, get_evaluator, list_evaluators,
    ... )

Submodules for domain-specific types:

    >>> from amp_evaluation.evaluators import BaseEvaluator, FunctionEvaluator
    >>> from amp_evaluation.evaluators.builtin.standard import LatencyEvaluator
    >>> from amp_evaluation.evaluators.builtin.deepeval import DeepEvalToolCorrectnessEvaluator

    >>> from amp_evaluation.trace import Trajectory, LLMSpan, ToolSpan, TokenUsage
    >>> from amp_evaluation.trace import parse_trace_for_evaluation, TraceFetcher

    >>> from amp_evaluation.aggregators import AggregationType, Aggregation
"""

__version__ = "0.0.0-dev"

# ============================================================================
# CORE MODELS
# ============================================================================
from .models import (
    # Exceptions
    DataNotAvailableError,
    # Observation
    Observation,
    # Task & Dataset (re-exported from dataset module)
    Task,
    Dataset,
    Constraints,
    TrajectoryStep,
    generate_id,
    # Results
    EvalResult,
    EvaluatorScore,
    EvaluatorSummary,
    # Agent (minimal - from config)
    Agent,
)

# ============================================================================
# RUNNERS
# ============================================================================
from .runner import BaseRunner, Experiment, Monitor, RunResult, RunType

# ============================================================================
# DATASET LOADING
# ============================================================================
from .dataset import (
    load_dataset_from_json,
    load_dataset_from_csv,
    save_dataset_to_json,
)

# ============================================================================
# CONFIGURATION
# ============================================================================
from .config import Config, AgentConfig, PlatformConfig, get_config, reload_config

# ============================================================================
# AGENT INVOKERS
# ============================================================================
from .invokers import AgentInvoker, InvokeResult, HttpAgentInvoker

# ============================================================================
# CONVENIENCE DECORATORS (allowed in main module for ergonomics)
# ============================================================================
# @evaluator decorator - for registering custom evaluators
from .registry import evaluator

# @aggregator decorator - for registering custom aggregators
from .aggregators.base import aggregator

# ============================================================================
# REGISTRY FUNCTIONS (convenience access)
# ============================================================================
from .registry import (
    get_evaluator,
    list_evaluators,
    get_evaluator_metadata,
    list_by_tag,
    get_registry,
    EvaluatorRegistry,
    register_evaluator,
    register_builtin,
    list_builtin_evaluators,
)

# ============================================================================
# AUTO-REGISTER BUILT-IN EVALUATORS
# ============================================================================
# Import evaluators package to trigger auto-registration of built-ins
from . import evaluators as _evaluators  # noqa: F401


__all__ = [
    # Version
    "__version__",
    # -------------------------------------------------------------------------
    # Core models
    # -------------------------------------------------------------------------
    "Observation",
    "Task",
    "Dataset",
    "Agent",
    "EvalResult",
    "EvaluatorScore",
    "EvaluatorSummary",
    "DataNotAvailableError",
    "generate_id",
    # Dataset schema
    "Constraints",
    "TrajectoryStep",
    "load_dataset_from_json",
    "load_dataset_from_csv",
    "save_dataset_to_json",
    # -------------------------------------------------------------------------
    # Runners
    # -------------------------------------------------------------------------
    "BaseRunner",
    "Experiment",
    "Monitor",
    "RunResult",
    "RunType",
    # -------------------------------------------------------------------------
    # Configuration
    # -------------------------------------------------------------------------
    "Config",
    "AgentConfig",
    "PlatformConfig",
    "get_config",
    "reload_config",
    # -------------------------------------------------------------------------
    # Agent invocation
    # -------------------------------------------------------------------------
    "AgentInvoker",
    "InvokeResult",
    "HttpAgentInvoker",
    # -------------------------------------------------------------------------
    # Loaders (deprecated - use functions directly)
    # -------------------------------------------------------------------------
    # -------------------------------------------------------------------------
    # Convenience decorators (allowed in main module)
    # -------------------------------------------------------------------------
    "evaluator",
    "aggregator",
    # -------------------------------------------------------------------------
    # Registry functions
    # -------------------------------------------------------------------------
    "get_evaluator",
    "list_evaluators",
    "get_evaluator_metadata",
    "list_by_tag",
    "get_registry",
    "EvaluatorRegistry",
    "register_evaluator",
    "register_builtin",
    "list_builtin_evaluators",
]
