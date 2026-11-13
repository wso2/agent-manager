"""Entrypoint for the insurance support agent. AM run command: ``python main.py``."""

from __future__ import annotations

import os

import uvicorn

from app import app

# Clients and the web UI default to 8000; override only if it is taken.
DEFAULT_PORT = 8000


def main() -> None:
    uvicorn.run(app, host="0.0.0.0", port=int(os.environ.get("PORT", DEFAULT_PORT)))


if __name__ == "__main__":
    main()
