"""Regression guards against passing a broken tool loop or unrelated/stale trace."""
import json
from pathlib import Path
import tempfile
import unittest

from platform_smoke import MANUAL, check_evidence, model
from external_agent import request, trace_matches, without_instrumentation_pin  # noqa: E402 - path set by platform_smoke

MARKER = "smoke-" + "a" * 32
TOOL_RESULT = json.dumps({"word_count": 4})


class SmokeGuards(unittest.TestCase):
    def test_evidence_requires_tool_result_and_marker(self):
        with model() as url:
            request(url + "/v1/embeddings", {"model": "text-embedding-3-small", "input": [MARKER]})
            with self.assertRaises(AssertionError):
                check_evidence(request(url + "/evidence"), MARKER)
            request(url + "/v1/chat/completions", {"model": "gpt-4o-mini", "messages": [
                {"role": "user", "content": MARKER},
                {"role": "tool", "tool_call_id": "call_smoke", "content": TOOL_RESULT}]})
            check_evidence(request(url + "/evidence"), MARKER)
            with self.assertRaises(AssertionError):
                check_evidence(request(url + "/evidence"), "smoke-" + "b" * 32)

    def test_trace_must_contain_this_invocation_on_a_model_span(self):
        self.assertFalse(trace_matches([{"attributes": {"gen_ai.operation.name": "embeddings", "content": MARKER}}], MARKER))
        self.assertTrue(trace_matches([{"attributes": {"gen_ai.operation.name": "chat", "content": MARKER}}], MARKER))

    def test_checkout_instrumentation_replaces_the_sample_pin(self):
        with tempfile.TemporaryDirectory() as work:
            filtered = without_instrumentation_pin(MANUAL.requirements, work)
            self.assertNotEqual(filtered, MANUAL.requirements)
            lines = Path(filtered).read_text().splitlines()
            self.assertFalse([line for line in lines if line.startswith("amp-instrumentation")])
            self.assertIn("fastapi", Path(filtered).read_text())

    def test_runs_with_its_own_exporter(self):
        self.assertFalse(MANUAL.auto_instrument)

    def test_registered_names_fit_the_project_name_limit(self):
        self.assertLessEqual(len(MANUAL.name_prefix) + 12, 25)


if __name__ == "__main__":
    unittest.main()
