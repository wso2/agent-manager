# AMP Evaluation Framework

A comprehensive, production-ready evaluation framework for AI agents that works with real execution traces to provide deep insights into agent performance.

## Overview

The evaluation framework is a trace-based system that analyzes real agent executions to measure quality, performance, and reliability. Built for both benchmarking and continuous production monitoring.

### Key Features

- **Trace-Based Evaluation**: Analyze real agent executions from OpenTelemetry/AMP traces
- **Rich Span Analysis**: Evaluate LLM calls, tool usage, retrievals, and agent reasoning
- **Built-in Evaluators**: 13+ ready-to-use evaluators for output quality, trajectory, and performance
- **Flexible Aggregation**: MEAN, MEDIAN, P95, PASS_RATE, and custom aggregations
- **Two Evaluation Modes**: Benchmark datasets with ground truth OR live production monitoring
- **Platform Integration**: Publish results to AMP Platform for tracking and dashboards
- **Extensible Architecture**: Easy to add custom evaluators and aggregations

## Installation

```bash
pip install amp-evaluation
```

Or install from source:

```bash
cd libs/amp-evaluation
pip install -e .
```

## Quick Start

### 1. Simple Evaluation with Built-in Evaluators

```python
from amp_eval import LiveRunner, Config

# Configure connection to trace service
config = Config.from_env()  # Loads from environment variables

# Create runner with built-in evaluators
runner = LiveRunner(
    config=config,
    evaluator_names=["answer-length", "exact-match"]
)

# Fetch and evaluate recent traces
result = runner.run()

print(f"Evaluated {result.trace_count} traces")
print(f"Results: {result.aggregated_results}")
```

### 2. Define a Custom Evaluator

```python
from amp_eval import BaseEvaluator, EvalContext, EvalResult, register

@register("answer-quality", tags=["quality", "output"])
class AnswerQualityEvaluator(BaseEvaluator):
    """Checks if answer meets quality standards."""
    
    def evaluate(self, context: EvalContext) -> EvalResult:
        trace = context.trace
        output_length = len(trace.output) if trace.output else 0
        
        # Score based on length and content
        has_content = output_length > 50
        no_errors = not trace.has_errors
        
        score = 1.0 if (has_content and no_errors) else 0.5
        
        return self._create_result(
            target_id=trace.trace_id,
            target_type="trace",
            score=score,
            passed=score >= 0.7,
            explanation=f"Quality check: {output_length} chars, errors={trace.has_errors}",
            details={
                "output_length": output_length,
                "error_count": trace.metrics.error_count
            }
        )
```

### 3. Use with Ground Truth (Benchmark Mode)

```python
from amp_eval import BenchmarkRunner, Dataset, Task

# Load benchmark dataset
dataset = Dataset.from_csv("benchmarks/qa_dataset.csv")

# Create benchmark runner
runner = BenchmarkRunner(
    config=config,
    evaluators=["exact-match", "answer-relevancy"],
    dataset=dataset
)

# Run evaluation
result = runner.run()

# Access aggregated results
for eval_name, agg_results in result.aggregated_results.items():
    print(f"{eval_name}:")
    print(f"  Mean: {agg_results['mean']:.3f}")
    print(f"  Median: {agg_results['median']:.3f}")
    print(f"  Pass Rate (≥0.7): {agg_results.get('pass_rate_threshold_0.7', 'N/A')}")
```

## Core Concepts

### EvalTrace
The main data structure representing a single agent execution extracted from OpenTelemetry spans.

```python
from amp_eval.trace import EvalTrace

# EvalTrace contains:
trace.trace_id           # Unique identifier
trace.input              # Agent input
trace.output             # Agent output
trace.llm_spans          # List of LLM calls
trace.tool_spans         # List of tool invocations
trace.retriever_spans    # List of retrieval operations
trace.agent_span         # Primary agent span
trace.metrics            # Aggregated metrics (tokens, duration, errors)
trace.success            # Whether trace succeeded (no errors)

# Convenience properties:
trace.has_output         # bool
trace.has_errors         # bool
trace.all_tool_names     # List[str]
trace.unique_models_used # List[str]
```

### EvalContext
Rich context object passed to evaluators containing the trace and optional ground truth.

