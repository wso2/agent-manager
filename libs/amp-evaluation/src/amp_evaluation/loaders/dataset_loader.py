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
Dataset loaders for flexible CSV and JSON dataset loading.
"""

from typing import List, Dict, Optional, Callable
import csv
from pathlib import Path

from ..models import Dataset, Task, generate_id
from ..dataset_schema import DatasetSchema, Constraints


class DatasetLoader:
    """
    Flexible dataset loader supporting CSV, JSON, and other formats.
    Primary method: from_json() loads JSON datasets via DatasetSchema.
    """

    @staticmethod
    def from_json(file_path: str) -> Dataset:
        """
        Load dataset from JSON file (recommended method).

        Uses DatasetSchema to parse JSON and converts to Dataset.

        Args:
            file_path: Path to JSON file

        Returns:
            Dataset loaded from JSON

        Example:
            >>> dataset = DatasetLoader.from_json("benchmarks/my_dataset.json")
        """
        # Load via DatasetSchema
        schema = DatasetSchema.from_json(file_path)
        return DatasetLoader.from_schema(schema, source=file_path)

    @staticmethod
    def from_schema(schema: DatasetSchema, source: Optional[str] = None) -> Dataset:
        """
        Convert DatasetSchema to Dataset.

        Applies dataset defaults to tasks and creates proper Task objects.

        Args:
            schema: DatasetSchema to convert
            source: Optional source path for metadata

        Returns:
            Dataset with populated tasks
        """
        tasks = []
        for dataset_task in schema.tasks:
            # Build constraints (merge with defaults)
            constraints = None
            if dataset_task.constraints or schema.defaults:
                constraints = Constraints(
                    max_latency_ms=dataset_task.constraints.max_latency_ms
                    if dataset_task.constraints
                    else schema.defaults.max_latency_ms
                    if schema.defaults
                    else None,
                    max_tokens=dataset_task.constraints.max_tokens
                    if dataset_task.constraints
                    else schema.defaults.max_tokens
                    if schema.defaults
                    else None,
                    max_iterations=dataset_task.constraints.max_iterations
                    if dataset_task.constraints
                    else schema.defaults.max_iterations
                    if schema.defaults
                    else None,
                )

            # Merge prohibited_content with defaults
            prohibited_content = dataset_task.prohibited_content
            if not prohibited_content and schema.defaults and schema.defaults.prohibited_content:
                prohibited_content = schema.defaults.prohibited_content

            # Convert expected trajectory from schema format to dict list
            expected_trajectory = None
            if dataset_task.reference_trajectory:
                expected_trajectory = [
                    {"tool": step.tool, "args": step.args, "expected_output": step.expected_output}
                    for step in dataset_task.reference_trajectory
                ]

            # Create Task
            task = Task(
                task_id=dataset_task.id,
                name=dataset_task.name or dataset_task.id,
                description=dataset_task.description or "",
                input=dataset_task.input,
                expected_output=dataset_task.reference_output,
                expected_trajectory=expected_trajectory,
                expected_outcome=dataset_task.expected_outcome,
                success_criteria_text=dataset_task.success_criteria,
                prohibited_content=prohibited_content,
                constraints=constraints,
                custom=dataset_task.custom,
                metadata=dataset_task.metadata,
            )
            tasks.append(task)

        # Create Dataset
        dataset = Dataset(
            dataset_id=generate_id("dataset_"),
            name=schema.name,
            description=schema.description or "",
            tasks=tasks,
            version=schema.version or "1.0",
            source=source,
            task_count=len(tasks),
        )

        # Add metadata if available
        if schema.metadata:
            dataset.metadata = {
                "created_by": schema.metadata.created_by,
                "created_at": schema.metadata.created_at,
                "domain": schema.metadata.domain,
                "tags": schema.metadata.tags,
                **schema.metadata.extra,
            }
            if schema.metadata.domain:
                dataset.domain = schema.metadata.domain

        return dataset

    @staticmethod
    def from_csv(
        file_path: str,
        schema: Optional[DatasetSchema] = None,
        parsers: Optional[Dict[str, Callable]] = None,
        task_id_column: str = "task_id",
        input_column: str = "input",
        name_column: Optional[str] = "name",
        description_column: Optional[str] = "description",
    ) -> Dataset:
        """
        Load dataset from CSV with flexible schema.

        Args:
            file_path: Path to CSV file
            schema: Define what fields are present (auto-detected if None)
            parsers: Custom parsers for complex fields (e.g., {"tools": lambda x: x.split(",")})
            task_id_column: Which column has task IDs (default: "task_id")
            input_column: Which column has inputs (default: "input")
            name_column: Which column has task names (optional)
            description_column: Which column has descriptions (optional)

        Returns:
            Dataset with tasks loaded from CSV

        Example:
            >>> dataset = DatasetLoader.from_csv(
            ...     "benchmarks/qa.csv",
            ...     parsers={"prohibited_content": lambda x: x.split(",")}
            ... )
        """
        parsers = parsers or {}

        with open(file_path, "r", encoding="utf-8") as f:
            reader = csv.DictReader(f)
            headers = reader.fieldnames or []

            # Auto-detect schema if not provided
            if schema is None:
                schema = DatasetLoader._auto_detect_schema(list(headers))

            tasks = []
            for row_num, row in enumerate(reader, start=1):
                # Extract task_id
                task_id = row.get(task_id_column, f"task_{row_num:04d}")

                # Extract input (simple string)
                input_text = row[input_column]

                # Build Constraints
                constraints = None
                max_latency_ms = None
                max_tokens = None
                max_iterations = None

                if schema.has_max_latency and "max_latency_ms" in row and row["max_latency_ms"]:
                    max_latency_ms = float(row["max_latency_ms"])

                if schema.has_max_tokens and "max_tokens" in row and row["max_tokens"]:
                    max_tokens = int(row["max_tokens"])

                if schema.has_max_iterations and "max_iterations" in row and row["max_iterations"]:
                    max_iterations = int(row["max_iterations"])

                if max_latency_ms or max_tokens or max_iterations:
                    constraints = Constraints(
                        max_latency_ms=max_latency_ms, max_tokens=max_tokens, max_iterations=max_iterations
                    )

                # Extract prohibited_content
                prohibited_content = None
                if schema.has_prohibited_content and "prohibited_content" in row and row["prohibited_content"]:
                    parser = parsers.get("prohibited_content", lambda x: [s.strip() for s in x.split(",")])
                    prohibited_content = parser(row["prohibited_content"])

                # Extract expected_output
                expected_output = None
                if schema.has_expected_output and "expected_output" in row and row["expected_output"]:
                    expected_output = row["expected_output"]
                elif schema.has_expected_output and "reference_output" in row and row["reference_output"]:
                    expected_output = row["reference_output"]

                # Build Task
                task = Task(
                    task_id=task_id,
                    name=row.get(name_column, f"Task {task_id}") if name_column else f"Task {task_id}",
                    description=row.get(description_column, "") if description_column else "",
                    input=input_text,
                    expected_output=expected_output,
                    prohibited_content=prohibited_content,
                    constraints=constraints,
                    task_type=row.get("task_type", "general"),
                    difficulty=row.get("difficulty", "medium"),
                    tags=[s.strip() for s in row.get("tags", "").split(",")] if row.get("tags") else [],
                    domain=row.get("domain"),
                )

                # Add custom fields to metadata
                for custom_field in schema.custom_fields:
                    if custom_field in row and row[custom_field]:
                        task.custom[custom_field] = row[custom_field]

                tasks.append(task)

        # Create dataset
        dataset = Dataset(
            dataset_id=generate_id("dataset_"),
            name=Path(file_path).stem,
            description=f"Loaded from {file_path}",
            tasks=tasks,
            dataset_type="golden_set",
            source=file_path,
            task_count=len(tasks),
        )

        # Update difficulty distribution
        for task in tasks:
            diff = task.difficulty
            dataset.difficulty_distribution[diff] = dataset.difficulty_distribution.get(diff, 0) + 1

        return dataset

    @staticmethod
    def _auto_detect_schema(headers: List[str]) -> DatasetSchema:
        """Auto-detect schema from CSV headers."""
        # Common header variations
        reference_output_headers = ["reference_output", "expected_output", "reference", "answer", "ground_truth"]
        reference_tools_headers = ["reference_tools", "expected_tools", "required_tools", "tools"]

        return DatasetSchema(
            has_input=True,
            has_reference_output=any(h in headers for h in reference_output_headers),
            has_reference_trajectory="reference_trajectory" in headers,
            has_reference_tools=any(h in headers for h in reference_tools_headers)
            or "expected_tool_sequence" in headers,
            has_required_content="required_content" in headers,
            has_prohibited_content="prohibited_content" in headers,
            has_max_latency="max_latency_ms" in headers,
            has_max_tokens="max_tokens" in headers,
            has_max_cost="max_cost_usd" in headers,
            has_max_iterations="max_iterations" in headers,
            custom_fields=[
                h
                for h in headers
                if h
                not in {
                    "task_id",
                    "input",
                    "name",
                    "description",
                    "context",
                    "files",
                    "reference_output",
                    "expected_output",
                    "reference",
                    "answer",
                    "ground_truth",
                    "reference_trajectory",
                    "reference_tools",
                    "expected_tools",
                    "required_tools",
                    "tools",
                    "expected_tool_sequence",
                    "required_content",
                    "prohibited_content",
                    "max_latency_ms",
                    "max_tokens",
                    "max_cost_usd",
                    "max_iterations",
                    "task_type",
                    "difficulty",
                    "tags",
                    "domain",
                }
            ],
        )

    @staticmethod
    def to_csv(dataset: Dataset, file_path: str, include_fields: Optional[List[str]] = None):
        """
        Save dataset to CSV file.

        Args:
            dataset: Dataset to save
            file_path: Where to save the CSV
            include_fields: Which fields to include (None = all available)
        """
        if not dataset.tasks:
            raise ValueError("Dataset has no tasks to export")

        # Determine fields to include
        if include_fields is None:
            # Auto-detect from first task
            include_fields = ["task_id", "input", "name", "description"]

            first_task = dataset.tasks[0]
            if first_task.expected_output:
                include_fields.append("expected_output")
            if first_task.prohibited_content:
                include_fields.append("prohibited_content")
            if first_task.constraints:
                if first_task.constraints.max_latency_ms:
                    include_fields.append("max_latency_ms")
                if first_task.constraints.max_tokens:
                    include_fields.append("max_tokens")
                if first_task.constraints.max_iterations:
                    include_fields.append("max_iterations")

        with open(file_path, "w", newline="", encoding="utf-8") as f:
            writer = csv.DictWriter(f, fieldnames=include_fields)
            writer.writeheader()

            for task in dataset.tasks:
                row = {}

                if "task_id" in include_fields:
                    row["task_id"] = task.task_id
                if "input" in include_fields:
                    row["input"] = task.input
                if "name" in include_fields:
                    row["name"] = task.name
                if "description" in include_fields:
                    row["description"] = task.description
                if "expected_output" in include_fields:
                    row["expected_output"] = task.expected_output or ""
                if "prohibited_content" in include_fields:
                    row["prohibited_content"] = ",".join(task.prohibited_content) if task.prohibited_content else ""
                if "max_latency_ms" in include_fields:
                    row["max_latency_ms"] = task.constraints.max_latency_ms if task.constraints else ""
                if "max_tokens" in include_fields:
                    row["max_tokens"] = task.constraints.max_tokens if task.constraints else ""
                if "max_iterations" in include_fields:
                    row["max_iterations"] = task.constraints.max_iterations if task.constraints else ""

                writer.writerow(row)
