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


"""Short, human-readable progress on stderr while a scan runs.

On a terminal (stderr is a TTY and ``TERM`` is not ``dumb``), each repository has one live status line:
downloading (bytes and speed, with a bar when the size is known), extracting, scanning (files done of the
total) and AI review (reviews done of those due). It is redrawn in place with a carriage return and an
erase to the end of the line, at most every REDRAW_INTERVAL seconds, and cut to the terminal's width so it
never wraps; a spinner turns while nothing else changes. When the repository ends, the live line is replaced
by its permanent ``done`` or ``failed`` line, with the time it took. Publishing shows a spinner too. Colour
(green ``done``, red ``failed``, dim times) is used only there, and not when ``NO_COLOR`` is set. Block and
Braille characters are used only on a UTF-8 stream; otherwise ASCII.

Anywhere else (CI, ``docker run`` without ``-t``, tests), each step prints one plain line with no carriage
returns, escape codes, byte counts or times, so a log reads like a terminal without the live line: AI review
prints one line when it starts, not one per review.

The runner has no quiet mode. Lines hold only repository names, counts and safe reasons, never a secret or a
URL. Control characters in names are replaced, so a name cannot move the cursor or recolour the terminal.

This is the only module that knows about terminals. While a live line may show, everything else is printed
with ``Progress.say``, which erases the live line first, and ``Progress.close`` (also on Ctrl-C) leaves the
cursor at the start of a clean line.
"""

import codecs
import functools
import math
import os
import shutil
import sys
import threading
import time
import unicodedata
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from typing import TextIO

from amp_shadow_runner.engine.scan import ScanResult
from amp_shadow_runner.report import Report, RepositoryStatus

REDRAW_INTERVAL = 0.1
"""The shortest time, in seconds, between two redraws of the live line (and the spinner's step)."""
_BAR_WIDTH = 20
_MIN_BAR_WIDTH = 5
_GREEN, _RED, _DIM = "32", "31", "2"


@dataclass(frozen=True)
class Glyphs:
    """The characters the live line is drawn with."""

    spinner: str
    full: str
    empty: str
    ellipsis: str


UNICODE = Glyphs("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏", "█", "░", "…")
ASCII = Glyphs("|/-\\", "#", "-", "...")


@dataclass(frozen=True)
class Status:
    """What the live line says: ``head`` (``[1/2] repo``), an optional spinner, ``label``, an optional bar
    filled to ``fraction``, and ``tail``."""

    head: str
    label: str
    tail: str = ""
    fraction: float | None = None
    spin: bool = False


def _clean(text: str) -> str:
    return "".join(c if c.isprintable() else "?" for c in text)


def _plural(count: int, noun: str, plural: str | None = None) -> str:
    return f"{count} {noun}" if count == 1 else f"{count} {plural or noun + 's'}"


def _char_width(c: str) -> int:
    if unicodedata.combining(c):
        return 0
    return 2 if unicodedata.east_asian_width(c) in ("W", "F") else 1


def _width(text: str) -> int:
    return sum(_char_width(c) for c in text)


def fit(text: str, columns: int, ellipsis: str) -> str:
    """``text`` cut to at most ``columns`` terminal cells, ending in ``ellipsis`` when cut."""
    if _width(text) <= columns:
        return text
    room = max(columns - _width(ellipsis), 0)
    kept: list[str] = []
    used = 0
    for c in text:
        used += _char_width(c)
        if used > room:
            break
        kept.append(c)
    return "".join(kept) + ellipsis[: max(columns - room, 0)]


def _columns(stream: TextIO) -> int:
    try:
        columns = os.get_terminal_size(stream.fileno()).columns
    except (AttributeError, ValueError, OSError):
        columns = 0
    return columns or shutil.get_terminal_size().columns or 80


def _is_utf8(stream: TextIO) -> bool:
    try:
        return codecs.lookup(getattr(stream, "encoding", None) or "ascii").name == "utf-8"
    except LookupError:
        return False


def _isatty(stream: TextIO) -> bool:
    try:
        return stream.isatty()
    except (AttributeError, ValueError, OSError):
        return False


