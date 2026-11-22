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


"""The Scanning section: what each repository is doing while a scan runs, on the command's ``Ui``.

On a terminal, each repository has one live status line under the section, ``  [1/2] repo  ⠋ downloading``:
downloading (bytes and speed, with a bar when the size is known), extracting, scanning (files done of the
total) and AI checks (done of those due). It is redrawn in place at most every REDRAW_INTERVAL
seconds and cut to the terminal's width; a spinner turns while nothing else changes. When the repository
ends, the live line is replaced by its permanent row, with the time it took::

      ✓ support-bot   2 agents · 1 in test code · 120 files · 5.7s
      ✗ legacy-app    download failed (HTTP 502)

Names are padded to one column. Uploading shows a spinner too.

Anywhere else (CI, ``docker run`` without ``-t``, tests), each step prints one plain line
(``  [1/2] repo  downloading``) and the same permanent rows, with no carriage returns, escape codes, byte
counts or times: AI checks print one line when it starts, not one per review.
"""

import time
from collections.abc import Callable, Iterable, Mapping
from typing import TextIO

from amp_shadow_runner.engine.scan import ScanResult
from amp_shadow_runner.terminal import (
    INDENT,
    Mark,
    Status,
    Ui,
    clean,
    duration,
    number,
    pad,
    plural,
    size,
    width,
)

__all__ = ["NAME_WIDTH", "Progress", "duration", "size"]

NAME_WIDTH = 32
"""The widest the name column grows; a longer name pushes its row's details along."""


class Progress:
    """Writes the Scanning section to ``ui`` (default: a ``Ui`` on ``out``, by default ``sys.stderr``).

    ``live``, ``env``, ``animate`` and ``columns`` are passed to that ``Ui``. ``clock`` (default
    ``time.monotonic``) times the repositories and the scan, and drives the live line.
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
        ui: Ui | None = None,
    ) -> None:
        self.ui = Ui(out, live=live, env=env, clock=clock, animate=animate, columns=columns) if ui is None else ui
        self._clock = clock
        self._began = clock()
        self._repo_began = self._began
        self._download_began: float | None = None
        self._name_width = 0

    @property
    def live(self) -> bool:
        return self.ui.live

    def say(self, line: str = "") -> None:
        """Print a permanent line, erasing the live line first."""
        self.ui.say(line)

    def close(self) -> None:
        """Erase the live line and stop its spinner. Called at the end of a scan, and on Ctrl-C."""
        self.ui.close()

    def begin(self, names: Iterable[str] = ()) -> None:
        """Start the scan's clock, and size the name column for ``names``."""
        self._began = self._clock()
        self._name_width = min(max((width(clean(n)) for n in names), default=0), NAME_WIDTH)

    def elapsed(self) -> float | None:
        """How long the scan has taken, on a terminal (plain output has no times)."""
        return self._clock() - self._began if self.ui.live else None

    def listing(self, organization: str) -> None:
        """The repositories are listed (only a spinner, on a terminal)."""
        label = f"Listing the repositories of {clean(organization)}{self.ui.glyphs.ellipsis}"
        self.ui.show(Status("", label, spin=True), force=True)

    def _step(self, index: int, total: int, name: str, text: str) -> None:
        self.say(f"{INDENT}[{index}/{total}] {clean(name)}  {text}")

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
        head = f"{INDENT}[{index}/{total}] {clean(name)}"
        self.ui.show(Status(head, label, tail, fraction, spin), force=force)

    def downloading(self, index: int, total: int, name: str) -> None:
        """A repository starts: its commit is resolved and its archive downloaded."""
        self._repo_began = self._clock()
        self._download_began = None
        if not self.live:
            self._step(index, total, name, "downloading")
        else:
            self._show(index, total, name, "downloading", spin=True, force=True)

    def received(self, index: int, total: int, name: str, done: int, length: int | None) -> None:
        """``done`` bytes of the archive are saved, of ``length`` (None when the host did not say)."""
        if not self.live:
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
        if self.live:
            self._show(index, total, name, "extracting", spin=True, force=True)

    def scanning(self, index: int, total: int, name: str) -> None:
        if not self.live:
            self._step(index, total, name, "scanning")
        else:
            self._show(index, total, name, "scanning", spin=True, force=True)

    def scanning_files(self, index: int, total: int, name: str, done: int, count: int) -> None:
        """``done`` of the repository's ``count`` files are read."""
        if self.live:
            fraction = done / count if count else 1.0
            tail = f"{done}/{count} files"
            self._show(index, total, name, "scanning", tail=tail, fraction=fraction, force=done == count)

    def reviewing(self, index: int, total: int, name: str, n: int, due: int) -> None:
        """Before the n-th of a repository's ``due`` AI reviews. Off a terminal, only the first says so."""
        if self.live:
            fraction = (n - 1) / due
            self._show(index, total, name, "AI check", tail=f"{n - 1}/{due}", fraction=fraction, spin=True)
        elif n == 1:
            self._step(index, total, name, f"AI check of {plural(due, 'not-sure agent')}")

    def scanned(self, index: int, total: int, name: str, result: ScanResult) -> None:
        """The repository's permanent row: ``✓ name   2 agents · 1 in test code · 120 files · 5.7s``."""
        agents = sum(f.tag != "test" for f in result.findings)
        tagged = len(result.findings) - agents
        ui = self.ui
        parts = [plural(agents, "agent") if agents else "no agents"]
        if tagged:
            parts.append(f"{number(tagged)} in test code")
        parts.append(plural(result.files_scanned, "file"))
        if result.files_skipped:
            parts.append(f"{number(result.files_skipped)} skipped")
        if self.live:
            parts.append(ui.dim(duration(self._clock() - self._repo_began)))
        ui.say(f"{INDENT}{ui.mark(Mark.OK)} {self._name(name)}{ui.join(*parts)}")

    def failed(self, index: int, total: int, name: str, reason: str) -> None:
        """The permanent row of a repository that could not be scanned: ``✗ name   reason``."""
        self.ui.say(f"{INDENT}{self.ui.mark(Mark.FAIL)} {self._name(name)}{clean(reason)}")

    def publishing(self) -> None:
        """Uploading to Agent Manager starts; the next line printed replaces the spinner."""
        self.ui.show(Status("", f"Uploading to Agent Manager{self.ui.glyphs.ellipsis}", spin=True), force=True)

    def _name(self, name: str) -> str:
        return pad(clean(name), self._name_width) + "   "
