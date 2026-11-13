"""Run the Insurance Support sample as an external agent and find its trace in Agent Manager."""
from __future__ import annotations

import contextlib
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from external_agent import ExternalSample, parse_args, run  # noqa: E402
from support import SAMPLE, check_evidence, invoke, local_model  # noqa: E402


@contextlib.contextmanager
def model():
    with local_model() as (_, url):
        yield url


INSURANCE_SUPPORT = ExternalSample(
    name_prefix="ins-smoke-",
    app_dir=SAMPLE,
    requirements=SAMPLE / "requirements.txt",
    command=["main.py"],
    health="/health",
    model=model,
    env=lambda model_url, port: {
        "MODEL_PROVIDER": "openai", "OPENAI_BASE_URL": model_url + "/v1", "OPENAI_API_KEY": "smoke-only",
        "OPENAI_MODEL": "smoke-model", "USE_MCP": "false", "PORT": str(port), "PYTHON_DOTENV_DISABLED": "1",
    },
    invoke=invoke,
    check_evidence=check_evidence,
)

if __name__ == "__main__":
    run(parse_args(__doc__), INSURANCE_SUPPORT)
