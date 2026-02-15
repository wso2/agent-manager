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

Tier 1 - Main module (amp_evaluation):
    Core types, runners, decorators, and registry functions.
    Everything a typical user needs in a single import.

    >>> from amp_evaluation import (
    ...     Experiment, Monitor,                    # Runners
    ...     evaluator, aggregator,                  # Decorators
    ...     Observation, EvalResult, Task, Dataset,  # Core types
    ...     register_builtin, list_evaluators,       # Registry
    ...     HttpAgentInvoker,                        # Invocation
    ...     Config,                                  # Configuration
    ... )

Tier 2 - Submodules for domain-specific types:

    >>> from amp_evaluation.evaluators import BaseEvaluator, Param
    >>> from amp_evaluation.trace import Trajectory, TraceFetcher, LLMSpan
    >>> from amp_evaluation.aggregators import AggregationType, Aggregation
    >>> from amp_evaluation.dataset import generate_id

Tier 3 - Direct access to built-in evaluator classes:

    >>> from amp_evaluation.evaluators.builtin.standard import LatencyEvaluator
    >>> from amp_evaluation.evaluators.builtin.deepeval import DeepEvalToolCorrectnessEvaluator
"""

__version__ = "0.0.0-dev"

# ============================================================================
# CORE MODELS
# ============================================================================
from .models import (
    Observation,
    EvalResult,
    EvaluatorScore,
    EvaluatorSummary,
    # Internal but still importable from .models directly
    DataNotAvailableError as DataNotAvailableError,
    Agent as Agent,
)

# ============================================================================
# DATASET TYPES AND I/O
# ============================================================================
from .dataset import (
    Task,
    Dataset,
    Constraints,
    TrajectoryStep,
    load_dataset_from_json,
    load_dataset_from_csv,
    save_dataset_to_json,
)

# ============================================================================
# RUNNERS
# ============================================================================
from .runner import (
    Experiment,
    Monitor,
    RunResult,
    # Internal but still importable from .runner directly
    BaseRunner as BaseRunner,
    RunType as RunType,
)

# ============================================================================
# CONFIGURATION
# ============================================================================
from .config import (
    Config,
    # Internal but still importable from .config directly
    AgentConfig as AgentConfig,
    PlatformConfig as PlatformConfig,
    get_config as get_config,
    reload_config as reload_config,
)

# ============================================================================
# AGENT INVOKERS
# ============================================================================
from .invokers import AgentInvoker, InvokeResult, HttpAgentInvoker

# ============================================================================
# CONVENIENCE DECORATORS (allowed in main module for ergonomics)
# ============================================================================
from .registry import evaluator
from .aggregators.base import aggregator

# ============================================================================
# REGISTRY FUNCTIONS
# ============================================================================
from .registry import (
    register_builtin,
    register_evaluator,
    get_evaluator,
    list_evaluators,
    list_by_tag,
    # Internal but still importable from .registry directly
    get_evaluator_metadata as get_evaluator_metadata,
    get_registry as get_registry,
    EvaluatorRegistry as EvaluatorRegistry,
    list_builtin_evaluators as list_builtin_evaluators,
)

# ============================================================================
# AUTO-REGISTER BUILT-IN EVALUATORS
# ============================================================================
from . import evaluators as _evaluators  # noqa: F401


__all__ = [
    # Version
    "__version__",
    # -------------------------------------------------------------------------
    # Runners (main entry points)
    # -------------------------------------------------------------------------
    "Experiment",
    "Monitor",
    "RunResult",
    # -------------------------------------------------------------------------
    # Decorators (main extension points)
    # -------------------------------------------------------------------------
    "evaluator",
    "aggregator",
    # -------------------------------------------------------------------------
    # Core types
    # -------------------------------------------------------------------------
    "Observation",
    "EvalResult",
    "EvaluatorScore",
    "EvaluatorSummary",
    "Task",
    "Dataset",
    "Constraints",
    "TrajectoryStep",
    # -------------------------------------------------------------------------
    # Agent invocation
    # -------------------------------------------------------------------------
    "AgentInvoker",
    "HttpAgentInvoker",
    "InvokeResult",
    # -------------------------------------------------------------------------
    # Dataset I/O
    # -------------------------------------------------------------------------
    "load_dataset_from_json",
    "load_dataset_from_csv",
    "save_dataset_to_json",
    # -------------------------------------------------------------------------
    # Registry operations
    # -------------------------------------------------------------------------
    "register_builtin",
    "register_evaluator",
    "list_evaluators",
    "get_evaluator",
    "list_by_tag",
    # -------------------------------------------------------------------------
    # Configuration
    # -------------------------------------------------------------------------
    "Config",
]
