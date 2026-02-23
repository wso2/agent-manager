"""Tests for dataset module (schema and loader)."""

import json
import sys
import pytest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

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
from amp_evaluation.dataset.loader import parse_dataset_dict, parse_task_dict, _resolve_task_field

FIXTURES_DIR = Path(__file__).parent / "fixtures"


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

        with pytest.raises(ValueError, match="must have.*'id'.*and 'input' columns"):
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


# ============================================================================
# REGRESSION TESTS: Bugs found during code review
# ============================================================================


class TestConstraintsValidation:
    """Regression: Constraints must reject invalid values (negative, zero)."""

    def test_rejects_negative_latency(self):
        """Negative max_latency_ms should raise ValidationError."""
        with pytest.raises(Exception):  # Pydantic ValidationError
            Constraints(max_latency_ms=-1.0)

    def test_rejects_zero_tokens(self):
        """Zero max_tokens should raise ValidationError (minimum is 1)."""
        with pytest.raises(Exception):
            Constraints(max_tokens=0)

    def test_rejects_zero_iterations(self):
        """Zero max_iterations should raise ValidationError (minimum is 1)."""
        with pytest.raises(Exception):
            Constraints(max_iterations=0)

    def test_rejects_negative_cost(self):
        """Negative max_cost should raise ValidationError."""
        with pytest.raises(Exception):
            Constraints(max_cost=-0.5)

    def test_accepts_zero_latency(self):
        """Zero latency is valid (ge=0.0)."""
        c = Constraints(max_latency_ms=0.0)
        assert c.max_latency_ms == 0.0

    def test_accepts_valid_values(self):
        """All positive values should be accepted."""
        c = Constraints(max_latency_ms=5000, max_tokens=100, max_iterations=3, max_cost=1.50)
        assert c.max_latency_ms == 5000
        assert c.max_tokens == 100
        assert c.max_iterations == 3
        assert c.max_cost == 1.50


class TestSuccessCriteriaType:
    """Regression: success_criteria must accept both str and List[str]."""

    def test_accepts_string(self):
        """Single string success_criteria."""
        task = Task(task_id="t1", input="test", success_criteria="Must be accurate")
        assert task.success_criteria == "Must be accurate"

    def test_accepts_list(self):
        """List of strings success_criteria (as in sample JSON)."""
        criteria = ["Provides clear instructions", "Mentions email verification"]
        task = Task(task_id="t1", input="test", success_criteria=criteria)
        assert task.success_criteria == criteria
        assert len(task.success_criteria) == 2

    def test_accepts_none(self):
        """None is valid (optional field)."""
        task = Task(task_id="t1", input="test")
        assert task.success_criteria is None


class TestDifficultyValidation:
    """Regression: difficulty must be one of the allowed Literal values."""

    def test_accepts_valid_difficulties(self):
        """All valid difficulty levels should be accepted."""
        for level in ["easy", "medium", "hard", "expert"]:
            task = Task(task_id="t1", input="test", difficulty=level)
            assert task.difficulty == level

    def test_rejects_invalid_difficulty(self):
        """Invalid difficulty value should raise ValidationError."""
        with pytest.raises(Exception):
            Task(task_id="t1", input="test", difficulty="impossible")

    def test_default_difficulty_is_medium(self):
        """Default difficulty should be 'medium'."""
        task = Task(task_id="t1", input="test")
        assert task.difficulty == "medium"


class TestDatasetIdPropagation:
    """Regression: dataset_id must be propagated to all tasks after parsing."""

    def test_dataset_id_propagated_in_json_loading(self):
        """parse_dataset_dict must set dataset_id on all tasks."""
        data = {
            "id": "ds_test_123",
            "name": "Test",
            "tasks": [
                {"id": "t1", "input": "Input 1"},
                {"id": "t2", "input": "Input 2"},
                {"id": "t3", "input": "Input 3"},
            ],
        }
        dataset = parse_dataset_dict(data)

        for task in dataset.tasks:
            assert task.dataset_id == "ds_test_123", f"Task {task.task_id} missing dataset_id"

    def test_dataset_id_propagated_in_csv_loading(self, tmp_path):
        """CSV loader must also propagate dataset_id to tasks."""
        csv_file = tmp_path / "test.csv"
        csv_file.write_text("id,input\nt1,Input 1\nt2,Input 2\n")

        dataset = load_dataset_from_csv(str(csv_file))

        for task in dataset.tasks:
            assert task.dataset_id == dataset.dataset_id


class TestMissingInputValidation:
    """Regression: parse_task_dict must raise ValueError when 'input' is missing."""

    def test_missing_input_raises_value_error(self):
        """Task without 'input' field should raise ValueError with clear message."""
        with pytest.raises(ValueError, match="missing required 'input' field"):
            parse_task_dict({"id": "bad_task", "expected_output": "something"})

    def test_missing_input_includes_task_id(self):
        """Error message should include the task_id for debugging."""
        with pytest.raises(ValueError, match="bad_task_99"):
            parse_task_dict({"task_id": "bad_task_99"})


