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
Built-in evaluators discovery and lazy loading.

Automatic discovery of all evaluator modules:
- Scans all .py files in amp_evaluation.evaluators package (not builtin/)
- Discovers evaluators based on metadata.name field
- No hardcoded module mappings needed

Example:
    discover_evaluator("latency")  # Auto-finds LatencyEvaluator in standard.py
    discover_evaluator("deepeval/plan-quality")  # Auto-finds in deepeval.py
"""

import importlib
import inspect
from pathlib import Path
from typing import Type, Optional, List, Dict, Any

from amp_evaluation.evaluators.base import BaseEvaluator


def _get_evaluator_modules() -> List[str]:
    """
    Automatically discover all evaluator modules in the builtin/ directory.

    Returns:
        List of module names (e.g., ["standard", "deepeval"])
    """
    # Get the builtin directory (current directory)
    builtin_dir = Path(__file__).parent

    modules = []
    for file in builtin_dir.glob("*.py"):
        # Skip __init__.py and private files
        if file.stem in ("__init__",) or file.stem.startswith("_"):
            continue
        modules.append(file.stem)

    return modules


def discover_evaluator(name: str) -> Optional[Type[BaseEvaluator]]:
    """
    Discover and return evaluator class by name through automatic module scanning.

    This function automatically discovers evaluators by:
    1. Scanning all Python modules in the evaluators/ directory
    2. For each module, finding classes that inherit from BaseEvaluator
    3. Matching the requested name against the evaluator's default instance name

    No hardcoded mappings needed - just add a new .py file and it's discovered!

    Args:
        name: Evaluator name from metadata (e.g., "latency", "deepeval/plan-quality")

    Returns:
        Evaluator class or None if not found

    Examples:
        >>> cls = discover_evaluator("latency")
        >>> cls.__name__
        'LatencyEvaluator'

        >>> cls = discover_evaluator("deepeval/plan-quality")
        >>> cls.__name__
        'DeepEvalPlanQualityEvaluator'
    """
    # Get all evaluator modules
    modules = _get_evaluator_modules()

    # Search through each module
    for module_name in modules:
        try:
            module = importlib.import_module(f"amp_evaluation.evaluators.builtin.{module_name}")

            # Look for evaluator classes in this module
            for class_name, obj in inspect.getmembers(module, inspect.isclass):
                # Skip non-evaluators and base classes
                if not issubclass(obj, BaseEvaluator) or obj is BaseEvaluator:
                    continue

                # Skip abstract base classes (those that end with "Base" or have abstract methods)
                if class_name.endswith("Base") or class_name.endswith("BaseEvaluator"):
                    continue

                # Skip classes with abstract methods
                abstract_methods: frozenset[str] = getattr(obj, "__abstractmethods__", frozenset())
                if abstract_methods:
                    continue

                # Skip classes imported from other modules
                if obj.__module__ != module.__name__:
                    continue

                # Instantiate to check the default name
                try:
                    instance = obj()
                    if instance.name == name:
                        return obj
                except Exception:
                    # Skip evaluators that can't be instantiated with defaults
                    continue

        except ImportError:
            # Module has missing dependencies, skip it
            continue

    return None


def list_builtin_evaluators() -> List[Dict[str, Any]]:
    """
    List all available built-in evaluators by scanning all modules.

    Automatically discovers evaluators from all .py files in the builtin/ directory.

    Returns:
        List of dicts with evaluator definition:
        - name, description, tags, version, config_schema (top level)
        - metadata: {class_name, module} (implementation details)
    """
    evaluators = []
    modules = _get_evaluator_modules()

    for module_name in modules:
        try:
            module = importlib.import_module(f"amp_evaluation.evaluators.builtin.{module_name}")

            for class_name, obj in inspect.getmembers(module, inspect.isclass):
                # Skip non-evaluators and base classes
                if not issubclass(obj, BaseEvaluator) or obj is BaseEvaluator:
                    continue

                # Skip abstract base classes (those that end with "Base" or have abstract methods)
                if class_name.endswith("Base") or class_name.endswith("BaseEvaluator"):
                    continue

                # Skip classes with abstract methods
                abstract_methods: frozenset[str] = getattr(obj, "__abstractmethods__", frozenset())
                if abstract_methods:
                    continue

                # Skip classes imported from other modules
                if obj.__module__ != module.__name__:
                    continue

                # Instantiate to get metadata
                try:
                    instance = obj()
                    metadata = instance.get_metadata()

                    # Ensure module name is in tags for filtering
                    tags = metadata.get("tags", [])
                    if module_name not in tags:
                        tags = [module_name] + tags

                    # Restructure: top-level functional details, metadata has implementation details
                    evaluators.append(
                        {
                            "name": metadata.get("name", instance.name),
                            "description": metadata.get("description", ""),
                            "tags": tags,
                            "version": metadata.get("version", "1.0"),
                            "config_schema": metadata.get("config_schema", []),
                            "metadata": {
                                "class_name": class_name,
                                "module": module_name,
                            },
                        }
                    )
                except Exception:
                    # Skip evaluators that can't be instantiated with defaults
                    continue

        except ImportError:
            # Module has missing dependencies, skip it
            continue

    return evaluators


def get_builtin_evaluator(name: str, **kwargs) -> BaseEvaluator:
    """
    Get a built-in evaluator instance by name with optional configuration.

    This is the recommended way to instantiate built-in evaluators by name
    instead of importing them directly.

    Args:
        name: Built-in evaluator name (e.g., "latency", "deepeval/plan-quality")
        **kwargs: Configuration parameters passed to evaluator constructor

    Returns:
        Configured evaluator instance

    Raises:
        ValueError: If the evaluator is not a known built-in
        ImportError: If the evaluator's dependencies are not installed
        TypeError: If invalid kwargs passed to constructor

    Examples:
        >>> # Default configuration
        >>> evaluator = get_builtin_evaluator("latency")

        >>> # Custom configuration
        >>> evaluator = get_builtin_evaluator("latency", max_latency_ms=500)
        >>> evaluator = get_builtin_evaluator("deepeval/tool-correctness",
        ...                                   threshold=0.8, evaluate_input=True)
    """
    evaluator_class = discover_evaluator(name)
    if evaluator_class is None:
        available = [ev["name"] for ev in list_builtin_evaluators()]
        raise ValueError(f"'{name}' is not a built-in evaluator.\nAvailable built-in evaluators: {available}")

    try:
        instance = evaluator_class(**kwargs)
        # Set name if not already set by the class
        if instance.name == instance.__class__.__name__:
            instance.name = name
        return instance
    except TypeError as e:
        raise TypeError(f"Invalid configuration for evaluator '{name}': {e}") from e


__all__ = ["discover_evaluator", "list_builtin_evaluators", "get_builtin_evaluator"]