def duration(seconds: float) -> str:
    """``3.2s``, ``31s``, ``2m 05s`` or ``1h 02m``."""
    if seconds < 10:
        return f"{max(seconds, 0):.1f}s"
    whole = round(seconds)
    if whole < 60:
        return f"{whole}s"
    minutes, secs = divmod(whole, 60)
    if minutes < 60:
        return f"{minutes}m {secs:02d}s"
    hours, minutes = divmod(minutes, 60)
    return f"{hours}h {minutes:02d}m"


def size(count: float) -> str:
    """``512 B``, ``12.4 kB``, ``3.1 MB`` or ``1.2 GB`` (powers of 1000)."""
    if count < 1000:
        return f"{int(count)} B"
    for unit in ("kB", "MB"):
        count /= 1000
        if count < 1000:
            return f"{count:.1f} {unit}"
    return f"{count / 1000:.1f} GB"


class LiveLine:
    """One status line on a terminal, redrawn in place.

    ``show`` replaces the status. It is drawn at once with ``force``, or when REDRAW_INTERVAL has passed since
    the last draw; otherwise the next draw shows it. ``print`` erases the line and prints a permanent one;
    no live line shows until the next ``show``. With ``animate``, a background thread redraws every
    REDRAW_INTERVAL, so the spinner turns and a skipped update is drawn; tests call ``tick`` instead.
    ``clock`` (default ``time.monotonic``) and ``columns`` (default: the terminal's width) are for tests.
    """

    def __init__(
        self,
        out: TextIO,
        *,
        glyphs: Glyphs,
        clock: Callable[[], float] = time.monotonic,
        columns: Callable[[], int] | None = None,
        animate: bool = False,
    ) -> None:
        self._out = out
        self._glyphs = glyphs
        self._clock = clock
        self._columns: Callable[[], int] = functools.partial(_columns, out) if columns is None else columns
        self._animate = animate
        self._lock = threading.Lock()
        self._stop = threading.Event()
        self._thread: threading.Thread | None = None
        self._status: Status | None = None
        self._drawn = False
        self._dirty = False
        self._broken = False
        self._last = -math.inf

    def show(self, status: Status, *, force: bool = False) -> None:
        with self._lock:
            self._status = status
            self._dirty = True
            now = self._clock()
            if not self._broken and (force or now - self._last >= REDRAW_INTERVAL):
                self._draw(now)
        if self._animate and self._thread is None and not self._stop.is_set():
            self._thread = threading.Thread(target=self._run, name="amp-progress", daemon=True)
            self._thread.start()

    def tick(self) -> None:
        """Redraw if the spinner should turn or an update was not drawn yet."""
        with self._lock:
            if not self._broken and self._status is not None and (self._dirty or self._status.spin):
                self._draw(self._clock())

    def print(self, line: str) -> None:
        with self._lock:
            self._erase()
            self._write(line + "\n")

    def clear(self) -> None:
        with self._lock:
            self._erase()

    def close(self) -> None:
        """Erase the line and stop the redraw thread."""
        self._stop.set()
        self.clear()

    def render(self, status: Status, now: float) -> str:
        """The line for ``status`` at ``now``, fitted to the terminal's width (less one column, so writing the
        last cell never wraps)."""
        columns = max(self._columns() - 1, 1)
        glyphs = self._glyphs
        line = f"{status.head}  " if status.head else ""
        if status.spin:
            line += glyphs.spinner[int(now / REDRAW_INTERVAL) % len(glyphs.spinner)] + " "
        line += status.label
        tail = status.tail
        if status.fraction is not None:
            bar = min(_BAR_WIDTH, columns - _width(line) - _width(tail) - 2)
            if bar >= _MIN_BAR_WIDTH:
                filled = round(bar * min(max(status.fraction, 0.0), 1.0))
                line += " " + glyphs.full * filled + glyphs.empty * (bar - filled)
            line += f" {tail}" if tail else ""
        elif tail:
            line += f"  {tail}"
        return fit(line, columns, glyphs.ellipsis)

    def _draw(self, now: float) -> None:
        assert self._status is not None
        self._drawn = True  # Before writing, so a Ctrl-C during the write still leaves the line erased.
        self._write("\r" + self.render(self._status, now) + "\x1b[K")
        self._dirty = False
        self._last = now

    def _erase(self) -> None:
        if self._drawn:
            self._write("\r\x1b[K")
        self._drawn = False
        self._status = None

    def _write(self, text: str) -> None:
        """Write and flush ``text``. A closed or broken stream stops all output, so progress never fails the scan."""
        if self._broken:
            return
        try:
            self._out.write(text)
            self._out.flush()
        except (OSError, ValueError):
            self._broken = True
            self._stop.set()

    def _run(self) -> None:
        while not self._stop.wait(REDRAW_INTERVAL):
            self.tick()


