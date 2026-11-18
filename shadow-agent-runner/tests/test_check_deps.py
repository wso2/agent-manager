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

"""scripts/check_deps.py: the licence allowlist, the denylist and the notice file."""

from email.message import Message
from pathlib import Path

import check_deps
import pytest
from check_deps import PackageInfo

ROOT = Path(__file__).resolve().parent.parent
FIXTURES = ROOT / "tests" / "fixtures" / "check_deps"
EMPTY_EXCEPTIONS = "# no exceptions\n"


def _meta(**fields: str | list[str]) -> Message:
    message = Message()
    for key, value in fields.items():
        for item in value if isinstance(value, list) else [value]:
            message[key.replace("_", "-")] = item
    return message


def _fake_lookup(table: dict[str, tuple[str, str]]) -> check_deps.Lookup:
    def lookup(name: str) -> PackageInfo | None:
        if name not in table:
            return None
        version, licence = table[name]
        return PackageInfo(name=name, version=version, license=licence, url=f"https://example.test/{name}")

    return lookup


@pytest.fixture
def exceptions_file(tmp_path: Path) -> Path:
    path = tmp_path / "exceptions.txt"
    path.write_text(EMPTY_EXCEPTIONS, encoding="utf-8")
    return path


def test_real_lock_passes_and_notice_is_current() -> None:
    """The committed lock, installed in this environment, passes the gate."""
    assert check_deps.main([]) == 0


