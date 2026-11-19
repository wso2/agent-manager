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

"""amp_shadow_runner.fetch: the scan workspace and safe extraction of repository archives."""

import io
import stat
import struct
import tarfile
import zipfile
from pathlib import Path
from typing import Literal

import pytest

from amp_shadow_runner import fetch
from amp_shadow_runner.fetch import (
    DEFAULT_MAX_FILES,
    DEFAULT_MAX_TOTAL_BYTES,
    ArchiveError,
    ExtractResult,
    extract,
    workspace,
)

TOP = "owner-repo-0123abc"

Member = tuple[tarfile.TarInfo, bytes | None]
Kind = Literal["tar.gz", "zip"]


def _file(name: str, data: bytes = b"x", mode: int = 0o644) -> Member:
    info = tarfile.TarInfo(name)
    info.size, info.mode = len(data), mode
    return info, data


def _special(name: str, kind: bytes, target: str = "") -> Member:
    info = tarfile.TarInfo(name)
    info.type, info.linkname = kind, target
    return info, None


def _tar(path: Path, members: list[Member], pax_headers: dict[str, str] | None = None) -> Path:
    with tarfile.open(path, "w:gz", format=tarfile.PAX_FORMAT, pax_headers=pax_headers) as tar:
        for info, data in members:
            tar.addfile(info, io.BytesIO(data) if data is not None else None)
    return path


def _zip(path: Path, members: dict[str, bytes], *, symlinks: dict[str, str] | None = None) -> Path:
    with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as zf:
        for name, data in members.items():
            zf.writestr(name, data)
        for name, target in (symlinks or {}).items():
            info = zipfile.ZipInfo(name)
            info.external_attr = (stat.S_IFLNK | 0o777) << 16
            zf.writestr(info, target)
    return path


def _refuse(
    archive: Path,
    dest: Path,
    kind: Kind = "tar.gz",
    *,
    max_total_bytes: int = DEFAULT_MAX_TOTAL_BYTES,
    max_files: int = DEFAULT_MAX_FILES,
) -> str:
    with pytest.raises(ArchiveError) as info:
        extract(archive, dest, kind=kind, max_total_bytes=max_total_bytes, max_files=max_files)
    assert not dest.exists(), "a refused archive leaves nothing extracted"
    assert not archive.exists(), "the archive is deleted after a failure"
    assert info.value.__context__ is None
    return str(info.value)


def _tree(root: Path) -> dict[str, bytes | None]:
    """Relative path to content (None for a directory). Fails on anything that is neither."""
    found: dict[str, bytes | None] = {}
    for path in sorted(root.rglob("*")):
        mode = path.lstat().st_mode
        assert stat.S_ISREG(mode) or stat.S_ISDIR(mode), f"{path} is not a regular file or directory"
        found[path.relative_to(root).as_posix()] = path.read_bytes() if stat.S_ISREG(mode) else None
    return found


def test_tarball_is_extracted_without_its_top_folder_and_with_safe_modes(tmp_path: Path) -> None:
    archive = _tar(
        tmp_path / "repo.tar.gz",
        [
            _special(f"{TOP}/", tarfile.DIRTYPE),
            _file(f"{TOP}/README.md", b"# repo"),
            _file(f"{TOP}/src/agent.py", b"import openai", mode=0o4755),
        ],
    )
    dest = tmp_path / "out"

    result = extract(archive, dest, kind="tar.gz")

    assert result == ExtractResult(files=2, directories=1, skipped=0, size=19, stripped=True)
    assert _tree(dest) == {"README.md": b"# repo", "src": None, "src/agent.py": b"import openai"}
    assert not archive.exists()
    assert stat.S_IMODE(dest.stat().st_mode) == 0o700
    assert stat.S_IMODE((dest / "src").stat().st_mode) == 0o700
    assert stat.S_IMODE((dest / "src" / "agent.py").stat().st_mode) == 0o600  # Not 4755 from the header.


