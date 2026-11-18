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

"""amp_shadow_runner.engine.files: which files a scan reads, what is pruned, skipped and tagged."""

import contextlib
import os
from collections.abc import Iterator
from pathlib import Path

import pytest

from amp_shadow_runner.engine.files import FileKind, RepoFile, SkipReason, WalkResult, walk
from amp_shadow_runner.engine.languages import Language


def _write(root: Path, *paths: str, data: bytes = b"x\n") -> None:
    for rel in paths:
        path = root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)


def _paths(result: WalkResult) -> list[str]:
    return [f.path for f in result.files]


def _by_path(result: WalkResult) -> dict[str, RepoFile]:
    return {f.path: f for f in result.files}


def test_keeps_source_files_with_their_language(tmp_path: Path) -> None:
    _write(tmp_path, "agent.py", "web/App.tsx", "web/index.jsx", "svc/Agent.cs", "bal/main.bal", "lib/x.mts")
    files = _by_path(walk(tmp_path))
    assert {p: (f.kind, f.language) for p, f in files.items()} == {
        "agent.py": (FileKind.SOURCE, Language.PYTHON),
        "web/App.tsx": (FileKind.SOURCE, Language.TSX),
        "web/index.jsx": (FileKind.SOURCE, Language.JAVASCRIPT),
        "svc/Agent.cs": (FileKind.SOURCE, Language.C_SHARP),
        "bal/main.bal": (FileKind.SOURCE, Language.BALLERINA),
        "lib/x.mts": (FileKind.SOURCE, Language.TYPESCRIPT),
    }
    assert files["web/App.tsx"].absolute == tmp_path / "web" / "App.tsx"


MANIFESTS = [
    "requirements.txt",
    "requirements-dev.txt",
    "requirements/base.txt",
    "pyproject.toml",
    "Pipfile",
    "setup.cfg",
    "web/package.json",
    "svc/Agent.csproj",
    "Directory.Packages.props",
    "svc/packages.config",
    "legacy/Directory.packages.props",
    "legacy/Packages.Config",
    "bal/Ballerina.toml",
]


def test_keeps_manifests_without_a_language(tmp_path: Path) -> None:
    _write(tmp_path, *MANIFESTS)
    files = walk(tmp_path).files
    assert sorted(f.path for f in files) == sorted(MANIFESTS)
    assert all(f.kind == FileKind.MANIFEST and f.language is None for f in files)


IGNORED = [
    "README.md",
    "logo.png",
    "notes.txt",
    # Lock files: no rule matches them.
    "package-lock.json",
    "yarn.lock",
    "pnpm-lock.yaml",
    "poetry.lock",
    "Pipfile.lock",
    "uv.lock",
    "svc/packages.lock.json",
    # Minified bundles.
    "static/app.min.js",
    "static/app.min.mjs",
    "static/app.min.cjs",
    "static/vendor.bundle.js",
]


def test_ignores_lock_minified_and_unrelated_files(tmp_path: Path) -> None:
    _write(tmp_path, *IGNORED, "static/app.js")
    result = walk(tmp_path, read_ci=True)
    assert _paths(result) == ["static/app.js"]
    assert result.skipped == 0


@pytest.mark.parametrize("read_ci", [False, True])
def test_never_keeps_env_files(tmp_path: Path, read_ci: bool) -> None:
    _write(tmp_path, ".env", ".env.local", ".ENV.production", "prod.env", "config/.env.py", "config/settings.py")
    assert _paths(walk(tmp_path, read_ci=read_ci)) == ["config/settings.py"]


PRUNED = [
    "node_modules",
    "bower_components",
    "vendor",
    "venv",
    "site-packages",
    "__pycache__",
    "dist",
    "build",
    "out",
    "target",
    "obj",
    "Obj",
    ".git",
    ".hg",
    ".svn",
    ".venv",
    ".tox",
    ".nox",
    ".mypy_cache",
    ".pytest_cache",
    ".next",
    ".nuxt",
    ".idea",
    ".vscode",
    ".yarn",
]


