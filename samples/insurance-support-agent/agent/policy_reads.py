"""Read-only operations over synthetic policy fixtures, shared by both transports."""

import json
from data import POLICIES


def list_policies() -> str:
    """List every insurance policy held by the current customer."""
    summary = [
        {
            "policy_number": p["policy_number"],
            "product": p["product"],
            "status": p["status"],
            "premium_monthly": p["premium_monthly"],
            "renews_on": p["renews_on"],
        }
        for p in POLICIES.values()
    ]
    return json.dumps({"policies": summary})


def lookup_policy(policy_number: str) -> str:
    """Get the full details and cover of one policy.

    Args:
        policy_number: Policy number, e.g. OZ-AUTO-4417.
    """
    policy = POLICIES.get(policy_number.strip().upper())
    if policy is None:
        return json.dumps({"error": f"No policy found with number {policy_number}."})
    return json.dumps(policy)
