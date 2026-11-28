# Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.

"""
Unit tests for TraceFetcher pagination and sample_traces.
"""

import json
from typing import List, Optional
from unittest.mock import patch, MagicMock

import pytest

from amp_evaluation.trace.fetcher import TraceFetcher, TraceLoader, sample_traces, _parse_timestamp


def _raw_trace(trace_id: str, start_time: str) -> dict:
    return {
        "traceId": trace_id,
        "rootSpanId": f"{trace_id}-root",
        "rootSpanName": "root",
        "startTime": start_time,
        "endTime": start_time,
        "spans": [],
    }


def _make_fetcher() -> TraceFetcher:
    return TraceFetcher(
        base_url="http://localhost:8001",
        organization="org",
        project="proj",
        agent="agent",
        environment="dev",
        token_provider=lambda: "test-token",
    )


class TestTraceLoaderEncoding:
    def test_loads_utf8_json_with_non_ascii_text(self, tmp_path):
        file_path = tmp_path / "traces.json"
        trace = _raw_trace("utf8-trace", "2026-01-01T00:00:00Z")
        trace["rootSpanName"] = "ආයුබෝවන්"
        file_path.write_text(json.dumps({"traces": [trace]}, ensure_ascii=False), encoding="utf-8")

        traces = TraceLoader(str(file_path)).load_traces()

        assert len(traces) == 1
        assert traces[0].rootSpanName == "ආයුබෝවන්"


def _mock_response(
    traces: List[dict],
    total_count: int,
    failed_ids: Optional[List[str]] = None,
    last_start_time: Optional[str] = None,
) -> MagicMock:
    resp = MagicMock()
    resp.raise_for_status.return_value = None
    body = {"traces": traces, "totalCount": total_count, "truncated": False}
    if failed_ids:
        body["failedTraceIds"] = failed_ids
    if last_start_time:
        body["lastStartTime"] = last_start_time
    resp.json.return_value = body
    return resp


