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
Trace loaders for loading evaluation data from various sources.

Supports:
- JSON files (exported traces)
- Production traces from OpenSearch/databases
- Golden datasets
- Synthetic data generation
"""
import json
from pathlib import Path
from typing import List, Dict, Any, Optional, Iterator
from datetime import datetime

from ..models import (
    Trace, Span, SpanStatus, Task, TaskInput, TaskSuccessCriteria, Dataset,
    generate_id
)


class TraceLoader:
    """Base class for trace loaders."""

    def load_traces(self, limit: Optional[int] = None) -> List[Trace]:
        """
        Load traces from source.

        Args:
            limit: Maximum number of traces to load

        Returns:
            List of traces
        """
        raise NotImplementedError

    def load_traces_iter(self, batch_size: int = 100) -> Iterator[List[Trace]]:
        """
        Load traces in batches (for large datasets).

        Args:
            batch_size: Number of traces per batch

        Yields:
            Batches of traces
        """
        raise NotImplementedError


class JSONFileTraceLoader(TraceLoader):
    """
    Load traces from a JSON file.
    Useful for local development and testing with exported traces.
    """

    def __init__(self, file_path: str):
        """
        Initialize JSON file loader.

        Args:
            file_path: Path to JSON file containing traces
        """
        self.file_path = Path(file_path)

    def load_traces(self, limit: Optional[int] = None) -> List[Trace]:
        """Load traces from JSON file."""
        if not self.file_path.exists():
            raise FileNotFoundError(f"Trace file not found: {self.file_path}")

        with open(self.file_path, 'r') as f:
            data = json.load(f)

        # Handle different JSON formats
        if isinstance(data, list):
            trace_dicts = data
        elif isinstance(data, dict) and 'traces' in data:
            trace_dicts = data['traces']
        else:
            raise ValueError(f"Invalid JSON format in {self.file_path}")

        traces = []
        for i, trace_dict in enumerate(trace_dicts):
            if limit and i >= limit:
                break

            trace = self._parse_trace(trace_dict)
            traces.append(trace)

        return traces

    def load_traces_iter(self, batch_size: int = 100) -> Iterator[List[Trace]]:
        """Load traces in batches."""
        traces = self.load_traces()

        for i in range(0, len(traces), batch_size):
            yield traces[i:i + batch_size]

    def _parse_trace(self, trace_dict: Dict[str, Any]) -> Trace:
        """Parse a trace dictionary into a Trace object."""
        # Extract spans
        spans = []
        for span_dict in trace_dict.get('spans', []):
            span = Span(
                span_id=span_dict.get('span_id', generate_id('span_')),
                kind=span_dict.get('kind', 'unknown'),
                input=span_dict.get('input'),
                output=span_dict.get('output'),
                status=SpanStatus(
                    error=span_dict.get('status', {}).get('error', False),
                    error_type=span_dict.get('status', {}).get('errorType'),
                    error_message=span_dict.get('status', {}).get('errorMessage')
                ),
                duration_ms=span_dict.get('duration_ms', 0.0),
                data=span_dict.get('data', {}),
                parent_span_id=span_dict.get('parent_span_id'),
                metadata=span_dict.get('metadata', {})
            )
            spans.append(span)

        # Create trace
        trace = Trace(
            trace_id=trace_dict.get('trace_id', generate_id('trace_')),
            agent_id=trace_dict.get('agent_id', 'unknown'),
            environment_id=trace_dict.get('environment_id'),
            input=trace_dict.get('input', ''),
            output=trace_dict.get('output', ''),
            spans=spans,
            timestamp=datetime.fromisoformat(trace_dict['timestamp']) if 'timestamp' in trace_dict else datetime.now(),
            metadata=trace_dict.get('metadata', {}),
            trial_id=trace_dict.get('trial_id'),
            task_id=trace_dict.get('task_id')
        )

        return trace


class ProductionTraceLoader(TraceLoader):
    """
    Load traces from production (e.g., OpenSearch, database).

    This is a template - actual implementation depends on your platform.
    """

    def __init__(
        self,
        agent_id: str,
        environment_id: Optional[str] = None,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None,
        filters: Optional[Dict[str, Any]] = None,
        client: Optional[Any] = None  # Platform client
    ):
        """
        Initialize production trace loader.

        Args:
            agent_id: Agent to load traces for
            environment_id: Optional environment filter
            start_time: Start of time range
            end_time: End of time range
            filters: Additional filters
            client: Platform API client
        """
        self.agent_id = agent_id
        self.environment_id = environment_id
        self.start_time = start_time
        self.end_time = end_time
        self.filters = filters or {}
        self.client = client

    def load_traces(self, limit: Optional[int] = None) -> List[Trace]:
        """Load traces from production."""
        if not self.client:
            raise ValueError("Platform client not configured")

        # This would call your platform API
        # traces = self.client.fetch_traces(
        #     agent_id=self.agent_id,
        #     environment_id=self.environment_id,
        #     start_time=self.start_time,
        #     end_time=self.end_time,
        #     filters=self.filters,
        #     limit=limit
        # )

        # For now, return empty list (template)
        return []

    def load_traces_iter(self, batch_size: int = 100) -> Iterator[List[Trace]]:
        """Load traces in batches (paginated)."""
        if not self.client:
            raise ValueError("Platform client not configured")

        offset = 0
        while True:
            # This would call your platform API with pagination
            # batch = self.client.fetch_traces(
            #     agent_id=self.agent_id,
            #     environment_id=self.environment_id,
            #     start_time=self.start_time,
            #     end_time=self.end_time,
            #     filters=self.filters,
            #     limit=batch_size,
            #     offset=offset
            # )

            # For now, yield empty (template)
            batch = []

            if not batch:
                break

            yield batch
            offset += len(batch)

            if len(batch) < batch_size:
                break


class DatasetLoader:
    """Load datasets (collections of tasks) from various sources."""

    @staticmethod
    def from_json(file_path: str) -> Dataset:
        """
        Load a dataset from JSON file.

        Args:
            file_path: Path to JSON file

        Returns:
            Dataset with tasks
        """
        path = Path(file_path)
        if not path.exists():
            raise FileNotFoundError(f"Dataset file not found: {path}")

        with open(path, 'r') as f:
            data = json.load(f)

        # Parse dataset metadata
        dataset = Dataset(
            dataset_id=data.get('dataset_id', generate_id('dataset_')),
            name=data.get('name', 'Unnamed Dataset'),
            description=data.get('description', ''),
            dataset_type=data.get('dataset_type', 'golden_set'),
            domain=data.get('domain'),
            version=data.get('version', '1.0'),
            source=str(path),
            tags=data.get('tags', [])
        )

        # Parse tasks
        for task_dict in data.get('tasks', []):
            task = DatasetLoader._parse_task(task_dict)
            dataset.add_task(task)

        return dataset

    @staticmethod
    def from_production_traces(
        traces: List[Trace],
        dataset_name: str,
        description: str = ""
    ) -> Dataset:
        """
        Create a dataset from production traces.

        Args:
            traces: List of production traces
            dataset_name: Name for the dataset
            description: Description

        Returns:
            Dataset with tasks created from traces
        """
        dataset = Dataset(
            dataset_id=generate_id('dataset_'),
            name=dataset_name,
            description=description,
            dataset_type='production_traces'
        )

        for trace in traces:
            # Create task from trace
            task = Task(
                task_id=generate_id('task_'),
                name=f"Task from {trace.trace_id}",
                description="Task created from production trace",
                input=TaskInput(
                    prompt=trace.input,
                    context=trace.metadata
                ),
                reference_output=trace.output,
                task_type="production"
            )

            dataset.add_task(task)

        return dataset

    @staticmethod
    def _parse_task(task_dict: Dict[str, Any]) -> Task:
        """Parse a task dictionary into a Task object."""
        # Parse input
        input_data = task_dict.get('input', {})
        task_input = TaskInput(
            prompt=input_data.get('prompt', ''),
            context=input_data.get('context', {}),
            files=input_data.get('files', []),
            parameters=input_data.get('parameters', {})
        )

        # Parse success criteria
        criteria_data = task_dict.get('success_criteria', {})
        success_criteria = TaskSuccessCriteria(
            expected_output=criteria_data.get('expected_output'),
            acceptable_outputs=criteria_data.get('acceptable_outputs', []),
            expected_tool_sequence=criteria_data.get('expected_tool_sequence', []),
            required_tools=criteria_data.get('required_tools', []),
            required_content=criteria_data.get('required_content', []),
            prohibited_content=criteria_data.get('prohibited_content', []),
            max_latency_ms=criteria_data.get('max_latency_ms'),
            max_tokens=criteria_data.get('max_tokens'),
            max_cost_usd=criteria_data.get('max_cost_usd'),
            max_iterations=criteria_data.get('max_iterations')
        )

        # Create task
        task = Task(
            task_id=task_dict.get('task_id', generate_id('task_')),
            name=task_dict.get('name', 'Unnamed Task'),
            description=task_dict.get('description', ''),
            input=task_input,
            success_criteria=success_criteria,
            task_type=task_dict.get('task_type', 'general'),
            difficulty=task_dict.get('difficulty', 'medium'),
            tags=task_dict.get('tags', []),
            domain=task_dict.get('domain'),
            reference_output=task_dict.get('reference_output'),
            dataset_id=task_dict.get('dataset_id'),
            metadata=task_dict.get('metadata', {})
        )

        return task

    @staticmethod
    def to_json(dataset: Dataset, file_path: str):
        """
        Save a dataset to JSON file.

        Args:
            dataset: Dataset to save
            file_path: Path to save to
        """
        # Convert dataset to dict
        data = {
            'dataset_id': dataset.dataset_id,
            'name': dataset.name,
            'description': dataset.description,
            'dataset_type': dataset.dataset_type,
            'domain': dataset.domain,
            'version': dataset.version,
            'tags': dataset.tags,
            'tasks': []
        }

        # Add tasks
        for task in dataset.tasks:
            task_dict = {
                'task_id': task.task_id,
                'name': task.name,
                'description': task.description,
                'task_type': task.task_type,
                'difficulty': task.difficulty,
                'tags': task.tags,
                'domain': task.domain,
                'input': {
                    'prompt': task.input.prompt,
                    'context': task.input.context,
                    'files': task.input.files,
                    'parameters': task.input.parameters
                },
                'success_criteria': {
                    'expected_output': task.success_criteria.expected_output,
                    'acceptable_outputs': task.success_criteria.acceptable_outputs,
                    'expected_tool_sequence': task.success_criteria.expected_tool_sequence,
                    'required_tools': task.success_criteria.required_tools,
                    'required_content': task.success_criteria.required_content,
                    'prohibited_content': task.success_criteria.prohibited_content,
                    'max_latency_ms': task.success_criteria.max_latency_ms,
                    'max_tokens': task.success_criteria.max_tokens,
                    'max_cost_usd': task.success_criteria.max_cost_usd,
                    'max_iterations': task.success_criteria.max_iterations
                },
                'expected_output': task.expected_output,
                'metadata': task.metadata
            }
            data['tasks'].append(task_dict)

        # Write to file
        path = Path(file_path)
        path.parent.mkdir(parents=True, exist_ok=True)

        with open(path, 'w') as f:
            json.dump(data, f, indent=2, default=str)
