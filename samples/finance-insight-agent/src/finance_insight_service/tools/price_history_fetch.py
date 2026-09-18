import json
import os
from datetime import datetime
from typing import Any
from urllib.parse import urlencode
from urllib.request import Request, urlopen

from pydantic import BaseModel, Field

from crewai.tools import BaseTool


class PriceHistoryFetchArgs(BaseModel):
    """Arguments for price history fetching."""
    symbol: str = Field(..., description="Ticker or symbol to fetch.")
    interval: str = Field("1day", description="Interval (1day, 1week, 1month).")
    outputsize: int = Field(365, description="Number of data points to return.")


class PriceHistoryFetchTool(BaseTool):
    """CrewAI tool for fetching OHLCV price history."""
    name: str = "price_history_fetch"
    description: str = (
        "Fetches OHLCV price history from Twelve Data. "
        "Requires TWELVE_DATA_API_KEY in the environment."
    )
    args_schema: type[BaseModel] = PriceHistoryFetchArgs

    def _run(self, symbol: str, interval: str = "1day", outputsize: int = 365) -> str:
        """Fetch OHLCV history for a symbol and return JSON."""
        symbol = (symbol or "").strip()
        if not symbol:
            return _error_payload("symbol is required")

        interval = (interval or "1day").strip().lower()
        outputsize = max(10, min(int(outputsize), 2000))

        payload = _fetch_twelve_data(symbol, interval, outputsize)
        return json.dumps(payload, ensure_ascii=True)


def _error_payload(message: str, provider: str = "") -> str:
    """Return a JSON error payload for price history fetches."""
    return json.dumps(
        {
            "provider": provider,
            "symbol": "",
            "interval": "",
            "fetched_at": datetime.utcnow().isoformat() + "Z",
            "data": [],
            "error": message,
        },
        ensure_ascii=True,
    )


def _fetch_twelve_data(symbol: str, interval: str, outputsize: int) -> dict[str, Any]:
    """Fetch OHLCV data from Twelve Data."""
    api_key = os.getenv("TWELVE_DATA_API_KEY")
    if not api_key:
        return _error_dict("twelve_data", symbol, interval, "TWELVE_DATA_API_KEY missing")

    query = urlencode(
        {
            "symbol": symbol,
            "interval": interval,
            "outputsize": outputsize,
            "apikey": api_key,
            "format": "JSON",
        }
    )
    url = f"https://api.twelvedata.com/time_series?{query}"
    request = Request(url, headers={"User-Agent": "FinanceInsightBot/1.0"})

    try:
        with urlopen(request, timeout=15) as response:
            payload = json.loads(response.read().decode("utf-8"))
    except Exception as exc:
        return _error_dict("twelve_data", symbol, interval, f"request failed: {exc}")

    if "values" not in payload:
        return _error_dict(
            "twelve_data",
            symbol,
            interval,
            payload.get("message", "unexpected response"),
        )

    values = payload["values"]
    data = []
    for row in values:
        try:
            data.append(
                {
                    "date": row["datetime"],
                    "open": float(row["open"]),
                    "high": float(row["high"]),
                    "low": float(row["low"]),
                    "close": float(row["close"]),
                    "volume": float(row.get("volume") or 0),
                }
            )
        except (KeyError, ValueError):
            continue

    data.reverse()
    return {
        "provider": "twelve_data",
        "symbol": symbol,
        "interval": interval,
        "fetched_at": datetime.utcnow().isoformat() + "Z",
        "data": data[-outputsize:],
        "error": "",
    }


def _error_dict(provider: str, symbol: str, interval: str, message: str) -> dict[str, Any]:
    """Build a structured error payload dictionary."""
    return {
        "provider": provider,
        "symbol": symbol,
        "interval": interval,
        "fetched_at": datetime.utcnow().isoformat() + "Z",
        "data": [],
        "error": message,
    }
