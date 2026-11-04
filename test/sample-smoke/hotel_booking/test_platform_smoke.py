"""Regression guards against passing a broken tool loop or unrelated/stale trace."""
import copy
import unittest
from unittest.mock import Mock, create_autospec, patch
from urllib.error import HTTPError

from platform_smoke import AmpClient, build_revision, trace_matches, validate_source

SHA = "1" * 40
REPO = "https://github.com/wso2/agent-manager"
MARKER = "hotel-smoke-" + "a" * 32


class SmokeGuards(unittest.TestCase):
    def test_trace_must_contain_this_invocation_on_a_model_span(self):
        self.assertFalse(trace_matches([], MARKER))
        self.assertFalse(trace_matches([{"attributes": {"gen_ai.operation.name": "chat", "content": "other request"}}], MARKER))
        self.assertFalse(trace_matches([{"attributes": {"gen_ai.operation.name": "execute_tool", "content": MARKER}}], MARKER))
        self.assertTrue(trace_matches([{"attributes": {"gen_ai.operation.name": "chat", "content": MARKER}}], MARKER))

    def test_source_requires_full_workflow_commit_and_correct_repository(self):
        build = {"buildParameters": {"repoUrl": REPO, "branch": "test-branch", "commitId": SHA[:8], "appPath": "/samples/hotel-booking-agent/agent"}}
        workflow = {"spec": {"workflow": {"parameters": {"repository": {"url": REPO, "revision": {"commit": SHA}}}}}}
        validate_source(build, workflow, SHA, REPO, "test-branch")
        with self.assertRaises(AssertionError):
            validate_source(build, workflow, SHA, REPO, "wrong-branch")
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

    def test_succeeded_must_reconcile_before_passing(self):
        client = create_autospec(AmpClient, instance=True)
        payloads = [{"builds": [{"buildName": "initial", "status": "Completed"}]},
                    {"buildName": "pinned"},
                    {"buildName": "pinned", "status": "Succeeded", "imageId": "image"},
                    {"buildName": "pinned", "status": "Completed", "imageId": "image"}]
        client._request.side_effect = [Mock(json=Mock(return_value=p)) for p in payloads]
        with patch("platform_smoke.time.sleep"):
            self.assertEqual(build_revision(client, "/builds", SHA)["status"], "Completed")
        self.assertEqual(client._request.call_count, 4)

    def test_stalled_reconciliation_identifies_initial_or_pinned_build(self):
        for initial in (True, False):
            with self.subTest(initial=initial):
                client = create_autospec(AmpClient, instance=True)
                detail = {"buildName": "initial" if initial else "pinned", "status": "Succeeded"}
                def respond(method, path, **kwargs):
                    if method == "POST":
                        payload = {"buildName": "pinned"}
                    elif path == "/builds":
                        payload = {"builds": [detail if initial else {"status": "Completed"}]}
                    else:
                        payload = detail
                    return Mock(json=Mock(return_value=payload))
                client._request.side_effect = respond
                clock = [0]
                def advance(_):
                    clock[0] += 301
                with patch("platform_smoke.time.monotonic", side_effect=lambda: clock[0]), patch("platform_smoke.time.sleep", side_effect=advance):
                    with self.assertRaisesRegex(TimeoutError, detail["buildName"] + " stayed Succeeded"):
                        build_revision(client, "/builds", SHA)


if __name__ == "__main__":
    unittest.main()
