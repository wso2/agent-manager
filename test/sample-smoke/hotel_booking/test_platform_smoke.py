"""Regression guards against passing an unrelated or stale trace."""
import unittest

from platform_smoke import HOTEL_BOOKING
from external_agent import trace_matches

MARKER = "hotel-smoke-" + "a" * 32


class SmokeGuards(unittest.TestCase):
    def test_trace_must_contain_this_invocation_on_a_model_span(self):
        self.assertFalse(trace_matches([], MARKER))
        self.assertFalse(trace_matches([{"attributes": {"gen_ai.operation.name": "chat", "content": "other request"}}], MARKER))
        self.assertFalse(trace_matches([{"attributes": {"gen_ai.operation.name": "execute_tool", "content": MARKER}}], MARKER))
        self.assertTrue(trace_matches([{"attributes": {"gen_ai.operation.name": "chat", "content": MARKER}}], MARKER))

    def test_marker_matches_what_the_model_stub_expects(self):
        self.assertTrue(MARKER.startswith(HOTEL_BOOKING.marker_prefix))
        self.assertLessEqual(len(HOTEL_BOOKING.name_prefix) + 12, 25)


if __name__ == "__main__":
    unittest.main()
