import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('summary', Path(__file__).parents[1] / 'dependabot_pr_summary.py')
summary = importlib.util.module_from_spec(spec)
spec.loader.exec_module(summary)


def pr(number=1, sha='abc', result='passed'):
    return dict(number=number, sha=sha, result=result, title='Update libraries',
                url=f'https://github.com/wso2/agent-manager/pull/{number}')


class SummaryTests(unittest.TestCase):


    def test_statuses_and_checks(self):
        completed = lambda conclusion: {'status': 'completed', 'conclusion': conclusion}
        self.assertTrue(summary.check_result([], []).startswith('pending'))
        self.assertEqual(summary.check_result([completed('cancelled')], []), 'failed / cancelled')
        self.assertEqual(summary.check_result([completed('skipped')], []), 'skipped / neutral')
        self.assertEqual(summary.check_result([completed('success')], [
            {'context': 'external', 'state': 'success'},
            {'context': 'external', 'state': 'failure'}]), 'passed')
        self.assertTrue(summary.check_result([completed('failure'), {'status': 'queued'}], []).startswith('pending'))

    def test_collect_head_and_merge_checks(self):
        item = {'number': 1, 'title': 'Update', 'html_url': 'https://github.com/owner/repo/pull/1',
                'head': {'sha': 'head'}, 'merge_commit_sha': 'merge'}
        for conclusion, expected in [('success', 'passed'), ('failure', 'failed / cancelled')]:
            with self.subTest(conclusion=conclusion), \
                 patch.object(summary, 'pull_requests', return_value=[item]), \
                 patch.object(summary, 'pages', side_effect=[[], [],
                     [{'status': 'completed', 'conclusion': conclusion}], []]) as pages:
                result = summary.collect('owner/repo')
                self.assertEqual(result[0]['result'], expected)
                self.assertEqual(pages.call_args_list[2].args[0],
                                 'repos/owner/repo/commits/merge/check-runs?filter=latest')

    def test_collect_does_not_hide_same_context_failure_across_commits(self):
        item = {'number': 1, 'title': 'Update', 'html_url': 'https://github.com/owner/repo/pull/1',
                'head': {'sha': 'head'}, 'merge_commit_sha': 'merge'}
        with patch.object(summary, 'pull_requests', return_value=[item]), \
             patch.object(summary, 'pages', side_effect=[[],
                 [{'context': 'ci', 'state': 'failure'}], [],
                 [{'context': 'ci', 'state': 'success'}]]):
            self.assertEqual(summary.collect('owner/repo')[0]['result'], 'failed / cancelled')

    def test_collect_missing_or_identical_merge_sha_queries_head_once(self):
        for merge_sha in [None, 'head']:
            item = {'number': 1, 'title': 'Update', 'html_url': 'https://github.com/owner/repo/pull/1',
                    'head': {'sha': 'head'}, 'merge_commit_sha': merge_sha}
            with self.subTest(merge_sha=merge_sha), \
                 patch.object(summary, 'pull_requests', return_value=[item]), \
                 patch.object(summary, 'pages', side_effect=[
                     [{'status': 'completed', 'conclusion': 'success'}], []]) as pages:
                self.assertEqual(summary.collect('owner/repo')[0]['result'], 'passed')
                self.assertEqual(pages.call_count, 2)

    def test_changed_merge_commit_does_not_report_old_success(self):
        item = dict(pr(), merge_sha='old-merge')
        current = [{'number': 1, 'title': 'Update', 'head': {'sha': 'abc'},
                    'merge_commit_sha': 'new-merge'}]
        text = self.run_summary([item], current)
        self.assertIn('pending / head or merge commit changed', text)
        self.assertNotIn('passed', text)

    def test_pagination(self):
        with patch.object(summary, 'api', side_effect=[{'check_runs': [0] * 100}, {'check_runs': [1]}]) as api:
            self.assertEqual(len(summary.pages('checks?filter=latest', 'check_runs')), 101)
            self.assertIn('&per_page=100&page=2', api.call_args.args[0])

    def test_only_ready_dependabot_prs(self):
        def raw(login, draft=False):
            return {'user': {'login': login}, 'draft': draft}
        with patch.object(summary, 'pages', return_value=[raw('dependabot[bot]'), raw('human'), raw('other[bot]'), raw('dependabot[bot]', True)]):
            self.assertEqual(len(summary.pull_requests('owner/repo')), 1)

    def test_message_escapes_title_but_keeps_pr_link(self):
        item = pr()
        item['title'] = '<users/all>\n*hello*'
        text = summary.message([item])
        self.assertNotIn('<users/all>', text)
        self.assertIn('|#1>', text)
        self.assertIn('passed', text)


    def run_summary(self, collected, current):
        with patch.dict('os.environ', {'GCHAT_PR_WEBHOOK_URL': 'fake', 'GITHUB_REPOSITORY': 'owner/repo'}), \
             patch.object(summary, 'collect', return_value=collected), \
             patch.object(summary, 'pull_requests', return_value=current), \
             patch.object(summary, 'send') as send:
            summary.main()
        send.assert_called_once()
        return send.call_args.args[1]

    def test_weekly_digest_includes_pending_without_waiting(self):
        item = pr(result='pending / awaiting checks')
        current = [{'number': 1, 'title': item['title'], 'head': {'sha': 'abc'}}]
        self.assertIn('pending / awaiting checks', self.run_summary([item], current))

    def test_empty_week_sends_one_short_summary(self):
        self.assertIn('No open, ready Dependabot PRs.', self.run_summary([], []))

    def test_changed_head_does_not_report_old_success(self):
        current = [{'number': 1, 'title': 'Updated title', 'head': {'sha': 'new'}}]
        text = self.run_summary([pr()], current)
        self.assertIn('head or merge commit changed', text)
        self.assertNotIn('passed', text)
        self.assertIn('Updated title', text)

    def test_closed_pr_removed_and_new_pr_included(self):
        current = [{'number': 2, 'title': 'New PR', 'head': {'sha': 'def'},
                    'html_url': 'https://github.com/owner/repo/pull/2'}]
        text = self.run_summary([pr()], current)
        self.assertNotIn('|#1>', text)
        self.assertIn('|#2>', text)
        self.assertIn('opened during collection', text)

    def test_delivery_failure_propagates(self):
        with patch.dict('os.environ', {'GCHAT_PR_WEBHOOK_URL': 'fake', 'GITHUB_REPOSITORY': 'owner/repo'}), \
             patch.object(summary, 'collect', return_value=[]), \
             patch.object(summary, 'pull_requests', return_value=[]), \
             patch.object(summary, 'send', side_effect=RuntimeError('delivery failed')):
            with self.assertRaisesRegex(RuntimeError, 'delivery failed'):
                summary.main()


if __name__ == '__main__':
    unittest.main()
