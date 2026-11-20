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

"""amp_shadow_runner.progress: progress lines on stderr."""

import io
from datetime import UTC, datetime

import pytest

from amp_shadow_runner.engine.classify import Evidence, Finding, Verdict
from amp_shadow_runner.engine.files import Tag
from amp_shadow_runner.engine.scan import ScanResult
from amp_shadow_runner.progress import Progress, duration, size
from amp_shadow_runner.report import Report, RepositoryReport, RepositoryStatus

NOW = datetime(2026, 11, 19, tzinfo=UTC)


def _finding(name: str, tag: Tag | None = None, verdict: Verdict = Verdict.PROVEN) -> Finding:
    return Finding(
        f"r:a.py:{name}",
        name,
        "r",
        "a.py",
        1,
        "crewai",
        verdict,
        tag,
        {"framework": "crewai"},
        (Evidence("d2_agent_import", "a.py", {"imports": 1}),),
    )


def _lines(write: object) -> list[str]:
    out = io.StringIO()
    assert callable(write)
    write(Progress(out))
    return out.getvalue().splitlines()


def test_repository_lines() -> None:
    result = ScanResult((_finding("a"), _finding("b", tag="example"), _finding("c", tag="test")), 120, 2, 0)

    def write(p: Progress) -> None:
        p.downloading(1, 3, "support-bot")
        p.scanning(1, 3, "support-bot")
        p.scanned(1, 3, "support-bot", result)
        p.scanned(2, 3, "tiny", ScanResult((_finding("x"),), 1, 0, 0))
        p.failed(3, 3, "legacy\x1b[31m", "download failed (HTTP 502)")

    assert _lines(write) == [
        "[1/3] support-bot: downloading",
        "[1/3] support-bot: scanning",
        "[1/3] support-bot: done, 2 agents (1 more in test code), 120 files scanned, 2 skipped",
        "[2/3] tiny: done, 1 agent, 1 file scanned",
        "[3/3] legacy?[31m: failed: download failed (HTTP 502)",
    ]


@pytest.mark.parametrize(
    ("repositories", "expected"),
    [
        (
            (
                RepositoryReport(
                    "a",
                    RepositoryStatus.SCANNED,
                    files_scanned=10,
                    files_skipped=1,
                    findings=(
                        _finding("x"),
                        _finding("y", verdict=Verdict.NOT_SURE),
                        _finding("e", tag="example"),
                        _finding("z", tag="test"),
                    ),
                ),
                RepositoryReport("b", RepositoryStatus.GONE),
                RepositoryReport("c", RepositoryStatus.FAILED, reason="r"),
            ),
            [
                "Scanned 2 repositories (1 not found, 1 failed).",
                "Found 3 agents: 2 proven, 1 not sure; 1 more in test code, not published.",
                "Files: 10 scanned, 1 skipped.",
                "Report: report.json in the current folder",
            ],
        ),
        (
            (RepositoryReport("a", RepositoryStatus.SCANNED, files_scanned=1),),
            [
                "Scanned 1 repository.",
                "Found 0 agents: 0 proven, 0 not sure.",
                "Files: 1 scanned, 0 skipped.",
                "Report: report.json in the current folder",
            ],
        ),
    ],
)
def test_summary(repositories: tuple[RepositoryReport, ...], expected: list[str]) -> None:
    report = Report("id", "github", "acme", NOW, NOW, repositories)
    assert _lines(lambda p: p.summary(report, "report.json in the current folder")) == expected


def test_default_stream_is_stderr(capsys: pytest.CaptureFixture[str]) -> None:
    Progress().scanning(1, 1, "r")
    captured = capsys.readouterr()
    assert (captured.out, captured.err) == ("", "[1/1] r: scanning\n")


class Tty(io.StringIO):
    """A terminal stand-in: ``isatty`` is True and the encoding is ``encoding``."""

    def __init__(self, encoding: str = "utf-8") -> None:
        super().__init__()
        self._encoding = encoding

    @property
    def encoding(self) -> str:  # type: ignore[override]
        return self._encoding

    def isatty(self) -> bool:
        return True


def _live(out: io.StringIO, now: list[float], *, columns: int = 100, env: dict[str, str] | None = None) -> Progress:
    return Progress(
        out, env={"TERM": "xterm"} if env is None else env, clock=lambda: now[0], animate=False, columns=lambda: columns
    )


def _frames(text: str) -> list[str]:
    """Each live frame drawn (the text between a carriage return and its erase-to-end-of-line)."""
    return [part.split("\x1b[K", 1)[0] for part in text.split("\r")[1:] if part.split("\x1b[K", 1)[0]]