```python
from amp_eval.models import EvalContext

# EvalContext provides:
context.trace              # EvalTrace object
context.trace_id           # str
context.expected_output    # Ground truth output (raises if not available)
context.expected_trajectory # Expected tool sequence (raises if not available)
context.prohibited_content # List of prohibited strings
context.constraints        # Performance constraints (latency, tokens, iterations)
context.metadata           # Additional context

# Check availability before access:
if context.has_expected_output():
    expected = context.expected_output
```

### BaseEvaluator
Abstract base class for all evaluators. Implements single `evaluate(context)` interface.

```python
from amp_eval import BaseEvaluator, EvalContext, EvalResult

class MyEvaluator(BaseEvaluator):
    def __init__(self, threshold: float = 0.7):
        super().__init__()
        self._name = "my-evaluator"
        self.threshold = threshold
    
    def evaluate(self, context: EvalContext) -> EvalResult:
        trace = context.trace
        
        # Your evaluation logic
        score = calculate_score(trace)
        
        return self._create_result(
            target_id=context.trace_id,
            target_type="trace",
            score=score,
            passed=score >= self.threshold,
            explanation="Detailed explanation",
            details={"metric1": 0.8, "metric2": 0.9}
        )
```

### Evaluator Types

**Code Evaluators** (Default)
- Deterministic, rule-based evaluation
- Fast and reliable
- Examples: exact match, length check, tool usage

**LLM-as-Judge Evaluators**
- Use language models to evaluate quality
- Flexible for subjective criteria
- Examples: relevancy, helpfulness, coherence

```python
from amp_eval.evaluators import LLMAsJudgeEvaluator

class RelevancyEvaluator(LLMAsJudgeEvaluator):
    def __init__(self):
        super().__init__(
            model="gpt-4",
            criteria="relevancy to the user's question"
        )
        self._name = "llm-relevancy"
```

**Human Evaluators**
- Async human review
- For subjective quality assessment
- Results collected asynchronously

### RunType Enum
Evaluation mode indicator.

```python
from amp_eval import RunType

# Two modes:
RunType.BENCHMARK  # Evaluating against ground truth dataset
RunType.LIVE       # Monitoring live production traces
```

### Aggregation System

Compute statistics across multiple evaluation results.

**Base Types and Configuration** (`aggregators/base.py`)

```python
from amp_eval.aggregators import AggregationType, Aggregation

# Simple aggregations (no parameters)
aggregations = [
    AggregationType.MEAN,
    AggregationType.MEDIAN,
    AggregationType.P95,
    AggregationType.MAX,
]

# Parameterized aggregations
aggregations = [
    Aggregation(AggregationType.PASS_RATE, threshold=0.7),
    Aggregation(AggregationType.PASS_RATE, threshold=0.9),
]

# Custom aggregations
def custom_range(scores, **kwargs):
    return max(scores) - min(scores)

aggregations = [
    AggregationType.MEAN,
    Aggregation(custom_range)  # Inline function
]
```

**Built-in Aggregations** (`aggregators/builtin.py`)

```python
# Statistical aggregations:
AggregationType.MEAN       # Average
AggregationType.MEDIAN     # Median
AggregationType.MIN        # Minimum
AggregationType.MAX        # Maximum
AggregationType.SUM        # Sum
AggregationType.COUNT      # Count
AggregationType.STDEV      # Standard deviation
AggregationType.VARIANCE   # Variance

# Percentiles:
AggregationType.P50        # 50th percentile
AggregationType.P75        # 75th percentile
AggregationType.P90        # 90th percentile
AggregationType.P95        # 95th percentile
AggregationType.P99        # 99th percentile

# Pass/fail based:
AggregationType.PASS_RATE  # Requires threshold parameter
```

**Execution Engine** (`aggregators/aggregation.py`)

```python
from amp_eval.aggregators import ResultAggregator, AggregatedResults

# Aggregate results
results = [result1, result2, result3, ...]  # List of EvalResult

aggregated = ResultAggregator.aggregate(
    results,
    aggregations=[
        AggregationType.MEAN,
        AggregationType.MEDIAN,
        Aggregation(AggregationType.PASS_RATE, threshold=0.7),
        Aggregation(AggregationType.PASS_RATE, threshold=0.9),
    ]
)

# Access results
print(aggregated.mean)                      # 0.85
print(aggregated.median)                    # 0.88
print(aggregated["pass_rate_threshold_0.7"]) # 0.92
print(aggregated.count)                     # 100
print(aggregated.individual_scores)         # [(trace_id, score), ...]

# Aggregate by evaluator
by_evaluator = ResultAggregator.aggregate_by_evaluator(results)
for eval_name, agg in by_evaluator.items():
    print(f"{eval_name}: mean={agg.mean:.3f}")
```