def test_github_tarball_pax_global_header_is_not_a_member(tmp_path: Path) -> None:
    sha = "0123abc" * 5 + "01234"
    archive = _tar(
        tmp_path / "repo.tar.gz",
        [_special(f"{TOP}/", tarfile.DIRTYPE), _file(f"{TOP}/agent.py")],
        pax_headers={"comment": sha},
    )

    result = extract(archive, tmp_path / "out", kind="tar.gz", max_files=2)

    assert result == ExtractResult(files=1, directories=0, skipped=0, size=1, stripped=True)
    assert _tree(tmp_path / "out") == {"agent.py": b"x"}


def test_a_folder_named_like_the_top_folder_survives_the_strip(tmp_path: Path) -> None:
    archive = _tar(tmp_path / "repo.tar.gz", [_file(f"{TOP}/{TOP}/x")])

    result = extract(archive, tmp_path / "out", kind="tar.gz")

    assert result.stripped
    assert _tree(tmp_path / "out") == {TOP: None, f"{TOP}/x": b"x"}


def test_zip_with_a_single_top_folder_is_not_stripped(tmp_path: Path) -> None:
    archive = _zip(tmp_path / "repo.zip", {"src/agent.py": b"a"})

    result = extract(archive, tmp_path / "out", kind="zip")

    assert not result.stripped
    assert _tree(tmp_path / "out") == {"src": None, "src/agent.py": b"a"}


def test_zip_without_a_common_top_folder_keeps_its_paths(tmp_path: Path) -> None:
    archive = _zip(tmp_path / "repo.zip", {"README.md": b"r", "src/app.ts": b"a", "src/lib/": b""})

    result = extract(archive, tmp_path / "out", kind="zip")

    assert result == ExtractResult(files=2, directories=2, skipped=0, size=2, stripped=False)
    assert _tree(tmp_path / "out") == {"README.md": b"r", "src": None, "src/app.ts": b"a", "src/lib": None}
    assert not archive.exists()


@pytest.mark.parametrize(
    ("name", "problem"),
    [
        ("../evil.txt", "'..'"),
        (f"{TOP}/../../evil.txt", "'..'"),
        ("/tmp/evil.txt", "absolute path"),
    ],
)
@pytest.mark.parametrize("kind", ["tar.gz", "zip"])
def test_unsafe_member_names_refuse_the_whole_archive(tmp_path: Path, kind: Kind, name: str, problem: str) -> None:
    members = {f"{TOP}/ok.txt": b"ok", name: b"evil"}
    archive = (
        _zip(tmp_path / "repo.zip", members)
        if kind == "zip"
        else _tar(tmp_path / "repo.tar.gz", [_file(n, d) for n, d in members.items()])
    )

    (tmp_path / "work").mkdir()
    message = _refuse(archive, tmp_path / "work" / "out", kind)

    assert problem in message
    assert "evil" not in message, "member names never reach the message"
    assert not list(tmp_path.rglob("evil.txt"))


@pytest.mark.parametrize("kind", ["tar.gz", "zip"])
def test_backslash_and_drive_letter_names_are_ordinary_on_posix(tmp_path: Path, kind: Kind) -> None:
    names = ["C:/a.txt", "dir\\b.txt"]
    archive = (
        _zip(tmp_path / "repo.zip", dict.fromkeys(names, b"x"))
        if kind == "zip"
        else _tar(tmp_path / "repo.tar.gz", [_file(n) for n in names])
    )

    extract(archive, tmp_path / "out", kind=kind)

    assert _tree(tmp_path / "out") == {"C:": None, "C:/a.txt": b"x", "dir\\b.txt": b"x"}


@pytest.mark.parametrize(
    "name",
    [
        f"{TOP}/Icon\r",
        f"{TOP}/{'a' * 256}",
        f"{TOP}/" + "/".join(["d" * 200] * 21) + "/x",
        f"{TOP}/" + "d/" * 128 + "x",
    ],
    ids=["control character", "long component", "long path", "too deep"],
)
def test_names_the_scan_cannot_write_are_skipped(tmp_path: Path, name: str) -> None:
    archive = _tar(tmp_path / "repo.tar.gz", [_file(f"{TOP}/ok.txt"), _file(name)])

    result = extract(archive, tmp_path / "out", kind="tar.gz")

    assert result.skipped == 1
    assert _tree(tmp_path / "out") == {"ok.txt": b"x"}


