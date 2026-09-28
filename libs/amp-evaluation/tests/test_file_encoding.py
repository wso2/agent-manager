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
Regression tests for JSON file encoding.

JSON is UTF-8 by specification (RFC 8259), but ``open()`` without an explicit
``encoding`` decodes using the platform default instead. On Windows that default
is the legacy ANSI code page (cp1252), so loading a trace or dataset file
containing non-ASCII text raised ``UnicodeDecodeError`` and made every
file-based flow unusable there.

Two kinds of assertions are used, deliberately:

* Round-trip tests read back non-ASCII content. These fail on cp1252 platforms
  but pass everywhere on a UTF-8 default locale, so on Linux CI they alone are
  not enough to catch a regression.
* Spy tests assert the loaders pass ``encoding="utf-8"`` explicitly. These are
  platform-independent and fail on any OS if the argument is dropped again.
"""

import builtins
import json

import pytest

from amp_evaluation.dataset import load_dataset_from_json, save_dataset_to_json
from amp_evaluation.trace.fetcher import TraceLoader

# UTF-8 encodes these as e3 81 93 ... — byte 0x81 is undefined in cp1252, so a
# non-UTF-8 read raises rather than silently mojibaking the text.
NON_ASCII = "こんにちは"


@pytest.fixture
def open_spy(monkeypatch):
    """Record the keyword arguments of every ``builtins.open`` call."""
    real_open = builtins.open
    calls = []

    def spy(file, *args, **kwargs):
        calls.append((str(file), kwargs))
        return real_open(file, *args, **kwargs)

    monkeypatch.setattr(builtins, "open", spy)
    return calls


def encodings_used_for(calls, path):
    """Encodings that ``open`` was called with for ``path``."""
    return [kwargs.get("encoding") for opened, kwargs in calls if opened == str(path)]


def write_utf8_json(path, payload):
    path.write_text(json.dumps(payload, ensure_ascii=False), encoding="utf-8")
    return path


def dataset_payload():
    return {
        "schema_version": "1.0",
        "metadata": {"name": NON_ASCII, "description": f"dataset about {NON_ASCII}"},
        "tasks": [{"task_id": "task_001", "input": NON_ASCII, "expected_output": NON_ASCII}],
    }


class TestDatasetJsonEncoding:
    """load_dataset_from_json / save_dataset_to_json must use UTF-8."""

    def test_load_reads_non_ascii_content(self, tmp_path):
        path = write_utf8_json(tmp_path / "dataset.json", dataset_payload())

        dataset = load_dataset_from_json(str(path))

        assert dataset.name == NON_ASCII
        assert dataset.tasks[0].input == NON_ASCII

    def test_load_requests_utf8_explicitly(self, tmp_path, open_spy):
        path = write_utf8_json(tmp_path / "dataset.json", dataset_payload())

        load_dataset_from_json(str(path))

        assert encodings_used_for(open_spy, path) == ["utf-8"]

    def test_save_round_trips_non_ascii(self, tmp_path):
        source = write_utf8_json(tmp_path / "dataset.json", dataset_payload())
        dataset = load_dataset_from_json(str(source))
        target = tmp_path / "saved.json"

        save_dataset_to_json(dataset, str(target))

        assert load_dataset_from_json(str(target)).tasks[0].input == NON_ASCII

    def test_save_requests_utf8_explicitly(self, tmp_path, open_spy):
        source = write_utf8_json(tmp_path / "dataset.json", dataset_payload())
        dataset = load_dataset_from_json(str(source))
        target = tmp_path / "saved.json"

        save_dataset_to_json(dataset, str(target))

        assert encodings_used_for(open_spy, target) == ["utf-8"]


def trace_payload():
    """A minimal trace in the shape TraceLoader parses, with non-ASCII text."""
    span = {
        "traceId": "abc123",
        "spanId": "span001",
        "name": "chat",
        "service": "support-agent",
        "startTime": "2026-01-01T00:00:00Z",
        "endTime": "2026-01-01T00:00:01Z",
        "durationInNanos": 1_000_000_000,
        "ampAttributes": {"kind": "agent", "input": NON_ASCII, "output": NON_ASCII},
    }
    return [
        {
            "traceId": "abc123",
            "rootSpanId": "span001",
            "rootSpanName": NON_ASCII,
            "startTime": "2026-01-01T00:00:00Z",
            "endTime": "2026-01-01T00:00:01Z",
            "spans": [span],
        }
    ]


class TestTraceLoaderEncoding:
    """TraceLoader reads trace files exported by the platform, which are UTF-8."""

    def test_load_traces_reads_non_ascii_content(self, tmp_path):
        path = write_utf8_json(tmp_path / "traces.json", trace_payload())

        loaded = TraceLoader(file_path=str(path)).load_traces()

        assert loaded[0].rootSpanName == NON_ASCII
        assert loaded[0].spans[0].ampAttributes.input == NON_ASCII

    def test_load_traces_requests_utf8_explicitly(self, tmp_path, open_spy):
        path = write_utf8_json(tmp_path / "traces.json", trace_payload())

        TraceLoader(file_path=str(path)).load_traces()

        assert encodings_used_for(open_spy, path) == ["utf-8"]
