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
Dataset data models.

This module contains the core Pydantic model definitions for datasets:
- Task: A single test case with inputs and expected outputs
- Dataset: A collection of tasks
- Constraints: Performance/resource constraints
- TrajectoryStep: A step in the expected execution trajectory
"""

from datetime import datetime
from typing import List, Dict, Any, Optional, Literal, Union
from uuid import uuid4

from pydantic import BaseModel, ConfigDict, Field, model_validator


def generate_id(prefix: str = "") -> str:
    """Generate a unique ID."""
    return f"{prefix}{uuid4().hex[:12]}" if prefix else uuid4().hex[:12]


# ============================================================================
# COMPONENT MODELS
# ============================================================================


class Constraints(BaseModel):
    """Performance and resource constraints for a task."""

    model_config = ConfigDict(extra="forbid")

    max_latency_ms: Optional[float] = Field(default=None, ge=0.0)
    max_tokens: Optional[int] = Field(default=None, ge=1)
    max_iterations: Optional[int] = Field(default=None, ge=1)
    max_cost: Optional[float] = Field(default=None, ge=0.0)


class TrajectoryStep(BaseModel):
    """A single step in the expected trajectory (ground truth)."""

    model_config = ConfigDict(extra="forbid")

    tool: str
    args: Dict[str, Any] = Field(default_factory=dict)
    expected_output: Optional[str] = None


# ============================================================================
# TASK MODEL
# ============================================================================


class Task(BaseModel):
    """
    A single test case - the fundamental building block of evaluation.

    Contains the input, expected outputs, success criteria, and metadata
    needed to evaluate an agent's performance on a specific task.
    """

    model_config = ConfigDict(extra="forbid")

    # === REQUIRED ===
    task_id: str  # Unique identifier
    input: Union[str, Dict[str, Any]]  # Query/prompt (string or JSON)

    # === IDENTIFICATION ===
    name: str = ""  # Human-readable name
    description: str = ""  # What this task tests

    # === GROUND TRUTH (for experiments) ===
    expected_output: Optional[str] = None  # Expected final output
    expected_trajectory: Optional[List[TrajectoryStep]] = None  # Expected tool sequence
    expected_outcome: Optional[Dict[str, Any]] = None  # Expected side effects/state
    success_criteria: Optional[Union[str, List[str]]] = None  # For LLM judges (string or list)

    # === CONSTRAINTS ===
    prohibited_content: Optional[List[str]] = None  # Content that MUST NOT appear
    constraints: Optional[Constraints] = None  # Performance limits (latency, tokens, etc.)

    # === CLASSIFICATION ===
    task_type: str = "general"  # "qa", "rag", "tool_use", "code_gen"
    difficulty: Literal["easy", "medium", "hard", "expert"] = "medium"
    domain: Optional[str] = None  # "medical", "legal", "finance"
    tags: List[str] = Field(default_factory=list)  # ["hallucination", "math"]

    # === EXTENSIBILITY ===
    custom: Dict[str, Any] = Field(default_factory=dict)  # Passed to evaluators
    metadata: Dict[str, Any] = Field(default_factory=dict)  # NOT passed to evaluators

    # === INTERNAL ===
    dataset_id: Optional[str] = None  # Parent dataset
    created_at: datetime = Field(default_factory=datetime.now)
    created_by: Optional[str] = None


# ============================================================================
# DATASET MODEL
# ============================================================================


class Dataset(BaseModel):
    """
    A collection of tasks designed to measure specific capabilities or behaviors.

    Can represent:
    - Golden set: Curated test cases with ground truth
    - Production traces: Real usage data
    - Synthetic data: Generated test cases
    - Human annotated: Manually created and verified
    """

    model_config = ConfigDict(extra="forbid")

    # === REQUIRED ===
    dataset_id: str  # Unique identifier
    name: str  # Human-readable name
    description: str = ""  # What this dataset measures

    # === TASKS ===
    tasks: List[Task] = Field(default_factory=list)

    # === CLASSIFICATION ===
    dataset_type: Literal["golden_set", "production_traces", "synthetic", "human_annotated"] = "golden_set"
    domain: Optional[str] = None  # "customer_support", "medical"
    version: str = "1.0"
    tags: List[str] = Field(default_factory=list)

    # === SOURCE INFO (for production traces) ===
    source: Optional[str] = None  # File path, API endpoint, DB query
    source_filters: Dict[str, Any] = Field(default_factory=dict)  # Query filters used

    # === STATISTICS (auto-calculated) ===
    task_count: int = 0
    difficulty_distribution: Dict[str, int] = Field(default_factory=dict)

    # === METADATA ===
    created_at: datetime = Field(default_factory=datetime.now)
    updated_at: datetime = Field(default_factory=datetime.now)
    created_by: Optional[str] = None

    @model_validator(mode="after")
    def _sync_task_count(self) -> "Dataset":
        """Keep task_count in sync with the actual number of tasks."""
        if self.tasks and self.task_count == 0:
            self.task_count = len(self.tasks)
        return self

    def add_task(self, task: Task):
        """Add a task to the dataset."""
        self.tasks.append(task)
        self.task_count = len(self.tasks)

        # Update difficulty distribution
        difficulty = task.difficulty
        self.difficulty_distribution[difficulty] = self.difficulty_distribution.get(difficulty, 0) + 1
