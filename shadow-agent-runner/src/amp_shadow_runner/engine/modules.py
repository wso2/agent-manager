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

"""Tell which Python imports name a module of the repository itself, and which file holds it.

Only the repository's file paths are used; nothing is read. The resolution is deliberately simple:

- A relative import (``from .tools import x``, ``from .. import nodes``) is looked up from the importing
  file's folder, one folder up for each dot after the first.
- An absolute import (``from myapp.tools import x``) is looked up from the file's package root, then from
  each folder above it up to the repository root, nearest first. The package root is the first folder
  above the file that has no ``__init__.py``: the folder of ``main.py`` for a script, ``src`` for
  ``src/myapp/graph.py`` when ``src/myapp`` is a package. So ``backend/app/graph.py`` finds ``app.nodes``
  at ``backend/app/nodes.py``, and ``src/scripts/run.py`` finds ``myapp`` at ``src/myapp/__init__.py``.
- ``a.b`` is the file ``a/b.py`` or ``a/b/__init__.py`` under that folder. A top-level module ``a`` is
  only ``a.py`` or ``a/__init__.py``: a folder ``a`` with no ``__init__.py`` (a namespace package) is never
  module ``a`` itself, since an installed package of the same name would win over it in Python. The files
  in such a folder still count as its submodules (``a/b.py`` is ``a.b``).
- ``from a import b`` names ``a/b.py`` (or ``a/b/__init__.py``) when that exists, since ``b`` may be a
  submodule, and otherwise ``a`` itself.
- The importing file is never the module it imports: ``app/agents.py`` importing ``agents`` does not name
  itself, and the lookup goes on to the next folder.
"""

from collections.abc import Iterable
from pathlib import PurePosixPath

from amp_shadow_runner.engine.imports import ImportKind, ImportTable

_ROOT = PurePosixPath(".")


class RepoModules:
    """The Python files of one repository, by path from its root."""

    def __init__(self, paths: Iterable[str]) -> None:
        self._paths = frozenset(paths)

    def file(self, importer: str, module: str, level: int = 0) -> str | None:
        """The repository path of ``module`` as imported by the file at ``importer``, or None when it is not a
        module of the repository. ``level`` is the number of leading dots of a relative import (``module``
        is then the part after them, ``""`` for ``from . import x``)."""
        parts = [p for p in module.split(".") if p]
        if level:
            folder = PurePosixPath(importer).parent
            for _ in range(level - 1):
                if folder == _ROOT:
                    return None
                folder = folder.parent
            folders: tuple[PurePosixPath, ...] = (folder,)
        else:
            if not parts:
                return None
            folders = self._roots(importer)
        for folder in folders:
            candidates = [folder.joinpath(*parts, "__init__.py")]
            if parts:
                candidates.insert(0, folder.joinpath(*parts[:-1], f"{parts[-1]}.py"))
            for candidate in candidates:
                if str(candidate) in self._paths and str(candidate) != importer:
                    return str(candidate)
        return None

    def is_local(self, importer: str, module: str) -> bool:
        """Whether the absolute ``module``, imported by the file at ``importer``, is a module of the repository."""
        return self.file(importer, module) is not None

    def imported(self, importer: str, imports: ImportTable) -> tuple[str, ...]:
        """The repository files the file at ``importer`` imports (one hop), sorted."""
        found: set[str] = set()
        for imp in imports.imports:
            if imp.type_only:
                continue
            path = None
            if imp.kind == ImportKind.MEMBER and imp.name is not None:
                submodule = f"{imp.module}.{imp.name}" if imp.module else imp.name
                path = self.file(importer, submodule, imp.level)
            if path is None and imp.kind in (
                ImportKind.MEMBER,
                ImportKind.MODULE,
                ImportKind.ALIAS,
                ImportKind.WILDCARD,
            ):
                path = self.file(importer, imp.module, imp.level)
            if path is not None:
                found.add(path)
        return tuple(sorted(found))

    def _roots(self, importer: str) -> tuple[PurePosixPath, ...]:
        """The folders an absolute import of the file at ``importer`` is looked up from (see the module docstring)."""
        folder = PurePosixPath(importer).parent
        while folder != _ROOT and str(folder / "__init__.py") in self._paths:
            folder = folder.parent
        return (folder, *folder.parents)
