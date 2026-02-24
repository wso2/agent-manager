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
import json
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

# Providers we support for LLM-as-judge evaluation.
# Models are curated text models only (no audio/realtime/vision-specific).
# Ordered powerful → lightweight. All strings are litellm-compatible model identifiers.
_PROVIDER_MODELS: Dict[str, List[str]] = {
    "openai": [
        "gpt-5",
        "o3",
        "gpt-4.1",
        "gpt-4o",
        "gpt-4.1-mini",
        "o3-mini",
        "gpt-4.1-nano",
        "gpt-4o-mini",
    ],
    "anthropic": [
        "claude-opus-4-6",
        "claude-sonnet-4-6",
        "claude-opus-4-5",
        "claude-3-5-sonnet-20241022",
        "claude-haiku-4-5-20251001",
        "claude-3-5-haiku-20241022",
    ],
    "gemini": [
        "gemini/gemini-3-pro-preview",
        "gemini/gemini-2.5-pro",
        "gemini/gemini-3-flash-preview",
        "gemini/gemini-2.5-flash",
        "gemini/gemini-2.5-flash-lite",
        "gemini/gemini-2.0-flash",
    ],
    "groq": [
        "groq/meta-llama/llama-4-maverick-17b-128e-instruct",
        "groq/llama-3.3-70b-versatile",
        "groq/qwen/qwen3-32b",
        "groq/meta-llama/llama-4-scout-17b-16e-instruct",
        "groq/llama-3.1-8b-instant",
    ],
    "mistral": [
        "mistral/mistral-large-3",
        "mistral/mistral-large-latest",
        "mistral/mistral-medium-latest",
        "mistral/mistral-small-latest",
        "mistral/open-mistral-nemo",
    ],
}


def get_llm_provider_catalog() -> List[LLMProviderInfo]:
    """
    Returns supported LLM providers with credential requirements and available models.

    Credential field definitions are read from litellm's provider_create_fields.json
    (shipped with the litellm package). The env_var on each LLMConfigField is the
    environment variable the platform must set on the evaluation job process — litellm
    reads these natively, so no changes to evaluator code are needed.

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
    import litellm

    fields_path = Path(litellm.__file__).parent / "proxy" / "public_endpoints" / "provider_create_fields.json"
    try:
        raw: list = json.loads(fields_path.read_text())
    except (FileNotFoundError, json.JSONDecodeError) as e:
        logger.warning("Could not load litellm provider_create_fields.json: %s", e)
        return []

    seen: set = set()
    result: List[LLMProviderInfo] = []
    for entry in raw:
        provider = entry.get("litellm_provider", "")
        if provider not in _PROVIDER_MODELS:
            continue
        if provider in seen:
            continue  # deduplicate — keep first (canonical) entry per provider
        seen.add(provider)

        # Derive env var using litellm's own convention: {PROVIDER}_API_KEY
        env_var = f"{provider.upper()}_API_KEY"
        config_fields = [
            LLMConfigField(
                key=f["key"],
                label=f.get("label", f["key"]),
                field_type=f.get("field_type", "text"),
                required=f.get("required", False),
                env_var=env_var,
            )
            for f in entry.get("credential_fields", [])
            if f.get("key") == "api_key"
        ]
        if config_fields:
            result.append(
                LLMProviderInfo(
                    name=provider,
                    display_name=entry.get("provider_display_name", provider),
                    config_fields=config_fields,
                    models=_PROVIDER_MODELS.get(provider, []),
                )
            )

    return result


__all__ = ["builtin", "list_builtin_evaluators", "builtin_evaluator_catalog", "get_llm_provider_catalog"]
