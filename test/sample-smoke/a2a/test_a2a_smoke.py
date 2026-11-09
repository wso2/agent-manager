"""Prevent accepting protocol errors, failed tasks, wrong artifacts or stale calls."""
import copy
import unittest
from unittest.mock import patch
from a2a_smoke import check_task, check_evidence
from a2a_platform_smoke import SAMPLE
from platform_smoke import validate_source, project_deleted

MARKER = 'smoke-' + 'a' * 32

class Guards(unittest.TestCase):
    def test_rejects_protocol_errors_failed_tasks_and_wrong_artifacts(self):
        task = {'status': {'state': 'TASK_STATE_COMPLETED'}, 'artifacts': [
            {'name': 'summary.txt', 'parts': [{'text': MARKER + ': Priya will cut the release branch Thursday.'}]}]}
        check_task({'result': {'task': task}}, MARKER, 'summary.txt')
        for bad in ({'error': {'code': -32603}}, {'task': {**task, 'status': {'state': 'TASK_STATE_FAILED'}}},
                    {'task': {**task, 'artifacts': []}}, {'task': {**task, 'artifacts': [{'name': 'summary.txt', 'parts': [{'text': MARKER}]}]}}):
            with self.subTest(response=bad), self.assertRaises(AssertionError):
                check_task(bad, MARKER, 'summary.txt')

    def test_both_model_calls_must_belong_to_this_invocation(self):
        check_evidence({'calls': [{'marker': MARKER, 'skill': s} for s in ('summary', 'actions')]}, MARKER)
        for calls in ([], [{'marker': MARKER, 'skill': 'summary'}], [{'marker': 'stale', 'skill': s} for s in ('summary', 'actions')]):
            with self.assertRaises(AssertionError): check_evidence({'calls': calls}, MARKER)

    def test_a2a_source_path_and_exact_commit_are_required(self):
        sha = '1' * 40; repo = 'https://github.com/wso2/agent-manager'
        build = {'buildParameters': {'repoUrl': repo, 'branch': 'feature', 'commitId': sha[:8], 'appPath': '/' + SAMPLE['path']}}
        workflow = {'spec': {'workflow': {'parameters': {'repository': {'url': repo, 'revision': {'commit': sha}}}}}}
        validate_source(build, workflow, sha, repo, 'feature', '/samples/a2a-agent')
        for field, value in (('appPath', '/samples/other-agent'), ('commitId', '2' * 8)):
            wrong = copy.deepcopy(build); wrong['buildParameters'][field] = value
            with self.assertRaises(AssertionError): validate_source(wrong, workflow, sha, repo, 'feature', '/samples/a2a-agent')

    def test_project_cleanup_requires_actual_removal_and_propagates_errors(self):
        with patch('platform_smoke.kubectl') as kubectl:
            kubectl.return_value = '{"items":[{"metadata":{"deletionTimestamp":"now"}}]}'
            self.assertFalse(project_deleted('a2a-smoke-test'))
            kubectl.return_value = '{"items":[]}'
            self.assertTrue(project_deleted('a2a-smoke-test'))
            self.assertIn('metadata.name=a2a-smoke-test', kubectl.call_args.args)
            kubectl.side_effect = RuntimeError('cluster unavailable')
            with self.assertRaises(RuntimeError): project_deleted('a2a-smoke-test')

if __name__ == '__main__': unittest.main()
