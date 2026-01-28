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
Dataset schema definitions for the evaluation framework.

Defines the structure of evaluation datasets in JSON format.
Supports both simple Q&A tasks and complex multi-step agent evaluations.

Example minimal dataset:
{
    "name": "Simple QA",
    "tasks": [
        {
            "id": "task-001",
            "input": "What is 2 + 2?",
            "reference_output": "4"
        }
    ]
}

Example full dataset:
{
    "name": "Agent Benchmark v1.0",
    "description": "Comprehensive agent evaluation dataset",
    "version": "1.0.0",
    "schema_version": "1.0",
    
    "metadata": {
        "created_by": "evaluation-team",
        "created_at": "2024-01-26",
        "domain": "customer-service",
        "tags": ["booking", "cancellation", "inquiry"]
    },
    
    "defaults": {
        "max_latency_ms": 5000,
        "max_tokens": 4096,
        "max_iterations": 10
    },
    
    "tasks": [
        {
            "id": "task-001",
            "name": "Flight Booking",
            "description": "User wants to book a flight",
            
            "input": "Book me a flight from NYC to LA on March 15",
            
            "reference_output": "I've booked your flight from NYC to LA on March 15. Confirmation #ABC123.",
            
            "reference_trajectory": [
                {"tool": "search_flights", "args": {"from": "NYC", "to": "LA", "date": "2024-03-15"}},
                {"tool": "book_flight", "args": {"flight_id": "FL123"}}
            ],
            
            "expected_outcome": {
                "booking_created": true,
                "confirmation_number": "ABC123"
            },
            
            "success_criteria": "Agent should search for available flights and complete the booking. User should receive a confirmation number.",
            
            "prohibited_content": ["error", "cannot", "unable", "sorry"],
            
            "constraints": {
                "max_latency_ms": 3000,
                "max_tokens": 2048,
                "max_iterations": 5
            },
            
            "custom": {
                "difficulty": "medium",
                "category": "booking",
                "priority": 1
            },
            
            "metadata": {
                "author": "john@example.com",
                "created_at": "2024-01-26"
            }
        }
    ]
}
"""
from dataclasses import dataclass, field
from typing import List, Dict, Any, Optional
import json
from pathlib import Path


# ============================================================================
# SCHEMA VERSION
# ============================================================================

SCHEMA_VERSION = "1.0"


# ============================================================================
# TASK SCHEMA
# ============================================================================

@dataclass
class TaskConstraints:
    """Performance and resource constraints for a task."""
    max_latency_ms: Optional[float] = None
    max_tokens: Optional[int] = None
    max_iterations: Optional[int] = None


@dataclass 
class TrajectoryStep:
    """A single step in the expected trajectory."""
    tool: str
    args: Dict[str, Any] = field(default_factory=dict)
    expected_output: Optional[str] = None


@dataclass
class DatasetTask:
    """
    A single task/test case in the dataset.
    
    Required fields:
        - id: Unique identifier
        - input: The user query/prompt
    
    Reference data (for evaluation):
        - reference_output: Expected final output
        - reference_trajectory: Expected sequence of tool calls
        - expected_outcome: Expected side effects/state
        - success_criteria: Human-readable success description (for LLM judges)
    
    Constraints:
        - prohibited_content: Content that should NOT appear in output
        - constraints: Performance limits (latency, tokens, iterations)
    
    Extensibility:
        - custom: User-defined attributes (passed to evaluators)
        - metadata: Task-level metadata (not passed to evaluators)
    """
    # Required
    id: str
    input: str
    
    # Optional identification
    name: Optional[str] = None
    description: Optional[str] = None
    
    # Reference data for evaluation
    reference_output: Optional[str] = None
    reference_trajectory: Optional[List[TrajectoryStep]] = None
    expected_outcome: Optional[Dict[str, Any]] = None
    success_criteria: Optional[str] = None
    
    # Constraints
    prohibited_content: Optional[List[str]] = None
    constraints: Optional[TaskConstraints] = None
    
    # Custom attributes (passed through to EvalContext)
    custom: Dict[str, Any] = field(default_factory=dict)
    
    # Metadata (not passed to evaluators)
    metadata: Dict[str, Any] = field(default_factory=dict)


@dataclass
class DatasetDefaults:
    """Default values applied to all tasks unless overridden."""
    max_latency_ms: Optional[float] = None
    max_tokens: Optional[int] = None
    max_iterations: Optional[int] = None
    prohibited_content: Optional[List[str]] = None


@dataclass
class DatasetMetadata:
    """Dataset-level metadata."""
    created_by: Optional[str] = None
    created_at: Optional[str] = None
    domain: Optional[str] = None
    tags: List[str] = field(default_factory=list)
    description: Optional[str] = None
    extra: Dict[str, Any] = field(default_factory=dict)


# ============================================================================
# DATASET SCHEMA
# ============================================================================

@dataclass
class DatasetSchema:
    """
    Complete dataset schema for evaluation.
    
    A dataset contains:
        - Metadata: Name, description, version, authorship
        - Defaults: Default constraints applied to all tasks
        - Tasks: List of test cases to evaluate
    
    Example JSON structure:
        {
            "name": "My Benchmark",
            "version": "1.0.0",
            "schema_version": "1.0",
            "defaults": {"max_latency_ms": 5000},
            "tasks": [
                {"id": "t1", "input": "Hello", "reference_output": "Hi there!"}
            ]
        }
    """
    # Required
    name: str
    tasks: List[DatasetTask]
    
    # Optional identification
    description: Optional[str] = None
    version: Optional[str] = None
    schema_version: str = SCHEMA_VERSION
    
    # Defaults and metadata
    defaults: Optional[DatasetDefaults] = None
    metadata: Optional[DatasetMetadata] = None
    
    @classmethod
    def from_json(cls, json_path: str) -> 'DatasetSchema':
        """Load dataset from JSON file."""
        path = Path(json_path)
        with open(path, 'r') as f:
            data = json.load(f)
        return cls.from_dict(data)
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> 'DatasetSchema':
        """Parse dataset from dictionary."""
        # Parse tasks
        tasks = []
        for task_data in data.get('tasks', []):
            # Parse constraints if present
            constraints = None
            if 'constraints' in task_data:
                constraints = TaskConstraints(**task_data['constraints'])
            
            # Parse trajectory if present
            trajectory = None
            if 'reference_trajectory' in task_data and task_data['reference_trajectory']:
                trajectory = [
                    TrajectoryStep(**step) if isinstance(step, dict) else step
                    for step in task_data['reference_trajectory']
                ]
            
            task = DatasetTask(
                id=task_data['id'],
                input=task_data['input'],
                name=task_data.get('name'),
                description=task_data.get('description'),
                reference_output=task_data.get('reference_output'),
                reference_trajectory=trajectory,
                expected_outcome=task_data.get('expected_outcome'),
                success_criteria=task_data.get('success_criteria'),
                prohibited_content=task_data.get('prohibited_content'),
                constraints=constraints,
                custom=task_data.get('custom', {}),
                metadata=task_data.get('metadata', {})
            )
            tasks.append(task)
        
        # Parse defaults
        defaults = None
        if 'defaults' in data:
            defaults = DatasetDefaults(**data['defaults'])
        
        # Parse metadata
        metadata = None
        if 'metadata' in data:
            meta_data = data['metadata']
            metadata = DatasetMetadata(
                created_by=meta_data.get('created_by'),
                created_at=meta_data.get('created_at'),
                domain=meta_data.get('domain'),
                tags=meta_data.get('tags', []),
                description=meta_data.get('description'),
                extra={k: v for k, v in meta_data.items() 
                       if k not in ['created_by', 'created_at', 'domain', 'tags', 'description']}
            )
        
        return cls(
            name=data['name'],
            tasks=tasks,
            description=data.get('description'),
            version=data.get('version'),
            schema_version=data.get('schema_version', SCHEMA_VERSION),
            defaults=defaults,
            metadata=metadata
        )
    
    def to_dict(self) -> Dict[str, Any]:
        """Convert dataset to dictionary."""
        result = {
            'name': self.name,
            'schema_version': self.schema_version,
            'tasks': []
        }
        
        if self.description:
            result['description'] = self.description
        if self.version:
            result['version'] = self.version
        
        # Serialize tasks
        for task in self.tasks:
            task_dict = {
                'id': task.id,
                'input': task.input
            }
            if task.name:
                task_dict['name'] = task.name
            if task.description:
                task_dict['description'] = task.description
            if task.expected_output:
                task_dict['expected_output'] = task.expected_output
            if task.expected_trajectory:
                task_dict['expected_trajectory'] = [
                    {'tool': step.tool, 'args': step.args, 'expected_output': step.expected_output}
                    for step in task.expected_trajectory
                ]
            if task.expected_outcome:
                task_dict['expected_outcome'] = task.expected_outcome
            if task.success_criteria:
                task_dict['success_criteria'] = task.success_criteria
            if task.prohibited_content:
                task_dict['prohibited_content'] = task.prohibited_content
            if task.constraints:
                task_dict['constraints'] = {
                    k: v for k, v in {
                        'max_latency_ms': task.constraints.max_latency_ms,
                        'max_tokens': task.constraints.max_tokens,
                        'max_iterations': task.constraints.max_iterations
                    }.items() if v is not None
                }
            if task.custom:
                task_dict['custom'] = task.custom
            if task.metadata:
                task_dict['metadata'] = task.metadata
            
            result['tasks'].append(task_dict)
        
        # Serialize defaults
        if self.defaults:
            result['defaults'] = {
                k: v for k, v in {
                    'max_latency_ms': self.defaults.max_latency_ms,
                    'max_tokens': self.defaults.max_tokens,
                    'max_iterations': self.defaults.max_iterations,
                    'prohibited_content': self.defaults.prohibited_content
                }.items() if v is not None
            }
        
        # Serialize metadata
        if self.metadata:
            result['metadata'] = {
                k: v for k, v in {
                    'created_by': self.metadata.created_by,
                    'created_at': self.metadata.created_at,
                    'domain': self.metadata.domain,
                    'tags': self.metadata.tags if self.metadata.tags else None,
                    'description': self.metadata.description,
                    **self.metadata.extra
                }.items() if v is not None
            }
        
        return result
    
    def to_json(self, path: str, indent: int = 2):
        """Save dataset to JSON file."""
        with open(path, 'w') as f:
            json.dump(self.to_dict(), f, indent=indent)
    
    def __len__(self) -> int:
        return len(self.tasks)
    
    def __iter__(self):
        return iter(self.tasks)


# ============================================================================
# CSV SUPPORT (Simple datasets only)
# ============================================================================

def from_csv(csv_path: str, name: Optional[str] = None) -> DatasetSchema:
    """
    Load a simple dataset from CSV.
    
    Expected columns:
        - id (required): Task identifier
        - input (required): User query
        - reference_output (optional): Expected output
        - success_criteria (optional): Success description
    
    For complex datasets with trajectories, use JSON format.
    """
    import csv
    from pathlib import Path
    
    path = Path(csv_path)
    tasks = []
    
    with open(path, 'r', newline='', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            if 'id' not in row or 'input' not in row:
                raise ValueError("CSV must have 'id' and 'input' columns")
            
            task = DatasetTask(
                id=row['id'],
                input=row['input'],
                name=row.get('name'),
                reference_output=row.get('reference_output'),
                success_criteria=row.get('success_criteria'),
                custom={k: v for k, v in row.items() 
                        if k not in ['id', 'input', 'name', 'reference_output', 'success_criteria']}
            )
            tasks.append(task)
    
    return DatasetSchema(
        name=name or path.stem,
        tasks=tasks,
        description=f"Loaded from {path.name}"
    )
