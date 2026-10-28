"""Tools exposed to the agent. Strands derives each schema from the signature
and the Args section of the docstring."""

from __future__ import annotations

import json
from datetime import date

from strands import tool

from data import CLAIMS, POLICIES, next_claim_number


from policy_reads import (
    list_policies as _list_policies,
    lookup_policy as _lookup_policy,
)

list_policies = tool(_list_policies)
lookup_policy = tool(_lookup_policy)


@tool
def list_claims() -> str:
    """List every claim the current customer has filed, with its status."""
    return json.dumps({"claims": list(CLAIMS.values())})


@tool
def get_claim_status(claim_number: str) -> str:
    """Get the current status and next step of one claim.

    Args:
        claim_number: Claim number, e.g. CLM-10432.
    """
    claim = CLAIMS.get(claim_number.strip().upper())
    if claim is None:
        return json.dumps({"error": f"No claim found with number {claim_number}."})
    return json.dumps(claim)


@tool
def file_claim(policy_number: str, description: str, amount_claimed: float) -> str:
    """Open a new claim against a policy.

    Args:
        policy_number: Policy the claim is made against, e.g. OZ-HOME-2280.
        description: What happened, in the customer's own words.
        amount_claimed: Estimated cost in pounds.
    """
    ref = policy_number.strip().upper()
    policy = POLICIES.get(ref)
    if policy is None:
        return json.dumps({"error": f"No policy found with number {policy_number}."})
    if policy["status"] not in ("active", "expires soon"):
        return json.dumps(
            {"error": f"Policy {ref} is {policy['status']}; it cannot accept a claim."}
        )

    claim = {
        "claim_number": next_claim_number(),
        "policy_number": ref,
        "type": policy["product"],
        "description": description,
        "amount_claimed": round(float(amount_claimed), 2),
        "status": "submitted",
        "filed_on": date.today().isoformat(),
        "next_step": f"A handler will be assigned within 2 working days. Excess is £{policy['excess']}.",
    }
    CLAIMS[claim["claim_number"]] = claim
    return json.dumps(claim)
