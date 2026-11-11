"""k3d/kubectl helpers for the heavy-tier driver.

The cluster bring-up + readiness is owned by the dev `make setup` chain the
heavy CI job runs (it builds the AMP component images from the working tree
and loads them into k3d), so the driver no longer restores a snapshot or
waits for readiness itself. The only thing left here is per-cell OpenSearch
index hygiene.
"""
from __future__ import annotations

import subprocess

_NS = "amp-observability"
_warned = False


def _find_opensearch_pod() -> str | None:
    """Return the OpenSearch pod name, or None. OpenSearch is not a Deployment
    named 'opensearch' (the old `deploy/opensearch` handle always 404'd), so
    discover the pod by name and skip the dashboards pod."""
    proc = subprocess.run(
        ["kubectl", "-n", _NS, "get", "pods", "--no-headers",
         "-o", "custom-columns=NAME:.metadata.name"],
        capture_output=True, text=True,
    )
    if proc.returncode != 0:
        return None
    for name in proc.stdout.split():
        if "opensearch" in name and "dashboard" not in name:
            return name
    return None


def reset_opensearch_indices() -> None:
    """Delete the spans-* indices so each cell starts from a clean slate.

    Best-effort: discovers the OpenSearch pod by name and warns at most once if
    it can't be found or the delete fails, so a missing handle is visible
    without spamming an identical per-cell warning.
    """
    global _warned
    pod = _find_opensearch_pod()
    if pod is None:
        if not _warned:
            print(f"::warning::OpenSearch pod not found in {_NS}; per-cell index "
                  "reset skipped (cells may see stale spans)")
            _warned = True
        return
    proc = subprocess.run(
        ["kubectl", "-n", _NS, "exec", pod, "--",
         "curl", "-sf", "-X", "DELETE", "http://localhost:9200/spans-*"],
        capture_output=True, text=True,
    )
    if proc.returncode != 0 and not _warned:
        print(f"::warning::OpenSearch index reset failed (rc={proc.returncode}): "
              f"{(proc.stderr or proc.stdout).strip()[:200]}")
        _warned = True