def test_zip_name_with_nul_is_skipped(tmp_path: Path) -> None:
    archive = _zip(tmp_path / "repo.zip", {"ok.txt": b"x", "a_b.txt": b"x"})
    raw = bytearray(archive.read_bytes())
    for header, name_offset in ((b"PK\x03\x04", 30), (b"PK\x01\x02", 46)):
        start = raw.index(header, raw.index(header) + 1)  # The second member's local and central headers.
        assert raw[start + name_offset : start + name_offset + 7] == b"a_b.txt"
        raw[start + name_offset + 1] = 0
    archive.write_bytes(bytes(raw))

    result = extract(archive, tmp_path / "out", kind="zip")

    assert result.skipped == 1
    assert _tree(tmp_path / "out") == {"ok.txt": b"x"}


def test_tar_links_devices_and_fifos_are_skipped(tmp_path: Path) -> None:
    archive = _tar(
        tmp_path / "repo.tar.gz",
        [
            _file(f"{TOP}/a.txt"),
            _special(f"{TOP}/sym", tarfile.SYMTYPE, "/etc/passwd"),
            _special(f"{TOP}/hard", tarfile.LNKTYPE, f"{TOP}/a.txt"),
            _special(f"{TOP}/fifo", tarfile.FIFOTYPE),
            _special(f"{TOP}/dev", tarfile.CHRTYPE),
        ],
    )

    result = extract(archive, tmp_path / "out", kind="tar.gz")

    assert result.skipped == 4
    assert _tree(tmp_path / "out") == {"a.txt": b"x"}


def test_zip_symlinks_are_skipped(tmp_path: Path) -> None:
    archive = _zip(tmp_path / "repo.zip", {"a.txt": b"x"}, symlinks={"link": "/etc/passwd"})

    result = extract(archive, tmp_path / "out", kind="zip")

    assert result.skipped == 1
    assert _tree(tmp_path / "out") == {"a.txt": b"x"}


def test_a_file_written_through_a_tar_symlink_stays_inside(tmp_path: Path) -> None:
    outside = tmp_path / "outside"
    outside.mkdir()
    (outside / "victim.txt").write_bytes(b"original")
    archive = _tar(
        tmp_path / "repo.tar.gz",
        [
            _special(f"{TOP}/escape", tarfile.SYMTYPE, str(outside)),
            _file(f"{TOP}/escape/pwned.txt", b"pwned"),
            _special(f"{TOP}/victim.txt", tarfile.SYMTYPE, str(outside / "victim.txt")),
            _file(f"{TOP}/victim.txt", b"pwned"),
        ],
    )

    extract(archive, tmp_path / "out", kind="tar.gz")

    assert _tree(outside) == {"victim.txt": b"original"}
    assert _tree(tmp_path / "out") == {"escape": None, "escape/pwned.txt": b"pwned", "victim.txt": b"pwned"}


def test_tar_over_the_size_cap_is_refused(tmp_path: Path) -> None:
    archive = _tar(tmp_path / "repo.tar.gz", [_file(f"{TOP}/a", b"a" * 600), _file(f"{TOP}/b", b"b" * 600)])

    assert "larger than 1000 bytes" in _refuse(archive, tmp_path / "out", max_total_bytes=1000)


def test_zip_whose_header_understates_the_size_is_refused(tmp_path: Path) -> None:
    archive = _zip(tmp_path / "repo.zip", {"big.bin": b"A" * 5000})
    raw = bytearray(archive.read_bytes())
    central = raw.index(b"PK\x01\x02")
    struct.pack_into("<I", raw, central + 24, 10)  # Uncompressed size in the central directory.
    archive.write_bytes(bytes(raw))

    # zipfile stops at the declared size and then fails the CRC check.
    assert _refuse(archive, tmp_path / "out", "zip", max_total_bytes=1000) == "the archive is corrupt or unreadable"


def test_the_size_cap_counts_bytes_written_not_the_declared_size(tmp_path: Path) -> None:
    writer = fetch._Writer(tmp_path, 1000, 10)

    with pytest.raises(ArchiveError, match="larger than 1000 bytes"):
        writer.add("big.bin", fetch._Kind.FILE, 10, lambda: io.BytesIO(b"A" * 5000))


