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

"""amp_shadow_runner.engine.modules: which imports name a module of the repository."""

import pytest

from amp_shadow_runner.engine.imports import extract_imports
from amp_shadow_runner.engine.languages import Language, parse
from amp_shadow_runner.engine.modules import RepoModules

MODULES = RepoModules(
    [
        "main.py",
        "tools.py",
        "app/main.py",
        "app/agents.py",
        "app/graph/__init__.py",
        "app/graph/build.py",
        "app/graph/nodes.py",
        "app/graph/tools/__init__.py",
        "app/graph/tools/search.py",
        "src/myapp/__init__.py",
        "src/myapp/graph.py",
        "src/myapp/tools.py",
        "pkg/__init__.py",
        "pkg/main.py",
        "pkg/agents.py",
        "ns/agents/impl.py",
        "ns/main.py",
        "backend/agents.py",
        "backend/app/graph.py",
        "backend/app/nodes.py",
        "backend/app/agents.py",
        "src/scripts/run.py",
    ]
)


@pytest.mark.parametrize(
    ("importer", "module", "level", "expected"),
    [
        # Relative imports, from the importer's folder, one folder up for each extra dot.
        ("app/graph/build.py", "nodes", 1, "app/graph/nodes.py"),
        ("app/graph/build.py", "tools", 1, "app/graph/tools/__init__.py"),
        ("app/graph/build.py", "tools.search", 1, "app/graph/tools/search.py"),
        ("app/graph/build.py", "", 1, "app/graph/__init__.py"),
        ("app/graph/build.py", "agents", 2, "app/agents.py"),
        ("app/graph/build.py", "missing", 1, None),
        ("main.py", "x", 3, None),
        # Absolute imports, from the package root, then the repository root.
        ("src/myapp/graph.py", "myapp.tools", 0, "src/myapp/tools.py"),
        ("app/main.py", "agents", 0, "app/agents.py"),
        ("app/graph/build.py", "app.graph.nodes", 0, "app/graph/nodes.py"),
        ("main.py", "tools", 0, "tools.py"),
        ("app/main.py", "tools", 0, "tools.py"),
        # Every folder from the package root up to the repository root, nearest first.
        ("backend/app/graph.py", "app.nodes", 0, "backend/app/nodes.py"),
        ("src/scripts/run.py", "myapp", 0, "src/myapp/__init__.py"),
        ("src/scripts/run.py", "myapp.tools", 0, "src/myapp/tools.py"),
        # The importing file never names itself; the lookup goes on.
        ("app/agents.py", "agents", 0, None),
        ("backend/app/agents.py", "agents", 0, "backend/agents.py"),
        # A namespace folder is not a top-level module, but its files are submodules.
        ("ns/main.py", "agents.impl", 0, "ns/agents/impl.py"),
        # A package's own sibling is not reachable as a top-level module.
        ("pkg/main.py", "agents", 0, None),
        # A namespace package (no __init__.py) holds no module file.
        ("ns/main.py", "agents", 0, None),
        ("main.py", "", 0, None),
    ],
)
def test_file(importer: str, module: str, level: int, expected: str | None) -> None:
    assert MODULES.file(importer, module, level) == expected


def test_is_local() -> None:
    assert MODULES.is_local("app/main.py", "agents")
    assert not MODULES.is_local("ns/main.py", "agents")
    # A package at the repository root is reachable from every file.
    root_package = RepoModules(["agents/__init__.py", "main.py", "pkg/__init__.py", "pkg/main.py"])
    assert root_package.file("main.py", "agents") == "agents/__init__.py"
    assert root_package.is_local("pkg/main.py", "agents")


def test_imported() -> None:
    source = (
        b"from . import nodes\n"
        b"from .tools import search\n"
        b"from .tools import helper\n"
        b"import app.agents\n"
        b"from app.graph import build\n"
        b"from langgraph.graph import StateGraph\n"
        b"from typing import TYPE_CHECKING\n"
        b"if TYPE_CHECKING:\n    from .missing import X\n    import tools\n"
    )
    tree = parse(Language.PYTHON, source)
    assert tree is not None
    imports = extract_imports(Language.PYTHON, tree, source)
    # ``from app.graph import build`` in build.py itself names the package, not the file.
    assert MODULES.imported("app/graph/build.py", imports) == (
        "app/agents.py",
        "app/graph/__init__.py",
        "app/graph/nodes.py",
        "app/graph/tools/__init__.py",
        "app/graph/tools/search.py",
    )
