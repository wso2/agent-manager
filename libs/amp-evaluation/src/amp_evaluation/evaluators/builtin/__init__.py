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
Built-in evaluators: factory, discovery, and catalog.

Three functions:
- builtin(name, **config): Factory to get a configured built-in evaluator
- list_builtin_evaluators(mode=None): Get names of all built-in evaluators
- builtin_evaluator_catalog(mode=None): Get full metadata for all built-ins

18 LLM-as-judge evaluators (single criterion per evaluator, 5-point rubrics):
  TRACE (10): helpfulness, clarity, accuracy, completeness, faithfulness,
              context_relevance, instruction_following, relevance,
              semantic_similarity, hallucination
  LLM (4):   coherence, conciseness, safety, tone
  AGENT (4):  goal_clarity, reasoning_quality, path_efficiency, error_recovery

Tagging Taxonomy
================
Every built-in evaluator has tags in this order: [origin, method, aspect(s), framework?]

- origin:    "builtin" for all built-in evaluators
- method:    "llm-judge" (uses LLM for evaluation) or "rule-based" (deterministic)
- aspect:    quality dimension measured. Use 1-2 from this list:
             correctness, relevance, quality, safety, efficiency,
             compliance, reasoning, tool-use
             Use 2 aspect tags when an evaluator spans two dimensions
             (e.g., conciseness -> quality + efficiency).
- framework: "deepeval" only for deepeval wrapper evaluators

