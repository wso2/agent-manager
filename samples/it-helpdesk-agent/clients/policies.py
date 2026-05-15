"""Mocked IT policy knowledge base client."""

from __future__ import annotations

from typing import Any

from ._data import load


def search(query: str, limit: int = 5) -> list[dict[str, Any]]:
    docs = load("policies.json")
    q = query.lower().strip()
    if not q:
        return docs[:limit]
    scored: list[tuple[int, dict[str, Any]]] = []
    for d in docs:
        score = 0
        text = f"{d['title']} {d['body']}".lower()
        for token in q.split():
            if token in text:
                score += 1
        if score > 0:
            scored.append((score, d))
    scored.sort(key=lambda x: x[0], reverse=True)
    return [d for _, d in scored[:limit]] or docs[:limit]