def test_encrypted_zip_is_refused(tmp_path: Path) -> None:
    archive = _zip(tmp_path / "repo.zip", {"a.txt": b"x"})
    raw = bytearray(archive.read_bytes())
    struct.pack_into("<H", raw, raw.index(b"PK\x01\x02") + 8, 0x1)  # Flag bits: encrypted.
    archive.write_bytes(bytes(raw))

    assert _refuse(archive, tmp_path / "out", "zip") == "the archive is encrypted"


def test_tar_with_dot_slash_names_is_extracted(tmp_path: Path) -> None:
    archive = _tar(tmp_path / "repo.tar.gz", [_special("./", tarfile.DIRTYPE), _file("./a.txt"), _file("./b/c.txt")])

    result = extract(archive, tmp_path / "out", kind="tar.gz")

    assert not result.stripped
    assert _tree(tmp_path / "out") == {"a.txt": b"x", "b": None, "b/c.txt": b"x"}


def test_implicit_folders_count_as_entries(tmp_path: Path) -> None:
    archive = _tar(tmp_path / "repo.tar.gz", [_file(f"{TOP}/a/b/c.txt")])

    assert "more than 3 entries" in _refuse(archive, tmp_path / "out", max_files=3)


@pytest.mark.parametrize("kind", ["tar.gz", "zip"])
def test_too_many_entries_are_refused(tmp_path: Path, kind: Kind) -> None:
    names = [f"{TOP}/{i}.txt" for i in range(4)]
    archive = (
        _zip(tmp_path / "repo.zip", dict.fromkeys(names, b"x"))
        if kind == "zip"
        else _tar(tmp_path / "repo.tar.gz", [_file(n) for n in names])
    )

    assert "more than 3 entries" in _refuse(archive, tmp_path / "out", kind, max_files=3)


@pytest.mark.parametrize(
    "members",
    [
        [_file(f"{TOP}/a"), _file(f"{TOP}/a")],
        [_file(f"{TOP}/a"), _file(f"{TOP}/a/b")],
    ],
    ids=["same path twice", "file and folder"],
)
def test_conflicting_paths_are_refused(tmp_path: Path, members: list[Member]) -> None:
    assert "same path" in _refuse(_tar(tmp_path / "repo.tar.gz", members), tmp_path / "out")


@pytest.mark.parametrize("kind", ["tar.gz", "zip"])
def test_corrupt_archive_is_refused(tmp_path: Path, kind: Kind) -> None:
    archive = tmp_path / "repo.bin"
    archive.write_bytes(b"\x1f\x8b not really an archive" * 10)

    assert _refuse(archive, tmp_path / "out", kind) == "the archive is corrupt or unreadable"


def test_zip_with_an_invalid_utf8_name_is_refused(tmp_path: Path) -> None:
    archive = _zip(tmp_path / "repo.zip", {"a.txt": b"x"})
    raw = bytearray(archive.read_bytes())
    central = raw.index(b"PK\x01\x02")
    struct.pack_into("<H", raw, central + 8, 0x800)  # Flag bits: the name is UTF-8.
    raw[central + 46] = 0xFF  # Not valid UTF-8.
    archive.write_bytes(bytes(raw))

    assert _refuse(archive, tmp_path / "out", "zip") == "the archive is corrupt or unreadable"


def test_workspace_is_private_and_removed_after_a_normal_exit(tmp_path: Path) -> None:
    with workspace(base=tmp_path) as path:
        assert path.parent == tmp_path
        assert stat.S_IMODE(path.stat().st_mode) == 0o700
        (path / "file.txt").write_text("code")

    assert not path.exists()


@pytest.mark.parametrize("error", [RuntimeError, KeyboardInterrupt])
def test_workspace_is_removed_after_an_exception(tmp_path: Path, error: type[BaseException]) -> None:
    with pytest.raises(error), workspace(base=tmp_path) as path:
        (path / "file.txt").write_text("code")
        raise error

    assert not path.exists()


def test_workspace_removal_does_not_follow_a_symlink_out(tmp_path: Path) -> None:
    outside = tmp_path / "outside"
    outside.mkdir()
    (outside / "keep.txt").write_text("keep")
    with workspace(base=tmp_path) as path:
        (path / "link").symlink_to(outside, target_is_directory=True)

    assert not path.exists()
    assert (outside / "keep.txt").read_text() == "keep"
