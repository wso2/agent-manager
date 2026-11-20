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

"""Key-by-key input on a terminal: hidden secrets shown as dots, and an arrow-key menu.

``TtyKeyboard`` puts the terminal in a mode with no echo, no line editing and no signal keys, and always
puts the old mode back (also on Ctrl-C). ``read_secret`` shows one dot for each character typed or pasted,
never the character itself; ``choose`` draws a menu and redraws it in place. Both raise KeyboardInterrupt on
Ctrl-C and EOFError on Ctrl-D (``read_secret`` only on an empty answer) or the end of input. Escape sequences
(arrow keys, bracketed paste markers) are read and ignored. Other control characters are kept in a secret, for
the caller to refuse it, rather than dropped without a word. When Enter ends a question, input typed or pasted
after it is discarded, so it cannot answer the next question. SIGTERM and SIGHUP exit (status 128 + the
signal) through the ``finally`` blocks, so the terminal's mode and cursor are put back then too.

Use them only when ``keyboard_for`` gives a keyboard and the ``Ui`` is live; otherwise ask with a line read.
"""

import codecs
import copy
import os
import signal
import threading
from collections.abc import Callable, Iterator, Sequence
from contextlib import contextmanager
from types import FrameType
from typing import Any, Protocol, TextIO

from amp_shadow_runner.terminal import CYAN, HIDE_CURSOR, SHOW_CURSOR, Ui, width

try:
    import termios
except ImportError:  # pragma: no cover - not on Linux or macOS.
    termios = None  # type: ignore[assignment]

DOT_CAP = 24
"""The most dots shown for a secret; past it, the count is shown instead, so a long paste never wraps."""
UP, DOWN = "<up>", "<down>"
_KEYS = {"\x1b[A": UP, "\x1bOA": UP, "\x1b[B": DOWN, "\x1bOB": DOWN}
_ENTER = ("\r", "\n")
_BACKSPACE = ("\x7f", "\x08")
_CTRL_C, _CTRL_D, _CTRL_U = "\x03", "\x04", "\x15"
_MAX_SEQUENCE = 32


class Keyboard(Protocol):
    """Where ``read_secret`` and ``choose`` read keys from."""

    def raw(self) -> Any:
        """A context manager: key-by-key input with no echo inside it, the old mode back after it."""
        ...

    def read(self) -> str:
        """The next keys typed or pasted (at least one character), or ``""`` at the end of input."""
        ...

    def discard(self) -> None:
        """Drop input typed or pasted that was not read yet."""
        ...


class TtyKeyboard:
    """The terminal on ``fd``. ``get_mode``, ``set_mode``, ``flush`` and ``read_bytes`` are for tests."""

    def __init__(
        self,
        fd: int,
        *,
        get_mode: Callable[[int], list[Any]] | None = None,
        set_mode: Callable[[int, int, list[Any]], None] | None = None,
        flush: Callable[[int, int], None] | None = None,
        read_bytes: Callable[[int, int], bytes] = os.read,
    ) -> None:
        assert termios is not None
        self._fd = fd
        self._get = termios.tcgetattr if get_mode is None else get_mode
        self._set = termios.tcsetattr if set_mode is None else set_mode
        self._flush = termios.tcflush if flush is None else flush
        self._read = read_bytes
        # Undecodable bytes become U+FFFD, which the callers' ASCII checks refuse.
        self._decoder = codecs.getincrementaldecoder("utf-8")(errors="replace")

    @contextmanager
    def raw(self) -> Iterator[None]:
        assert termios is not None
        saved = self._get(self._fd)
        mode = copy.deepcopy(saved)
        mode[0] &= ~(termios.IXON | termios.ICRNL)  # Ctrl-S and Ctrl-Q are keys; Enter stays \r.
        mode[3] &= ~(termios.ECHO | termios.ICANON | termios.ISIG | termios.IEXTEN)  # Ctrl-C is a key too.
        mode[6][termios.VMIN] = 1
        mode[6][termios.VTIME] = 0
        self._set(self._fd, termios.TCSANOW, mode)
        try:
            yield
        finally:
            self._set(self._fd, termios.TCSANOW, saved)

    def read(self) -> str:
        while True:
            data = self._read(self._fd, 4096)
            if not data:
                return self._decoder.decode(b"", final=True)
            text = self._decoder.decode(data)
            if text:  # Not just the start of a character split across reads.
                return text

    def discard(self) -> None:
        assert termios is not None
        self._decoder.reset()
        try:
            self._flush(self._fd, termios.TCIFLUSH)
        except (OSError, termios.error):
            pass


def read_fresh_line(
    fd: int,
    *,
    flush: Callable[[int, int], None] | None = None,
    read_bytes: Callable[[int, int], bytes] = os.read,
) -> str:
    """A line typed on the terminal ``fd`` from now on (``""`` at the end of input). Input typed or pasted
    before, which nothing has read yet, is discarded first, so it can never answer this question. Read from
    the file descriptor, not ``sys.stdin``, whose buffer may also hold such input."""
    if termios is not None:
        try:
            (termios.tcflush if flush is None else flush)(fd, termios.TCIFLUSH)
        except (OSError, termios.error):
            pass
    data = b""
    while not data.endswith(b"\n"):
        chunk = read_bytes(fd, 1024)
        if not chunk:
            break
        data += chunk
    return data.decode(errors="replace")


