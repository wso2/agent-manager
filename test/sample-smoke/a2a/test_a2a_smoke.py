"""Prevent accepting protocol errors, failed tasks, wrong artifacts or stale calls."""
import unittest
from a2a_smoke import check_task, check_evidence
from a2a_platform_smoke import A2A
from external_agent import trace_matches

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

    def test_trace_must_contain_this_invocation_on_a_model_span(self):
        self.assertFalse(trace_matches([], MARKER))
        self.assertFalse(trace_matches([{'attributes': {'gen_ai.operation.name': 'chat', 'content': 'other request'}}], MARKER))
        self.assertTrue(trace_matches([{'attributes': {'gen_ai.operation.name': 'chat', 'content': MARKER}}], MARKER))

    def test_registered_names_fit_the_project_name_limit(self):
        self.assertLessEqual(len(A2A.name_prefix) + 12, 25)

if __name__ == '__main__': unittest.main()
