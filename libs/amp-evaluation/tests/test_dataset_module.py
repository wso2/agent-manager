"""Tests for dataset module (schema and loader)."""

import json
import pytest

from amp_evaluation.dataset import (
    Task,
    Dataset,
    Constraints,
    TrajectoryStep,
    generate_id,
    load_dataset_from_json,
    load_dataset_from_csv,
    save_dataset_to_json,
)
from amp_evaluation.dataset.loader import parse_dataset_dict, parse_task_dict


class TestGenerateId:
    """Test ID generation."""

    def test_generate_id_no_prefix(self):
        """Test generating ID without prefix."""
        id1 = generate_id()
        id2 = generate_id()

        assert len(id1) == 12
        assert len(id2) == 12
        assert id1 != id2

    def test_generate_id_with_prefix(self):
        """Test generating ID with prefix."""
        id1 = generate_id("task_")
        id2 = generate_id("dataset_")

        assert id1.startswith("task_")
        assert id2.startswith("dataset_")
        assert len(id1) == 17  # "task_" + 12 chars
        assert len(id2) == 20  # "dataset_" + 12 chars


class TestConstraints:
    """Test Constraints model."""

    def test_constraints_all_fields(self):
        """Test Constraints with all fields."""
        constraints = Constraints(max_latency_ms=5000, max_tokens=1000, max_iterations=5, max_cost=0.10)
        assert constraints.max_latency_ms == 5000
        assert constraints.max_tokens == 1000
        assert constraints.max_iterations == 5
        assert constraints.max_cost == 0.10

    def test_constraints_partial(self):
        """Test Constraints with partial fields."""
        constraints = Constraints(max_latency_ms=3000)
        assert constraints.max_latency_ms == 3000
        assert constraints.max_tokens is None
        assert constraints.max_iterations is None
        assert constraints.max_cost is None


class TestTrajectoryStep:
    """Test TrajectoryStep model."""

    def test_trajectory_step_minimal(self):
        """Test TrajectoryStep with minimal fields."""
        step = TrajectoryStep(tool="search", args={"query": "test"})
        assert step.tool == "search"
        assert step.args == {"query": "test"}
        assert step.expected_output is None

    def test_trajectory_step_with_output(self):
        """Test TrajectoryStep with expected output."""
        step = TrajectoryStep(
            tool="lookup_order", args={"order_id": "12345"}, expected_output="Order found: Status=Shipped"
        )
        assert step.tool == "lookup_order"
        assert step.args == {"order_id": "12345"}
        assert step.expected_output == "Order found: Status=Shipped"


class TestTask:
    """Test Task model."""

    def test_task_minimal(self):
        """Test Task with minimal required fields."""
        task = Task(task_id="test_001", input="What is the capital of France?")

        assert task.task_id == "test_001"
        assert task.input == "What is the capital of France?"
        assert task.name == ""
        assert task.description == ""
        assert task.expected_output is None
        assert task.expected_trajectory is None
        assert task.success_criteria is None

    def test_task_with_all_fields(self):
        """Test Task with all fields populated."""
        constraints = Constraints(max_latency_ms=3000)
        trajectory = [TrajectoryStep(tool="search", args={"q": "test"})]

        task = Task(
            task_id="test_002",
            input="Complex query",
            name="Test Task",
            description="A test task",
            expected_output="Expected result",
            expected_trajectory=trajectory,
            success_criteria="Must be accurate",
            constraints=constraints,
            task_type="qa",
            difficulty="hard",
            domain="medical",
            tags=["test", "medical"],
            custom={"custom_field": "value"},
            metadata={"priority": "high"},
        )

        assert task.task_id == "test_002"
        assert task.name == "Test Task"
        assert task.expected_output == "Expected result"
        assert len(task.expected_trajectory) == 1
        assert task.success_criteria == "Must be accurate"
        assert task.constraints.max_latency_ms == 3000
        assert task.difficulty == "hard"
        assert task.domain == "medical"
        assert "test" in task.tags
        assert task.custom["custom_field"] == "value"


class TestDataset:
    """Test Dataset model."""

    def test_dataset_minimal(self):
        """Test Dataset with minimal fields."""
        dataset = Dataset(dataset_id="ds_001", name="Test Dataset", description="A test dataset")

        assert dataset.dataset_id == "ds_001"
        assert dataset.name == "Test Dataset"
        assert dataset.description == "A test dataset"
        assert len(dataset.tasks) == 0
        assert dataset.task_count == 0

    def test_dataset_add_task(self):
        """Test adding tasks to dataset."""
        dataset = Dataset(dataset_id="ds_001", name="Test Dataset", description="A test dataset")

        task1 = Task(task_id="t1", input="Input 1", difficulty="easy")
        task2 = Task(task_id="t2", input="Input 2", difficulty="hard")

        dataset.add_task(task1)
        dataset.add_task(task2)

        assert len(dataset.tasks) == 2
        assert dataset.task_count == 2
        assert dataset.difficulty_distribution["easy"] == 1
        assert dataset.difficulty_distribution["hard"] == 1