def test_prunes_dependency_build_cache_and_hidden_folders(tmp_path: Path) -> None:
    _write(tmp_path, "src/agent.py", *(f"{d}/pkg/index.js" for d in PRUNED), "src/node_modules/x/package.json")
    assert _paths(walk(tmp_path)) == ["src/agent.py"]


def test_bin_folders_are_walked(tmp_path: Path) -> None:
    _write(tmp_path, "bin/agent.mjs", "scripts/bin/run_agent.py")
    assert _paths(walk(tmp_path)) == ["bin/agent.mjs", "scripts/bin/run_agent.py"]


CI = [".github/workflows/build.yml", ".github/workflows/release.yaml", "azure-pipelines.yml"]
CI_ALSO = ["deploy/azure-pipelines-prod.yaml", ".azure-pipelines/ci.yml", ".azure-pipelines/templates/build.yml"]
NOT_CI = [".github/dependabot.yml", ".github/workflows/sub/x.yml", "docker-compose.yml", "src/.github/workflows/a.yml"]


def test_ci_files_only_with_read_ci(tmp_path: Path) -> None:
    _write(tmp_path, *CI, *CI_ALSO, *NOT_CI, ".github/scripts/label.py")
    assert _paths(walk(tmp_path)) == [".github/scripts/label.py"]
    files = _by_path(walk(tmp_path, read_ci=True))
    assert sorted(files) == sorted([*CI, *CI_ALSO, ".github/scripts/label.py"])
    assert all(files[p].kind == FileKind.CI and files[p].language is None for p in CI + CI_ALSO)


def test_size_cap_skips_and_counts(tmp_path: Path) -> None:
    _write(tmp_path, "small.py", data=b"x" * 10)
    _write(tmp_path, "exact.py", data=b"x" * 64)
    _write(tmp_path, "big.py", "package.json", data=b"x" * 65)
    _write(tmp_path, "big.md", data=b"x" * 65)  # Not a scanned file, so not counted.
    result = walk(tmp_path, max_file_bytes=64)
    assert _paths(result) == ["exact.py", "small.py"]
    assert result.skipped == 2
    assert result.skipped_by_reason == {SkipReason.TOO_LARGE: 2}


def test_default_size_cap_is_one_mib(tmp_path: Path) -> None:
    _write(tmp_path, "ok.py", data=b"x" * 1024 * 1024)
    _write(tmp_path, "big.py", data=b"x" * (1024 * 1024 + 1))
    result = walk(tmp_path)
    assert _paths(result) == ["ok.py"]
    assert result.skipped_by_reason == {SkipReason.TOO_LARGE: 1}


def test_links_are_never_followed_or_kept(tmp_path: Path) -> None:
    outside = tmp_path / "outside"
    _write(outside, "secret.py", "pkg/agent.py")
    root = tmp_path / "repo"
    _write(root, "app.py")
    (root / "link.py").symlink_to(outside / "secret.py")
    (root / "linked_dir").symlink_to(outside / "pkg", target_is_directory=True)
    (root / "dangling.py").symlink_to(root / "missing.py")
    result = walk(root)
    assert _paths(result) == ["app.py"]
    assert result.skipped_by_reason == {SkipReason.NOT_REGULAR: 3}


def test_fifo_with_a_source_name_is_skipped(tmp_path: Path) -> None:
    os.mkfifo(tmp_path / "pipe.py")
    os.mkfifo(tmp_path / "pipe.md")
    result = walk(tmp_path)
    assert result.files == ()
    assert result.skipped_by_reason == {SkipReason.NOT_REGULAR: 1}


def test_unreadable_folder_is_counted(tmp_path: Path) -> None:
    _write(tmp_path, "locked/agent.py", "app.py")
    (tmp_path / "locked").chmod(0)
    try:
        if os.access(tmp_path / "locked", os.R_OK):
            pytest.skip("running with permissions that ignore file modes")
        result = walk(tmp_path)
    finally:
        (tmp_path / "locked").chmod(0o700)
    assert _paths(result) == ["app.py"]
    assert result.skipped_by_reason == {SkipReason.UNREADABLE: 1}


