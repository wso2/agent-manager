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
import json
from pathlib import Path

from ..models import (
    Dataset, DatasetSchema, Task, TaskInput, TaskSuccessCriteria,
    generate_id
)


class DatasetLoader:
    """
    Flexible dataset loader supporting CSV, JSON, and other formats.
    """

    @staticmethod
    def from_csv(
        file_path: str,
        schema: Optional[DatasetSchema] = None,
        parsers: Optional[Dict[str, Callable]] = None,
        task_id_column: str = "task_id",
        input_column: str = "input",
        name_column: Optional[str] = "name",
        description_column: Optional[str] = "description"
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
            ...     parsers={"reference_tools": lambda x: x.split(",")}
            ... )
        """
        parsers = parsers or {}

        with open(file_path, 'r', encoding='utf-8') as f:
            reader = csv.DictReader(f)
            headers = reader.fieldnames or []

            # Auto-detect schema if not provided
            if schema is None:
                schema = DatasetLoader._auto_detect_schema(list(headers))

            tasks = []
            for row_num, row in enumerate(reader, start=1):
                # Extract task_id
                task_id = row.get(task_id_column, f"task_{row_num:04d}")

                # Build TaskInput
                task_input = TaskInput(
                    prompt=row[input_column],
                    context=json.loads(row["context"]) if "context" in row and row["context"] else {},
                    files=parsers.get("files", lambda x: x.split(",") if x else [])(row.get("files", ""))
                )

                # Build TaskSuccessCriteria
                success_criteria = TaskSuccessCriteria()

                if schema.has_required_content and "required_content" in row and row["required_content"]:
                    parser = parsers.get("required_content", lambda x: [s.strip() for s in x.split(",")])
                    success_criteria.required_content = parser(row["required_content"])

                if schema.has_prohibited_content and "prohibited_content" in row and row["prohibited_content"]:
                    parser = parsers.get("prohibited_content", lambda x: [s.strip() for s in x.split(",")])
                    success_criteria.prohibited_content = parser(row["prohibited_content"])

                if schema.has_max_latency and "max_latency_ms" in row and row["max_latency_ms"]:
                    success_criteria.max_latency_ms = float(row["max_latency_ms"])

                if schema.has_max_tokens and "max_tokens" in row and row["max_tokens"]:
                    success_criteria.max_tokens = int(row["max_tokens"])

                if schema.has_max_cost and "max_cost_usd" in row and row["max_cost_usd"]:
                    success_criteria.max_cost_usd = float(row["max_cost_usd"])

                if schema.has_max_iterations and "max_iterations" in row and row["max_iterations"]:
                    success_criteria.max_iterations = int(row["max_iterations"])

                if schema.has_reference_tools:
                    if "reference_tools" in row and row["reference_tools"]:
                        parser = parsers.get("reference_tools", lambda x: [s.strip() for s in x.split(",")])
                        success_criteria.required_tools = parser(row["reference_tools"])

                    if "expected_tool_sequence" in row and row["expected_tool_sequence"]:
                        parser = parsers.get("expected_tool_sequence", lambda x: [s.strip() for s in x.split("->")])
                        success_criteria.expected_tool_sequence = parser(row["expected_tool_sequence"])

                # Build Task
                task = Task(
                    task_id=task_id,
                    name=row.get(name_column, f"Task {task_id}") if name_column else f"Task {task_id}",
                    description=row.get(description_column, "") if description_column else "",
                    input=task_input,
                    success_criteria=success_criteria,
                    task_type=row.get("task_type", "general"),
                    difficulty=row.get("difficulty", "medium"),
                    tags=[s.strip() for s in row.get("tags", "").split(",")] if row.get("tags") else [],
                    domain=row.get("domain")
                )

                # Add expected output (support both 'expected_output' and 'reference_output' column names)
                if schema.has_expected_output and "expected_output" in row and row["expected_output"]:
                    task.expected_output = row["expected_output"]
                elif schema.has_expected_output and "reference_output" in row and row["reference_output"]:
                    # Backward compatibility: map reference_output to expected_output
                    task.expected_output = row["reference_output"]

                # Also add to success_criteria for backward compatibility
                if task.expected_output:
                    task.success_criteria.expected_output = task.expected_output

                # Add custom fields to metadata
                for custom_field in schema.custom_fields:
                    if custom_field in row and row[custom_field]:
                        task.metadata[custom_field] = row[custom_field]

                tasks.append(task)

        # Create dataset
        dataset = Dataset(
            dataset_id=generate_id("dataset_"),
            name=Path(file_path).stem,
            description=f"Loaded from {file_path}",
            tasks=tasks,
            dataset_type="golden_set",
            source=file_path,
            task_count=len(tasks)
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
            has_reference_tools=any(h in headers for h in reference_tools_headers) or "expected_tool_sequence" in headers,
            has_required_content="required_content" in headers,
            has_prohibited_content="prohibited_content" in headers,
            has_max_latency="max_latency_ms" in headers,
            has_max_tokens="max_tokens" in headers,
            has_max_cost="max_cost_usd" in headers,
            has_max_iterations="max_iterations" in headers,
            custom_fields=[h for h in headers if h not in {
                "task_id", "input", "name", "description", "context", "files",
                "reference_output", "expected_output", "reference", "answer", "ground_truth",
                "reference_trajectory", "reference_tools", "expected_tools", "required_tools", "tools",
                "expected_tool_sequence", "required_content", "prohibited_content",
                "max_latency_ms", "max_tokens", "max_cost_usd", "max_iterations",
                "task_type", "difficulty", "tags", "domain"
            }]
        )

    @staticmethod
    def to_csv(
        dataset: Dataset,
        file_path: str,
        include_fields: Optional[List[str]] = None
    ):
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
            if first_task.success_criteria.required_content:
                include_fields.append("required_content")
            if first_task.success_criteria.prohibited_content:
                include_fields.append("prohibited_content")
            if first_task.success_criteria.max_latency_ms:
                include_fields.append("max_latency_ms")
            if first_task.success_criteria.max_tokens:
                include_fields.append("max_tokens")
            if first_task.success_criteria.required_tools:
                include_fields.append("reference_tools")

        with open(file_path, 'w', newline='', encoding='utf-8') as f:
            writer = csv.DictWriter(f, fieldnames=include_fields)
            writer.writeheader()

            for task in dataset.tasks:
                row = {}

                if "task_id" in include_fields:
                    row["task_id"] = task.task_id
                if "input" in include_fields:
                    row["input"] = task.input.prompt
                if "name" in include_fields:
                    row["name"] = task.name
                if "description" in include_fields:
                    row["description"] = task.description
                if "expected_output" in include_fields:
                    row["expected_output"] = task.expected_output or ""
                # Backward compatibility: also support reference_output field name
                if "reference_output" in include_fields:
                    row["reference_output"] = task.expected_output or ""
                if "required_content" in include_fields:
                    row["required_content"] = ",".join(task.success_criteria.required_content)
                if "prohibited_content" in include_fields:
                    row["prohibited_content"] = ",".join(task.success_criteria.prohibited_content)
                if "max_latency_ms" in include_fields:
                    row["max_latency_ms"] = task.success_criteria.max_latency_ms or ""
                if "max_tokens" in include_fields:
                    row["max_tokens"] = task.success_criteria.max_tokens or ""
                if "reference_tools" in include_fields:
                    row["reference_tools"] = ",".join(task.success_criteria.required_tools)

                writer.writerow(row)

    @staticmethod
    def from_json(file_path: str) -> Dataset:
        """
        Load dataset from JSON file.

        Args:
            file_path: Path to JSON file

        Returns:
            Dataset loaded from JSON
        """
        with open(file_path, 'r', encoding='utf-8') as f:
            data = json.load(f)

        # Reconstruct tasks
        tasks = []
        for task_data in data.get("tasks", []):
            # Build TaskInput
            input_data = task_data["input"]
            task_input = TaskInput(
                prompt=input_data["prompt"],
                context=input_data.get("context", {}),
                files=input_data.get("files", [])
            )

            # Build TaskSuccessCriteria
            criteria_data = task_data.get("success_criteria", {})
            success_criteria = TaskSuccessCriteria(
                expected_output=criteria_data.get("expected_output"),
                required_content=criteria_data.get("required_content", []),
                prohibited_content=criteria_data.get("prohibited_content", []),
                max_latency_ms=criteria_data.get("max_latency_ms"),
                max_tokens=criteria_data.get("max_tokens"),
                max_cost_usd=criteria_data.get("max_cost_usd"),
                max_iterations=criteria_data.get("max_iterations"),
                required_tools=criteria_data.get("required_tools", []),
                expected_tool_sequence=criteria_data.get("expected_tool_sequence", [])
            )

            # Build Task
            task = Task(
                task_id=task_data["task_id"],
                name=task_data["name"],
                description=task_data.get("description", ""),
                input=task_input,
                success_criteria=success_criteria,
                reference_output=task_data.get("reference_output"),
                task_type=task_data.get("task_type", "general"),
                difficulty=task_data.get("difficulty", "medium"),
                tags=task_data.get("tags", []),
                domain=task_data.get("domain"),
                metadata=task_data.get("metadata", {})
            )

            tasks.append(task)

        # Create dataset
        dataset = Dataset(
            dataset_id=data.get("dataset_id", generate_id("dataset_")),
            name=data["name"],
            description=data.get("description", ""),
            tasks=tasks,
            dataset_type=data.get("dataset_type", "golden_set"),
            domain=data.get("domain"),
            version=data.get("version", "1.0"),
            source=file_path,
            task_count=len(tasks)
        )

        return dataset

    @staticmethod
    def to_json(dataset: Dataset, file_path: str):
        """
        Save dataset to JSON file.

        Args:
            dataset: Dataset to save
            file_path: Where to save JSON
        """
        data = {
            "dataset_id": dataset.dataset_id,
            "name": dataset.name,
            "description": dataset.description,
            "dataset_type": dataset.dataset_type,
            "domain": dataset.domain,
            "version": dataset.version,
            "task_count": dataset.task_count,
            "difficulty_distribution": dataset.difficulty_distribution,
            "tasks": []
        }

        for task in dataset.tasks:
            task_data = {
                "task_id": task.task_id,
                "name": task.name,
                "description": task.description,
                "input": {
                    "prompt": task.input.prompt,
                    "context": task.input.context,
                    "files": task.input.files
                },
                "success_criteria": {
                    "expected_output": task.success_criteria.expected_output,
                    "required_content": task.success_criteria.required_content,
                    "prohibited_content": task.success_criteria.prohibited_content,
                    "max_latency_ms": task.success_criteria.max_latency_ms,
                    "max_tokens": task.success_criteria.max_tokens,
                    "max_cost_usd": task.success_criteria.max_cost_usd,
                    "max_iterations": task.success_criteria.max_iterations,
                    "required_tools": task.success_criteria.required_tools,
                    "expected_tool_sequence": task.success_criteria.expected_tool_sequence
                },
                "expected_output": task.expected_output,
                "task_type": task.task_type,
                "difficulty": task.difficulty,
                "tags": task.tags,
                "domain": task.domain,
                "metadata": task.metadata
            }
            data["tasks"].append(task_data)

        with open(file_path, 'w', encoding='utf-8') as f:
            json.dump(data, f, indent=2, default=str)