class TestParseTaskDict:
    """Test parse_task_dict function."""

    def test_parse_minimal_task(self):
        """Test parsing minimal task."""
        task_dict = {"id": "task_001", "input": "What is 2+2?"}

        task = parse_task_dict(task_dict)

        assert task.task_id == "task_001"
        assert task.input == "What is 2+2?"

    def test_parse_full_task(self):
        """Test parsing task with all fields."""
        task_dict = {
            "id": "task_002",
            "name": "Math Task",
            "description": "Simple addition",
            "input": "What is 2+2?",
            "expected_output": "4",
            "success_criteria": "Must be correct",
            "constraints": {"max_latency_ms": 1000, "max_tokens": 100},
            "expected_trajectory": [{"tool": "calculator", "args": {"expression": "2+2"}, "expected_output": "4"}],
            "custom": {"difficulty": "easy"},
            "metadata": {"priority": "low"},
        }

        task = parse_task_dict(task_dict)

        assert task.task_id == "task_002"
        assert task.name == "Math Task"
        assert task.expected_output == "4"
        assert task.success_criteria == "Must be correct"
        assert task.constraints.max_latency_ms == 1000
        assert len(task.expected_trajectory) == 1
        assert task.expected_trajectory[0].tool == "calculator"
        assert task.custom["difficulty"] == "easy"


class TestParseDatasetDict:
    """Test parse_dataset_dict function."""

    def test_parse_minimal_dataset(self):
        """Test parsing minimal dataset."""
        dataset_dict = {
            "name": "Test Dataset",
            "tasks": [{"id": "t1", "input": "Input 1"}, {"id": "t2", "input": "Input 2"}],
        }

        dataset = parse_dataset_dict(dataset_dict)

        assert dataset.name == "Test Dataset"
        assert len(dataset.tasks) == 2
        assert dataset.tasks[0].task_id == "t1"

    def test_parse_dataset_with_metadata(self):
        """Test parsing dataset with metadata."""
        dataset_dict = {
            "name": "Test Dataset",
            "description": "A test dataset",
            "version": "2.0",
            "metadata": {"domain": "customer_support", "tags": ["test", "qa"], "created_by": "test_user"},
            "tasks": [{"id": "t1", "input": "Input 1"}],
        }

        dataset = parse_dataset_dict(dataset_dict)

        assert dataset.name == "Test Dataset"
        assert dataset.version == "2.0"
        assert dataset.domain == "customer_support"
        assert "test" in dataset.tags
        assert dataset.created_by == "test_user"

    def test_parse_dataset_with_defaults(self):
        """Test parsing dataset with defaults."""
        dataset_dict = {
            "name": "Test Dataset",
            "defaults": {"max_latency_ms": 5000, "max_tokens": 1000, "prohibited_content": ["bad_word"]},
            "tasks": [
                {"id": "t1", "input": "Input 1"},
                {"id": "t2", "input": "Input 2", "constraints": {"max_latency_ms": 2000}},
            ],
        }

        dataset = parse_dataset_dict(dataset_dict)

        # First task gets defaults
        assert dataset.tasks[0].constraints.max_latency_ms == 5000
        assert dataset.tasks[0].constraints.max_tokens == 1000
        assert dataset.tasks[0].prohibited_content == ["bad_word"]

        # Second task overrides latency but inherits tokens
        assert dataset.tasks[1].constraints.max_latency_ms == 2000
        assert dataset.tasks[1].constraints.max_tokens == 1000