def _api_simulator(
    all_traces: List[dict],
    resolution_us: int = 1,
    unreadable: Optional[set] = None,
    report_last_start: bool = True,
):
    """Faithfully simulate /traces/export as a `requests.get` side_effect.

    Filters `startTime` to [cursor, end] using parsed datetimes (like a real
    time-based store), sorts ascending, and caps at `limit`. `resolution_us`
    truncates both trace timestamps and the cursor before comparing, so
    `resolution_us=1000` models a millisecond-resolution store (used to prove the
    self-correcting cursor step escapes a rounded-away increment).
    """

    def _bucket(dt):
        epoch_us = int(dt.timestamp() * 1_000_000)
        return (epoch_us // resolution_us) * resolution_us

    ordered = sorted(all_traces, key=lambda t: _parse_timestamp(t["startTime"]))

    def _get(url, params=None, headers=None, timeout=None, **kwargs):
        cursor = _bucket(_parse_timestamp(params["startTime"]))
        end = _bucket(_parse_timestamp(params["endTime"]))
        limit = int(params["limit"])
        matching = [t for t in ordered if cursor <= _bucket(_parse_timestamp(t["startTime"])) <= end]
        selected = matching[:limit]
        # Like the observer: unreadable traces are left out of the page and listed,
        # and lastStartTime names the last selected trace, readable or not.
        bad = unreadable or set()
        return _mock_response(
            [t for t in selected if t["traceId"] not in bad],
            total_count=len(matching),
            failed_ids=[t["traceId"] for t in selected if t["traceId"] in bad],
            last_start_time=selected[-1]["startTime"] if selected and report_last_start else None,
        )

    return _get


class TestFetchTracesPagination:
    def test_paginates_across_multiple_pages(self):
        fetcher = _make_fetcher()

        page1 = [_raw_trace(f"t{i}", f"2026-01-01T00:00:{i:02d}Z") for i in range(0, 50)]
        page2 = [_raw_trace(f"t{i}", f"2026-01-01T00:01:{i - 50:02d}Z") for i in range(50, 100)]
        page3 = [_raw_trace(f"t{i}", f"2026-01-01T00:02:{i - 100:02d}Z") for i in range(100, 125)]

        responses = [
            _mock_response(page1, total_count=125),
            _mock_response(page2, total_count=125),
            _mock_response(page3, total_count=125),
        ]

        with patch("requests.get", side_effect=responses) as mock_get:
            result = list(
                fetcher.fetch_traces(start_time="2026-01-01T00:00:00Z", end_time="2026-01-01T01:00:00Z", page_size=50)
            )

        assert [t.traceId for t in result] == [f"t{i}" for i in range(125)]
        assert mock_get.call_count == 3
        # last call advanced the cursor to the last trace's startTime of the prior page
        assert mock_get.call_args_list[1].kwargs["params"]["startTime"] == page1[-1]["startTime"]
        assert mock_get.call_args_list[2].kwargs["params"]["startTime"] == page2[-1]["startTime"]

    def test_stops_when_page_smaller_than_page_size(self):
        fetcher = _make_fetcher()
        page = [_raw_trace("t0", "2026-01-01T00:00:00Z")]

        with patch("requests.get", return_value=_mock_response(page, total_count=1)) as mock_get:
            result = list(fetcher.fetch_traces(start_time="s", end_time="e", page_size=50))

        assert [t.traceId for t in result] == ["t0"]
        assert mock_get.call_count == 1

    def test_dedupes_traces_tied_at_page_boundary(self):
        """A startTime tie that exactly fills the page (and is the whole window) is
        yielded once each and terminates — no duplicates, no loop."""
        fetcher = _make_fetcher()
        tie = "2026-01-01T00:00:00Z"
        traces = [_raw_trace("t0", tie), _raw_trace("t1", tie)]

        with patch("requests.get", side_effect=_api_simulator(traces)):
            result = list(
                fetcher.fetch_traces(
                    start_time="2026-01-01T00:00:00Z",
                    end_time="2026-02-01T00:00:00Z",
                    page_size=2,
                )
            )

        assert [t.traceId for t in result] == ["t0", "t1"]

    def test_tie_exceeding_page_size_skips_only_tail(self):
        """When more than page_size traces share one startTime, the cursor steps past
        it: the unseen tail at that timestamp is dropped, but every later trace is
        still returned (no silent truncation of the rest) and it terminates."""
        fetcher = _make_fetcher()
        t2 = "2026-01-01T00:00:02.000000Z"
        t3 = "2026-01-01T00:00:03.000000Z"
        traces = [_raw_trace(f"a{i}", t2) for i in range(5)] + [_raw_trace(f"b{i}", t3) for i in range(2)]

        with patch("requests.get", side_effect=_api_simulator(traces, resolution_us=1)):
            result = list(
                fetcher.fetch_traces(
                    start_time="2026-01-01T00:00:00Z",
                    end_time="2026-02-01T00:00:00Z",
                    page_size=2,
                )
            )

        ids = [t.traceId for t in result]
        # First page_size of the tie are seen; the rest of the T2 tail (a2,a3,a4) are
        # skipped; both T3 traces are preserved.
        assert ids == ["a0", "a1", "b0", "b1"]

    def test_coarse_store_step_grows_and_terminates(self):
        """If the store resolves time at millisecond granularity, a 1µs step rounds
        back to the tie; the step must grow until it advances — and terminate."""
        fetcher = _make_fetcher()
        # Three traces within the same millisecond bucket (sub-ms apart) + one later.
        traces = [
            _raw_trace("a0", "2026-01-01T00:00:02.000100Z"),
            _raw_trace("a1", "2026-01-01T00:00:02.000200Z"),
            _raw_trace("a2", "2026-01-01T00:00:02.000300Z"),
            _raw_trace("b0", "2026-01-01T00:00:05.000000Z"),
        ]

        with patch("requests.get", side_effect=_api_simulator(traces, resolution_us=1000)):
            result = list(
                fetcher.fetch_traces(
                    start_time="2026-01-01T00:00:00Z",
                    end_time="2026-02-01T00:00:00Z",
                    page_size=2,
                )
            )

        ids = [t.traceId for t in result]
        # a0,a1 seen (first page of the ms-tie); a2 is the dropped tail; b0 preserved.
        assert "b0" in ids  # did not silently truncate everything after the tie
        assert ids == ["a0", "a1", "b0"]

    @pytest.mark.parametrize("report_last_start", [True, False], ids=["lastStartTime", "older-api"])
    def test_unreadable_trace_does_not_end_pagination(self, report_last_start):
        """An unreadable trace makes the returned page short, but the window isn't exhausted."""
        fetcher = _make_fetcher()
        traces = [_raw_trace(f"t{i:02d}", f"2026-01-01T00:00:{i:02d}Z") for i in range(20)]

        simulator = _api_simulator(traces, unreadable={"t03"}, report_last_start=report_last_start)
        with patch("requests.get", side_effect=simulator):
            ids = [
                t.traceId
                for t in fetcher.fetch_traces(
                    start_time="2026-01-01T00:00:00Z", end_time="2026-01-01T01:00:00Z", page_size=10
                )
            ]

        assert ids == [f"t{i:02d}" for i in range(20) if i != 3]
        assert fetcher.failed_trace_ids == {"t03"}

    def test_trace_readable_on_retry_is_not_reported_failed(self):
        fetcher = _make_fetcher()
        page1 = [_raw_trace(f"t{i}", f"2026-01-01T00:00:0{i}Z") for i in range(4)]
        # t4 couldn't be read on the first page; the next page (from t3's time) reads it.
        page2 = [_raw_trace("t3", "2026-01-01T00:00:03Z"), _raw_trace("t4", "2026-01-01T00:00:04Z")]

        with patch(
            "requests.get",
            side_effect=[_mock_response(page1, 6, failed_ids=["t4"]), _mock_response(page2, 3)],
        ):
            ids = [t.traceId for t in fetcher.fetch_traces(start_time="s", end_time="e", page_size=5)]

        assert ids == ["t0", "t1", "t2", "t3", "t4"]
        assert fetcher.failed_trace_ids == set()

    def test_trace_recovered_during_stall_probe_is_yielded(self):
        """B is unreadable on a stalled page but reads in the stall probe: it is still fetched."""
        fetcher = _make_fetcher()
        ts = "2026-01-01T00:00:00Z"
        a, b = _raw_trace("A", ts), _raw_trace("B", ts)
        stalled = _mock_response([a], 2, failed_ids=["B"], last_start_time=ts)

        with patch(
            "requests.get",
            side_effect=[
                stalled,  # first page: A read, B unreadable
                _mock_response([a], 2, failed_ids=["B"], last_start_time=ts),  # same page again: stalled
                _mock_response([b], 1, last_start_time=ts),  # stall probe: B now readable
                _mock_response([b], 1, last_start_time=ts),  # page from the stepped cursor
            ],
        ):
            ids = [t.traceId for t in fetcher.fetch_traces(start_time=ts, end_time="2026-01-01T01:00:00Z", page_size=2)]

        assert ids == ["A", "B"]
        assert fetcher.failed_trace_ids == set()

    def test_page_of_only_unreadable_traces_does_not_end_pagination(self):
        """A, then unreadable B and C filling a page, then D: D is still fetched."""
        fetcher = _make_fetcher()
        traces = [_raw_trace(tid, f"2026-01-01T00:00:0{i}Z") for i, tid in enumerate(["A", "B", "C", "D"])]

        with patch("requests.get", side_effect=_api_simulator(traces, unreadable={"B", "C"})):
            ids = [
                t.traceId
                for t in fetcher.fetch_traces(
                    start_time="2026-01-01T00:00:00Z", end_time="2026-01-01T01:00:00Z", page_size=2
                )
            ]

        assert ids == ["A", "D"]
        assert fetcher.failed_trace_ids == {"B", "C"}

    def test_long_run_of_unreadable_traces_is_crossed(self):
        fetcher = _make_fetcher()
        traces = [_raw_trace(f"t{i:02d}", f"2026-01-01T00:00:{i:02d}Z") for i in range(30)]
        unreadable = {f"t{i:02d}" for i in range(5, 22)}  # more than a page in a row

        with patch("requests.get", side_effect=_api_simulator(traces, unreadable=unreadable)):
            ids = [
                t.traceId
                for t in fetcher.fetch_traces(
                    start_time="2026-01-01T00:00:00Z", end_time="2026-01-01T01:00:00Z", page_size=10
                )
            ]

        assert ids == [f"t{i:02d}" for i in range(30) if f"t{i:02d}" not in unreadable]
        assert fetcher.failed_trace_ids == unreadable

    def test_page_size_must_be_positive(self):
        fetcher = _make_fetcher()
        with pytest.raises(ValueError):
            list(fetcher.fetch_traces(start_time="s", end_time="e", page_size=0))

    def test_max_traces_stops_early(self):
        fetcher = _make_fetcher()
        page = [_raw_trace(f"t{i}", f"2026-01-01T00:00:{i:02d}Z") for i in range(10)]

        with patch("requests.get", return_value=_mock_response(page, total_count=10)):
            result = list(fetcher.fetch_traces(start_time="s", end_time="e", page_size=10, max_traces=3))

        assert [t.traceId for t in result] == ["t0", "t1", "t2"]

    def test_empty_page_stops_iteration(self):
        fetcher = _make_fetcher()

        with patch("requests.get", return_value=_mock_response([], total_count=0)) as mock_get:
            result = list(fetcher.fetch_traces(start_time="s", end_time="e"))

        assert result == []
        assert mock_get.call_count == 1

    def test_constructor_page_size_used_when_not_overridden(self):
        fetcher = TraceFetcher(
            base_url="http://localhost:8001",
            organization="org",
            project="proj",
            agent="agent",
            environment="dev",
            token_provider=lambda: "test-token",
            page_size=25,
        )
        page = [_raw_trace("t0", "2026-01-01T00:00:00Z")]

        with patch("requests.get", return_value=_mock_response(page, total_count=1)) as mock_get:
            list(fetcher.fetch_traces(start_time="s", end_time="e"))

        assert mock_get.call_args.kwargs["params"]["limit"] == "25"

    def test_per_call_page_size_overrides_constructor_default(self):
        fetcher = TraceFetcher(
            base_url="http://localhost:8001",
            organization="org",
            project="proj",
            agent="agent",
            environment="dev",
            token_provider=lambda: "test-token",
            page_size=25,
        )
        page = [_raw_trace("t0", "2026-01-01T00:00:00Z")]

        with patch("requests.get", return_value=_mock_response(page, total_count=1)) as mock_get:
            list(fetcher.fetch_traces(start_time="s", end_time="e", page_size=5))

        assert mock_get.call_args.kwargs["params"]["limit"] == "5"

    def test_default_page_size_is_small(self):
        """The constructor default must stay memory-bound, not the API's 1000 max."""
        fetcher = _make_fetcher()
        assert fetcher.page_size == 10


class TestSampleTraces:
    def test_deterministic_across_runs(self):
        traces = [MagicMock(traceId=f"trace-{i}") for i in range(500)]

        first = [t.traceId for t in sample_traces(traces, sample_rate=0.3)]
        second = [t.traceId for t in sample_traces(traces, sample_rate=0.3)]

        assert first == second
        assert len(first) > 0

    def test_approximate_rate_over_large_set(self):
        traces = [MagicMock(traceId=f"trace-{i}") for i in range(5000)]

        kept = list(sample_traces(traces, sample_rate=0.2))

        ratio = len(kept) / len(traces)
        assert 0.15 < ratio < 0.25

    def test_sample_rate_one_keeps_everything(self):
        traces = [MagicMock(traceId=f"trace-{i}") for i in range(20)]
        assert list(sample_traces(traces, sample_rate=1.0)) == traces

    def test_invalid_sample_rate_raises(self):
        with pytest.raises(ValueError):
            list(sample_traces([], sample_rate=0))
        with pytest.raises(ValueError):
            list(sample_traces([], sample_rate=1.5))
