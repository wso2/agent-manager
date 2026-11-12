"""Run the IT Helpdesk sample as an external agent and find its trace in Agent Manager."""
from __future__ import annotations

import contextlib
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from external_agent import ExternalSample, parse_args, run  # noqa: E402
from smoke import SAMPLE, check_evidence, invoke, local_model  # noqa: E402

VERSION = "external-smoke"


@contextlib.contextmanager
def model():
    with local_model() as (_, url):
        yield url


IT_HELPDESK = ExternalSample(
    name_prefix="it-smoke-",
    app_dir=SAMPLE,
    requirements=SAMPLE / "requirements.txt",
    command=["-m", "uvicorn", "app:app", "--host", "127.0.0.1", "--port", "{port}"],
    health="/health",
    model=model,
    env=lambda model_url, _port: {
        "LANGCHAIN_TRACING_V2": "false", "LANGSMITH_TRACING": "false",
        "USE_LLM_PROVIDER": "true", "LLM_PROVIDER_URL": model_url + "/v1", "LLM_PROVIDER_KEY": "smoke-only",
        "USE_MCP": "false", "AGENT_VERSION": VERSION,
    },
    invoke=lambda endpoint, marker: invoke(endpoint, marker, version=VERSION),
    check_evidence=check_evidence,
)

if __name__ == "__main__":
    run(parse_args(__doc__), IT_HELPDESK)