class TestJSONLoading:
    """Test JSON loading functions."""

    def test_save_and_load_roundtrip(self, tmp_path):
        """Test saving and loading preserves data."""
        original_dataset = Dataset(
            dataset_id="ds_rt1",
            name="Roundtrip Test",
            description="Test dataset",
            tasks=[
                Task(
                    task_id="t1",
                    input="Test input",
                    expected_output="Test output",
                    success_criteria="Must work",
                    constraints=Constraints(max_latency_ms=2000),
                    expected_trajectory=[TrajectoryStep(tool="test_tool", args={"arg": "value"})],
                )
            ],
        )

        output_file = tmp_path / "test_dataset.json"
        save_dataset_to_json(original_dataset, str(output_file))

        loaded_dataset = load_dataset_from_json(str(output_file))

        assert loaded_dataset.name == original_dataset.name
        assert loaded_dataset.description == original_dataset.description
        assert len(loaded_dataset.tasks) == 1

        orig_task = original_dataset.tasks[0]
        loaded_task = loaded_dataset.tasks[0]

        assert loaded_task.task_id == orig_task.task_id
        assert loaded_task.input == orig_task.input
        assert loaded_task.expected_output == orig_task.expected_output
        assert loaded_task.constraints.max_latency_ms == orig_task.constraints.max_latency_ms
        assert len(loaded_task.expected_trajectory) == 1
        assert loaded_task.expected_trajectory[0].tool == "test_tool"

    def test_load_nonexistent_file(self):
        """Test loading from nonexistent file."""
        with pytest.raises(FileNotFoundError):
            load_dataset_from_json("/nonexistent/path/dataset.json")

    def test_load_invalid_json(self, tmp_path):
        """Test loading invalid JSON."""
        invalid_file = tmp_path / "invalid.json"
        invalid_file.write_text("{ invalid json }")

        with pytest.raises(json.JSONDecodeError):
            load_dataset_from_json(str(invalid_file))


class TestCSVLoading:
    """Test CSV loading functions."""

    def test_load_simple_csv(self, tmp_path):
        """Test loading simple CSV file."""
        csv_file = tmp_path / "test.csv"
        csv_file.write_text(
            "id,input,expected_output,success_criteria\n"
            "t1,What is 2+2?,4,Must be correct\n"
            "t2,What is 3+3?,6,Must be accurate\n"
        )

        dataset = load_dataset_from_csv(str(csv_file), name="Test CSV Dataset")

        assert dataset.name == "Test CSV Dataset"
        assert len(dataset.tasks) == 2
        assert dataset.tasks[0].task_id == "t1"
        assert dataset.tasks[0].input == "What is 2+2?"
        assert dataset.tasks[0].expected_output == "4"
        assert dataset.tasks[0].success_criteria == "Must be correct"

    def test_csv_default_name(self, tmp_path):
        """Test CSV loader uses filename as default name."""
        csv_file = tmp_path / "my_dataset.csv"
        csv_file.write_text("id,input\nt1,Input 1\n")

        dataset = load_dataset_from_csv(str(csv_file))

        assert dataset.name == "my_dataset"

    def test_csv_missing_required_columns(self, tmp_path):
        """Test CSV with missing required columns."""
        csv_file = tmp_path / "bad.csv"
        csv_file.write_text("wrong_column\nvalue\n")

        with pytest.raises(ValueError, match="must have 'id' and 'input' columns"):
            load_dataset_from_csv(str(csv_file))

    def test_csv_nonexistent_file(self):
        """Test loading from nonexistent CSV file."""
        with pytest.raises(FileNotFoundError):
            load_dataset_from_csv("/nonexistent/path/dataset.csv")


class TestIntegration:
    """Integration tests."""

    def test_create_dataset_programmatically(self):
        """Test creating dataset programmatically."""
        dataset = Dataset(
            dataset_id=generate_id("dataset_"), name="Programmatic Dataset", description="Created via API"
        )

        for i in range(3):
            task = Task(
                task_id=generate_id("task_"),
                input=f"Test input {i}",
                expected_output=f"Expected output {i}",
                difficulty="easy" if i == 0 else "medium",
            )
            dataset.add_task(task)

        assert len(dataset.tasks) == 3
        assert dataset.task_count == 3
        assert dataset.difficulty_distribution["easy"] == 1
        assert dataset.difficulty_distribution["medium"] == 2

    def test_dataset_filtering(self):
        """Test filtering tasks by metadata."""
        dataset = Dataset(dataset_id="ds_filter", name="Filter Test", description="Test filtering")

        dataset.add_task(Task(task_id="t1", input="I1", difficulty="easy", domain="medical"))
        dataset.add_task(Task(task_id="t2", input="I2", difficulty="hard", domain="legal"))
        dataset.add_task(Task(task_id="t3", input="I3", difficulty="easy", domain="medical"))

        # Filter by difficulty
        easy_tasks = [t for t in dataset.tasks if t.difficulty == "easy"]
        assert len(easy_tasks) == 2

        # Filter by domain
        medical_tasks = [t for t in dataset.tasks if t.domain == "medical"]
        assert len(medical_tasks) == 2

    def test_json_schema_version(self, tmp_path):
        """Test that saved JSON includes schema version."""
        dataset = Dataset(
            dataset_id="ds_version", name="Version Test", description="Test", tasks=[Task(task_id="t1", input="Input")]
        )

        output_file = tmp_path / "versioned.json"
        save_dataset_to_json(dataset, str(output_file))

        with open(output_file) as f:
            data = json.load(f)

        assert "schema_version" in data
        assert data["schema_version"] == "1.0"
