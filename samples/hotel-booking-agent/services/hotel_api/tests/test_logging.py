"""Regression coverage for policy logs through the application's logging setup."""

import importlib
import io
import json
import logging
import unittest
from types import SimpleNamespace
from unittest.mock import Mock, patch

import policies
import service


class PolicyLoggingTests(unittest.TestCase):
    def setUp(self):
        """Capture application logs while preserving the host logging configuration."""
        root = logging.getLogger()
        previous_handlers, previous_level = root.handlers[:], root.level
        self.output = io.StringIO()
        self.handler = logging.StreamHandler(self.output)
        root.handlers = [self.handler]
        root.setLevel(logging.INFO)

        def restore_logging():
            """Return the host logger to its original handlers and level."""
            root.handlers = previous_handlers
            root.setLevel(previous_level)
            self.handler.close()

        self.addCleanup(restore_logging)
        # Exercise the service's actual setup, including an existing host handler.
        importlib.reload(service)
        self.assertEqual(root.handlers, [self.handler])

    def read_event(self):
        """Require a complete JSON event on exactly one physical line."""
        lines = self.output.getvalue().splitlines()
        self.assertEqual(len(lines), 1, "one complete JSON event per physical line")
        event = json.loads(lines[0])
        self.assertTrue(event["time"])
        return event

    def test_default_handler_contains_message_line_breaks(self):
        """Keep message line breaks inside the default handler JSON event."""
        root = logging.getLogger()
        root.handlers = []
        with patch("sys.stderr", self.output):
            importlib.reload(service)
        for handler in root.handlers:
            self.addCleanup(handler.close)
        logging.getLogger("policies").warning("message\nforged")
        event = self.read_event()
        self.assertEqual(event["level"], "WARNING")
        self.assertEqual(event["message"], "message\nforged")

    def test_policy_search_success_contains_hostile_hotel_ids(self):
        """Preserve successful search results while containing hostile identifiers."""
        for hotel_id in [
            "hotel\r\nERROR forged event",
            'hotel","level":"ERROR","message":"forged',
            'hotel\x00\x1b\t\\"\u2028\u2029',
        ]:
            with self.subTest(hotel_id=hotel_id):
                self.output.seek(0)
                self.output.truncate(0)
                doc = SimpleNamespace(page_content="policy document", metadata={"page": 2})
                store = Mock()
                store.similarity_search.return_value = [doc]
                with patch.object(policies, "_store", store), patch.object(policies, "_backend", "pinecone"):
                    result = policies.search_policies_route(hotel_id, "private query", k=3)
                self.assertEqual(result["hotel_id"], hotel_id)
                self.assertEqual(result["results"], [{"text": "policy document", "page": 2}])
                store.similarity_search.assert_called_once_with(
                    "private query", k=3, filter={"hotel_id": {"$eq": hotel_id}}
                )
                event = self.read_event()
                self.assertEqual(event["level"], "INFO")
                self.assertEqual(event["logger"], "policies")
                self.assertEqual(
                    event["message"],
                    "policy search (pinecone) returned 1 results for hotel_id=%r" % hotel_id,
                )
                self.assertNotIn("private query", self.output.getvalue())
                self.assertNotIn("policy document", self.output.getvalue())

    def test_policy_search_failure_contains_exception_and_preserves_response(self):
        """Keep exception details in one event without changing the error response."""
        hotel_id = "hotel\r\nERROR forged event"
        upstream_error = 'upstream\r\nERROR forged failure\x1b\x00\u2028\u2029'
        store = Mock()
        store.similarity_search.side_effect = RuntimeError(upstream_error)
        with patch.object(policies, "_store", store), patch.object(policies, "_backend", "pinecone"):
            result = policies.search_policies_route(hotel_id, "private query", k=3)
        self.assertEqual(result.status_code, 500)
        self.assertEqual(json.loads(result.body), {
            "message": "Policy search failed", "error_code": "POLICY_SEARCH_FAILED",
        })
        event = self.read_event()
        self.assertEqual(event["level"], "ERROR")
        self.assertEqual(event["logger"], "policies")
        self.assertEqual(event["message"], "policy search failed: hotel_id=%r" % hotel_id)
        self.assertIn("Traceback (most recent call last)", event["exception"])
        self.assertIn("RuntimeError", event["exception"])
        self.assertIn(upstream_error, event["exception"])
        self.assertNotIn("private query", self.output.getvalue())

    def test_unconfigured_policy_search_remains_unavailable(self):
        """Preserve the unavailable response when policy search is not configured."""
        with patch.object(policies, "_store", None):
            result = policies.search_policies_route("hotel", "question", k=3)
        self.assertEqual(result.status_code, 503)
        self.assertEqual(json.loads(result.body)["error_code"], "POLICY_SEARCH_UNAVAILABLE")
        self.assertEqual(self.output.getvalue(), "")

    def test_stack_info_and_extra_fields_cannot_forge_records(self):
        """Keep stack details and extra fields from altering trusted event metadata."""
        logging.getLogger("policies").info(
            "message\nforged", stack_info=True,
            extra={"level": "ERROR", "logger": "forged", "exception": "forged"},
        )
        event = self.read_event()
        self.assertEqual(event["level"], "INFO")
        self.assertEqual(event["logger"], "policies")
        self.assertEqual(event["message"], "message\nforged")
        self.assertNotIn("exception", event)
        self.assertIn("Stack (most recent call last)", event["stack"])


if __name__ == "__main__":
    unittest.main()
