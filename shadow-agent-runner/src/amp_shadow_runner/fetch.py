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

"""A private, always-deleted workspace, and safe extraction of repository archives into it.

A repository's files are shaped by anyone who can push to it, so an archive is
treated as hostile. Only regular files and directories are written, with
modes 0600 and 0700 whatever the archive says. Links, devices, FIFOs and names
the scan cannot safely write (control characters, too long, too deep) are
skipped, so nothing written can point outside the tree. A member path with
``..`` or a leading ``/``, a size or entry count over the caps, or an
unreadable archive refuses the whole archive.

Error messages never contain member names or file contents.
"""

import functools
import gzip
import os
import re
import secrets
import shutil
import tarfile
import tempfile
import zlib
from collections.abc import Callable, Iterator
from contextlib import contextmanager, suppress
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from typing import IO, ClassVar

from amp_shadow_runner.exitcodes import ExitCode

DEFAULT_MAX_TOTAL_BYTES = 2 * 1024**3
"""Bytes written for one repository (2 GiB). Far above a normal repository's
source, and a bound on the workspace's disk use in ``/tmp``."""
DEFAULT_MAX_FILES = 200_000
"""Files, directories and skipped entries in one archive. Large monorepos have
tens of thousands of files; this leaves room for them while bounding inode use
and scan time."""
MAX_NAME_BYTES = 255
"""The longest path component Linux file systems accept; longer ones are skipped."""
MAX_PATH_BYTES = 4096
"""The longest member path, matching Linux's ``PATH_MAX``; longer ones are skipped."""
MAX_DEPTH = 128
"""The deepest member path; deeper ones are skipped, so deleting the tree stays within open-file and recursion limits."""

_CHUNK = 1024 * 1024
_FILE_MODE = 0o600
_DIR_MODE = 0o700
_CONTROL = re.compile(r"[\x00-\x1f\x7f]")
# Raised by tarfile, gzip and zlib on a corrupt, truncated or unsupported archive.
_UNREADABLE = (
    tarfile.TarError,
    gzip.BadGzipFile,
    EOFError,
    zlib.error,
    NotImplementedError,
    UnicodeDecodeError,
)
# Accepted gaps, both needing a malicious Git server (its URL is set by an admin): tarfile reads a
# whole pax header into memory, and the data of skipped tar members is decompressed without counting.


class ArchiveError(Exception):
    """The archive was refused or could not be extracted. The message is safe to show."""

    exit_code: ClassVar[ExitCode] = ExitCode.RUNTIME_ERROR


@dataclass(frozen=True)
class ExtractResult:
    """What ``extract`` wrote."""

    files: int
    directories: int
    skipped: int
    """Links, devices, FIFOs, other non-regular entries, and names with control characters or over the length or
    depth caps."""
    size: int
    """Bytes written."""
    stripped: bool
    """True when the tarball's single top-level folder was dropped."""


@contextmanager
def workspace(prefix: str = "amp-scan-", *, base: Path | None = None) -> Iterator[Path]:
    """Create a private (0700) temporary directory and always delete it on exit.

    It is created under ``base``, or ``tempfile.gettempdir()`` by default, and
    deleted on a normal exit, on an exception and on KeyboardInterrupt.
    """
    path = Path(tempfile.mkdtemp(prefix=prefix, dir=base))
    try:
        yield path
    finally:
        remove_tree(path)


def remove_tree(root: Path) -> None:
    """Delete ``root`` and everything in it. Symlinks inside are removed, never followed.

    The tree only ever holds this module's 0700 directories and 0600 files, so
    no permission fixing is needed. A missing ``root`` is not an error.
    """
    if os.path.lexists(root):
        shutil.rmtree(root, onexc=_ignore_missing)


def _ignore_missing(func: Callable[..., object], path: str, exc: BaseException) -> None:
    if not isinstance(exc, FileNotFoundError):
        raise exc


def extract(
    archive: Path,
    dest: Path,
    *,
    max_total_bytes: int = DEFAULT_MAX_TOTAL_BYTES,
    max_files: int = DEFAULT_MAX_FILES,
) -> ExtractResult:
    """Extract the regular files and directories of the ``tar.gz`` ``archive`` (a GitHub tarball) into ``dest``.

    ``dest`` must not exist; it is created with mode 0700. On any failure,
    ``dest`` is deleted again, so the caller has nothing to clean. ``archive``
    is deleted in every case (if that fails, the workspace deletes it later).

    The tarball's single top-level folder, such as ``owner-repo-<sha>/``, is dropped.

    Raises ArchiveError when the archive is refused or unreadable, or a file
    cannot be written. An error creating ``dest`` is raised as the OSError itself.
    """
    try:
        os.mkdir(dest, _DIR_MODE)
        try:
            return _extract(archive, _Writer(dest, max_total_bytes, max_files))
        except BaseException:
            remove_tree(dest)
            raise
    finally:
        with suppress(OSError):
            archive.unlink()


def _extract(archive: Path, writer: "_Writer") -> ExtractResult:
    message: str
    try:
        _read_tar(archive, writer)
        return writer.finish()
    except _UNREADABLE:
        message = "the archive is corrupt or unreadable"
    except OSError as exc:
        message = f"could not extract the archive: {exc.strerror or 'input/output error'}"
    raise ArchiveError(message)  # Outside the except block, so nothing is chained onto it.


