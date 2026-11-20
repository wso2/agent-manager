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


"""amp_shadow_runner.terminal: sections, rows, marks, colour and glyphs."""

import io
import re

import pytest

from amp_shadow_runner.terminal import Mark, Ui, clean, fit, number, pad, plural, width


class Log(io.StringIO):
    """A stream that is not a terminal, in ``encoding``."""

    def __init__(self, encoding: str = "utf-8") -> None:
        super().__init__()
        self._encoding = encoding

    @property
    def encoding(self) -> str:  # type: ignore[override]
        return self._encoding


class Tty(Log):
    def isatty(self) -> bool:
        return True


def screen(text: str) -> str:
    """What a terminal shows after ``text``: carriage returns, erases and cursor-up moves applied, colour and
    cursor visibility codes dropped."""
    lines, row, col = [""], 0, 0
    i = 0
    while i < len(text):
        match = re.match(r"\x1b\[(\??)(\d*)([A-Za-z])", text[i:])
        if match:
            code = match.group(3)
            if code == "K":
                lines[row] = lines[row][:col]
            elif code == "A":
                row = max(row - int(match.group(2) or 1), 0)
            elif code == "J":
                lines[row] = lines[row][:col]
                del lines[row + 1 :]
            i += match.end()
            continue
        c = text[i]
        i += 1
        if c == "\r":
            col = 0
        elif c == "\n":
            row, col = row + 1, 0
            if row == len(lines):
                lines.append("")
        else:
            line = lines[row].ljust(col)
            lines[row] = line[:col] + c + line[col + 1 :]
            col += 1
    return "\n".join(lines)


def _layout(ui: Ui) -> None:
    ui.header("AMP shadow agent scan", "acme on github.com")
    ui.section("Repositories (2)")
    ui.item("support-bot")
    ui.item(pad("billing", 14) + ui.dim("private"))
    ui.warn("No GitHub token: about 25 repositories an hour.", hint="Pass -e GITHUB_TOKEN for more.")
    ui.section("Results")
    ui.row("Agents", "6: 5 proven, 1 confirmed by AI")
    ui.row("API key", f"{ui.mark(Mark.OK)} received (164 characters)")
    ui.fail("legacy   download failed")
    ui.note("A dim note.")
    ui.say()
    ui.ok("Published to Agent Manager: 6 agents saved.", indent="")
    ui.write(ui.question("Scan 2 repositories?", default="y/N", indent="") + "\n")


PLAIN = """AMP shadow agent scan · acme on github.com

◆ Repositories (2)
  support-bot
  billing       private
  ! No GitHub token: about 25 repositories an hour.
    Pass -e GITHUB_TOKEN for more.

◆ Results
  Agents     6: 5 proven, 1 confirmed by AI
  API key    ✓ received (164 characters)
  ✗ legacy   download failed
  A dim note.

✓ Published to Agent Manager: 6 agents saved.
? Scan 2 repositories? (y/N) 
"""


def test_plain_layout_off_a_terminal() -> None:
    out = Log()
    _layout(Ui(out))
    assert out.getvalue() == PLAIN


def test_a_terminal_with_no_color_has_the_same_layout_and_no_escape_codes() -> None:
    out = Tty()
    _layout(Ui(out, env={"TERM": "xterm", "NO_COLOR": "1"}, columns=lambda: 100))
    assert out.getvalue() == PLAIN


def test_colour_on_a_terminal() -> None:
    out = Tty()
    _layout(Ui(out, env={"TERM": "xterm"}, columns=lambda: 100))
    text = out.getvalue()
    for coloured in (
        "\x1b[32m✓\x1b[0m",
        "\x1b[33m!\x1b[0m",
        "\x1b[31m✗\x1b[0m",
        "\x1b[36m?\x1b[0m",
        "\x1b[36m◆\x1b[0m \x1b[1mResults\x1b[0m",
        "\x1b[2mAgents     \x1b[0m6: 5 proven",  # Labels are dim.
        "\x1b[2mprivate\x1b[0m",
        "\x1b[2m · \x1b[0macme",
        "\x1b[2m (y/N)\x1b[0m",
    ):
        assert coloured in text
    assert screen(text) == PLAIN.rstrip("\n").replace("(y/N) \n", "(y/N) ") + "\n"


