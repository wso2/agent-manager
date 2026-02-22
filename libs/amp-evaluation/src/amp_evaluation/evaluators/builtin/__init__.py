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
"""

import importlib
import inspect
from pathlib import Path
from typing import Type, Optional, List

from amp_evaluation.evaluators.base import BaseEvaluator
from amp_evaluation.models import EvaluatorInfo


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
        answer_rel = builtin("answer_relevancy")
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

                    # Ensure module name is in tags
                    if module_name not in info.tags:
                        info.tags = [module_name] + info.tags

                    info.class_name = class_name
                    info.module = module_name

                    evaluators.append(info)
                except Exception:
                    continue

        except ImportError:
            continue

    if mode:
        evaluators = [ev for ev in evaluators if mode in ev.modes]

    return evaluators


__all__ = ["builtin", "list_builtin_evaluators", "builtin_evaluator_catalog"]
