import ast
import datetime as datetime_module
import io
import json
import math
import statistics
import time
import traceback
import textwrap
from json import JSONDecodeError, JSONDecoder
from typing import Any
from contextlib import redirect_stdout

from pydantic import BaseModel, Field

from crewai.tools import BaseTool


class SafePythonExecArgs(BaseModel):
    code: str = Field(..., description="Python code to execute.")
    data_json: Any | None = Field(
        None,
        description=(
            "Optional JSON string or object; available in code as `data`."
        ),
    )


class SafePythonExecTool(BaseTool):
    name: str = "safe_python_exec"
    description: str = (
        "Executes Python code in a restricted environment and returns a status JSON. "
        "Provide code that prints the final JSON output. Accepts JSON string or object."
    )
    args_schema: type[BaseModel] = SafePythonExecArgs

    def _run(self, code: str, data_json: Any | None = None) -> str:
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

        context = {"__builtins__": safe_builtins}
        context.update(allowed_modules)
        context["np"] = np
        context["pd"] = pd

        if data_json:
            try:
                context["data"] = _parse_json_payload(data_json)
            except ValueError as exc:
                return json.dumps(
                    {
                        "status": "CODE_ERROR",
                        "error": f"data_json invalid: {exc}",
                        "code": code_to_run,
                    },
                    ensure_ascii=True,
                )
        else:
            context["data"] = None

        stdout = io.StringIO()
        try:
            with redirect_stdout(stdout):
                exec(code_to_run, context)
        except Exception as exc:
            return json.dumps(
                {
                    "status": "CODE_ERROR",
                    "error": f"execution failed: {exc}",
                    "traceback": traceback.format_exc(),
                    "code": code_to_run,
                },
                ensure_ascii=True,
            )

        output = stdout.getvalue().strip()
        return json.dumps(
            {"status": "SUCCESS", "final_output": output, "code": code_to_run},
            ensure_ascii=True,
        )


def _parse_json_payload(value: object):
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
    except JSONDecodeError:
        pass

    decoder = JSONDecoder()
    for start in (text.find("{"), text.find("[")):
        if start == -1:
            continue
        try:
            parsed, _ = decoder.raw_decode(text[start:])
            return _normalize_parsed_payload(parsed)
        except JSONDecodeError:
            continue

    try:
        parsed = ast.literal_eval(text)
        return _normalize_parsed_payload(parsed)
    except (ValueError, SyntaxError) as exc:
        raise ValueError(str(exc)) from exc


def _normalize_parsed_payload(payload: Any):
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
                    except JSONDecodeError:
                        pass
                    try:
                        normalized.append(ast.literal_eval(text))
                        continue
                    except (ValueError, SyntaxError):
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
