import os
import unittest
from unittest.mock import patch

from tools.web_search import tavily_tool


class TavilyToolTests(unittest.TestCase):
    @patch.dict(os.environ, {}, clear=True)
    def test_without_key_web_search_is_disabled(self):
        self.assertIsNone(tavily_tool())

    @patch.dict(os.environ, {"TAVILY_API_KEY": "test-key"})
    @patch("tools.web_search.TavilyClient")
    def test_public_search_returns_sources(self, client):
        result = {"title": "Airport", "url": "https://example.com", "content": "Take the train."}
        client.return_value.search.return_value = {"results": [result]}
        query = "Geneva airport public transport"
        self.assertEqual(tavily_tool().invoke({"query": query}), [result])
        client.assert_called_once_with(api_key="test-key")
        client.return_value.search.assert_called_once_with(query, max_results=3)
