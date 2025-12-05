# Testing

## Setup

```bash
# Install package in development mode
pip install -e .

# Install test dependencies
pip install pytest pytest-cov
```

## Run Tests

```bash
# All tests
pytest

# Verbose output
pytest -v

# Specific test file
pytest tests/test_cli.py

# Specific test method
pytest tests/test_cli.py::TestCLI::test_cli_passes_args

# With coverage report
pytest --cov-report=html
open htmlcov/index.html
```

## Test Files

- `test_cli.py` - CLI functionality and argument handling
- `test_initialization.py` - Instrumentation setup and configuration
- `conftest.py` - Shared test fixtures and setup

## Troubleshooting

**Import errors:** Ensure the package is installed in development mode:

```bash
pip install -e .
pip list | grep wso2-agent-instrumentation
```

**Missing dependencies:**

```bash
pip install pytest pytest-cov
```