**Custom Aggregator Registration**

```python
from amp_eval.aggregators import register_aggregator

def weighted_average(scores, weights=None, **kwargs):
    if weights:
        return sum(s * w for s, w in zip(scores, weights)) / sum(weights)
    return sum(scores) / len(scores)

register_aggregator("weighted_avg", weighted_average)

# Now use it:
aggregations = [
    Aggregation("weighted_avg", weights=[0.5, 0.3, 0.2])
]
```

### Datasets & Benchmarks

Create reusable benchmark datasets with ground truth.

```python
from amp_eval import Dataset, Task

# Create dataset
dataset = Dataset(
    dataset_id="qa-benchmark-v1",
    name="Q&A Benchmark",
    description="100 question-answering scenarios with ground truth"
)

# Add tasks with ground truth
task = Task(
    task_id="task_001",
    input="What is the capital of France?",
    expected_output="Paris",
    metadata={"category": "geography", "difficulty": "easy"}
)
dataset.add_task(task)

# Save for version control
dataset.to_csv("benchmarks/qa_benchmark_v1.csv")
dataset.to_json("benchmarks/qa_benchmark_v1.json")

# Load later
dataset = Dataset.from_csv("benchmarks/qa_benchmark_v1.csv")
dataset = Dataset.from_json("benchmarks/qa_benchmark_v1.json")
```

### Runners

**BenchmarkRunner** - Evaluate against ground truth dataset

```python
from amp_eval import BenchmarkRunner, Config

config = Config.from_env()
dataset = Dataset.from_csv("benchmarks/qa_benchmark.csv")

runner = BenchmarkRunner(
    config=config,
    evaluators=["exact-match", "contains-match"],
    dataset=dataset
)

result = runner.run()
```

**LiveRunner** - Monitor production traces

```python
from amp_eval import LiveRunner, Config

config = Config.from_env()

runner = LiveRunner(
    config=config,
    evaluator_names=["has-output", "error-free"],
    batch_size=50  # Process 50 traces per batch
)

# Fetch and evaluate recent traces
result = runner.run(
    start_time="2024-01-26T00:00:00Z",
    end_time="2024-01-26T23:59:59Z"
)
```

**Filtering Evaluators**

```python
# By tags
runner = LiveRunner(
    config=config,
    include_tags=["quality", "safety"],    # Only run these
    exclude_tags=["slow", "experimental"]  # Skip these
)

# By name
runner = LiveRunner(
    config=config,
    evaluator_names=["exact-match", "answer-length"]
)
```

## Built-in Evaluators

The framework includes 13 production-ready evaluators in `evaluators/builtin.py`:

### Output Quality Evaluators

| Evaluator                    | Description                                  | Parameters                                  |
| ---------------------------- | -------------------------------------------- | ------------------------------------------- |
| `AnswerLengthEvaluator`      | Validates answer length is within bounds     | `min_length`, `max_length`                  |
| `AnswerRelevancyEvaluator`   | Checks word overlap between input and output | `min_overlap_ratio`                         |
| `RequiredContentEvaluator`   | Ensures required strings/patterns present    | `required_strings`, `required_patterns`     |
| `ProhibitedContentEvaluator` | Ensures prohibited content absent            | `prohibited_strings`, `prohibited_patterns` |
| `ExactMatchEvaluator`        | Exact match with expected output             | `case_sensitive`, `strip_whitespace`        |
| `ContainsMatchEvaluator`     | Expected output contained in actual          | `case_sensitive`                            |

### Trajectory Evaluators

| Evaluator                  | Description                           | Parameters                    |
| -------------------------- | ------------------------------------- | ----------------------------- |
| `ToolSequenceEvaluator`    | Validates tool call sequence          | `expected_sequence`, `strict` |
| `RequiredToolsEvaluator`   | Checks required tools were used       | `required_tools`              |
| `StepSuccessRateEvaluator` | Measures trajectory step success rate | `min_success_rate`            |

