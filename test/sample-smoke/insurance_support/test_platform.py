"""Regression guards against passing a broken tool loop or unrelated/stale trace."""
import copy
import unittest
from unittest.mock import Mock, create_autospec, patch
from urllib.error import HTTPError

from support import check_evidence, local_model, request
from platform_smoke import AmpClient, build_revision, ready_pod, trace_matches, validate_source

SHA = "1" * 40
REPO = "https://github.com/wso2/agent-manager"
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

    def test_health_probe_requires_ready_pinned_image(self):
        pod = {"metadata": {}, "spec": {"containers": [{"image": "old"}]},
               "status": {"conditions": [{"type": "Ready", "status": "True"}]}}
        self.assertIsNone(ready_pod([pod], "pinned"))
        pod["spec"]["containers"][0]["image"] = "pinned"
        self.assertIs(ready_pod([pod], "pinned"), pod)
        pod["status"]["conditions"][0]["status"] = "False"
        self.assertIsNone(ready_pod([pod], "pinned"))
        pod["status"]["conditions"][0]["status"] = "True"
        pod["metadata"]["deletionTimestamp"] = "terminating"
        self.assertIsNone(ready_pod([pod], "pinned"))

    def test_trace_must_contain_this_invocation_on_a_model_span(self):
        self.assertFalse(trace_matches([], MARKER))
        self.assertFalse(trace_matches([{"attributes": {"gen_ai.operation.name": "chat", "content": "other request"}}], MARKER))
        self.assertFalse(trace_matches([{"attributes": {"gen_ai.operation.name": "execute_tool", "content": MARKER}}], MARKER))
        self.assertTrue(trace_matches([{"attributes": {"gen_ai.operation.name": "chat", "content": MARKER}}], MARKER))

    def test_source_requires_full_workflow_commit_and_correct_repository(self):
        build = {"buildParameters": {"repoUrl": REPO, "branch": "test-branch", "commitId": SHA[:8], "appPath": "/samples/insurance-support-agent/agent"}}
        workflow = {"spec": {"workflow": {"parameters": {"repository": {"url": REPO, "revision": {"commit": SHA}}}}}}
        validate_source(build, workflow, SHA, REPO, "test-branch")
        wrong = copy.deepcopy(workflow)
        wrong["spec"]["workflow"]["parameters"]["repository"]["revision"]["commit"] = SHA[:8] + "2" * 32
        with self.assertRaises(AssertionError):
            validate_source(build, wrong, SHA, REPO, "test-branch")
        with self.assertRaises(AssertionError):
            validate_source(build, workflow, SHA, REPO + "-other", "test-branch")

    def test_pinned_build_waits_for_initial_build_and_polls_returned_id(self):
        client = create_autospec(AmpClient, instance=True)
        payloads = [
            {"builds": [{"buildName": "initial", "status": "Running"}]},
            {"builds": [{"buildName": "initial", "status": "Failed"}]},
            {"buildName": "pinned"},
            {"buildName": "pinned", "status": "Running"},
            {"buildName": "pinned", "status": "Completed", "imageId": "pinned-image"},
        ]
        client._request.side_effect = [Mock(json=Mock(return_value=p)) for p in payloads]
        with patch("platform_smoke.time.sleep"):
            result = build_revision(client, "/builds", SHA)
        self.assertEqual(result["imageId"], "pinned-image")
        calls = client._request.call_args_list
        self.assertEqual(calls[2].args, ("POST", "/builds?commitId=" + SHA))
        self.assertEqual([c.args for c in calls[3:]], [("GET", "/builds/pinned")] * 2)

    def test_failed_or_imageless_pinned_build_cannot_pass(self):
        for detail in ({"status": "Failed"}, {"status": "Completed"}):
            with self.subTest(detail=detail):
                client = create_autospec(AmpClient, instance=True)
                client._request.side_effect = [Mock(json=Mock(return_value=p)) for p in (
                    {"builds": [{"status": "Completed"}]}, {"buildName": "pinned"}, detail)]
                with self.assertRaises((RuntimeError, AssertionError)):
                    build_revision(client, "/builds", SHA)


if __name__ == "__main__":
    unittest.main()