@contextmanager
def exit_on_signals() -> Iterator[None]:
    """Inside it, SIGTERM and SIGHUP raise SystemExit(128 + the signal), so the ``finally`` blocks that put the
    terminal back run, then the command exits as if killed. Only in the main thread; a signal already ignored
    stays ignored. The old handlers are put back after it."""
    if threading.current_thread() is not threading.main_thread():
        yield
        return

    def leave(signum: int, frame: FrameType | None) -> None:
        raise SystemExit(128 + signum)

    saved: list[tuple[signal.Signals, Any]] = []
    try:
        for sig in (signal.SIGTERM, signal.SIGHUP):
            previous = signal.getsignal(sig)
            if previous is not signal.SIG_IGN:
                saved.append((sig, previous))
                signal.signal(sig, leave)
        yield
    finally:
        for number, previous in saved:
            signal.signal(number, previous if previous is not None else signal.SIG_DFL)


def keyboard_for(stream: TextIO | None) -> TtyKeyboard | None:
    """A keyboard for ``stream`` (stdin) when it is a terminal whose mode can be set, else None."""
    if termios is None or stream is None:
        return None
    try:
        fd = stream.fileno()
        if not os.isatty(fd):
            return None
        termios.tcgetattr(fd)
    except (AttributeError, ValueError, OSError, termios.error):
        return None
    return TtyKeyboard(fd)


class KeyDecoder:
    """Splits what is read into keys: a character, or ``UP``/``DOWN``. Other escape sequences are dropped,
    including one split across reads."""

    def __init__(self) -> None:
        self._sequence = ""

    def feed(self, text: str) -> list[str]:
        keys: list[str] = []
        for c in text:
            if not self._sequence:
                if c == "\x1b":
                    self._sequence = c
                else:
                    keys.append(c)
                continue
            self._sequence += c
            sequence = self._sequence
            if len(sequence) == 2:
                if c not in "[O":  # Esc, then a key: the Esc is dropped, the key kept.
                    self._sequence = ""
                    if c == "\x1b":
                        self._sequence = c
                    else:
                        keys.append(c)
                continue
            if sequence[1] == "O" or "\x40" <= c <= "\x7e" or len(sequence) > _MAX_SEQUENCE:
                self._sequence = ""
                if sequence in _KEYS:
                    keys.append(_KEYS[sequence])
        return keys


def dots(count: int, ui: Ui, cap: int = DOT_CAP) -> str:
    """One dot for each character, at most ``cap``; then the count too: ``••••…•• 164``."""
    dot = ui.glyphs.dot
    if count <= cap:
        return dot * count
    return dot * (cap - 1) + ui.glyphs.ellipsis + dot + ui.dim(f" {count}")


def read_secret(ui: Ui, label: str, keyboard: Keyboard) -> str:
    """Ask for a secret on the current line: ``  ? label ••••``. Returns what was typed (not stripped).

    The line is erased when Enter is pressed, so the caller can print the answer's row in its place. On
    Ctrl-C (KeyboardInterrupt) and Ctrl-D (EOFError) the line is left as it is. Nothing typed is ever written.
    Control characters other than the editing keys are kept (and shown as dots), for the caller to refuse.
    """
    prompt = ui.question(label, reserve=DOT_CAP + 6)
    # Room for the dots, an ellipsis and a 4-digit count, so the line never wraps.
    cap = max(min(DOT_CAP, ui.columns() - 1 - width(prompt) - 7), 1) if ui.live else DOT_CAP
    chars: list[str] = []
    decoder = KeyDecoder()
    with exit_on_signals(), keyboard.raw():
        ui.write("\r\x1b[K" + prompt)
        while True:
            text = keyboard.read()
            if not text:
                raise EOFError
            for key in decoder.feed(text):
                if key == _CTRL_C:
                    raise KeyboardInterrupt
                if key == _CTRL_D:
                    if not chars:
                        raise EOFError
                    continue  # As on a terminal line: ignored once something is typed.
                if key in _ENTER:
                    keyboard.discard()  # The rest of a paste must not answer the next question.
                    ui.write("\r\x1b[K")
                    return "".join(chars)
                if key in _BACKSPACE:
                    if chars:
                        chars.pop()
                elif key == _CTRL_U:
                    chars.clear()
                elif len(key) == 1:
                    chars.append(key)
            ui.write("\r\x1b[K" + prompt + dots(len(chars), ui, cap))


def choose(ui: Ui, label: str, options: Sequence[str], keyboard: Keyboard, *, default: int = 0) -> int:
    """Ask to choose one of ``options`` with the arrow keys (or j and k, or its number) and Enter. Returns its
    index. The menu is erased once chosen."""
    index = default
    start = ui.rows
    decoder = KeyDecoder()
    hint = ui.dim(f"  {ui.glyphs.keys} and Enter")

    def draw() -> None:
        ui.erase_since(start)
        ui.say(ui.question(label, reserve=width(hint)).rstrip() + hint)
        for i, option in enumerate(options):
            if i == index:
                ui.say("    " + ui.paint(f"{ui.glyphs.pointer} {option}", CYAN))
            else:
                ui.say(f"      {option}")

    ui.write(HIDE_CURSOR)
    try:
        with exit_on_signals(), keyboard.raw():
            draw()
            while True:
                text = keyboard.read()
                if not text:
                    raise EOFError
                for key in decoder.feed(text):
                    if key == _CTRL_C:
                        raise KeyboardInterrupt
                    if key == _CTRL_D:
                        raise EOFError
                    if key in _ENTER:
                        keyboard.discard()
                        ui.erase_since(start)
                        return index
                    if key in (UP, "k"):
                        index = (index - 1) % len(options)
                    elif key in (DOWN, "j"):
                        index = (index + 1) % len(options)
                    elif len(key) == 1 and key in "123456789" and int(key) <= len(options):
                        index = int(key) - 1
                draw()
    finally:
        ui.write(SHOW_CURSOR)