### Performance Evaluators

| Evaluator                  | Description               | Parameters       |
| -------------------------- | ------------------------- | ---------------- |
| `LatencyEvaluator`         | Checks latency within SLA | `max_latency_ms` |
| `TokenEfficiencyEvaluator` | Validates token usage     | `max_tokens`     |
| `IterationCountEvaluator`  | Checks iteration count    | `max_iterations` |

### Outcome Evaluators

| Evaluator                  | Description                              | Parameters |
| -------------------------- | ---------------------------------------- | ---------- |
| `ExpectedOutcomeEvaluator` | Validates trace success matches expected | -          |

### Using Built-in Evaluators

```python
from amp_eval.evaluators import (
    AnswerLengthEvaluator,
    ExactMatchEvaluator,
    LatencyEvaluator
)

# Instantiate with custom parameters
evaluators = [
    AnswerLengthEvaluator(min_length=10, max_length=500),
    ExactMatchEvaluator(case_sensitive=False),
    LatencyEvaluator(max_latency_ms=2000)
]

# Or use by name (registered automatically)
runner = LiveRunner(
    config=config,
    evaluator_names=["answer-length", "exact-match", "latency"]
)
```

## Advanced Usage

### Custom Evaluators with Aggregations

```python
from amp_eval import BaseEvaluator, EvalContext, register
from amp_eval.aggregators import AggregationType, Aggregation

@register("semantic-similarity", tags=["quality", "nlp"])
class SemanticSimilarityEvaluator(BaseEvaluator):
    def __init__(self):
        super().__init__()
        self._name = "semantic-similarity"
        
        # Configure custom aggregations
        self._aggregations = [
            AggregationType.MEAN,
            AggregationType.MEDIAN,
            AggregationType.P95,
            Aggregation(AggregationType.PASS_RATE, threshold=0.8),
        ]
    
    def evaluate(self, context: EvalContext) -> EvalResult:
        # Your similarity calculation
        similarity = calculate_similarity(
            context.trace.output,
            context.expected_output
        )
        
        return self._create_result(
            target_id=context.trace_id,
            target_type="trace",
            score=similarity,
            passed=similarity >= 0.8,
            explanation=f"Semantic similarity: {similarity:.3f}"
        )
```

### LLM-as-Judge Pattern

```python
from amp_eval.evaluators import LLMAsJudgeEvaluator
import openai

class HelpfulnessEvaluator(LLMAsJudgeEvaluator):
    def __init__(self):
        super().__init__(
            model="gpt-4",
            criteria="helpfulness, clarity, and completeness"
        )
        self._name = "llm-helpfulness"
    
    def call_llm(self, prompt: str) -> dict:
        response = openai.chat.completions.create(
            model=self.model,
            messages=[
                {"role": "system", "content": "You are an expert evaluator."},
                {"role": "user", "content": prompt}
            ],
            temperature=0.0
        )
        
        # Parse structured response
        content = response.choices[0].message.content
        score = parse_score(content)  # Extract score 0-1
        explanation = parse_explanation(content)
        
        return {
            "score": score,
            "explanation": explanation
        }
```

### Composite Evaluators

Combine multiple evaluators into one.

```python
from amp_eval.evaluators import CompositeEvaluator

class OverallQualityEvaluator(CompositeEvaluator):
    def __init__(self):
        # Automatically runs all sub-evaluators
        sub_evaluators = [
            AnswerLengthEvaluator(),
            AnswerRelevancyEvaluator(),
            RequiredContentEvaluator(required_strings=["important", "keyword"])
        ]
        super().__init__(sub_evaluators, name="overall-quality")
```

### Function-Based Evaluators

Quick evaluators using the `@register` decorator.

```python
from amp_eval import register, EvalContext

@register("has-greeting", tags=["output", "simple"])
def check_greeting(context: EvalContext) -> float:
    """Simple function-based evaluator."""
    output = context.trace.output.lower()
    return 1.0 if any(g in output for g in ["hello", "hi", "greetings"]) else 0.0
```

### Configuration from Environment

