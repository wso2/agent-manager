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


"""amp_shadow_runner.keyboard: hidden secrets shown as dots, the arrow-key menu, and the terminal mode."""

import io
import os
import signal
import termios
from collections.abc import Callable, Iterator
from contextlib import contextmanager
from typing import Any

import pytest

from amp_shadow_runner.keyboard import (
    DOWN,
    UP,
    KeyDecoder,
    TtyKeyboard,
    choose,
    exit_on_signals,
    keyboard_for,
    read_fresh_line,
    read_secret,
)
from amp_shadow_runner.terminal import HIDE_CURSOR, SHOW_CURSOR, Ui
from tests.test_terminal import Tty, screen

SECRET = "sk-s3cr3t-llm-key"


class Keys:
    """A fake keyboard: each read gives the next chunk (``""`` once they run out), or calls it. Records the mode
    changes and discards; ``discard`` drops the next ``queued`` chunks, as if they were already typed."""

    def __init__(self, *chunks: str | Callable[[], str], queued: int = 0) -> None:
        self.chunks = list(chunks)
        self.queued = queued
        self.modes: list[str] = []

    @contextmanager
    def raw(self) -> Iterator[None]:
        self.modes.append("raw")
        try:
            yield
        finally:
            self.modes.append("restored")

    def read(self) -> str:
        chunk = self.chunks.pop(0) if self.chunks else ""
        return chunk() if callable(chunk) else chunk

    def discard(self) -> None:
        self.modes.append("discarded")
        del self.chunks[: self.queued]
        self.queued = 0


def _ui(out: Tty, *, columns: int = 80, colour: bool = False) -> Ui:
    env = {"TERM": "xterm"} if colour else {"TERM": "xterm", "NO_COLOR": "1"}
    return Ui(out, env=env, columns=lambda: columns)


def _frames(text: str) -> list[str]:
    """Each state of the secret's line (the text after each erase-line)."""
    return [part for part in text.split("\r\x1b[K")[1:]]


def _secret(*chunks: str, out: Tty | None = None) -> tuple[str, Tty, Keys]:
    out = Tty() if out is None else out
    keys = Keys(*chunks)
    return read_secret(_ui(out), "LLM API key", keys), out, keys


def test_each_character_shows_a_dot_and_enter_erases_the_line() -> None:
    value, out, keys = _secret(*SECRET, "\r")
    assert value == SECRET
    frames = _frames(out.getvalue())
    assert frames[0] == "  ? LLM API key "
    assert frames[3] == "  ? LLM API key •••"
    assert frames[-1] == ""  # Erased, for the caller to print the answer's row.
    assert SECRET not in out.getvalue() and "s3cr3t" not in out.getvalue()
    assert keys.modes == ["raw", "discarded", "restored"]


def test_the_rest_of_a_paste_after_enter_is_discarded() -> None:
    keys = Keys("token\ry\n", "y\n", queued=1)
    value = read_secret(_ui(Tty()), "Token", keys)
    assert value == "token"
    assert keys.chunks == []  # Input not read yet is dropped too, so it cannot answer the next question.
    assert keys.modes == ["raw", "discarded", "restored"]


def test_a_paste_in_one_read_is_drawn_once_and_ends_at_its_newline() -> None:
    value, out, _ = _secret(f"  {SECRET}\r\n")
    assert value == f"  {SECRET}"  # The caller strips it.
    assert len(_frames(out.getvalue())) == 2  # The prompt, then the erase on Enter.


def test_backspace_and_ctrl_u() -> None:
    assert _secret("abc", "\x7f", "\x08", "d\r")[0] == "ad"
    assert _secret("\x7f\x7fx\r")[0] == "x"  # Backspace on nothing does nothing.
    value, out, _ = _secret("wrong", "\x15", "right\n")
    assert value == "right"
    assert "  ? LLM API key \r" in out.getvalue() + "\r"  # Ctrl-U clears the dots.


def test_escape_sequences_are_ignored() -> None:
    value, out, _ = _secret("a\x1b[A\x1b[D\x1bOB", "\x1b[200~bc\x1b[201~", "\x1b", "[1;5C", "d\r")
    assert value == "abcd"
    assert "\x1b[A" not in out.getvalue()


def test_a_lone_escape_drops_only_itself() -> None:
    assert _secret("ab", "\x1b", "cd\r")[0] == "abcd"
    assert _secret("a\x1b\x1b[Ab\r")[0] == "ab"


def test_other_control_characters_are_kept_for_the_caller_to_refuse() -> None:
    value, out, _ = _secret("b\tc\x00\x1a", "\r")
    assert value == "b\tc\x00\x1a"
    assert "\t" not in out.getvalue() and "\x00" not in out.getvalue()
    assert _frames(out.getvalue())[-2].endswith("•••••")


