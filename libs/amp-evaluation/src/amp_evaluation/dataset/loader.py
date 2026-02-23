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

This module provides functions to load datasets from files.
"""

import csv
import json
from dataclasses import dataclass, field
from pathlib import Path
from typing import Dict, Any, Optional, List

from .schema import Dataset, Task, generate_id, Constraints, TrajectoryStep

# Constants
SCHEMA_VERSION = "1.0"


# Helper classes for parsing
@dataclass
class DatasetDefaults:
    """Default values applied to all tasks unless overridden."""

    max_latency_ms: Optional[float] = None
    max_tokens: Optional[int] = None
    max_iterations: Optional[int] = None
    max_cost: Optional[float] = None
    prohibited_content: Optional[List[str]] = None


@dataclass
class DatasetMetadata:
    """Dataset-level metadata (not passed to evaluators)."""

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

    Example:
        >>> dataset = load_dataset_from_json("benchmarks/my_dataset.json")
    """
    path = Path(json_path)
    with open(path, "r") as f:
        data = json.load(f)

    return parse_dataset_dict(data)


def parse_dataset_dict(data: Dict[str, Any]) -> Dataset:
    """Parse dataset from dictionary."""
    defaults = None
    if "defaults" in data:
        defaults = DatasetDefaults(**data["defaults"])

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

    tasks = []
    for task_data in data.get("tasks", []):
        task = parse_task_dict(task_data, defaults)
        tasks.append(task)

    dataset = Dataset(
        dataset_id=data.get("id", generate_id("dataset_")),
        name=data["name"],
        description=data.get("description", ""),
        tasks=tasks,
        version=data.get("version", "1.0"),
        domain=meta.domain if meta else None,
        tags=meta.tags if meta else [],
        created_by=meta.created_by if meta else None,
        task_count=len(tasks),
    )

    return dataset


def parse_task_dict(task_data: Dict[str, Any], defaults: Optional[DatasetDefaults] = None) -> Task:
    """Parse a single task from dictionary."""
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

    trajectory = None
    if "expected_trajectory" in task_data and task_data["expected_trajectory"]:
        trajectory = [
            TrajectoryStep(**step) if isinstance(step, dict) else step for step in task_data["expected_trajectory"]
        ]

    prohibited = task_data.get("prohibited_content")
    if prohibited is None and defaults:
        prohibited = defaults.prohibited_content

    task = Task(
        task_id=task_data.get("id", generate_id("task_")),
        name=task_data.get("name", ""),
        description=task_data.get("description", ""),
        input=task_data["input"],
        expected_output=task_data.get("expected_output"),
        expected_trajectory=trajectory,
        expected_outcome=task_data.get("expected_outcome"),
        success_criteria=task_data.get("success_criteria"),
        prohibited_content=prohibited,
        constraints=constraints,
        task_type=task_data.get("custom", {}).get("task_type", "general"),
        difficulty=task_data.get("custom", {}).get("difficulty", "medium"),
        domain=task_data.get("custom", {}).get("domain"),
        tags=task_data.get("custom", {}).get("tags", []),
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

    Args:
        csv_path: Path to CSV file
        name: Optional dataset name (defaults to filename)

    Returns:
        Dataset object

    Example:
        >>> dataset = load_dataset_from_csv("data.csv", name="My Dataset")
    """
    path = Path(csv_path)
    tasks = []

    with open(path, "r", newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            if "id" not in row or "input" not in row:
                raise ValueError("CSV must have 'id' and 'input' columns")

            task = Task(
                task_id=row["id"],
                name=row.get("name", ""),
                description=row.get("description", ""),
                input=row["input"],
                expected_output=row.get("expected_output"),
                success_criteria=row.get("success_criteria"),
                custom={
                    k: v
                    for k, v in row.items()
                    if k not in ["id", "input", "name", "description", "expected_output", "success_criteria"]
                },
            )
            tasks.append(task)

    return Dataset(
        dataset_id=generate_id("dataset_"),
        name=name or path.stem,
        description=f"Loaded from {path.name}",
        tasks=tasks,
        task_count=len(tasks),
    )


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