def _run_one(p: Progress, now: list[float]) -> None:
    p.downloading(1, 2, "ag-ui-copy")
    for done in range(0, 4_000_001, 1_000_000):
        now[0] += 0.3
        p.received(1, 2, "ag-ui-copy", done, None)
    p.extracting(1, 2, "ag-ui-copy")
    p.scanning(1, 2, "ag-ui-copy")
    for done in range(0, 11):
        now[0] += 0.2
        p.scanning_files(1, 2, "ag-ui-copy", done, 10)
    for n in (1, 2):
        now[0] += 1
        p.reviewing(1, 2, "ag-ui-copy", n, 2)
    p.scanned(1, 2, "ag-ui-copy", ScanResult((_finding("a"), _finding("t", tag="test")), 10, 0, 0))
    p.publishing()
    p.say("Published 1 agent.")


def test_terminal_redraws_one_line_in_place_and_ends_with_the_permanent_line() -> None:
    out, now = Tty(), [100.0]
    _run_one(_live(out, now, env={"TERM": "xterm", "NO_COLOR": "1"}), now)
    text = out.getvalue()
    frames = _frames(text)
    assert frames[0] == "[1/2] ag-ui-copy  ⠋ downloading"
    assert any(f.endswith("downloading  4.0 MB  3.3 MB/s") for f in frames)
    assert any("extracting" in f for f in frames)
    assert any(f.endswith("scanning ██████████░░░░░░░░░░ 5/10 files") for f in frames)
    assert any(f.endswith("AI review ██████████░░░░░░░░░░ 1/2") for f in frames)
    assert "Publishing to Agent Manager…" in frames[-1]
    permanent = [line.rsplit("\x1b[K", 1)[-1] for line in text.split("\n")[:-1]]
    assert permanent == [
        "[1/2] ag-ui-copy: done in 5.7s, 1 agent (1 more in test code), 10 files scanned",
        "Published 1 agent.",
    ]
    assert text.endswith("\r\x1b[KPublished 1 agent.\n")  # The live line is erased before each permanent one.
    assert "AI review of" not in text  # No milestone line on a terminal.


def test_redraws_are_throttled_but_the_final_state_is_drawn() -> None:
    out, now = Tty(), [100.0]
    p = _live(out, now)
    p.scanning(1, 1, "r")
    for done in range(1001):  # All at one instant.
        p.scanning_files(1, 1, "r", done, 1000)
    frames = _frames(out.getvalue())
    assert len(frames) == 2  # "scanning", then only the forced final 1000/1000.
    assert frames[-1].endswith("1000/1000 files")
    now[0] += 0.05
    p.scanning(1, 1, "r")  # A new step is always drawn.
    p.scanning_files(1, 1, "r", 1, 1000)  # Too soon after: drawn by the next tick.
    assert len(_frames(out.getvalue())) == 3
    assert p._live is not None
    p._live.tick()
    assert _frames(out.getvalue())[-1].endswith("1/1000 files")


def test_the_live_line_is_cut_to_the_terminal_width() -> None:
    out, now = Tty(), [100.0]
    p = _live(out, now, columns=30)
    name = "a-very-long-repository-name-that-goes-on"
    p.downloading(1, 1, name)
    p.scanning_files(1, 1, name, 5, 10)
    now[0] += 1
    p.received(1, 1, name, 1_000_000, 2_000_000)
    frames = _frames(out.getvalue())
    assert frames and all(len(f) <= 29 for f in frames)
    assert all(f.endswith("…") for f in frames)


def test_the_bar_shrinks_to_fit_before_the_line_is_cut() -> None:
    out, now = Tty(), [100.0]
    p = _live(out, now, columns=40)
    p.scanning_files(1, 1, "repo", 5, 10)
    (frame,) = _frames(out.getvalue())
    assert frame.endswith(" 5/10 files") and "█" in frame and len(frame) == 39


def test_colour_only_without_no_color() -> None:
    coloured, plain, now = Tty(), Tty(), [100.0]
    for out, env in ((coloured, {"TERM": "xterm"}), (plain, {"TERM": "xterm", "NO_COLOR": "1"})):
        p = _live(out, now, env=env)
        p.scanned(1, 2, "a", ScanResult((), 1, 0, 0))
        p.failed(2, 2, "b", "HTTP 502")
    assert "\x1b[32mdone\x1b[0m" in coloured.getvalue() and "\x1b[31mfailed\x1b[0m" in coloured.getvalue()
    assert "\x1b[3" not in plain.getvalue() and "\x1b[2m" not in plain.getvalue()
    assert plain.getvalue().endswith("[2/2] b: failed: HTTP 502\n")