def test_utf8_and_spaces_are_kept_for_the_caller_to_check() -> None:
    assert _secret("a bé\r")[0] == "a bé"


def test_the_dots_stop_at_the_cap_and_the_count_is_shown() -> None:
    value, out, _ = _secret("x" * 164, "\r")
    assert len(value) == 164
    assert _frames(out.getvalue())[1] == "  ? LLM API key " + "•" * 23 + "…• 164"


def test_ctrl_c_raises_and_restores_the_mode() -> None:
    keys = Keys("abc", "\x03")
    with pytest.raises(KeyboardInterrupt):
        read_secret(_ui(Tty()), "Token", keys)
    assert keys.modes == ["raw", "restored"]


@pytest.mark.parametrize("chunks", [("\x04",), ()], ids=["ctrl-d", "end of input"])
def test_ctrl_d_on_an_empty_answer_or_the_end_of_input_is_eof(chunks: tuple[str, ...]) -> None:
    keys = Keys(*chunks)
    with pytest.raises(EOFError):
        read_secret(_ui(Tty()), "Token", keys)
    assert keys.modes == ["raw", "restored"]


def test_ctrl_d_after_some_characters_is_ignored() -> None:
    assert _secret("ab\x04c\r")[0] == "abc"


def test_the_prompt_is_cut_so_the_dots_fit_one_row() -> None:
    out = Tty()
    read_secret(_ui(out, columns=40), "Azure DevOps personal access token (Code read)", Keys("x" * 100, "\r"))
    assert all(len(frame) < 40 for frame in _frames(out.getvalue()))


def test_decoder() -> None:
    decoder = KeyDecoder()
    assert decoder.feed("a\x1b[Ab\x1bOB") == ["a", UP, "b", DOWN]
    assert decoder.feed("\x1b") == []  # Split across reads.
    assert decoder.feed("[B\x1b[") == [DOWN]
    assert decoder.feed("1;5Cz") == ["z"]  # Ctrl-Right: dropped.
    assert decoder.feed("\x1bqy") == ["q", "y"]  # Esc, then q: only the Esc is dropped.


def _menu(*chunks: str, default: int = 0) -> tuple[int, Tty, Keys]:
    out, keys = Tty(), Keys(*chunks)
    return choose(_ui(out), "LLM provider", ("openai", "anthropic"), keys, default=default), out, keys


def test_the_menu_starts_on_the_default_and_moves_with_arrows_j_k_and_numbers() -> None:
    assert _menu("\r")[0] == 0
    assert _menu("\r", default=1)[0] == 1
    assert _menu("\x1b[B", "\r")[0] == 1
    assert _menu("\x1b[B\x1b[B\r")[0] == 0  # Wraps around.
    assert _menu("\x1b[A", "\r")[0] == 1
    assert _menu("j", "k", "j", "\n")[0] == 1
    assert _menu("2", "\r")[0] == 1
    assert _menu("9", "x", "\x1b[C", "\r")[0] == 0  # Other keys do nothing.


def test_the_menu_is_drawn_in_place_and_erased_once_chosen() -> None:
    _, out, keys = _menu("\x1b[B", "\r")
    text = out.getvalue()
    first = text.split("\x1b[3A")[0]
    assert screen(first) == "  ? LLM provider  ↑/↓ and Enter\n    › openai\n      anthropic\n"
    second = text.split("\x1b[3A\r\x1b[J")[1]
    assert screen(second).startswith("  ? LLM provider  ↑/↓ and Enter\n      openai\n    › anthropic\n")
    assert screen(text) == ""  # Erased: the caller prints the answer's row.
    assert text.startswith(HIDE_CURSOR) and text.endswith(SHOW_CURSOR)
    assert keys.modes == ["raw", "discarded", "restored"]


def test_the_chosen_option_is_cyan() -> None:
    out = Tty()
    choose(_ui(out, colour=True), "LLM provider", ("openai", "anthropic"), Keys("\r"))
    assert "\x1b[36m› openai\x1b[0m" in out.getvalue()


@pytest.mark.parametrize(("key", "error"), [("\x03", KeyboardInterrupt), ("\x04", EOFError), ("", EOFError)])
def test_cancelling_the_menu_restores_the_mode_and_the_cursor(key: str, error: type[BaseException]) -> None:
    out, keys = Tty(), Keys("\x1b[B", key) if key else Keys("\x1b[B")
    with pytest.raises(error):
        choose(_ui(out), "LLM provider", ("openai", "anthropic"), keys)
    assert keys.modes == ["raw", "restored"]
    assert out.getvalue().endswith(SHOW_CURSOR)
    assert screen(out.getvalue()).endswith("\n")  # The cursor is on a clean line.


