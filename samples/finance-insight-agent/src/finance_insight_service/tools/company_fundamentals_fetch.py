import json
import os
from datetime import datetime
from typing import Any
from urllib.parse import urlencode
from urllib.request import Request, urlopen

from pydantic import BaseModel, Field

from crewai.tools import BaseTool


class CompanyFundamentalsFetchArgs(BaseModel):
    symbol: str = Field(..., description="Ticker or symbol to fetch fundamentals for.")
    limit: int = Field(1, description="Number of periods to return (latest first).")


class CompanyFundamentalsFetchTool(BaseTool):
    name: str = "company_fundamentals_fetch"
    description: str = (
        "Fetches fundamentals from Alpha Vantage (overview, income statement, "
        "balance sheet, cash flow). Requires ALPHAVANTAGE_API_KEY in the environment."
    )
    args_schema: type[BaseModel] = CompanyFundamentalsFetchArgs

    def _run(self, symbol: str, limit: int = 1) -> str:
        symbol = (symbol or "").strip().upper()
        if not symbol:
            return _error_payload("symbol is required")

        api_key = (os.getenv("ALPHAVANTAGE_API_KEY") or "").strip().strip("\"'").strip()
        if not api_key:
            return _error_payload("ALPHAVANTAGE_API_KEY missing", provider="alpha_vantage")

        limit = max(1, min(int(limit), 12))

        overview, overview_error = _fetch_alpha("OVERVIEW", symbol, api_key)
        income_raw, income_error = _fetch_alpha("INCOME_STATEMENT", symbol, api_key)
        balance_raw, balance_error = _fetch_alpha("BALANCE_SHEET", symbol, api_key)
        cash_raw, cash_error = _fetch_alpha("CASH_FLOW", symbol, api_key)

        errors = [
            err
            for err in [overview_error, income_error, balance_error, cash_error]
            if err
        ]
        payload: dict[str, Any] = {
            "provider": "alpha_vantage",
            "symbol": symbol,
            "fetched_at": datetime.utcnow().isoformat() + "Z",
            "fundamentals": {
                "overview": overview if isinstance(overview, dict) else {},
                "income_statement": _trim_reports(income_raw, limit),
                "balance_sheet": _trim_reports(balance_raw, limit),
                "cash_flow": _trim_reports(cash_raw, limit),
            },
            "error": "; ".join(errors) if errors else "",
        }
        return json.dumps(payload, ensure_ascii=True)


def _fetch_alpha(function: str, symbol: str, api_key: str) -> tuple[Any, str]:
    base_url = "https://www.alphavantage.co/query"
    query = {"function": function, "symbol": symbol, "apikey": api_key}
    url = f"{base_url}?{urlencode(query)}"
    request = Request(url, headers={"User-Agent": "FinanceInsightBot/1.0"})

    try:
        with urlopen(request, timeout=20) as response:
            payload = json.loads(response.read().decode("utf-8"))
    except Exception as exc:
        return {}, f"{function} request failed: {exc}"

    if isinstance(payload, dict):
        message = payload.get("Error Message") or payload.get("Note") or payload.get(
            "Information"
        )
        if message:
            return {}, message

    return payload, ""


def _trim_reports(payload: Any, limit: int) -> dict[str, list[dict[str, Any]]]:
    if not isinstance(payload, dict):
        return {"annual": [], "quarterly": []}

    annual = payload.get("annualReports") or []
    quarterly = payload.get("quarterlyReports") or []
    if not isinstance(annual, list):
        annual = []
    if not isinstance(quarterly, list):
        quarterly = []

    return {"annual": annual[:limit], "quarterly": quarterly[:limit]}


def _error_payload(message: str, provider: str = "") -> str:
    return json.dumps(
        {
            "provider": provider,
            "symbol": "",
            "fetched_at": datetime.utcnow().isoformat() + "Z",
            "fundamentals": {
                "overview": {},
                "income_statement": {"annual": [], "quarterly": []},
                "balance_sheet": {"annual": [], "quarterly": []},
                "cash_flow": {"annual": [], "quarterly": []},
            },
            "error": message,
        },
        ensure_ascii=True,
    )