class TestDuplicateTaskIdDetection:
    """Regression: parse_dataset_dict should warn on duplicate task IDs."""

    def test_duplicate_task_ids_logged(self, caplog):
        """Duplicate task_id should produce a warning log."""
        import logging

        data = {
            "name": "Test",
            "tasks": [
                {"id": "dup_id", "input": "Input 1"},
                {"id": "dup_id", "input": "Input 2"},
            ],
        }

        with caplog.at_level(logging.WARNING):
            dataset = parse_dataset_dict(data)

        # Both tasks are still loaded (warning, not error)
        assert len(dataset.tasks) == 2
        # Warning was emitted
        assert any("Duplicate task_id" in msg for msg in caplog.messages)

    def test_unique_task_ids_no_warning(self, caplog):
        """Unique task_ids should not produce warnings."""
        import logging

        data = {
            "name": "Test",
            "tasks": [
                {"id": "t1", "input": "Input 1"},
                {"id": "t2", "input": "Input 2"},
            ],
        }

        with caplog.at_level(logging.WARNING):
            parse_dataset_dict(data)

        assert not any("Duplicate task_id" in msg for msg in caplog.messages)


class TestResolveTaskField:
    """Regression: classification fields must be read from top-level → metadata → custom."""

    def test_top_level_takes_priority(self):
        """Top-level field should win over metadata and custom."""
        task_data = {
            "difficulty": "hard",
            "metadata": {"difficulty": "easy"},
            "custom": {"difficulty": "medium"},
        }
        assert _resolve_task_field(task_data, "difficulty") == "hard"

    def test_metadata_fallback(self):
        """When not at top level, metadata value should be used."""
        task_data = {
            "metadata": {"difficulty": "easy"},
            "custom": {"difficulty": "medium"},
        }
        assert _resolve_task_field(task_data, "difficulty") == "easy"

    def test_custom_fallback(self):
        """When not at top level or metadata, custom value should be used."""
        task_data = {
            "custom": {"difficulty": "expert"},
        }
        assert _resolve_task_field(task_data, "difficulty") == "expert"

    def test_default_when_missing_everywhere(self):
        """When field is missing everywhere, default should be returned."""
        task_data = {"metadata": {}, "custom": {}}
        assert _resolve_task_field(task_data, "difficulty", "medium") == "medium"

    def test_difficulty_read_from_fixture(self):
        """The real fixture file should have difficulty parsed correctly (top-level field)."""
        dataset = load_dataset_from_json(str(FIXTURES_DIR / "sample_dataset.json"))
        task_001 = next(t for t in dataset.tasks if t.task_id == "task_001")
        # difficulty is at top level in fixture, should be parsed (not default 'medium')
        assert task_001.difficulty == "easy"


# ============================================================================
# TESTS: LOADING FROM REAL FIXTURE FILES
# ============================================================================


class TestFixtureFileLoading:
    """Test loading from the actual fixture files used by all samples."""

    def test_load_sample_dataset_json(self):
        """Load the real sample_dataset.json fixture and verify parsed correctly."""
        dataset = load_dataset_from_json(str(FIXTURES_DIR / "sample_dataset.json"))

        # Name resolved from metadata.name
        assert dataset.name == "Customer Support Dataset"
        assert len(dataset.tasks) == 5

        # Task IDs come from task_id field (not id)
        task_ids = [t.task_id for t in dataset.tasks]
        assert "task_001" in task_ids
        assert "task_002" in task_ids

        # Constraints merged from dataset defaults + per-task overrides
        task_001 = next(t for t in dataset.tasks if t.task_id == "task_001")
        assert task_001.constraints is not None
        # task_001 overrides max_latency_ms=3000, max_tokens=500 (both override defaults)
        assert task_001.constraints.max_latency_ms == 3000
        assert task_001.constraints.max_tokens == 500  # task-level override of default 1000

        # task_003 has no per-task constraints → all from dataset defaults
        task_003 = next(t for t in dataset.tasks if t.task_id == "task_003")
        assert task_003.constraints is not None
        assert task_003.constraints.max_latency_ms == 5000

        # Expected trajectory is parsed
        assert task_001.expected_trajectory is not None
        assert len(task_001.expected_trajectory) == 2
        assert task_001.expected_trajectory[0].tool == "search_knowledge_base"

    def test_load_simple_dataset_csv(self):
        """Load the real simple_dataset.csv fixture and verify parsed correctly."""
        dataset = load_dataset_from_csv(str(FIXTURES_DIR / "simple_dataset.csv"), name="CSV Fixture")

        assert dataset.name == "CSV Fixture"
        assert len(dataset.tasks) == 5

        # task_id column (not id) is used
        task_ids = [t.task_id for t in dataset.tasks]
        assert "csv_001" in task_ids
        assert "csv_005" in task_ids

        first_task = next(t for t in dataset.tasks if t.task_id == "csv_001")
        assert "return policy" in first_task.input.lower()
        assert first_task.expected_output is not None
