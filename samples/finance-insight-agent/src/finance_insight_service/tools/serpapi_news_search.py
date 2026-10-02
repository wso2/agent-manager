import json
import os
from datetime import datetime
from typing import Any
from urllib.parse import urlencode
from urllib.request import Request, urlopen

from pydantic import BaseModel, Field

from crewai.tools import BaseTool


class SerpApiNewsSearchArgs(BaseModel):
    """Arguments for news search via SerpAPI."""
    search_query: str = Field(
        ..., description="Search query for recent news articles."
    )
    location: str | None = Field(
        None, description="Optional location to scope the search (e.g., United States)."
    )
    n_results: int = Field(10, description="Maximum number of results to return.")


class SerpApiNewsSearchTool(BaseTool):
    """CrewAI tool for fetching recent news results."""
    name: str = "serpapi_news_search"
    description: str = (
        "Searches recent news via SerpAPI. Requires SERPAPI_API_KEY in the environment."
    )
    args_schema: type[BaseModel] = SerpApiNewsSearchArgs

    def _run(
        self,
        search_query: str,
        location: str | None = None,
        n_results: int = 10,
    ) -> str:
        """Search recent news and return results as JSON."""
        query = (search_query or "").strip()
        if not query:
            return _error_payload("search_query is required")

        api_key = os.getenv("SERPAPI_API_KEY")
        if not api_key:
            return _error_payload("SERPAPI_API_KEY missing")

        n_results = max(1, min(int(n_results or 10), 20))
        params = {
            "engine": "google",
            "q": query,
            "tbm": "nws",
            "num": n_results,
            "api_key": api_key,
        }
        if location:
            params["location"] = location

        url = f"https://serpapi.com/search.json?{urlencode(params)}"
        request = Request(url, headers={"User-Agent": "FinanceInsightBot/1.0"})

        try:
            with urlopen(request, timeout=15) as response:
                payload = json.loads(response.read().decode("utf-8"))
        except Exception as exc:
            return _error_payload(f"request failed: {exc}")

        if isinstance(payload, dict) and payload.get("error"):
            return _error_payload(str(payload.get("error")))

        news_items = _extract_news_items(payload, n_results)
        return json.dumps(
            {
                "provider": "serpapi",
                "query": query,
                "fetched_at": datetime.utcnow().isoformat() + "Z",
                "news": news_items,
                "error": "",
            },
            ensure_ascii=True,
        )


def _extract_news_items(payload: dict[str, Any], n_results: int) -> list[dict[str, str]]:
    """Extract news entries from a SerpAPI payload."""
    items: list[dict[str, str]] = []
    for entry in payload.get("news_results", [])[:n_results]:
        items.append(
            {
                "title": str(entry.get("title") or ""),
                "link": str(entry.get("link") or ""),
                "snippet": str(entry.get("snippet") or ""),
                "date": str(entry.get("date") or ""),
                "source": str(entry.get("source") or ""),
            }
        )

    if items:
        return items

    for entry in payload.get("organic_results", [])[:n_results]:
        items.append(
            {
                "title": str(entry.get("title") or ""),
                "link": str(entry.get("link") or ""),
                "snippet": str(entry.get("snippet") or ""),
                "date": "",
                "source": "",
            }
        )
    return items


def _error_payload(message: str) -> str:
    """Return a JSON error payload for news searches."""
    return json.dumps(
        {
            "provider": "serpapi",
            "query": "",
            "fetched_at": datetime.utcnow().isoformat() + "Z",
            "news": [],
            "error": message,
        },
        ensure_ascii=True,
    )
