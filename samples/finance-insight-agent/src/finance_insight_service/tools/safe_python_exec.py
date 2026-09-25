import ast
import datetime as datetime_module
import io
import json
import math
import statistics
import subprocess
import sys
import time
import traceback
import textwrap
from typing import Any
from contextlib import redirect_stdout

from pydantic import BaseModel, Field

from crewai.tools import BaseTool


class SafePythonExecArgs(BaseModel):
    """Arguments for safe Python execution."""
    code: str = Field(..., description="Python code to execute.")
    data_json: Any | None = Field(
        None,
        description=(
            "Optional JSON string or object; available in code as `data`."
        ),
    )


class SafePythonExecTool(BaseTool):
    """CrewAI tool for executing restricted, trusted Python code only.

    This is a best-effort sandbox intended for agent-generated code in a
    controlled environment. It is not safe for untrusted or adversarial input.
    """
    name: str = "safe_python_exec"
    description: str = (
        "Executes Python code in a restricted environment and returns a status JSON. "
        "Provide code that prints the final JSON output. Accepts JSON string or object."
    )
    args_schema: type[BaseModel] = SafePythonExecArgs

    def _run(self, code: str, data_json: Any | None = None) -> str:
        """Execute code in a restricted environment and return JSON."""
        code_to_run = textwrap.dedent(code or "").replace("\t", "    ").strip()
        code_to_run = _normalize_indentation(code_to_run)
        if not code_to_run:
            return json.dumps(
                {"status": "CODE_ERROR", "error": "code is empty", "code": code_to_run},
                ensure_ascii=True,
            )
        try:
            import numpy as np
            import pandas as pd
        except ImportError as exc:
            return json.dumps(
                {
                    "status": "CODE_ERROR",
                    "error": f"module import failed: {exc}",
                    "code": code,
                },
                ensure_ascii=True,
            )

        def _deny_exit(*_args, **_kwargs):
            raise RuntimeError("exit is not allowed; return limitations instead")

        data_payload = None
        if data_json is not None:
            try:
                data_payload = _parse_json_payload(data_json)
            except ValueError as exc:
                return json.dumps(
                    {
                        "status": "CODE_ERROR",
                        "error": f"data_json invalid: {exc}",
                        "code": code_to_run,
                    },
                    ensure_ascii=True,
                )

        try:
            payload_json = json.dumps(
                {"code": code_to_run, "data": data_payload},
                ensure_ascii=True,
            )
        except TypeError:
            payload_json = json.dumps(
                {"code": code_to_run, "data": str(data_payload)},
                ensure_ascii=True,
            )

        try:
            completed = subprocess.run(
                [sys.executable, "-c", _EXEC_RUNNER],
                input=payload_json,
                text=True,
                capture_output=True,
                timeout=EXEC_TIMEOUT_SECONDS,
            )
        except subprocess.TimeoutExpired:
            return json.dumps(
                {
                    "status": "TIMEOUT",
                    "error": f"execution timed out after {EXEC_TIMEOUT_SECONDS}s",
                    "code": code_to_run,
                },
                ensure_ascii=True,
            )

        stdout_text = (completed.stdout or "").strip()
        if not stdout_text:
            return json.dumps(
                {
                    "status": "CODE_ERROR",
                    "error": "execution failed: no output from sandbox",
                    "traceback": completed.stderr,
                    "code": code_to_run,
                },
                ensure_ascii=True,
            )
        try:
            # Runner outputs a single JSON payload to stdout.
            json.loads(stdout_text)
            return stdout_text
        except json.JSONDecodeError:
            return json.dumps(
                {
                    "status": "CODE_ERROR",
                    "error": "execution failed: invalid sandbox output",
                    "traceback": completed.stderr,
                    "code": code_to_run,
                },
                ensure_ascii=True,
            )


def _parse_json_payload(value: object):
    """Parse a JSON payload from string, dict, or list inputs."""
    if isinstance(value, (dict, list)):
        return value
    if not isinstance(value, str):
        raise ValueError("data_json must be a JSON string")

    text = value.strip()

    try:
        parsed = json.loads(text)
        if isinstance(parsed, str):
            return _parse_json_payload(parsed)
        return _normalize_parsed_payload(parsed)
    except json.JSONDecodeError:
        # Fall back to raw decode / literal_eval strategies below.
        pass

    decoder = json.JSONDecoder()
    for start in (text.find("{"), text.find("[")):
        if start == -1:
            continue
        try:
            parsed, _ = decoder.raw_decode(text[start:])
            return _normalize_parsed_payload(parsed)
        except json.JSONDecodeError:
            continue

    try:
        parsed = ast.literal_eval(text)
        return _normalize_parsed_payload(parsed)
    except (ValueError, SyntaxError) as exc:
        raise ValueError(str(exc)) from exc