```python
import os
from amp_eval import Config

# Set environment variables
os.environ["AGENT_UID"] = "my-agent-123"
os.environ["ENVIRONMENT_UID"] = "production"
os.environ["TRACE_LOADER_MODE"] = "platform"
os.environ["PUBLISH_RESULTS"] = "true"
os.environ["AMP_API_URL"] = "http://localhost:8001"
os.environ["AMP_API_KEY"] = "your-api-key"

# Automatically validates required fields
config = Config.from_env()
```

### Publishing Results to Platform

```python
from amp_eval import LiveRunner, Config

config = Config.from_env()
config.publish_results = True  # Enable platform publishing

# Results automatically published
runner = LiveRunner(config=config, evaluator_names=["quality-check"])
result = runner.run()

# Results now visible in platform dashboard
print(f"Run ID: {result.run_id}")
print(f"Published: {result.metadata.get('published', False)}")
```

## Project Structure

```
amp-evaluation/
├── src/amp_evaluation/
│   ├── __init__.py            # Public API exports
│   ├── config.py              # Configuration management
│   ├── models.py              # Core data models (EvalResult, EvalContext, etc.)
│   ├── registry.py            # Evaluator registration system
│   ├── runner.py              # Evaluation runners (Benchmark, Live)
│   │
│   ├── evaluators/            # Evaluator system
│   │   ├── __init__.py
│   │   ├── base.py            # BaseEvaluator, LLMAsJudgeEvaluator, etc.
│   │   └── builtin.py         # 13 built-in evaluators
│   │
│   ├── aggregators/           # Aggregation system
│   │   ├── __init__.py
│   │   ├── base.py            # AggregationType, Aggregation, registry
│   │   ├── builtin.py         # Built-in aggregation functions
│   │   └── aggregation.py     # ResultAggregator execution engine
│   │
│   ├── trace/                 # Trace handling
│   │   ├── __init__.py
│   │   ├── models.py          # EvalTrace, Span models
│   │   ├── parser.py          # OTEL → EvalTrace conversion
│   │   └── fetcher.py         # TraceFetcher for API integration
│   │
│   └── loaders/               # Data loading
│       ├── __init__.py
│       ├── dataset_loader.py  # Dataset CSV/JSON loading
│       └── trace_loader.py    # Trace loading utilities
│
├── examples/
│   ├── complete_example.py    # Full demonstration
│   ├── agent.py               # Simple agent example
│   └── datasets/
│       └── simple_qa.csv      # Example dataset
│
├── tests/                     # Comprehensive test suite
│   ├── test_aggregators.py
│   ├── test_evaluators.py
│   └── test_runner.py
│
├── docs/                      # Documentation
│   ├── ARCHITECTURE.md
│   ├── QUICKSTART.md
│   └── CAPABILITIES.md
│
├── pyproject.toml            # Package configuration
└── README.md                 # This file
```

## Architecture Overview

### Three-Layer Design

1. **Evaluation Layer** (`evaluators/`)
   - Base classes and interfaces
   - Built-in evaluators
   - Custom evaluator registration

2. **Aggregation Layer** (`aggregators/`)
   - Type definitions and registry (`base.py`)
   - Built-in aggregation functions (`builtin.py`)
   - Execution engine (`aggregation.py`)

3. **Execution Layer** (`runner.py`)
   - BenchmarkRunner for datasets
   - LiveRunner for production monitoring
   - Result publishing and reporting

## Examples

### Complete Working Example

