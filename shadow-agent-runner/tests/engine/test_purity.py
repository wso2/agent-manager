# Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.

"""The engine is pure: no network, no subprocesses.

The static check scans every module under ``amp_shadow_runner/engine`` for imports of networking or
process modules. The runtime checks import every engine module, and walk and parse a small tree, with
sockets and process creation patched to raise. When ``scan_tree`` lands, it gets the same runtime check.
"""

import ast
import importlib
import pkgutil
import socket
import subprocess
from pathlib import Path
from typing import NoReturn

import pytest

import amp_shadow_runner.engine as engine
from amp_shadow_runner.engine.files import FileKind, walk
from amp_shadow_runner.engine.languages import parse

ENGINE_DIR = Path(engine.__file__).resolve().parent

BANNED_MODULES = frozenset(
    {
        "aiohttp",
        "asyncio",
        "ftplib",
        "http",
        "httpx",
        "imaplib",
        "poplib",
        "requests",
        "smtplib",
        "socket",
        "ssl",
        "subprocess",
        "telnetlib",
        "urllib",
        "urllib3",
        "websocket",
        "websockets",
        "xmlrpc",
    }
)
BANNED_OS_CALLS = frozenset({"system", "popen", "fork", "forkpty"})
BANNED_OS_PREFIXES = ("exec", "spawn", "posix_spawn")


def _banned(module: str) -> bool:
    return module.split(".", 1)[0] in BANNED_MODULES


def find_impure_calls(source: str, filename: str = "<string>") -> list[str]:
    """Return a description of every banned import or call in the source."""
    found: list[str] = []
    for node in ast.walk(ast.parse(source, filename=filename)):
        if isinstance(node, ast.Import):
            found += [f"import {alias.name}" for alias in node.names if _banned(alias.name)]
        elif isinstance(node, ast.ImportFrom) and node.level == 0 and node.module and _banned(node.module):
            found.append(f"from {node.module} import ...")
        elif isinstance(node, ast.Call):
            func = node.func
            dynamic = (isinstance(func, ast.Name) and func.id == "__import__") or (
                isinstance(func, ast.Attribute) and func.attr == "import_module"
            )
            if dynamic and node.args and isinstance(node.args[0], ast.Constant):
                target = node.args[0].value
                if isinstance(target, str) and _banned(target):
                    found.append(f"dynamic import of {target}")
            if (
                isinstance(func, ast.Attribute)
                and isinstance(func.value, ast.Name)
                and func.value.id == "os"
                and (func.attr in BANNED_OS_CALLS or func.attr.startswith(BANNED_OS_PREFIXES))
            ):
                found.append(f"os.{func.attr}()")
    return found


def engine_files() -> list[Path]:
    return sorted(ENGINE_DIR.rglob("*.py"))


def test_engine_files_are_found() -> None:
    assert ENGINE_DIR / "__init__.py" in engine_files()


@pytest.mark.parametrize("path", engine_files(), ids=lambda p: str(p.relative_to(ENGINE_DIR)))
def test_engine_has_no_network_imports(path: Path) -> None:
    assert find_impure_calls(path.read_text(encoding="utf-8"), str(path)) == []


@pytest.mark.parametrize(
    "source",
    [
        "import socket",
        "import http.client as client",
        "from urllib.request import urlopen",
        "from requests import Session",
        "import asyncio, json",
        "import subprocess",
        "__import__('ssl')",
        "importlib.import_module('httpx')",
        "os.system('ls')",
        "os.execv('/bin/ls', [])",
    ],
)
def test_purity_check_catches_banned_code(source: str) -> None:
    assert find_impure_calls(source) != []


def test_purity_check_allows_pure_code() -> None:
    assert (
        find_impure_calls("import json\nimport tomllib\nfrom pathlib import Path\nfrom . import socket_utils\n") == []
    )


def _refuse(*_args: object, **_kwargs: object) -> NoReturn:
    raise AssertionError("the engine tried to use the network or start a process")


@pytest.fixture
def no_network(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(socket, "socket", _refuse)
    monkeypatch.setattr(socket, "create_connection", _refuse)
    monkeypatch.setattr(socket, "getaddrinfo", _refuse)
    monkeypatch.setattr(subprocess, "Popen", _refuse)


@pytest.mark.usefixtures("no_network")
def test_engine_import_never_touches_network() -> None:
    modules = [engine.__name__] + [m.name for m in pkgutil.walk_packages(engine.__path__, f"{engine.__name__}.")]
    for name in modules:
        importlib.reload(importlib.import_module(name))


@pytest.mark.usefixtures("no_network")
def test_walk_and_parse_never_touch_network(tmp_path: Path) -> None:
    (tmp_path / "pkg").mkdir()
    (tmp_path / "pkg" / "agent.py").write_bytes(b"import openai\n")
    (tmp_path / "package.json").write_bytes(b"{}\n")
    (tmp_path / "node_modules").mkdir()
    result = walk(tmp_path, read_ci=True)
    assert [(f.path, f.kind) for f in result.files] == [
        ("package.json", FileKind.MANIFEST),
        ("pkg/agent.py", FileKind.SOURCE),
    ]
    for f in result.files:
        if f.language is not None:
            assert parse(f.language, f.absolute.read_bytes()) is not None