class Progress:
    """Writes progress to ``out`` (default ``sys.stderr``).

    ``live`` (default: ``out`` is a terminal and ``TERM`` is not ``dumb``) chooses the live line over plain
    lines; ``env`` (default ``os.environ``) is read for ``TERM`` and ``NO_COLOR``. ``clock`` (default
    ``time.monotonic``) times the repositories and the scan, and ``animate`` (default: ``live``) turns the
    spinner from a background thread.
    """

    def __init__(
        self,
        out: TextIO | None = None,
        *,
        live: bool | None = None,
        env: Mapping[str, str] | None = None,
        clock: Callable[[], float] = time.monotonic,
        animate: bool | None = None,
        columns: Callable[[], int] | None = None,
    ) -> None:
        self._out = sys.stderr if out is None else out
        env = os.environ if env is None else env
        if live is None:
            live = _isatty(self._out) and env.get("TERM", "") != "dumb"
        self._glyphs = UNICODE if _is_utf8(self._out) else ASCII
        self._live = (
            LiveLine(
                self._out,
                glyphs=self._glyphs,
                clock=clock,
                columns=columns,
                animate=live if animate is None else animate,
            )
            if live
            else None
        )
        self._colour = live and not env.get("NO_COLOR")
        self._clock = clock
        self._began = clock()
        self._repo_began = self._began
        self._download_began: float | None = None

    def say(self, line: str = "") -> None:
        """Print a permanent line, erasing the live line first."""
        if self._live is not None:
            self._live.print(line)
        else:
            print(line, file=self._out, flush=True)

    def clear(self) -> None:
        """Erase the live line, if one shows."""
        if self._live is not None:
            self._live.clear()

    def close(self) -> None:
        """Erase the live line and stop its spinner. Called at the end of a scan, and on Ctrl-C."""
        if self._live is not None:
            self._live.close()

    def begin(self) -> None:
        """Start the scan's clock (the summary says how long the scan took)."""
        self._began = self._clock()

    def _repo(self, index: int, total: int, name: str, text: str) -> None:
        self.say(f"[{index}/{total}] {_clean(name)}: {text}")

    def _show(
        self,
        index: int,
        total: int,
        name: str,
        label: str,
        *,
        tail: str = "",
        fraction: float | None = None,
        spin: bool = False,
        force: bool = False,
    ) -> None:
        assert self._live is not None
        head = f"[{index}/{total}] {_clean(name)}"
        self._live.show(Status(head, label, tail, fraction, spin), force=force)

    def downloading(self, index: int, total: int, name: str) -> None:
        """A repository starts: its commit is resolved and its archive downloaded."""
        self._repo_began = self._clock()
        self._download_began = None
        if self._live is None:
            self._repo(index, total, name, "downloading")
        else:
            self._show(index, total, name, "downloading", spin=True, force=True)

    def received(self, index: int, total: int, name: str, done: int, length: int | None) -> None:
        """``done`` bytes of the archive are saved, of ``length`` (None when the host did not say)."""
        if self._live is None:
            return
        now = self._clock()
        if done == 0 or self._download_began is None:  # The body starts, or starts again on a retry.
            self._download_began = now
        elapsed = now - self._download_began
        tail = size(done)
        if elapsed >= 0.5:
            tail += f"  {size(done / elapsed)}/s"
        if length:
            percent = min(done * 100 // length, 100)
            fraction = done / length
            self._show(index, total, name, "downloading", tail=f"{percent}%  {tail}", fraction=fraction)
        else:
            self._show(index, total, name, "downloading", tail=tail, spin=True)

    def extracting(self, index: int, total: int, name: str) -> None:
        if self._live is not None:
            self._show(index, total, name, "extracting", spin=True, force=True)

    def scanning(self, index: int, total: int, name: str) -> None:
        if self._live is None:
            self._repo(index, total, name, "scanning")
        else:
            self._show(index, total, name, "scanning", spin=True, force=True)

    def scanning_files(self, index: int, total: int, name: str, done: int, count: int) -> None:
        """``done`` of the repository's ``count`` files are read."""
        if self._live is not None:
            fraction = done / count if count else 1.0
            tail = f"{done}/{count} files"
            self._show(index, total, name, "scanning", tail=tail, fraction=fraction, force=done == count)

    def reviewing(self, index: int, total: int, name: str, n: int, due: int) -> None:
        """Before the n-th of a repository's ``due`` AI reviews. Off a terminal, only the first says so."""
        if self._live is not None:
            fraction = (n - 1) / due
            self._show(index, total, name, "AI review", tail=f"{n - 1}/{due}", fraction=fraction, spin=True)
        elif n == 1:
            self._repo(index, total, name, f"AI review of {_plural(due, 'not-sure agent')}")

    def scanned(self, index: int, total: int, name: str, result: ScanResult) -> None:
        agents = sum(f.tag != "test" for f in result.findings)
        text = self._paint("done", _GREEN)
        if self._live is not None:
            text += self._paint(f" in {duration(self._clock() - self._repo_began)}", _DIM)
        text += f", {_plural(agents, 'agent')}"
        tagged = len(result.findings) - agents
        if tagged:
            # Next to the agent count, so it reads as agents, not files.
            text += f" ({tagged} more in test code)"
        text += f", {_plural(result.files_scanned, 'file')} scanned"
        if result.files_skipped:
            text += f", {result.files_skipped} skipped"
        self._repo(index, total, name, text)

    def failed(self, index: int, total: int, name: str, reason: str) -> None:
        self._repo(index, total, name, f"{self._paint('failed', _RED)}: {_clean(reason)}")

    def publishing(self) -> None:
        """Publishing to Agent Manager starts; the next ``say`` replaces the spinner."""
        if self._live is not None:
            self._live.show(Status("", f"Publishing to Agent Manager{self._glyphs.ellipsis}", spin=True), force=True)

    def summary(self, report: Report, location: str) -> None:
        """The closing lines: repositories by status, agents found, files, and where the report is."""
        summary = report.summary
        repos = summary.repositories
        scanned = repos[RepositoryStatus.SCANNED] + repos[RepositoryStatus.GONE]
        extra = [
            f"{repos[status]} {label}"
            for status, label in (
                (RepositoryStatus.GONE, "not found"),
                (RepositoryStatus.FAILED, "failed"),
                (RepositoryStatus.SKIPPED, "skipped"),
            )
            if repos[status]
        ]
        line = f"Scanned {_plural(scanned, 'repository', 'repositories')}"
        if self._live is not None:
            line += f" in {duration(self._clock() - self._began)}"
        self.say(line + (f" ({', '.join(extra)})." if extra else "."))
        line = f"Found {_plural(summary.proven + summary.not_sure, 'agent')}: "
        line += f"{summary.proven} proven, {summary.not_sure} not sure"
        if summary.test:
            line += f"; {summary.test} more in test code, not published"
        self.say(line + ".")
        self.say(f"Files: {summary.files_scanned} scanned, {summary.files_skipped} skipped.")
        self.say(f"Report: {_clean(location)}")

    def _paint(self, text: str, code: str) -> str:
        return f"\x1b[{code}m{text}\x1b[0m" if self._colour else text
