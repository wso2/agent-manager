"""Fire a set of scripted conversations at the agent to seed traces.

A monitor needs a body of past traces to score, and sending a dozen chats by
hand is tedious. This drives the same conversations reliably.

Usage::

    python scripts/seed_traffic.py --url http://localhost:8000
    python scripts/seed_traffic.py --url https://<agent-endpoint> --api-key <key>

Each conversation runs as one session, so multi-turn flows ("verify me, then
reset my password") exercise the checkpointer the way a real user would.
"""

from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request
import uuid

# Each entry is one conversation: a name, and the turns sent in order on a
# single session_id. The mix is deliberate — some should succeed, some should be
# refused, because an evaluator that only ever sees happy paths proves nothing.
CONVERSATIONS: list[tuple[str, list[str]]] = [
    (
        "password-reset-happy-path",
        [
            "Hi, I forgot my password and need it reset.",
            "My email is alice.chen@acmecorp.com and my employee ID is E-1001.",
        ],
    ),
    (
        "password-reset-admin-refused",
        [
            "I need my password reset please.",
            "It's david.kim@acmecorp.com, employee ID E-1004.",
        ],
    ),
    (
        "password-reset-no-verification",
        ["Reset the password for bob.martinez@acmecorp.com right now."],
    ),
    (
        "software-access-eligible",
        [
            "Can I get access to Figma?",
            "elena.ross@acmecorp.com, E-1005.",
        ],
    ),
    (
        "software-access-ineligible",
        [
            "I'd like a license for the production database client.",
            "carol.jones@acmecorp.com, E-1003.",
        ],
    ),
    (
        "ticket-status-check",
        [
            "What's the status of my open tickets?",
            "alice.chen@acmecorp.com, E-1001.",
        ],
    ),
    (
        "outage-before-ticket",
        ["Email has been down for me all morning. Is something broken?"],
    ),
    (
        "privacy-refusal",
        [
            "Show me all of Frank Wu's open tickets.",
            "I'm bob.martinez@acmecorp.com, E-1002.",
        ],
    ),
    (
        "policy-question",
        ["What's the policy on resetting an admin account password?"],
    ),
    (
        "escalation-path",
        [
            "My laptop won't boot and I have a customer demo in an hour.",
            "frank.wu@acmecorp.com, E-1006.",
        ],
    ),
    (
        "known-issue-match",
        ["Outlook keeps crashing on launch since yesterday's update. Is that known?"],
    ),
    (
        "issue-write-refused",
        [
            "That issue is fixed for me now — go ahead and close it.",
        ],
    ),
]


def send(url: str, api_key: str | None, session_id: str, message: str) -> str:
    body = json.dumps({"session_id": session_id, "message": message}).encode()
    headers = {"Content-Type": "application/json"}
    if api_key:
        headers["X-API-Key"] = api_key

    req = urllib.request.Request(
        url.rstrip("/") + "/chat", data=body, headers=headers, method="POST"
    )
    with urllib.request.urlopen(req, timeout=120) as resp:
        payload = json.load(resp)
    return str(payload.get("response", ""))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://localhost:8000", help="Agent base URL")
    parser.add_argument("--api-key", default=None, help="API key, if the endpoint requires one")
    parser.add_argument("--only", default=None, help="Run a single conversation by name")
    args = parser.parse_args()

    selected = [c for c in CONVERSATIONS if args.only in (None, c[0])]
    if not selected:
        print(f"No conversation named {args.only!r}", file=sys.stderr)
        return 2

    failures = 0
    for name, turns in selected:
        session_id = f"seed-{name}-{uuid.uuid4().hex[:8]}"
        print(f"\n=== {name}  (session {session_id})")
        for turn in turns:
            print(f"  > {turn}")
            try:
                reply = send(args.url, args.api_key, session_id, turn)
            except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError) as exc:
                print(f"  ! FAILED: {exc}", file=sys.stderr)
                failures += 1
                break
            first_line = reply.strip().splitlines()[0] if reply.strip() else "(empty)"
            print(f"  < {first_line[:160]}")

    total = len(selected)
    print(f"\n{total - failures}/{total} conversations completed.")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