def _normalize_parsed_payload(payload: Any):
    """Normalize parsed JSON payloads and nested data values."""
    if isinstance(payload, str):
        return payload

    if isinstance(payload, list):
        normalized = []
        for item in payload:
            if isinstance(item, str):
                text = item.strip()
                if text.startswith("{") or text.startswith("["):
                    try:
                        normalized.append(json.loads(text))
                        continue
                    except json.JSONDecodeError:
                        # If not valid JSON, try literal_eval or keep as-is.
                        pass
                    try:
                        normalized.append(ast.literal_eval(text))
                        continue
                    except (ValueError, SyntaxError):
                        # Fall back to original item if not parseable.
                        pass
            normalized.append(item)
        return normalized

    if isinstance(payload, dict) and "data" in payload:
        data_value = payload["data"]
        if isinstance(data_value, str):
            payload["data"] = _parse_json_payload(data_value)
        elif isinstance(data_value, list):
            payload["data"] = _normalize_parsed_payload(data_value)
    return payload


def _normalize_indentation(code: str) -> str:
    """Normalize indentation to avoid syntax errors in code."""
    if not code:
        return code

    lines = code.splitlines()
    normalized: list[str] = []
    prev_indent = 0
    prev_line = ""

    for line in lines:
        stripped = line.lstrip()
        if not stripped:
            normalized.append("")
            continue

        indent = len(line) - len(stripped)
        first_token = stripped.split()[0] if stripped.split() else ""

        if first_token in {"elif", "else", "except", "finally"}:
            indent = max(prev_indent - 4, 0)
        elif prev_line.endswith(":"):
            indent = prev_indent + 4
        else:
            if prev_indent == 0 and indent > 0:
                indent = 0
            elif indent > prev_indent:
                indent = prev_indent

        normalized.append(" " * indent + stripped)
        prev_indent = indent
        prev_line = stripped

    return "\n".join(normalized)


EXEC_TIMEOUT_SECONDS = 60


_EXEC_RUNNER = r"""
import json
import math
import statistics
import datetime as datetime_module
import time
import traceback
import io
import sys
from contextlib import redirect_stdout

try:
    import numpy as np
    import pandas as pd
except Exception as exc:
    print(
        json.dumps(
            {
                "status": "CODE_ERROR",
                "error": f"module import failed: {exc}",
                "code": "",
            },
            ensure_ascii=True,
        )
    )
    raise SystemExit(1)


def _deny_exit(*_args, **_kwargs):
    raise RuntimeError("exit is not allowed; return limitations instead")


safe_builtins = {
    "abs": abs,
    "all": all,
    "any": any,
    "bool": bool,
    "dict": dict,
    "enumerate": enumerate,
    "Exception": Exception,
    "exit": _deny_exit,
    "float": float,
    "int": int,
    "isinstance": isinstance,
    "len": len,
    "list": list,
    "max": max,
    "min": min,
    "NameError": NameError,
    "print": print,
    "range": range,
    "round": round,
    "set": set,
    "sorted": sorted,
    "str": str,
    "sum": sum,
    # NOTE: Including `type` enables basic introspection. This sandbox is
    # only for trusted, agent-generated code and is not a security
    # boundary for untrusted input.
    "type": type,
    "zip": zip,
}

allowed_modules = {
    "math": math,
    "statistics": statistics,
    "datetime": datetime_module,
    "json": json,
    "time": time,
    "numpy": np,
    "pandas": pd,
}


def _limited_import(name, globals=None, locals=None, fromlist=(), level=0):
    if name in allowed_modules:
        return allowed_modules[name]
    raise ImportError(f"Module not allowed: {name}")


safe_builtins["__import__"] = _limited_import

payload = json.load(sys.stdin)
code_to_run = payload.get("code") or ""
data = payload.get("data")

context = {"__builtins__": safe_builtins}
context.update(allowed_modules)
context["np"] = np
context["pd"] = pd
context["data"] = data

stdout = io.StringIO()
try:
    with redirect_stdout(stdout):
        exec(code_to_run, context)
    output = stdout.getvalue().strip()
    print(
        json.dumps(
            {"status": "SUCCESS", "final_output": output, "code": code_to_run},
            ensure_ascii=True,
        )
    )
except Exception as exc:
    print(
        json.dumps(
            {
                "status": "CODE_ERROR",
                "error": f"execution failed: {exc}",
                "traceback": traceback.format_exc(),
                "code": code_to_run,
            },
            ensure_ascii=True,
        )
    )
    raise SystemExit(1)
"""
