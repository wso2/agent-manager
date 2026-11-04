"""Encode complete Hotel API log events as single-line JSON records."""

import json
import logging
from datetime import datetime, timezone


class JSONFormatter(logging.Formatter):
    """Keep untrusted message and exception text inside JSON string values."""

    def format(self, record: logging.LogRecord) -> str:
        event = {
            "time": datetime.fromtimestamp(record.created, timezone.utc).isoformat(),
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
        }
        if record.exc_info:
            event["exception"] = self.formatException(record.exc_info)
        if record.stack_info:
            event["stack"] = self.formatStack(record.stack_info)
        # ASCII escaping also contains Unicode line separators and terminal controls.
        return json.dumps(event, ensure_ascii=True)


def configure_logging() -> None:
    """Configure root output, including handlers already installed by a host."""
    logging.basicConfig(level=logging.INFO)
    formatter = JSONFormatter()
    for handler in logging.getLogger().handlers:
        handler.setFormatter(formatter)
