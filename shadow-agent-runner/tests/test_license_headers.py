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

"""Every source file starts with the WSO2 Apache-2.0 licence header (ruff has no stable rule for it)."""

import re
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parent.parent
SKIP_DIRS = {".venv", "venv", "build", "dist", ".mypy_cache", ".pytest_cache", ".ruff_cache", "__pycache__"}

FIRST_LINE = re.compile(r"^# Copyright \(c\) \d{4}, WSO2 LLC\. \(https://www\.wso2\.com\)\.$")
REST = """#
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
# under the License.""".splitlines()


def has_wso2_header(text: str) -> bool:
    """True when the text starts (after an optional shebang) with the WSO2 Apache-2.0 header."""
    lines = text.splitlines()
    if lines and lines[0].startswith("#!"):
        lines = lines[1:]
    return len(lines) > len(REST) and bool(FIRST_LINE.match(lines[0])) and lines[1 : 1 + len(REST)] == REST


def source_files() -> list[Path]:
    files = [p for p in ROOT.rglob("*.py") if not SKIP_DIRS.intersection(p.relative_to(ROOT).parts)]
    files.append(ROOT / "Dockerfile")
    return sorted(files)


def test_source_files_are_found() -> None:
    names = {p.name for p in source_files()}
    assert {"cli.py", "check_deps.py", "test_license_headers.py", "Dockerfile"} <= names


@pytest.mark.parametrize("path", source_files(), ids=lambda p: str(p.relative_to(ROOT)))
def test_every_python_file_has_wso2_header(path: Path) -> None:
    assert has_wso2_header(path.read_text(encoding="utf-8")), f"{path} is missing the WSO2 licence header"


def test_header_check_rejects_missing_or_altered_header() -> None:
    header = (ROOT / "src" / "amp_shadow_runner" / "__init__.py").read_text(encoding="utf-8")
    assert has_wso2_header(header)
    assert has_wso2_header("#!/usr/bin/env python3\n" + header)
    assert not has_wso2_header('"""No header."""\n')
    assert not has_wso2_header(header.replace("Apache License,", "MIT License,"))
