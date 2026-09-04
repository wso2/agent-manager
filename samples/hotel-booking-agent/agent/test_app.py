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

"""
Regression tests for app._sanitize_for_log.

app.py builds a real ChatOpenAI-backed graph at import time (via
graph.build_graph()), which needs OPENAI_API_KEY/PINECONE_API_KEY etc. to
even construct. Since _sanitize_for_log has no dependency on any of that,
`graph` is stubbed out before import so this file can run without secrets
or the full requirements.txt installed.
"""

import sys
import unittest
from unittest.mock import MagicMock

sys.modules.setdefault("graph", MagicMock())

from app import _sanitize_for_log  # noqa: E402


class SanitizeForLogTests(unittest.TestCase):
    def test_cr_is_escaped(self):
        self.assertEqual(_sanitize_for_log("a\rb"), "a\\rb")

    def test_lf_is_escaped(self):
        self.assertEqual(_sanitize_for_log("a\nb"), "a\\nb")

    def test_crlf_is_escaped(self):
        self.assertEqual(_sanitize_for_log("a\r\nb"), "a\\r\\nb")

    def test_c0_control_characters_are_removed(self):
        # NUL, BEL, and a handful of others from the C0 control range.
        for ch in ("\x00", "\x01", "\x07", "\x08", "\x0b", "\x0c", "\x1f"):
            with self.subTest(char=repr(ch)):
                self.assertEqual(_sanitize_for_log(f"a{ch}b"), "ab")

    def test_del_is_removed(self):
        self.assertEqual(_sanitize_for_log("a\x7fb"), "ab")

    def test_esc_is_removed(self):
        # ESC is the basis of ANSI/terminal escape sequences (e.g. a fake
        # color code) — must not survive into the log.
        self.assertEqual(_sanitize_for_log("a\x1b[31mb\x1b[0m"), "a[31mb[0m")

    def test_printable_content_is_unchanged(self):
        self.assertEqual(_sanitize_for_log("session-123_ABC"), "session-123_ABC")

    def test_unicode_content_is_unchanged(self):
        self.assertEqual(_sanitize_for_log("héllo wörld — ok"), "héllo wörld — ok")

    def test_empty_string_is_unchanged(self):
        self.assertEqual(_sanitize_for_log(""), "")


if __name__ == "__main__":
    unittest.main()
