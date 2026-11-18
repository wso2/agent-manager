"""Regression guards for the hotel booking platform smoke."""
import unittest
import urllib.request

from platform_smoke import HOTEL_BOOKING
from external_agent import span_sink

MARKER = "hotel-smoke-" + "a" * 32


class SmokeGuards(unittest.TestCase):
    def test_span_sink_counts_exports_that_carry_the_key(self):
        # Straight to the local sink, whatever proxy the environment names.
        direct = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        with span_sink("agent-key") as sink:
            url = f"http://127.0.0.1:{sink.server_port}/otel/v1/traces"
            for key in ("agent-key", "other-key", None):
                headers = {"Content-Type": "application/x-protobuf"}
                if key:
                    headers["x-amp-api-key"] = key
                with direct.open(urllib.request.Request(url, data=b"spans", headers=headers), timeout=5) as response:
                    self.assertEqual(response.status, 200)
            self.assertEqual(sink.exports, 3)
            self.assertEqual(sink.keyed_exports, 1)

    def test_marker_matches_what_the_model_stub_expects(self):
        self.assertTrue(MARKER.startswith(HOTEL_BOOKING.marker_prefix))
        self.assertLessEqual(len(HOTEL_BOOKING.name_prefix) + 12, 25)


if __name__ == "__main__":
    unittest.main()