@pytest.mark.parametrize("env", [{"TERM": "dumb"}, {"TERM": "xterm", "NO_COLOR": "1"}])
def test_no_colour_on_a_dumb_terminal_or_with_no_color(env: dict[str, str]) -> None:
    out = Tty()
    _layout(Ui(out, env=env))
    assert "\x1b" not in out.getvalue()


def test_ascii_on_a_stream_that_is_not_utf8() -> None:
    out = Tty("latin-1")
    _layout(Ui(out, env={"TERM": "xterm", "NO_COLOR": "1"}, columns=lambda: 100))
    text = out.getvalue()
    assert all(ord(c) < 128 for c in text)
    assert text.splitlines()[:3] == ["AMP shadow agent scan - acme on github.com", "", "* Repositories (2)"]
    assert "  API key    v received (164 characters)\n" in text
    assert "  x legacy" in text and "  ! No GitHub token" in text and "? Scan 2" in text


def test_status_rows_wrap_at_the_terminal_width_only_on_a_terminal() -> None:
    text = "The list may be incomplete, so Agent Manager will not mark any agent as gone."
    live, plain = Tty(), Log()
    Ui(live, env={"TERM": "xterm", "NO_COLOR": "1"}, columns=lambda: 40).warn(text, hint="A hint that wraps as well.")
    Ui(plain).warn(text)
    assert live.getvalue() == (
        "  ! The list may be incomplete, so\n"  # 39 cells at most, so the last column is never written.
        "    Agent Manager will not mark any\n"
        "    agent as gone.\n"
        "    A hint that wraps as well.\n"
    )
    assert plain.getvalue() == f"  ! {text}\n"


def test_erase_since_removes_rows_only_on_a_terminal() -> None:
    live, plain = Tty(), Log()
    for out in (live, plain):
        ui = Ui(out, env={"TERM": "xterm", "NO_COLOR": "1"}, columns=lambda: 20)
        ui.say("kept")
        start = ui.rows
        ui.say("a question")
        ui.say("x" * 25)  # Two rows on a 20-column terminal.
        ui.erase_since(start)
        ui.say("answer")
        assert ui.rows == (start + 1 if out is live else 0)
    assert live.getvalue().endswith("x\n\x1b[3A\r\x1b[Janswer\n")
    assert "\x1b" not in plain.getvalue()


def test_on_a_narrow_terminal_the_question_is_cut_and_the_default_kept() -> None:
    ui = Ui(Tty(), env={"TERM": "xterm", "NO_COLOR": "1"}, columns=lambda: 30)
    prompt = ui.question("Scan 1,200 repositories and publish the results?", default="y/N", indent="")
    assert prompt == "? Scan 1,200 reposito… (y/N) "
    assert width(prompt) < 30
    long_default = ui.question("LLM address", default="https://api.openai.com/v1")
    assert long_default.endswith(" (https://api.openai.com/v1) ")  # Never dropped, even when it is long.


def test_helpers() -> None:
    assert clean("evil\x1b]0;t\x07\n") == "evil?]0;t??"
    assert number(1194) == "1,194"
    assert (plural(1, "agent"), plural(1194, "file"), plural(2, "repository", "repositories")) == (
        "1 agent",
        "1,194 files",
        "2 repositories",
    )
    assert width("\x1b[2m界a\x1b[0m") == 3
    assert pad("界", 4) == "界  "
    assert fit("abcdef", 4, "…") == "abc…"


def test_a_command_and_an_address_in_a_message_are_never_split() -> None:
    address = "https://agent-manager.example.com/api/v1/shadow-agent-publisher/scans/3f2b/results"
    text = (
        f"Could not publish ({address} failed). Publish it with:\n  docker run --rm -it <image> upload --scan-id 3f2b"
    )
    live, plain = Tty(), Log()
    Ui(live, env={"TERM": "xterm", "NO_COLOR": "1"}, columns=lambda: 40).fail(text, indent="")
    Ui(plain).fail(text, indent="")
    assert live.getvalue() == (
        "✗ Could not publish\n"
        f"  ({address}\n"
        "  failed). Publish it with:\n"
        "    docker run --rm -it <image> upload --scan-id 3f2b\n"
    )
    assert plain.getvalue() == (
        f"✗ Could not publish ({address} failed). Publish it with:\n    docker run --rm -it <image> upload --scan-id 3f2b\n"
    )