def test_denylisted_package_fails_by_name(
    exceptions_file: Path, tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    # Even a reviewed exception cannot let a denylisted package in.
    exceptions_file.write_text("tree-sitter-language-pack==0.9.0\n", encoding="utf-8")
    lock = FIXTURES / "denied-language-pack.txt"
    status = check_deps.main(
        ["--lock", str(lock), "--exceptions", str(exceptions_file), "--notice", str(tmp_path / "N.md")]
    )
    assert status == 1
    assert "DENIED: tree-sitter-language-pack==0.9.0 is on the denylist" in capsys.readouterr().err


def test_psf_licensed_package_fails(exceptions_file: Path, tmp_path: Path) -> None:
    lookup = _fake_lookup({"tree-sitter": ("0.26.0", "MIT"), "typing-extensions": ("4.16.0", "PSF-2.0")})
    status, messages = check_deps.run(
        FIXTURES / "psf-licensed.txt", exceptions_file, tmp_path / "N.md", write=True, lookup=lookup
    )
    assert status == 1
    assert any(m.startswith("LICENCE: typing-extensions==4.16.0 is licensed 'PSF-2.0'") for m in messages)


def test_reviewed_exception_admits_package(exceptions_file: Path, tmp_path: Path) -> None:
    exceptions_file.write_text("typing_extensions==4.16.0  # PSF-2.0, reviewed\n", encoding="utf-8")
    lookup = _fake_lookup({"tree-sitter": ("0.26.0", "MIT"), "typing-extensions": ("4.16.0", "PSF-2.0")})
    notice = tmp_path / "N.md"
    status, messages = check_deps.run(FIXTURES / "psf-licensed.txt", exceptions_file, notice, write=True, lookup=lookup)
    assert (status, messages[0][:3]) == (0, "OK:")
    assert "| typing-extensions | 4.16.0 | PSF-2.0 |" in notice.read_text(encoding="utf-8")


def test_copyleft_cannot_be_excepted(exceptions_file: Path, tmp_path: Path) -> None:
    exceptions_file.write_text("typing-extensions==4.16.0\n", encoding="utf-8")
    lookup = _fake_lookup({"tree-sitter": ("0.26.0", "MIT"), "typing-extensions": ("4.16.0", "GPL-3.0-only")})
    status, messages = check_deps.run(FIXTURES / "psf-licensed.txt", exceptions_file, tmp_path / "N.md", True, lookup)
    assert status == 1
    assert any("copyleft licences cannot be excepted" in m for m in messages)


def test_unneeded_exception_fails(exceptions_file: Path, tmp_path: Path) -> None:
    exceptions_file.write_text("tree-sitter==0.26.0\n", encoding="utf-8")
    lookup = _fake_lookup({"tree-sitter": ("0.26.0", "MIT"), "typing-extensions": ("4.16.0", "MIT")})
    status, messages = check_deps.run(FIXTURES / "psf-licensed.txt", exceptions_file, tmp_path / "N.md", True, lookup)
    assert status == 1
    assert any(m.startswith("STALE EXCEPTION: tree-sitter==0.26.0") for m in messages)


def test_missing_and_mismatched_packages_fail(exceptions_file: Path, tmp_path: Path) -> None:
    lookup = _fake_lookup({"tree-sitter": ("0.25.0", "MIT")})
    status, messages = check_deps.run(FIXTURES / "psf-licensed.txt", exceptions_file, tmp_path / "N.md", True, lookup)
    assert status == 1
    assert any(m.startswith("MISMATCH: tree-sitter is locked at 0.26.0") for m in messages)
    assert any(m.startswith("MISSING: typing-extensions==4.16.0") for m in messages)


def test_stale_notice_fails(exceptions_file: Path, tmp_path: Path) -> None:
    lookup = _fake_lookup({"tree-sitter": ("0.26.0", "MIT"), "typing-extensions": ("4.16.0", "Apache-2.0")})
    notice = tmp_path / "THIRD_PARTY_LICENSES.md"
    notice.write_text("hand-edited\n", encoding="utf-8")
    status, messages = check_deps.run(FIXTURES / "psf-licensed.txt", exceptions_file, notice, False, lookup)
    assert status == 1
    assert messages == ["STALE NOTICE: THIRD_PARTY_LICENSES.md is stale; run `make licenses` and commit the result"]


def test_malformed_exception_is_an_input_error(exceptions_file: Path, tmp_path: Path) -> None:
    exceptions_file.write_text("typing-extensions\n", encoding="utf-8")
    status, messages = check_deps.run(FIXTURES / "psf-licensed.txt", exceptions_file, tmp_path / "N.md", False)
    assert status == 2
    assert "expected 'name==version'" in messages[0]


@pytest.mark.parametrize(
    ("expression", "allowed"),
    [
        ("MIT", True),
        ("Apache-2.0", True),
        ("MIT OR GPL-3.0-only", True),
        ("MIT AND Apache-2.0", True),
        ("MIT AND BSD-3-Clause", False),
        ("PSF-2.0", False),
        ("Apache-2.0 WITH LLVM-exception", False),
        ("(MIT OR Apache-2.0)", False),
        ("UNKNOWN", False),
    ],
)
def test_license_allowed(expression: str, allowed: bool) -> None:
    assert check_deps.license_allowed(expression) is allowed


@pytest.mark.parametrize(
    ("meta", "expected"),
    [
        (_meta(License_Expression="Apache-2.0", Classifier="License :: OSI Approved :: MIT License"), "Apache-2.0"),
        (_meta(Classifier="License :: OSI Approved :: MIT License"), "MIT"),
        (
            _meta(
                Classifier=[
                    "License :: OSI Approved :: MIT License",
                    "License :: OSI Approved :: Apache Software License",
                ]
            ),
            "Apache-2.0 AND MIT",
        ),
        (_meta(Classifier="License :: OSI Approved :: Python Software Foundation License"), "PSF-2.0"),
        (_meta(License="MIT License"), "MIT"),
        (_meta(License="Line one\nline two of a full licence text"), "UNKNOWN"),
        (_meta(), "UNKNOWN"),
    ],
)
def test_detect_license(meta: Message, expected: str) -> None:
    assert check_deps.detect_license(meta) == expected


def test_project_url_prefers_source() -> None:
    meta = _meta(Project_URL=["Homepage, https://home.test", "Source, https://src.test"])
    assert check_deps.project_url("x", meta) == "https://src.test"
    assert check_deps.project_url("x", _meta()) == "https://pypi.org/project/x/"
