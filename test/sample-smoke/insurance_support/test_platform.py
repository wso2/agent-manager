"""Regression guards against passing a broken tool loop."""
import unittest
from urllib.error import HTTPError

from support import check_evidence, local_model, request
from platform_smoke import INSURANCE_SUPPORT

MARKER = "smoke-" + "a" * 32


class SmokeGuards(unittest.TestCase):
    def test_stub_requires_real_tool_result(self):
        with local_model() as (_, url):
            messages = [{"role": "user", "content": MARKER}]
            body = {"model": "gpt-4o-mini", "messages": messages,
                    "tools": [{"type": "function", "function": {"name": "list_policies"}}]}
            first = request(url + "/v1/chat/completions", body)
            call = first["choices"][0]["message"]["tool_calls"][0]
            self.assertEqual(call["function"]["name"], "list_policies")
            with self.assertRaises(AssertionError):
                check_evidence(request(url + "/evidence"), MARKER)
            messages.extend([first["choices"][0]["message"], {"role": "tool", "tool_call_id": call["id"], "content": '{"policies":[{"policy_number":"OZ-AUTO-4417"},{"policy_number":"OZ-HOME-2280"},{"policy_number":"OZ-TRAV-9153"}]}'}])
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
        self.assertLessEqual(len(INSURANCE_SUPPORT.name_prefix) + 12, 25)


if __name__ == "__main__":
    unittest.main()
