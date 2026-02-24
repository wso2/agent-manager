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
    Core types, runners, decorators, base classes, and discovery utilities.
    Everything a typical user needs in a single import.

    >>> from amp_evaluation import (
    ...     Experiment, Monitor,                    # Runners
    ...     evaluator, llm_judge,                   # Decorators
    ...     BaseEvaluator, LLMAsJudgeEvaluator,     # Base classes
    ...     EvalResult, Task, Dataset,              # Core types
    ...     Param,                                  # Parameter descriptor
    ...     builtin, builtin_evaluator_catalog,     # Built-in evaluators
    ...     discover_evaluators,                    # Module scanning
    ...     HttpAgentInvoker,                       # Invocation
    ...     Config,                                 # Configuration
    ...     EvaluationLevel,                        # Evaluation levels
    ... )

Tier 2 - Submodules for domain-specific types:

    >>> from amp_evaluation.trace import Trace, TraceFetcher, LLMSpan, AgentTrace
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
    EvalResult,
    EvaluatorInfo,
    EvaluatorScore,
    EvaluatorSummary,
    LLMConfigField,
    LLMProviderInfo,
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
)

# ============================================================================
# CONFIGURATION
# ============================================================================
from .config import (
    Config,
    # Internal but still importable from .config directly
    AgentConfig as AgentConfig,
    TraceConfig as TraceConfig,
    get_config as get_config,
    reload_config as reload_config,
)

# ============================================================================
# AGENT INVOKERS
# ============================================================================
from .invokers import AgentInvoker, InvokeResult, HttpAgentInvoker

# ============================================================================
# EVALUATION LEVELS AND MODES
# ============================================================================
from .evaluators.params import EvaluationLevel, EvalMode, Param

# ============================================================================
# BASE CLASSES
# ============================================================================
from .evaluators.base import BaseEvaluator, LLMAsJudgeEvaluator

# ============================================================================
# DECORATORS
# ============================================================================
from .registry import evaluator, llm_judge

# ============================================================================
# DISCOVERY AND BUILT-IN EVALUATORS
# ============================================================================
from .registry import discover_evaluators
from .evaluators.builtin import (
    builtin,
    list_builtin_evaluators,
    builtin_evaluator_catalog,
    get_llm_provider_catalog,
)


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
    "llm_judge",
    # -------------------------------------------------------------------------
    # Base classes
    # -------------------------------------------------------------------------
    "BaseEvaluator",
    "LLMAsJudgeEvaluator",
    # -------------------------------------------------------------------------
    # Parameter descriptor
    # -------------------------------------------------------------------------
    "Param",
    # -------------------------------------------------------------------------
    # Core types
    # -------------------------------------------------------------------------
    "EvalResult",
    "EvaluatorInfo",
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
    # Discovery and built-in evaluators
    # -------------------------------------------------------------------------
    "builtin",
    "list_builtin_evaluators",
    "builtin_evaluator_catalog",
    "get_llm_provider_catalog",
    "discover_evaluators",
    # -------------------------------------------------------------------------
    # LLM provider catalog types
    # -------------------------------------------------------------------------
    "LLMConfigField",
    "LLMProviderInfo",
    # -------------------------------------------------------------------------
    # Configuration
    # -------------------------------------------------------------------------
    "Config",
    # -------------------------------------------------------------------------
    # Evaluation levels and modes
    # -------------------------------------------------------------------------
    "EvaluationLevel",
    "EvalMode",
]
