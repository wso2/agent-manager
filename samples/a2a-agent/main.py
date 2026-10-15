"""Local runner: ``python main.py`` serves the A2A agent on port 9099."""

import os

# The agent emits its own A2A spans (tracing.py) in place of the SDK's, which
# carry no A2A attributes. The SDK reads this once, when a2a is first imported.
os.environ.setdefault("OTEL_INSTRUMENTATION_A2A_SDK_ENABLED", "false")

import uvicorn  # noqa: E402

from app import PORT, app  # noqa: E402

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=PORT)
