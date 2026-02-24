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
Dataset loaders for JSON and CSV formats.

This module provides functions to load datasets from files and
serialize them back. Supports both JSON and CSV formats.
"""

import csv
import json
import logging
from dataclasses import dataclass, field
from pathlib import Path
from typing import Dict, Any, Optional, List

from .models import Dataset, Task, generate_id, Constraints, TrajectoryStep

logger = logging.getLogger(__name__)

# Constants
SCHEMA_VERSION = "1.0"


# Helper classes for parsing (internal to loader, not user-facing)
@dataclass
class DatasetDefaults:
    """Default constraint values applied to all tasks unless overridden at the task level."""

    max_latency_ms: Optional[float] = None
    max_tokens: Optional[int] = None
    max_iterations: Optional[int] = None
    max_cost: Optional[float] = None
    prohibited_content: Optional[List[str]] = None


@dataclass
class DatasetMetadata:
    """Dataset-level metadata parsed from JSON (not passed to evaluators)."""

    created_by: Optional[str] = None
    created_at: Optional[str] = None
    domain: Optional[str] = None
    tags: List[str] = field(default_factory=list)
    description: Optional[str] = None
    extra: Dict[str, Any] = field(default_factory=dict)


# ============================================================================
# JSON LOADING
# ============================================================================


def load_dataset_from_json(json_path: str) -> Dataset:
    """
    Load dataset from JSON file.

    Args:
        json_path: Path to JSON file

    Returns:
        Dataset object

    Raises:
        FileNotFoundError: If the JSON file doesn't exist
        ValueError: If the JSON data is invalid

    Example:
        >>> dataset = load_dataset_from_json("benchmarks/my_dataset.json")
    """
    path = Path(json_path)
    with open(path, "r") as f:
        data = json.load(f)

    return parse_dataset_dict(data)


def _parse_defaults(defaults_data: Dict[str, Any]) -> DatasetDefaults:
    """Parse dataset defaults, supporting both flat and nested constraint formats.

    Flat format (fields directly on defaults):
        {"max_latency_ms": 5000, "max_tokens": 1000}

    Nested format (mirroring task structure):
        {"constraints": {"max_latency_ms": 5000, "max_tokens": 1000},
         "prohibited_content": [...]}

    Note: success_criteria in defaults is intentionally not parsed — each task
    should define its own success criteria since they are task-specific.
    """
    constraints_data = defaults_data.get("constraints", {})

    return DatasetDefaults(
        max_latency_ms=constraints_data.get("max_latency_ms", defaults_data.get("max_latency_ms")),
        max_tokens=constraints_data.get("max_tokens", defaults_data.get("max_tokens")),
        max_iterations=constraints_data.get("max_iterations", defaults_data.get("max_iterations")),
        max_cost=constraints_data.get("max_cost", defaults_data.get("max_cost")),
        prohibited_content=defaults_data.get("prohibited_content"),
    )


def _resolve_task_field(task_data: Dict[str, Any], field_name: str, default: Any = None) -> Any:
    """Resolve a task field by checking top-level first, then metadata, then custom.

    Classification fields (difficulty, task_type, domain, tags) can appear in
    different places depending on the JSON format. This function checks in order:
    1. Top-level task fields (preferred location)
    2. metadata dict (common in existing datasets)
    3. custom dict (legacy/fallback)
    """
    if field_name in task_data:
        return task_data[field_name]
    if field_name in task_data.get("metadata", {}):
        return task_data["metadata"][field_name]
    if field_name in task_data.get("custom", {}):
        return task_data["custom"][field_name]
    return default


def parse_dataset_dict(data: Dict[str, Any]) -> Dataset:
    """Parse dataset from dictionary.

    Args:
        data: Dictionary containing dataset definition

    Returns:
        Parsed Dataset object

    Raises:
        ValueError: If task data is invalid (e.g., missing required 'input' field)
    """
    defaults = None
    if "defaults" in data:
        defaults = _parse_defaults(data["defaults"])

    meta = None
    if "metadata" in data:
        meta_data = data["metadata"]
        meta = DatasetMetadata(
            created_by=meta_data.get("created_by"),
            created_at=meta_data.get("created_at"),
            domain=meta_data.get("domain"),
            tags=meta_data.get("tags", []),
            description=meta_data.get("description"),
            extra={
                k: v
                for k, v in meta_data.items()
                if k not in ["created_by", "created_at", "domain", "tags", "description"]
            },
        )

    # Parse tasks and check for duplicates
    tasks = []
    seen_task_ids: set = set()
    for task_data in data.get("tasks", []):
        task = parse_task_dict(task_data, defaults)
        if task.task_id in seen_task_ids:
            logger.warning(f"Duplicate task_id '{task.task_id}' found in dataset")
        seen_task_ids.add(task.task_id)
        tasks.append(task)

    if not tasks:
        logger.warning("Dataset has no tasks")

    # Resolve name/description/version from root or metadata
    meta_data = data.get("metadata", {})
    name = data.get("name") or meta_data.get("name", "")
    description = data.get("description") or meta_data.get("description", "")
    version = data.get("version") or meta_data.get("version", "1.0")

    dataset_id = data.get("id", generate_id("dataset_"))

    dataset = Dataset(
        dataset_id=dataset_id,
        name=name,
        description=description,
        tasks=tasks,
        version=version,
        domain=meta.domain if meta else None,
        tags=meta.tags if meta else [],
        created_by=meta.created_by if meta else None,
        task_count=len(tasks),
    )

    # Propagate dataset_id to all tasks
    for task in dataset.tasks:
        task.dataset_id = dataset_id

    return dataset


def parse_task_dict(task_data: Dict[str, Any], defaults: Optional[DatasetDefaults] = None) -> Task:
    """Parse a single task from dictionary.

    Args:
        task_data: Dictionary containing task definition
        defaults: Optional default constraint values to apply

    Returns:
        Parsed Task object

    Raises:
        ValueError: If required 'input' field is missing
    """
    # Validate required field
    if "input" not in task_data:
        task_id = task_data.get("id") or task_data.get("task_id") or "unknown"
        raise ValueError(f"Task '{task_id}' is missing required 'input' field")

    # Merge task constraints with defaults
    constraints = None
    if "constraints" in task_data or defaults:
        task_constraints = task_data.get("constraints", {})
        constraints = Constraints(
            max_latency_ms=task_constraints.get("max_latency_ms", defaults.max_latency_ms if defaults else None),
            max_tokens=task_constraints.get("max_tokens", defaults.max_tokens if defaults else None),
            max_iterations=task_constraints.get("max_iterations", defaults.max_iterations if defaults else None),
            max_cost=task_constraints.get("max_cost", defaults.max_cost if defaults else None),
        )

    # Parse expected trajectory
    trajectory = None
    if "expected_trajectory" in task_data and task_data["expected_trajectory"]:
        trajectory = [
            TrajectoryStep(**step) if isinstance(step, dict) else step for step in task_data["expected_trajectory"]
        ]

    # Resolve prohibited content (task-level overrides defaults)
    prohibited = task_data.get("prohibited_content")
    if prohibited is None and defaults:
        prohibited = defaults.prohibited_content

    # Resolve classification fields from top-level, metadata, or custom (in priority order)
    task_type = _resolve_task_field(task_data, "task_type", "general")
    difficulty = _resolve_task_field(task_data, "difficulty", "medium")
    domain = _resolve_task_field(task_data, "domain")
    tags = _resolve_task_field(task_data, "tags", [])

    task = Task(
        task_id=(task_data.get("id") or "").strip() or (task_data.get("task_id") or "").strip() or generate_id("task_"),
        name=task_data.get("name", ""),
        description=task_data.get("description", ""),
        input=task_data["input"],
        expected_output=task_data.get("expected_output"),
        expected_trajectory=trajectory,
        expected_outcome=task_data.get("expected_outcome"),
        success_criteria=task_data.get("success_criteria"),
        prohibited_content=prohibited,
        constraints=constraints,
        task_type=task_type,
        difficulty=difficulty,
        domain=domain,
        tags=tags,
        custom=task_data.get("custom", {}),
        metadata=task_data.get("metadata", {}),
    )

    return task


# ============================================================================
# CSV LOADING
# ============================================================================


def load_dataset_from_csv(csv_path: str, name: Optional[str] = None) -> Dataset:
    """
    Load a simple dataset from CSV.

    Required columns: 'id' (or 'task_id') and 'input'.
    Optional columns: 'name', 'description', 'expected_output', 'success_criteria'.
    All other columns are stored in the task's 'custom' dict.

    Args:
        csv_path: Path to CSV file
        name: Optional dataset name (defaults to filename)

    Returns:
        Dataset object

    Raises:
        FileNotFoundError: If the CSV file doesn't exist
        ValueError: If required columns are missing

    Example:
        >>> dataset = load_dataset_from_csv("data.csv", name="My Dataset")
    """
    path = Path(csv_path)
    tasks = []

    with open(path, "r", newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            task_id = row.get("id") or row.get("task_id")
            if not task_id or "input" not in row or not row["input"]:
                raise ValueError("CSV must have non-empty 'id' (or 'task_id') and 'input' columns")

            id_key = "id" if "id" in row else "task_id"
            task = Task(
                task_id=task_id,
                name=row.get("name", ""),
                description=row.get("description", ""),
                input=row["input"],
                expected_output=row.get("expected_output"),
                success_criteria=row.get("success_criteria"),
                custom={
                    k: v
                    for k, v in row.items()
                    if k not in [id_key, "input", "name", "description", "expected_output", "success_criteria"]
                },
            )
            tasks.append(task)

    dataset_id = generate_id("dataset_")
    dataset = Dataset(
        dataset_id=dataset_id,
        name=name or path.stem,
        description=f"Loaded from {path.name}",
        tasks=tasks,
        task_count=len(tasks),
    )

    # Propagate dataset_id to all tasks
    for task in dataset.tasks:
        task.dataset_id = dataset_id

    return dataset


# ============================================================================
# JSON SAVING
# ============================================================================


def save_dataset_to_json(dataset: Dataset, path: str, indent: int = 2):
    """Save Dataset object to JSON file."""
    data = dataset_to_dict(dataset)
    with open(path, "w") as f:
        json.dump(data, f, indent=indent)


def dataset_to_dict(dataset: Dataset) -> Dict[str, Any]:
    """Convert Dataset object to dictionary for JSON serialization."""
    result: Dict[str, Any] = {
        "name": dataset.name,
        "description": dataset.description,
        "version": dataset.version,
        "schema_version": SCHEMA_VERSION,
        "tasks": [],
    }

    for task in dataset.tasks:
        task_dict = task_to_dict(task)
        result["tasks"].append(task_dict)

    if dataset.domain or dataset.tags or dataset.created_by:
        result["metadata"] = {}
        if dataset.created_by:
            result["metadata"]["created_by"] = dataset.created_by
        if dataset.domain:
            result["metadata"]["domain"] = dataset.domain
        if dataset.tags:
            result["metadata"]["tags"] = dataset.tags

    return result


def task_to_dict(task: Task) -> Dict[str, Any]:
    """Convert Task object to dictionary for JSON serialization."""
    result: Dict[str, Any] = {"id": task.task_id, "input": task.input}

    if task.name:
        result["name"] = task.name
    if task.description:
        result["description"] = task.description
    if task.expected_output:
        result["expected_output"] = task.expected_output
    if task.expected_trajectory:
        result["expected_trajectory"] = [
            {
                "tool": step.tool,
                "args": step.args,
                **({"expected_output": step.expected_output} if step.expected_output else {}),
            }
            for step in task.expected_trajectory
        ]
    if task.expected_outcome:
        result["expected_outcome"] = task.expected_outcome
    if task.success_criteria:
        result["success_criteria"] = task.success_criteria
    if task.prohibited_content:
        result["prohibited_content"] = task.prohibited_content
    if task.constraints:
        result["constraints"] = {
            k: v
            for k, v in {
                "max_latency_ms": task.constraints.max_latency_ms,
                "max_tokens": task.constraints.max_tokens,
                "max_iterations": task.constraints.max_iterations,
                "max_cost": task.constraints.max_cost,
            }.items()
            if v is not None
        }
    if task.custom:
        result["custom"] = task.custom
    if task.metadata:
        result["metadata"] = task.metadata

    return result