class FakeTermios:
    """``tcgetattr`` and ``tcsetattr`` on a fake terminal; ``read`` gives ``data`` in turn."""

    def __init__(self, *data: bytes) -> None:
        self.mode: list[Any] = [0xFFFF, 0, 0, 0xFFFF, 0, 0, [b"\x00"] * 32]
        self.sets: list[list[Any]] = []
        self.flushes: list[int] = []
        self.data = list(data)

    def flush(self, fd: int, queue: int) -> None:
        self.flushes.append(queue)

    def get(self, fd: int) -> list[Any]:
        return [*self.mode[:6], list(self.mode[6])]

    def set(self, fd: int, when: int, mode: list[Any]) -> None:
        self.sets.append(mode)
        self.mode = mode

    def read(self, fd: int, count: int) -> bytes:
        item = self.data.pop(0) if self.data else b""
        if isinstance(item, BaseException):
            raise item
        return item


def test_tty_keyboard_turns_off_echo_lines_and_signals_and_always_restores() -> None:
    fake = FakeTermios(b"a")
    keyboard = TtyKeyboard(0, get_mode=fake.get, set_mode=fake.set, read_bytes=fake.read)
    saved = fake.get(0)
    with pytest.raises(KeyboardInterrupt), keyboard.raw():
        raw = fake.mode
        assert raw[3] & (termios.ECHO | termios.ICANON | termios.ISIG | termios.IEXTEN) == 0
        assert raw[0] & (termios.IXON | termios.ICRNL) == 0
        assert (raw[6][termios.VMIN], raw[6][termios.VTIME]) == (1, 0)
        raise KeyboardInterrupt
    assert fake.mode == saved  # Back as it was, also on Ctrl-C.
    assert len(fake.sets) == 2


def test_tty_keyboard_reads_utf8_split_across_reads() -> None:
    data = "é".encode()
    fake = FakeTermios(b"a" + data[:1], data[1:], b"\xff", b"")
    keyboard = TtyKeyboard(0, get_mode=fake.get, set_mode=fake.set, flush=fake.flush, read_bytes=fake.read)
    assert [keyboard.read(), keyboard.read(), keyboard.read(), keyboard.read()] == ["a", "é", "\ufffd", ""]
    keyboard.discard()
    assert fake.flushes == [termios.TCIFLUSH]


def test_a_fresh_line_discards_what_was_typed_ahead() -> None:
    fake = FakeTermios(b"y", b"es\n", b"ignored\n")
    assert read_fresh_line(0, flush=fake.flush, read_bytes=fake.read) == "yes\n"
    assert fake.flushes == [termios.TCIFLUSH]
    assert read_fresh_line(0, flush=fake.flush, read_bytes=FakeTermios().read) == ""  # Ctrl-D.


def test_keyboard_for_needs_a_terminal() -> None:
    assert keyboard_for(io.StringIO()) is None
    assert keyboard_for(None) is None


@pytest.mark.parametrize("sig", [signal.SIGTERM, signal.SIGHUP])
def test_sigterm_or_sighup_exits_through_the_restores(sig: int) -> None:
    before = signal.getsignal(sig)

    def killed() -> str:
        handler = signal.getsignal(sig)
        assert callable(handler)
        handler(sig, None)
        return "x"

    out, keys = Tty(), Keys("ab", killed)
    with pytest.raises(SystemExit) as raised:
        read_secret(_ui(out), "Token", keys)
    assert raised.value.code == 128 + sig
    assert keys.modes == ["raw", "restored"]
    assert signal.getsignal(sig) is before  # The old handler is back.

    out, keys = Tty(), Keys(killed)
    with pytest.raises(SystemExit):
        choose(_ui(out), "LLM provider", ("openai", "anthropic"), keys)
    assert keys.modes == ["raw", "restored"] and out.getvalue().endswith(SHOW_CURSOR)
    assert signal.getsignal(sig) is before


def test_a_real_sigterm_exits_and_an_ignored_signal_stays_ignored() -> None:
    with pytest.raises(SystemExit) as raised, exit_on_signals():
        os.kill(os.getpid(), signal.SIGTERM)
    assert raised.value.code == 128 + signal.SIGTERM
    before = signal.signal(signal.SIGHUP, signal.SIG_IGN)
    try:
        with exit_on_signals():
            assert signal.getsignal(signal.SIGHUP) is signal.SIG_IGN
    finally:
        signal.signal(signal.SIGHUP, before)