When adding new evaluators, follow these tagging rules:
1. Always start with "builtin"
2. Choose "llm-judge" or "rule-based" based on implementation
3. Pick the best-fitting aspect tag(s) from the list above
4. Only add "deepeval" if wrapping a DeepEval metric
"""

import importlib
import inspect
import logging
from pathlib import Path
from typing import Dict, Type, Optional, List

from amp_evaluation.evaluators.base import BaseEvaluator, validate_unique_evaluator_names
from amp_evaluation.models import EvaluatorInfo, LLMConfigField, LLMProviderInfo

logger = logging.getLogger(__name__)


def _get_evaluator_modules() -> List[str]:
    """Discover all evaluator modules in the builtin/ directory."""
    builtin_dir = Path(__file__).parent

    modules = []
    for file in builtin_dir.glob("*.py"):
        if file.stem in ("__init__",) or file.stem.startswith("_"):
            continue
        modules.append(file.stem)

    return modules


def _discover_builtin_class(name: str) -> Optional[Type[BaseEvaluator]]:
    """
    Internal helper to find a built-in evaluator class by name.

    Scans all modules in the builtin/ directory for evaluator classes
    and matches by the class's `name` attribute.
    """
    modules = _get_evaluator_modules()

    for module_name in modules:
        try:
            module = importlib.import_module(f"amp_evaluation.evaluators.builtin.{module_name}")

            for class_name, obj in inspect.getmembers(module, inspect.isclass):
                if not issubclass(obj, BaseEvaluator) or obj is BaseEvaluator:
                    continue

                if class_name.endswith("Base") or class_name.endswith("BaseEvaluator"):
                    continue

                abstract_methods: frozenset = getattr(obj, "__abstractmethods__", frozenset())
                if abstract_methods:
                    continue

                if obj.__module__ != module.__name__:
                    continue

                # Check the class's name attribute directly
                class_eval_name = getattr(obj, "name", "")
                if class_eval_name == name:
                    return obj

        except ImportError:
            continue

    return None


def builtin(name: str, **kwargs) -> BaseEvaluator:
    """
    Factory to get a configured built-in evaluator by name.

    Args:
        name: Built-in evaluator name (e.g., "latency", "deepeval/plan-quality")
        **kwargs: Configuration parameters passed to evaluator constructor

    Returns:
        Configured evaluator instance

    Raises:
        ValueError: If the evaluator name is not found
        TypeError: If invalid kwargs passed to constructor

    Example:
        latency = builtin("latency", max_latency_ms=5000)
        hallucination = builtin("hallucination")
        safety = builtin("safety", context="customer support")
    """
    evaluator_class = _discover_builtin_class(name)
    if evaluator_class is None:
        available = list_builtin_evaluators()
        raise ValueError(f"Unknown built-in evaluator '{name}'.\nAvailable: {available}")

    try:
        instance = evaluator_class(**kwargs)
        return instance
    except TypeError as e:
        raise TypeError(f"Invalid configuration for evaluator '{name}': {e}") from e


def list_builtin_evaluators(mode: Optional[str] = None) -> List[str]:
    """
    List names of all available built-in evaluators.

    Args:
        mode: Optional filter — "experiment" or "monitor".
              If provided, only evaluators supporting that mode are returned.

    Returns:
        List of evaluator name strings.

    Example:
        all_names = list_builtin_evaluators()
        monitor_names = list_builtin_evaluators(mode="monitor")
    """
    catalog = builtin_evaluator_catalog(mode=mode)
    return [info.name for info in catalog]


def builtin_evaluator_catalog(mode: Optional[str] = None) -> List[EvaluatorInfo]:
    """
    Get full metadata for all built-in evaluators.

    Returns EvaluatorInfo with complete metadata, config schemas, level, modes.

    Args:
        mode: Optional filter — "experiment" or "monitor".

    Returns:
        List of EvaluatorInfo objects.

    Example:
        catalog = builtin_evaluator_catalog()
        for info in catalog:
            print(info.name, info.level, info.config_schema)
    """
    evaluators: List[EvaluatorInfo] = []
    instances: List[BaseEvaluator] = []
    modules = _get_evaluator_modules()

    for module_name in modules:
        try:
            module = importlib.import_module(f"amp_evaluation.evaluators.builtin.{module_name}")

            for class_name, obj in inspect.getmembers(module, inspect.isclass):
                if not issubclass(obj, BaseEvaluator) or obj is BaseEvaluator:
                    continue

                if class_name.endswith("Base") or class_name.endswith("BaseEvaluator"):
                    continue

                abstract_methods: frozenset = getattr(obj, "__abstractmethods__", frozenset())
                if abstract_methods:
                    continue

                if obj.__module__ != module.__name__:
                    continue

                try:
                    instance = obj()
                    info = instance.info

                    info.class_name = class_name
                    info.module = module_name

                    instances.append(instance)
                    evaluators.append(info)
                except Exception as e:
                    logger.debug(f"Skipping {class_name} in {module_name}: {e}")
                    continue

        except ImportError:
            continue

    # Validate no duplicate names across all builtin modules
    try:
        validate_unique_evaluator_names(instances)
    except ValueError as e:
        logger.error(f"Built-in evaluator name conflict: {e}")
        raise

    if mode:
        evaluators = [ev for ev in evaluators if mode in ev.modes]

    return evaluators


# ── LLM Provider Catalog ─────────────────────────────────────────────────────

# Supported LLM providers for LLM-as-judge evaluation.
# Models are curated chat/completion models only — no audio/realtime/vision/embedding/image.
# Model names use provider/model format (litellm-compatible identifiers).
# Ordered powerful → lightweight within each provider.
# Curated from litellm.model_cost (2026-02). Review periodically when major model releases are announced.
_SUPPORTED_PROVIDERS: Dict[str, dict] = {
    "openai": {
        "display_name": "OpenAI",
        "env_var": "OPENAI_API_KEY",
        "models": [
            "openai/gpt-5.2",
            "openai/gpt-5.1",
            "openai/gpt-5",
            "openai/gpt-5-mini",
            "openai/gpt-5-nano",
            "openai/gpt-4.5-preview",
            "openai/gpt-4.1",
            "openai/gpt-4.1-mini",
            "openai/gpt-4.1-nano",
            "openai/gpt-4o",
            "openai/gpt-4o-mini",
            "openai/o3",
            "openai/o3-mini",
            "openai/o1",
            "openai/o1-mini",
        ],
    },
    "anthropic": {
        "display_name": "Anthropic",
        "env_var": "ANTHROPIC_API_KEY",
        "models": [
            "anthropic/claude-opus-4-6",
            "anthropic/claude-sonnet-4-6",
            "anthropic/claude-opus-4-5",
            "anthropic/claude-sonnet-4-5",
            "anthropic/claude-haiku-4-5",
            "anthropic/claude-opus-4-1",
            "anthropic/claude-3-7-sonnet-latest",
            "anthropic/claude-3-5-sonnet-latest",
            "anthropic/claude-3-5-haiku-latest",
            "anthropic/claude-3-opus-latest",
        ],
    },
    "gemini": {
        "display_name": "Google AI Studio",
        "env_var": "GEMINI_API_KEY",
        "models": [
            "gemini/gemini-3.1-pro-preview",
            "gemini/gemini-3-pro-preview",
            "gemini/gemini-3-flash-preview",
            "gemini/gemini-2.5-pro",
            "gemini/gemini-2.5-flash",
            "gemini/gemini-2.5-flash-lite",
            "gemini/gemini-2.0-flash",
            "gemini/gemini-2.0-flash-lite",
            "gemini/gemini-1.5-pro",
            "gemini/gemini-1.5-flash",
        ],
    },
    "groq": {
        "display_name": "Groq",
        "env_var": "GROQ_API_KEY",
        "models": [
            "groq/meta-llama/llama-4-maverick-17b-128e-instruct",
            "groq/meta-llama/llama-4-scout-17b-16e-instruct",
            "groq/llama-3.3-70b-versatile",
            "groq/qwen/qwen3-32b",
            "groq/llama-3.1-8b-instant",
        ],
    },
    "mistral": {
        "display_name": "Mistral AI",
        "env_var": "MISTRAL_API_KEY",
        "models": [
            "mistral/mistral-large-latest",
            "mistral/mistral-medium-latest",
            "mistral/mistral-small-latest",
            "mistral/magistral-medium-latest",
            "mistral/magistral-small-latest",
            "mistral/codestral-latest",
        ],
    },
}


def get_llm_provider_catalog() -> List[LLMProviderInfo]:
    """
    Returns supported LLM providers with credential requirements and available models.

    Provider metadata (env vars, models, display names) is defined in _SUPPORTED_PROVIDERS.
    The env_var on each LLMConfigField is the environment variable the platform must set
    on the evaluation job process — litellm reads these natively.

    Returns:
        List of LLMProviderInfo, one per supported provider.

    Example:
        catalog = get_llm_provider_catalog()
        for provider in catalog:
            for field in provider.config_fields:
                # platform injects: os.environ[field.env_var] = user_input
                print(f"{provider.display_name}: set {field.env_var}")
            print(f"  Models: {provider.models}")
    """
    result: List[LLMProviderInfo] = []
    for name, meta in _SUPPORTED_PROVIDERS.items():
        env_var = meta["env_var"]
        config_fields = [
            LLMConfigField(
                key="api_key",
                label="API Key",
                field_type="password",
                required=True,
                env_var=env_var,
            )
        ]
        result.append(
            LLMProviderInfo(
                name=name,
                display_name=meta["display_name"],
                config_fields=config_fields,
                models=meta["models"],
            )
        )

    return result


__all__ = ["builtin", "list_builtin_evaluators", "builtin_evaluator_catalog", "get_llm_provider_catalog"]
