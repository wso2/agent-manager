"""Regression guards against passing a broken tool loop."""
import unittest
from urllib.error import HTTPError

from smoke import check_evidence, local_model, request
from platform_smoke import IT_HELPDESK

MARKER = "smoke-" + "a" * 32


class SmokeGuards(unittest.TestCase):
    def test_stub_requires_real_tool_result(self):
        with local_model() as (_, url):
            messages = [{"role": "user", "content": MARKER}]
            body = {"model": "gpt-4o-mini", "messages": messages,
                    "tools": [{"type": "function", "function": {"name": "check_system_status"}}]}
            first = request(url + "/v1/chat/completions", body)
            call = first["choices"][0]["message"]["tool_calls"][0]
            self.assertEqual(call["function"]["name"], "check_system_status")
            with self.assertRaises(AssertionError):
                check_evidence(request(url + "/evidence"), MARKER)
            messages.extend([first["choices"][0]["message"], {"role": "tool", "tool_call_id": call["id"], "content": '[{"service":"VPN Gateway","status":"operational"}]'}])
            request(url + "/v1/chat/completions", body)
            check_evidence(request(url + "/evidence"), MARKER)

    def test_stub_rejects_wrong_tool_output(self):
        with local_model() as (_, url):
            with self.assertRaises(HTTPError):
                request(url + "/v1/chat/completions", {"model": "test", "messages": [
                    {"role": "user", "content": MARKER},
                    {"role": "tool", "tool_call_id": "call_" + MARKER, "content": '[]'}]})
            self.assertTrue(request(url + "/evidence")["errors"])

    def test_registered_names_fit_the_project_name_limit(self):
        self.assertLessEqual(len(IT_HELPDESK.name_prefix) + 12, 25)


if __name__ == "__main__":
    unittest.main()
