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

"""scripts/check_lock.py: the hermetic lock consistency check."""

from pathlib import Path

import check_lock
import pytest

HASH = "    --hash=sha256:" + "0" * 64
PYPROJECT = '[project]\nname = "x"\ndependencies = ["alpha==1.0", "beta==2.0"]\n'
REQUIREMENTS = "# comment\nalpha==1.0\nbeta==2.0\n"
LOCK = f"""# header
alpha==1.0 \\
{HASH}
    # via -r requirements.in
beta==2.0 \\
{HASH}
    # via
    #   -r requirements.in
    #   alpha
gamma==3.0 ; sys_platform == 'linux' \\
{HASH}
    # via beta
"""


def _run(
    tmp_path: Path, requirements: str = REQUIREMENTS, lock: str = LOCK, pyproject: str = PYPROJECT
) -> tuple[int, list[str]]:
    paths = {"requirements.in": requirements, "lock.txt": lock, "pyproject.toml": pyproject}
    for name, text in paths.items():
        (tmp_path / name).write_text(text, encoding="utf-8")
    return check_lock.run(tmp_path / "requirements.in", tmp_path / "lock.txt", tmp_path / "pyproject.toml")


def test_committed_lock_passes() -> None:
    assert check_lock.main([]) == 0


def test_consistent_inputs_pass(tmp_path: Path) -> None:
    status, messages = _run(tmp_path)
    assert status == 0, messages


def test_version_mismatch_fails(tmp_path: Path) -> None:
    status, messages = _run(tmp_path, requirements="alpha==1.1\nbeta==2.0\n", pyproject=PYPROJECT.replace("1.0", "1.1"))
    assert status == 1
    assert "MISMATCH: alpha is 1.1 in requirements.in but 1.0 in the lock" in messages


def test_new_requirement_missing_from_lock_fails(tmp_path: Path) -> None:
    pyproject = PYPROJECT.replace('"beta==2.0"', '"beta==2.0", "delta==4.0"')
    status, messages = _run(tmp_path, requirements=REQUIREMENTS + "delta==4.0\n", pyproject=pyproject)
    assert status == 1
    assert any(m.startswith("MISSING: delta==4.0") for m in messages)


def test_removed_requirement_leaves_orphan(tmp_path: Path) -> None:
    status, messages = _run(tmp_path, requirements="alpha==1.0\n", pyproject=PYPROJECT.replace(', "beta==2.0"', ""))
    assert status == 1
    assert any(m.startswith("ORPHAN: beta") for m in messages)


def test_pyproject_drift_fails(tmp_path: Path) -> None:
    status, messages = _run(tmp_path, pyproject=PYPROJECT.replace("beta==2.0", "beta==2.1"))
    assert status == 1
    assert "DRIFT: beta is 2.0 in requirements.in but 2.1 in pyproject.toml" in messages


def test_missing_hash_fails(tmp_path: Path) -> None:
    status, messages = _run(tmp_path, lock=LOCK.replace(f"alpha==1.0 \\\n{HASH}\n", "alpha==1.0\n"))
    assert status == 1
    assert any(m.startswith("NO HASH: alpha==1.0") for m in messages)


@pytest.mark.parametrize("line", ["alpha>=1.0", "alpha", "-e ."])
def test_unpinned_requirement_is_an_input_error(tmp_path: Path, line: str) -> None:
    status, messages = _run(tmp_path, requirements=f"{line}\nbeta==2.0\n")
    assert status == 2
    assert "is not an exact 'name==version' pin" in messages[0]
