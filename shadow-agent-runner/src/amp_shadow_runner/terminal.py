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

"""What the command prints on stderr, and the one place that knows about terminals.

``Ui`` lays the output out in sections::

    AMP shadow agent scan · acme on github.com

    ◆ Repositories (2)
      support-bot
      billing      private
      ! No GitHub token: about 25 repositories an hour.
        Pass -e GITHUB_TOKEN for more.

A section has a ``◆`` title and two-space indented content: label and value rows, plain items, and status
rows marked ``✓`` (green), ``!`` (yellow), ``✗`` (red) or ``?`` (a question, cyan). Secondary text is dim.

On a terminal (the stream is a TTY and ``TERM`` is not ``dumb``) the output is *live*: ``LiveLine`` keeps one
status line redrawn in place, questions are replaced by their answers (``erase_since``), and long status rows
wrap at the terminal's width. Colour is used only there, and not when ``NO_COLOR`` is set. Anywhere else (CI,
``docker run`` without ``-t``, tests) the same structure is printed as plain lines, with no colour and no
cursor movement. Block, Braille and other symbols are used only on a UTF-8 stream; otherwise ASCII.

Lines hold only names, counts and safe reasons, never a secret. Text from a Git host goes through ``clean``,
so a name cannot move the cursor or recolour the terminal.
"""

import codecs
import functools
import math
import os
import re
import shutil
import sys
import textwrap
import threading
import time
import unicodedata
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from enum import StrEnum
from typing import TextIO

REDRAW_INTERVAL = 0.1
"""The shortest time, in seconds, between two redraws of the live line (and the spinner's step)."""
LABEL_WIDTH = 11
"""The column a row's value starts in, after the two-space indent."""
INDENT = "  "
_BAR_WIDTH = 20
_MIN_BAR_WIDTH = 5
_ANSI = re.compile(r"\x1b\[[0-9;?]*[A-Za-z]")

GREEN, RED, YELLOW, CYAN, DIM, BOLD = "32", "31", "33", "36", "2", "1"
HIDE_CURSOR, SHOW_CURSOR = "\x1b[?25l", "\x1b[?25h"


@dataclass(frozen=True)
class Glyphs:
    """The characters the output is drawn with."""

    spinner: str
    full: str
    empty: str
    ellipsis: str
    section: str
    ok: str
    warn: str
    fail: str
    ask: str
    dot: str
    sep: str
    pointer: str
    keys: str


UNICODE = Glyphs("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏", "█", "░", "…", "◆", "✓", "!", "✗", "?", "•", " · ", "›", "↑/↓")
ASCII = Glyphs("|/-\\", "#", "-", "...", "*", "v", "!", "x", "?", "*", " - ", ">", "Up/Down")


class Mark(StrEnum):
    """The mark at the start of a status row."""

    OK = "ok"
    WARN = "warn"
    FAIL = "fail"
    ASK = "ask"


_MARK_COLOURS = {Mark.OK: GREEN, Mark.WARN: YELLOW, Mark.FAIL: RED, Mark.ASK: CYAN}


def clean(text: str) -> str:
    """``text`` with each non-printable character (escape codes, newlines) replaced by ``?``."""
    return "".join(c if c.isprintable() else "?" for c in text)


def plural(count: int, noun: str, plural_noun: str | None = None) -> str:
    return f"{number(count)} {noun}" if count == 1 else f"{number(count)} {plural_noun or noun + 's'}"


def number(count: int) -> str:
    """``1,194``."""
    return f"{count:,}"


def _char_width(c: str) -> int:
    if unicodedata.combining(c):
        return 0
    return 2 if unicodedata.east_asian_width(c) in ("W", "F") else 1


def width(text: str) -> int:
    """The terminal cells ``text`` takes, not counting colour codes."""
    return sum(_char_width(c) for c in _ANSI.sub("", text))


def pad(text: str, cells: int) -> str:
    """``text`` with spaces after it up to ``cells`` terminal cells."""
    return text + " " * max(cells - width(text), 0)


def fit(text: str, columns: int, ellipsis: str) -> str:
    """``text`` (without colour codes) cut to at most ``columns`` terminal cells, ending in ``ellipsis`` when cut."""
    if width(text) <= columns:
        return text
    room = max(columns - width(ellipsis), 0)
    kept: list[str] = []
    used = 0
    for c in text:
        used += _char_width(c)
        if used > room:
            break
        kept.append(c)
    return "".join(kept) + ellipsis[: max(columns - room, 0)]


def terminal_columns(stream: TextIO) -> int:
    try:
        columns = os.get_terminal_size(stream.fileno()).columns
    except (AttributeError, ValueError, OSError):
        columns = 0
    return columns or shutil.get_terminal_size().columns or 80


