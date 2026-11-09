import os

from langchain_core.tools import tool
from tavily import TavilyClient


def tavily_tool():
    """Create the public web search tool when a Tavily key is configured."""
    api_key = os.getenv("TAVILY_API_KEY")
    if not api_key:
        return None
    client = TavilyClient(api_key=api_key)

    @tool
    def search_public_travel(query: str) -> list[dict]:
        """Search public travel information. Omit passenger and booking details."""
        results = client.search(query, max_results=3)["results"]
        return [
            {"title": item["title"], "url": item["url"], "content": item["content"]}
            for item in results
        ]

    return search_public_travel
