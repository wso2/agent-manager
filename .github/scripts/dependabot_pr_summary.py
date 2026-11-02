"""Collect ready Dependabot PRs without checking out or executing their code."""
import json
import os
import subprocess
import urllib.request

def api(path):
    return json.loads(subprocess.check_output(['gh', 'api', path]))


def pages(path, key=None):
    result = []
    separator = '&' if '?' in path else '?'
    for page in range(1, 10001):
        data = api(f'{path}{separator}per_page=100&page={page}')
        items = data[key] if key else data
        result.extend(items)
        if len(items) < 100:
            return result
    raise RuntimeError('GitHub pagination limit reached')


def pull_requests(repo):
    return [p for p in pages(f'repos/{repo}/pulls?state=open')
            if p['user']['login'] == 'dependabot[bot]' and not p['draft']]


def check_result(checks, statuses):
    # statuses are newest first; retain only the latest value for each context.
    contexts = {}
    for status in statuses:
        contexts.setdefault(status['context'], status['state'])
    pending = not checks and not contexts
    pending |= any(c['status'] != 'completed' for c in checks)
    pending |= any(s == 'pending' for s in contexts.values())
    failures = any(c['status'] == 'completed' and c['conclusion'] not in
                   ('success', 'neutral', 'skipped') for c in checks)
    failures |= any(s in ('failure', 'error') for s in contexts.values())
    if pending:
        return 'pending (some checks failed)' if failures else 'pending / awaiting checks'
    if failures:
        return 'failed / cancelled'
    if checks and all(c['conclusion'] in ('neutral', 'skipped') for c in checks) and not contexts:
        return 'skipped / neutral'
    return 'passed'


def collect(repo):
    result = []
    for p in pull_requests(repo):
        sha = p['head']['sha']
        checks = pages(f'repos/{repo}/commits/{sha}/check-runs?filter=latest', 'check_runs')
        statuses = pages(f'repos/{repo}/commits/{sha}/statuses')
        result.append({'number': p['number'], 'title': p['title'], 'url': p['html_url'],
                       'sha': sha, 'result': check_result(checks, statuses)})
    return sorted(result, key=lambda p: p['number'])


def message(prs):
    lines = [f'*Weekly Dependabot summary — {len(prs)} PR(s)*']
    for p in prs:
        # Avoid interpreting user-controlled titles as Chat links/mentions/markup.
        title = ' '.join(p['title'].split()).translate(str.maketrans({'<': '‹', '>': '›', '*': '＊', '`': '′', '_': '＿', '~': '～'}))
        lines.append(f"• <{p['url']}|#{p['number']}> {title} — {p['result']}")
    if not prs:
        lines.append('No open, ready Dependabot PRs.')
    return '\n'.join(lines)


def send(webhook, text):
    payload = json.dumps({'text': text}).encode()
    if len(payload) > 30000:
        raise RuntimeError('Summary exceeds Chat message size budget; message not sent')
    request = urllib.request.Request(webhook, data=payload,
                                     headers={'Content-Type': 'application/json; charset=UTF-8'})
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            if response.status != 200:
                raise RuntimeError('Google Chat did not acknowledge summary')
    except Exception:
        # Do not expose webhook credentials through exception URLs or responses.
        raise RuntimeError('Google Chat delivery failed') from None


def main():
    webhook = os.environ.get('GCHAT_PR_WEBHOOK_URL')
    if not webhook:
        raise RuntimeError('GCHAT_PR_WEBHOOK_URL is missing')
    repo = os.environ['GITHUB_REPOSITORY']
    prs = collect(repo)
    # Keep the weekly report useful if heads change during collection. Do not
    # attach old check results to a newer commit or include now-closed PRs.
    current = {p['number']: p for p in pull_requests(repo)}
    prs = [p for p in prs if p['number'] in current]
    observed = {p['number'] for p in prs}
    for p in prs:
        latest = current[p['number']]
        p['title'] = latest['title']
        if p['sha'] != latest['head']['sha']:
            p['result'] = 'pending / head changed during collection'
    for number, p in current.items():
        if number not in observed:
            prs.append({'number': number, 'title': p['title'], 'url': p['html_url'],
                        'result': 'pending / opened during collection'})
    prs.sort(key=lambda p: p['number'])
    send(webhook, message(prs))
    print(f'Sent weekly summary for {len(prs)} PR(s)')


if __name__ == '__main__':
    main()