```python
from amp_eval import (
    Config, LiveRunner, BaseEvaluator, EvalContext, EvalResult,
    register, AggregationType, Aggregation
)

# 1. Define custom evaluator
@register("custom-quality", tags=["quality", "custom"])
class CustomQualityEvaluator(BaseEvaluator):
    def __init__(self):
        super().__init__()
        self._name = "custom-quality"
        self._aggregations = [
            AggregationType.MEAN,
            AggregationType.P95,
            Aggregation(AggregationType.PASS_RATE, threshold=0.8)
        ]
    
    def evaluate(self, context: EvalContext) -> EvalResult:
        trace = context.trace
        
        # Multi-factor quality score
        has_output = 1.0 if trace.has_output else 0.0
        no_errors = 1.0 if not trace.has_errors else 0.0
        reasonable_length = 1.0 if 10 <= len(trace.output) <= 1000 else 0.5
        
        score = (has_output + no_errors + reasonable_length) / 3
        
        return self._create_result(
            target_id=trace.trace_id,
            target_type="trace",
            score=score,
            passed=score >= 0.8,
            explanation=f"Quality score: {score:.2f}",
            details={
                "has_output": has_output,
                "no_errors": no_errors,
                "length_ok": reasonable_length
            }
        )

# 2. Configure
config = Config.from_env()

# 3. Create runner with multiple evaluators
runner = LiveRunner(
    config=config,
    evaluator_names=["custom-quality"],
    include_tags=["quality"],
    batch_size=100
)

# 4. Run evaluation
result = runner.run()

# 5. Analyze results
print(f"Run ID: {result.run_id}")
print(f"Run Type: {result.run_type}")
print(f"Traces Evaluated: {result.trace_count}")
print(f"Duration: {result.duration_seconds:.2f}s")

for eval_name, agg_results in result.aggregated_results.items():
    print(f"\n{eval_name}:")
    print(f"  Mean: {agg_results.mean:.3f}")
    print(f"  P95: {agg_results['p95']:.3f}")
    print(f"  Pass Rate (≥0.8): {agg_results['pass_rate_threshold_0.8']:.1%}")
    print(f"  Count: {agg_results.count}")
```

See `examples/complete_example.py` for a full working demonstration.

## Testing

Run the test suite:

```bash
# All tests
pytest

# Specific test file
pytest tests/test_aggregators.py -v

# With coverage
pytest --cov=amp_eval --cov-report=html
```

## Key Features in Detail

### 1. Trace-Based Architecture
- Works with real OpenTelemetry traces
- No synthetic data generation needed
- Supports any agent framework (LangChain, CrewAI, custom, etc.)

### 2. Flexible Evaluation
- Code-based evaluators (fast, deterministic)
- LLM-as-judge evaluators (flexible, subjective criteria)
- Human-in-the-loop support
- Composite evaluators

### 3. Rich Aggregations
- 15+ built-in aggregations
- Custom aggregation functions
- Parameterized aggregations
- Per-evaluator configuration

### 4. Two Evaluation Modes
- **Benchmark**: Compare against ground truth datasets
- **Live**: Monitor production traces continuously

### 5. Production Ready
- Config validation
- Error handling
- Async support
- Platform integration
- Comprehensive logging

## Getting Started Checklist

- [ ] Install package: `pip install amp-evaluation`
- [ ] Set up environment variables
- [ ] Start trace service or configure OpenSearch
- [ ] Try built-in evaluators with `LiveRunner`
- [ ] Create custom evaluator for your use case
- [ ] Set up benchmark dataset (optional)
- [ ] Configure platform publishing (optional)

## Configuration

The library reads configuration from environment variables when using `Config.from_env()`:

### Core Configuration (Required)

```bash
# Agent identification
AGENT_UID="your-agent-id"
ENVIRONMENT_UID="production"

# Trace loading mode
TRACE_LOADER_MODE="platform"  # or "file"

# Publishing results to platform
PUBLISH_RESULTS="true"

# Platform API (required when PUBLISH_RESULTS=true or TRACE_LOADER_MODE=platform)
AMP_API_URL="http://localhost:8001"
AMP_API_KEY="xxxxx"

# If using file mode for traces:
TRACE_FILE_PATH="./traces/my_traces.json"
```

That's it! All configuration is handled through these environment variables.

For detailed configuration options, see `src/amp_evaluation/config.py`.

## Contributing

Contributions welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass: `pytest`
5. Submit a pull request

## License

Apache License 2.0 - see LICENSE file for details.

## Tips & Best Practices

1. **Start Simple**: Use built-in evaluators first
2. **Use Tags**: Organize evaluators with tags for easy filtering
3. **Configure Aggregations**: Set per-evaluator aggregations
4. **Validate Config**: Always use `Config.from_env()`
5. **Monitor Production**: Use `LiveRunner` for continuous monitoring

## FAQ

**Q: Can I use this with LangChain/CrewAI/other frameworks?**  
A: Yes! Works with any agent producing OpenTelemetry traces.

**Q: Do I need ground truth data?**  
A: No. Use `LiveRunner` without ground truth, or `BenchmarkRunner` with datasets.

**Q: How do I create custom evaluators?**  
A: Extend `BaseEvaluator` and implement `evaluate(context)`.