def _read_tar(archive: Path, writer: "_Writer") -> None:
    with tarfile.open(archive, mode="r:gz") as tar:
        for info in tar:  # Streams: each member is read once, in order.
            if info.isreg():
                writer.add(info.name, _Kind.FILE, info.size, functools.partial(_tar_reader, tar, info))
            elif info.isdir():
                writer.add(info.name, _Kind.DIR)
            else:
                writer.add(info.name, _Kind.SKIP)


def _tar_reader(tar: tarfile.TarFile, info: tarfile.TarInfo) -> IO[bytes]:
    reader = tar.extractfile(info)
    if reader is None:
        raise ArchiveError("the archive is corrupt or unreadable")
    return reader


class _Kind(Enum):
    FILE = "file"
    DIR = "dir"
    SKIP = "skip"


class _Writer:
    """Writes members into ``dest`` one at a time, enforcing the caps and the path rules."""

    def __init__(self, dest: Path, max_total_bytes: int, max_files: int) -> None:
        self._dest = os.path.abspath(dest)
        self._max_total_bytes = max_total_bytes
        self._max_files = max_files
        self._created: dict[tuple[str, ...], _Kind] = {}
        self._files = 0
        self._dirs = 0
        self._skipped = 0
        self._size = 0
        self._tops: set[str] = set()
        self._top_level_non_dir = False

    def check_count(self, entries: int) -> None:
        if entries > self._max_files:
            raise ArchiveError(f"the archive has more than {self._max_files} entries")

    def add(
        self,
        name: str,
        kind: _Kind,
        declared_size: int = 0,
        open_member: Callable[[], IO[bytes]] | None = None,
    ) -> None:
        parts = _parts(name)
        if not parts:
            if kind is _Kind.DIR:
                return  # "./", the archive's root.
            raise ArchiveError("the archive has a member with an empty name")
        self._tops.add(parts[0])
        if len(parts) == 1 and kind is not _Kind.DIR:
            self._top_level_non_dir = True
        if kind is _Kind.SKIP or not _writable(name, parts):
            self._skipped += 1
            self._check_total()
            return
        if kind is _Kind.DIR:
            self._make_dirs(parts)
            return
        self._make_dirs(parts[:-1])
        if parts in self._created:
            raise ArchiveError("the archive has two members with the same path")
        if self._size + declared_size > self._max_total_bytes:
            raise self._too_large()
        assert open_member is not None
        self._created[parts] = _Kind.FILE
        self._files += 1
        self._check_total()
        self._copy(open_member, self._target(parts))

    def finish(self) -> ExtractResult:
        stripped = False
        if len(self._tops) == 1 and not self._top_level_non_dir:
            top = next(iter(self._tops))
            if self._created.get((top,)) is _Kind.DIR:
                _strip(Path(self._dest), top)
                self._dirs -= 1
                stripped = True
        return ExtractResult(self._files, self._dirs, self._skipped, self._size, stripped)

    def _check_total(self) -> None:
        self.check_count(self._files + self._dirs + self._skipped)

    def _make_dirs(self, parts: tuple[str, ...]) -> None:
        for i in range(1, len(parts) + 1):
            key = parts[:i]
            created = self._created.get(key)
            if created is _Kind.DIR:
                continue
            if created is _Kind.FILE:
                raise ArchiveError("the archive has a file and a folder with the same path")
            self._dirs += 1
            self._check_total()
            os.mkdir(self._target(key), _DIR_MODE)
            self._created[key] = _Kind.DIR

    def _target(self, parts: tuple[str, ...]) -> str:
        target = os.path.normpath(os.path.join(self._dest, *parts))
        if not target.startswith(self._dest + os.sep):
            raise ArchiveError("the archive has a member that would land outside the destination")
        return target

    def _copy(self, open_member: Callable[[], IO[bytes]], target: str) -> None:
        # O_EXCL and O_NOFOLLOW: never write through an existing file or link.
        flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC
        with os.fdopen(os.open(target, flags, _FILE_MODE), "wb") as out, open_member() as src:
            # Count what is written, not what the header says: headers can lie.
            while chunk := src.read(_CHUNK):
                self._size += len(chunk)
                if self._size > self._max_total_bytes:
                    raise self._too_large()
                out.write(chunk)

    def _too_large(self) -> ArchiveError:
        return ArchiveError(f"the archive is larger than {self._max_total_bytes} bytes when extracted")


def _parts(name: str) -> tuple[str, ...]:
    """The member's path components, or ArchiveError when the path leaves the destination."""
    if name.startswith("/"):
        raise ArchiveError("the archive has a member with an absolute path")
    parts = tuple(p for p in name.split("/") if p not in ("", "."))
    if ".." in parts:
        raise ArchiveError("the archive has a member path with '..'")
    return parts


def _writable(name: str, parts: tuple[str, ...]) -> bool:
    """False for names that real repositories can hold but the scan skips (such as macOS ``Icon\\r``)."""
    return (
        not _CONTROL.search(name)
        and len(parts) <= MAX_DEPTH
        and len(os.fsencode(name)) <= MAX_PATH_BYTES
        and all(len(os.fsencode(p)) <= MAX_NAME_BYTES for p in parts)
    )


def _strip(dest: Path, top: str) -> None:
    """Move the contents of ``dest/top`` up into ``dest``, staying inside ``dest`` throughout."""
    holder = dest / f".amp-strip-{secrets.token_hex(8)}"
    os.rename(dest / top, holder)
    for child in os.listdir(holder):
        os.rename(holder / child, dest / child)
    os.rmdir(holder)