def is_utf8(stream: TextIO) -> bool:
    try:
        return codecs.lookup(getattr(stream, "encoding", None) or "ascii").name == "utf-8"
    except LookupError:
        return False


def isatty(stream: TextIO | None) -> bool:
    try:
        return stream is not None and stream.isatty()
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


@dataclass(frozen=True)
class Status:
    """What the live line says: ``head`` (``  [1/2] repo``), an optional spinner, ``label``, an optional bar
    filled to ``fraction``, and ``tail``."""

    head: str
    label: str
    tail: str = ""
    fraction: float | None = None
    spin: bool = False


class LiveLine:
    """One status line on a terminal, redrawn in place.

    ``show`` replaces the status. It is drawn at once with ``force``, or when REDRAW_INTERVAL has passed since
    the last draw; otherwise the next draw shows it. ``write`` erases the line and writes text; no live line
    shows until the next ``show``. With ``animate``, a background thread redraws every REDRAW_INTERVAL, so the
    spinner turns and a skipped update is drawn; tests call ``tick`` instead. ``clock`` (default
    ``time.monotonic``) and ``columns`` (default: the terminal's width) are for tests.
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
        self._columns: Callable[[], int] = functools.partial(terminal_columns, out) if columns is None else columns
        self._animate = animate
        self._lock = threading.Lock()
        self._stop = threading.Event()
        self._thread: threading.Thread | None = None
        self._status: Status | None = None
        self._drawn = False
        self._dirty = False
        self.broken = False
        self._last = -math.inf

    def show(self, status: Status, *, force: bool = False) -> None:
        with self._lock:
            self._status = status
            self._dirty = True
            now = self._clock()
            if not self.broken and (force or now - self._last >= REDRAW_INTERVAL):
                self._draw(now)
        if self._animate and self._thread is None and not self._stop.is_set():
            self._thread = threading.Thread(target=self._run, name="amp-progress", daemon=True)
            self._thread.start()

    def tick(self) -> None:
        """Redraw if the spinner should turn or an update was not drawn yet."""
        with self._lock:
            if not self.broken and self._status is not None and (self._dirty or self._status.spin):
                self._draw(self._clock())

    def write(self, text: str) -> None:
        """Erase the line, then write ``text``."""
        with self._lock:
            self._erase()
            self._write(text)

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
            bar = min(_BAR_WIDTH, columns - width(line) - width(tail) - 2)
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
        """Write and flush ``text``. A closed or broken stream stops all output, so output never fails the scan."""
        if self.broken:
            return
        try:
            self._out.write(text)
            self._out.flush()
        except (OSError, ValueError):
            self.broken = True
            self._stop.set()

    def _run(self) -> None:
        while not self._stop.wait(REDRAW_INTERVAL):
            self.tick()


class Ui:
    """Writes the command's output to ``out`` (default ``sys.stderr``).

    ``live`` (default: ``out`` is a terminal and ``TERM`` is not ``dumb``) turns on the live line, in-place
    answers and colour; ``env`` (default ``os.environ``) is read for ``TERM`` and ``NO_COLOR``. ``clock``,
    ``animate`` (default: ``live``) and ``columns`` are passed to the live line.
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
        self.out: TextIO = sys.stderr if out is None else out
        env = os.environ if env is None else env
        self.tty = isatty(self.out)
        """The stream is a terminal, so what is typed at it is echoed there."""
        self.live = self.tty and env.get("TERM", "") != "dumb" if live is None else live
        self.colour = self.live and not env.get("NO_COLOR")
        self.glyphs = UNICODE if is_utf8(self.out) else ASCII
        self._columns: Callable[[], int] = functools.partial(terminal_columns, self.out) if columns is None else columns
        self._line = (
            LiveLine(
                self.out,
                glyphs=self.glyphs,
                clock=clock,
                columns=self._columns,
                animate=self.live if animate is None else animate,
            )
            if self.live
            else None
        )
        self.rows = 0
        """Rows written so far, for ``erase_since``."""

    # Writing.

    def write(self, text: str) -> None:
        """Write ``text`` as it is (a prompt, cursor movement), erasing the live line first."""
        if self._line is not None:
            self._line.write(text)
        else:
            self.out.write(text)
            self.out.flush()

    def say(self, line: str = "") -> None:
        """Print a permanent line, erasing the live line first."""
        self.write(line + "\n")
        self.count(line)

    def count(self, text: str) -> None:
        """Count the rows ``text``, ended by a newline, took (``say`` already does)."""
        if self.live:
            self.rows += max((width(text) - 1) // max(self._columns(), 1) + 1, 1)

    def erase_since(self, rows: int) -> None:
        """On a terminal, erase what was written after ``self.rows`` was ``rows`` (questions once answered)."""
        if self.live and self.rows > rows:
            self.write(f"\x1b[{self.rows - rows}A\r\x1b[J")
        self.rows = min(self.rows, rows)

    def show(self, status: Status, *, force: bool = False) -> None:
        """Show ``status`` on the live line (only on a terminal)."""
        if self._line is not None:
            self._line.show(status, force=force)

    def tick(self) -> None:
        if self._line is not None:
            self._line.tick()

    def clear(self) -> None:
        """Erase the live line, if one shows."""
        if self._line is not None:
            self._line.clear()

    def close(self) -> None:
        """Erase the live line and stop its spinner. Called at the end of a scan, and on Ctrl-C."""
        if self._line is not None:
            self._line.close()

    def columns(self) -> int:
        return self._columns()

    # Styles.

    def paint(self, text: str, code: str) -> str:
        return f"\x1b[{code}m{text}\x1b[0m" if self.colour and text else text

    def dim(self, text: str) -> str:
        return self.paint(text, DIM)

    def bold(self, text: str) -> str:
        return self.paint(text, BOLD)

    def mark(self, kind: Mark) -> str:
        glyph = {Mark.OK: self.glyphs.ok, Mark.WARN: self.glyphs.warn, Mark.FAIL: self.glyphs.fail}.get(
            kind, self.glyphs.ask
        )
        return self.paint(glyph, _MARK_COLOURS[kind])

    def join(self, *parts: str) -> str:
        """``parts`` joined with a dim `` · ``."""
        return self.dim(self.glyphs.sep).join(p for p in parts if p)

    # Layout.

    def header(self, title: str, detail: str) -> None:
        """The first line: ``AMP shadow agent scan · acme on github.com``."""
        self.say(self.bold(title) + self.dim(self.glyphs.sep) + clean(detail))

    def section(self, title: str) -> None:
        """A blank line and a section title."""
        self.say()
        self.say(self.paint(self.glyphs.section, CYAN) + " " + self.bold(title))

    def item(self, text: str) -> None:
        self.say(INDENT + text)

    def row(self, label: str, value: str) -> None:
        """``  Label      value``: values start in one column."""
        self.say(INDENT + self.dim(pad(label, max(LABEL_WIDTH, width(label) + 2))) + value)

    def note(self, text: str, *, indent: str = INDENT) -> None:
        """Dim secondary text."""
        for line in self._wrap(text, indent, indent):
            self.say(self.dim(line))

    def status(self, kind: Mark, text: str, *, hint: str = "", indent: str = INDENT) -> None:
        """``  ✓ text``, with an optional dim ``hint`` under the text. Wrapped at the terminal's width."""
        lines = self._wrap(text, indent + "  ", indent + "  ")
        self.say(indent + self.mark(kind) + " " + lines[0][len(indent) + 2 :])
        for line in lines[1:]:
            self.say(line)
        if hint:
            for line in self._wrap(hint, indent + "  ", indent + "  "):
                self.say(self.dim(line))

    def ok(self, text: str, *, hint: str = "", indent: str = INDENT) -> None:
        self.status(Mark.OK, text, hint=hint, indent=indent)

    def warn(self, text: str, *, hint: str = "", indent: str = INDENT) -> None:
        self.status(Mark.WARN, text, hint=hint, indent=indent)

    def fail(self, text: str, *, hint: str = "", indent: str = INDENT) -> None:
        self.status(Mark.FAIL, text, hint=hint, indent=indent)

    def question(self, label: str, *, default: str | None = None, indent: str = INDENT, reserve: int = 0) -> str:
        """The prompt ``  ? label (default) `` (the default dim), cut to leave ``reserve`` cells for the answer."""
        tail = f" ({default})" if default is not None else ""
        if self.live:  # One row, so it can be redrawn and erased. The default always shows; the label is cut.
            room = self.columns() - 1 - width(indent) - 2 - reserve - 1 - width(tail)
            label = fit(label, max(room, 4), self.glyphs.ellipsis)
        return f"{indent}{self.mark(Mark.ASK)} {self.bold(label)}{self.dim(tail)} "

    def _wrap(self, text: str, first: str, rest: str) -> list[str]:
        """``text`` after ``first``, its other lines after ``rest``. On a terminal each line is wrapped to the
        width, except an indented one (a command to copy); a long word such as an address is never split."""
        cells = max(self.columns() - 1, 20) if self.live else 0
        lines: list[str] = []
        for i, part in enumerate(text.split("\n")):
            lead = first if i == 0 else rest
            if not cells or part[:1].isspace():
                lines.append(lead + part)
            else:
                lines += textwrap.wrap(
                    part,
                    cells,
                    initial_indent=lead,
                    subsequent_indent=rest,
                    break_on_hyphens=False,
                    break_long_words=False,
                ) or [lead]
        return lines