def test_ascii_on_a_stream_that_is_not_utf8() -> None:
    out, now = Tty("latin-1"), [100.0]
    p = _live(out, now)
    p.scanning(1, 1, "r")
    now[0] += 1
    p.scanning_files(1, 1, "r", 5, 10)
    p.publishing()
    text = out.getvalue()
    assert "|" in _frames(text)[0] and "##########----------" in text and "Agent Manager..." in text
    assert all(ord(c) < 128 for c in text)


@pytest.mark.parametrize("out", [io.StringIO(), Tty()], ids=["not a terminal", "TERM=dumb"])
def test_plain_lines_off_a_terminal(out: io.StringIO) -> None:
    now = [100.0]
    p = Progress(out, env={"TERM": "dumb"}, clock=lambda: now[0])
    _run_one(p, now)
    p.summary(Report("id", "github", "acme", NOW, NOW, ()), "report.json")
    text = out.getvalue()
    assert "\r" not in text and "\x1b" not in text
    assert text.splitlines()[:5] == [
        "[1/2] ag-ui-copy: downloading",
        "[1/2] ag-ui-copy: scanning",
        "[1/2] ag-ui-copy: AI review of 2 not-sure agents",  # Once, not per review.
        "[1/2] ag-ui-copy: done, 1 agent (1 more in test code), 10 files scanned",
        "Published 1 agent.",
    ]
    assert "Scanned 0 repositories." in text


def test_the_terminal_summary_says_how_long_the_scan_took() -> None:
    out, now = Tty(), [100.0]
    p = _live(out, now)
    p.begin()
    now[0] += 52
    p.summary(Report("id", "github", "acme", NOW, NOW, (RepositoryReport("c", RepositoryStatus.FAILED),)), "r")
    assert "Scanned 0 repositories in 52s (1 failed).\n" in out.getvalue()


def test_close_erases_the_live_line() -> None:
    out, now = Tty(), [100.0]
    p = _live(out, now)
    p.extracting(1, 1, "r")
    p.close()
    assert out.getvalue().endswith("\r\x1b[K")
    p.close()  # Nothing more to erase.
    assert out.getvalue().count("\r\x1b[K") == 1


class _BrokenTty(Tty):
    """A terminal whose writes fail once ``broken`` is set, as stderr does after its reader goes away."""

    broken = False

    def write(self, text: str) -> int:
        if self.broken:
            raise BrokenPipeError(32, "Broken pipe")
        return super().write(text)


def test_a_broken_terminal_never_fails_the_scan() -> None:
    out, now = _BrokenTty(), [100.0]
    p = _live(out, now)
    p.downloading(1, 2, "a")
    out.broken = True
    now[0] += 1
    p.received(1, 2, "a", 1000, None)
    p.failed(1, 2, "a", "HTTP 502")
    p.scanning(2, 2, "b")
    p.say("Published 1 agent.")
    p.close()


class _InterruptedTty(Tty):
    """A terminal whose next write raises KeyboardInterrupt, as Ctrl-C can in the middle of a draw."""

    interrupt = False

    def write(self, text: str) -> int:
        if self.interrupt:
            self.interrupt = False
            raise KeyboardInterrupt
        return super().write(text)


def test_ctrl_c_during_a_draw_still_leaves_a_clean_line() -> None:
    out, now = _InterruptedTty(), [100.0]
    p = _live(out, now)
    out.interrupt = True
    with pytest.raises(KeyboardInterrupt):
        p.downloading(1, 1, "r")
    p.close()
    assert out.getvalue().endswith("\r\x1b[K")


def test_a_name_cannot_inject_escape_codes_into_the_live_line() -> None:
    out, now = Tty(), [100.0]
    _live(out, now).downloading(1, 1, "evil\x1b]0;title\x07\r")
    assert _frames(out.getvalue()) == ["[1/1] evil?]0;title??  ⠋ downloading"]


@pytest.mark.parametrize(("seconds", "text"), [(3.24, "3.2s"), (31.4, "31s"), (125, "2m 05s"), (3720, "1h 02m")])
def test_duration(seconds: float, text: str) -> None:
    assert duration(seconds) == text


@pytest.mark.parametrize(("count", "text"), [(512, "512 B"), (12_400, "12.4 kB"), (3_100_000, "3.1 MB")])
def test_size(count: int, text: str) -> None:
    assert size(count) == text