def test_missing_root_raises(tmp_path: Path) -> None:
    with pytest.raises(FileNotFoundError):
        walk(tmp_path / "missing")


@pytest.mark.parametrize(
    ("path", "tag"),
    [
        ("tests/agent.py", "test"),
        ("pkg/Tests/helpers.py", "test"),
        ("web/__tests__/App.tsx", "test"),
        ("spec/agent.ts", "test"),
        ("specs/agent.ts", "test"),
        ("testing/util.py", "test"),
        ("test/package.json", "test"),
        ("pkg/test_agent.py", "test"),
        ("pkg/agent_test.py", "test"),
        ("web/App.test.tsx", "test"),
        ("web/app.spec.js", "test"),
        ("web/app.test.mjs", "test"),
        ("svc/AgentTests.cs", "test"),
        ("svc/AgentTest.cs", "test"),
        ("examples/agent.py", "example"),
        ("docs/Example/agent.py", "example"),
        ("sample/agent.py", "example"),
        ("samples/agent.py", "example"),
        ("demo/agent.py", "example"),
        ("demos/package.json", "example"),
        ("examples/tests/agent.py", "test"),
        ("examples/test_agent.py", "test"),
        ("src/agent.py", None),
        ("src/latest.py", None),
        ("src/testament.py", None),
        ("svc/Contest.cs", None),
        ("src/examples.py", None),
        ("Agents.Tests/AgentRunner.cs", "test"),
        ("MyApp.UnitTests/Runner.cs", "test"),
        ("X.IntegrationTests/Runner.cs", "test"),
        ("integration_tests/agent.py", "test"),
        ("unit-test/agent.ts", "test"),
        ("app/tests.py", "test"),
        ("app/test.py", "test"),
        ("conftest.py", "test"),
        ("contest/agent.py", None),
        ("latest/agent.py", None),
        ("testing_utils/agent.py", None),
        ("src/attest.py", None),
    ],
)
def test_tags(tmp_path: Path, path: str, tag: str | None) -> None:
    _write(tmp_path, path)
    (found,) = walk(tmp_path).files
    assert found.path == path
    assert found.tag == tag


def test_order_is_sorted_and_paths_are_relative_posix(tmp_path: Path) -> None:
    names = ["z.py", "a/b/c.py", "a/a.py", "B.py", "a/b.py", "m/package.json"]
    _write(tmp_path, *names)
    result = walk(tmp_path)
    assert _paths(result) == sorted(names)
    assert _paths(walk(tmp_path)) == _paths(result)
    assert all(not f.path.startswith("/") and "\\" not in f.path for f in result.files)


def test_deep_tree_is_walked(tmp_path: Path) -> None:
    deep = tmp_path
    for _ in range(60):  # Bounded by PATH_MAX on macOS; the walk itself has no recursion.
        deep = deep / "dddd"
    _write(deep, "agent.py")
    (found,) = walk(tmp_path).files
    assert found.path.endswith("/agent.py") and found.path.count("/") == 60


class _Renamed:
    """A directory entry reported under another name, as Python reports undecodable bytes in a file name."""

    def __init__(self, entry: os.DirEntry[str], name: str) -> None:
        self._entry = entry
        self.name = name
        self.path = entry.path

    def __getattr__(self, attribute: str) -> object:
        return getattr(self._entry, attribute)


def test_non_utf8_names_are_skipped_and_never_reported(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    # macOS refuses such names, so the walker is shown surrogate-escaped names for real files instead.
    _write(tmp_path, "agent.py", "bad.py", "baddir/inner.py")
    real = os.scandir

    @contextlib.contextmanager
    def scandir(path: str) -> Iterator[list[object]]:
        with real(path) as it:
            yield [_Renamed(e, e.name.replace("bad", "bad\udcff")) if "bad" in e.name else e for e in it]

    monkeypatch.setattr(os, "scandir", scandir)
    result = walk(tmp_path)
    assert _paths(result) == ["agent.py"]
    assert result.skipped_by_reason == {SkipReason.UNREADABLE: 2}
