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

"""amp_shadow_runner.review: snippets, prompts, reply parsing and the reviewer, with a fake LLM (no network)."""

import re
from pathlib import Path

import pytest

from amp_shadow_runner.engine.classify import Evidence, Finding, Verdict
from amp_shadow_runner.engine.files import Tag
from amp_shadow_runner.llm import LlmError
from amp_shadow_runner.publish import reviewed_by
from amp_shadow_runner.review import (
    AI_REVIEW_ATTRIBUTE,
    MAX_AI_REVIEWS,
    MAX_FAILURES_IN_A_ROW,
    MAX_LINE_CHARS,
    MAX_SNIPPET_LINES,
    REVIEW_ALLOWANCE_SECONDS,
    Reviewer,
    ReviewOutcome,
    build_prompt,
    parse_reply,
    review_counts,
    review_outcome,
    snippet,
)

_LINE = re.compile(r"^(>>|  ) +(\d+) \| ", re.MULTILINE)


def _numbers(text: str) -> list[int]:
    return [int(m.group(2)) for m in _LINE.finditer(text)]


def _marked(text: str) -> list[int]:
    return [int(m.group(2)) for m in _LINE.finditer(text) if m.group(1) == ">>"]


def _line_of(source: str, needle: str) -> int:
    return next(i for i, line in enumerate(source.split("\n"), 1) if needle in line)


PYTHON = """\
import os
from crewai import (
    Agent,
    Task,
)


def helper():
    return 1


class Support:
    def build(self):
        agent = Agent(role="x")
        return agent


@register
def make():
    return Agent(role="y")
"""


def test_python_keeps_the_imports_and_the_widest_enclosing_class() -> None:
    line = _line_of(PYTHON, "agent = Agent")
    text = snippet(PYTHON.encode(), "src/a.py", line)
    assert _numbers(text) == [1, 2, 3, 4, 5, 12, 13, 14, 15]  # The whole multi-line import, then the class.
    assert _marked(text) == [line]
    assert "def helper" not in text
    assert "         ..." in text


def test_python_decorators_belong_to_the_function() -> None:
    line = _line_of(PYTHON, 'Agent(role="y")')
    assert _numbers(snippet(PYTHON.encode(), "a.py", line))[-3:] == [18, 19, 20]


def test_module_level_matches_get_the_file() -> None:
    source = "from strands import Agent\n\nx = 1\nsupport = Agent()\nprint(support)\n"
    text = snippet(source.encode(), "a.py", 4)
    assert _numbers(text) == [1, 2, 3, 4, 5, 6]
    assert _marked(text) == [4]


def test_a_block_over_the_limit_gets_a_window_around_the_match_inside_it() -> None:
    body = "".join(f"    step_{i} = {i}\n" for i in range(400))
    source = f"from crewai import Agent\n\n\ndef build():\n{body}    agent = Agent()\n{body}\n\nAFTER = 1\n"
    line = _line_of(source, "agent = Agent()")
    text = snippet(source.encode(), "a.py", line)
    numbers = _numbers(text)
    assert numbers[0] == 1
    assert len(numbers) == MAX_SNIPPET_LINES
    assert line in numbers and _marked(text) == [line]
    assert numbers[1] == line - (MAX_SNIPPET_LINES - 1) // 3  # A third of the window before the match.
    assert "AFTER" not in text


def test_typescript_keeps_es_and_require_imports_and_the_class() -> None:
    source = """\
import { Agent } from "@mastra/core/agent";
const tools = require("./tools");

function other() {
  return 1;
}

export class Bot {
  run() {
    const agent = new Agent({ name: "a" });
    return [1].map((x) => x);
  }
}
"""
    line = _line_of(source, "new Agent")
    text = snippet(source.encode(), "src/bot.ts", line)
    assert _numbers(text) == [1, 2, 8, 9, 10, 11, 12, 13]
    assert "function other" not in text


def test_csharp_keeps_the_usings_and_the_class() -> None:
    source = """\
using Microsoft.SemanticKernel;
using Microsoft.SemanticKernel.Agents;

namespace Demo;

public class Helper { public int X() => 1; }

public class Bot
{
    public void Run()
    {
        var agent = new ChatCompletionAgent { Name = "a" };
    }
}
"""
    line = _line_of(source, "new ChatCompletionAgent")
    text = snippet(source.encode(), "Bot.cs", line)
    assert _numbers(text) == [1, 2, 8, 9, 10, 11, 12, 13, 14]
    assert "Helper" not in text


def test_ballerina_keeps_its_imports_and_a_window() -> None:
    lines = ["import ballerina/ai;", "import ballerina/http;", "", *[f"int n{i} = {i};" for i in range(300)]]
    lines.insert(250, "final ai:Agent support = check new (model);")
    source = "\n".join(lines) + "\n"
    text = snippet(source.encode(), "main.bal", 251)
    numbers = _numbers(text)
    assert numbers[:2] == [1, 2]
    assert _marked(text) == [251]
    assert len(numbers) == MAX_SNIPPET_LINES


def test_long_lines_and_control_characters_are_cut_and_replaced() -> None:
    source = "from crewai import Agent\nx = Agent()  # " + "y" * 5000 + "\x1b[31m\n"
    text = snippet(source.encode(), "a.py", 2)
    assert "\x1b" not in text
    assert max(len(line) for line in text.split("\n")) < MAX_LINE_CHARS + 20


@pytest.mark.parametrize(
    ("reply", "expected"),
    [
        ('{"agent": true, "reason": "Runs an agent with tools."}', (True, "Runs an agent with tools.")),
        ('  {"agent": false, "reason": "A plain chat call."}\n', (False, "A plain chat call.")),
        ('```json\n{"agent": true, "reason": "ok"}\n```', (True, "ok")),
        ('```\n{"agent": false}\n```', (False, "")),
        ('{"agent": true, "reason": "x", "confidence": 0.9}', (True, "x")),
        ('{"agent": true, "reason": "' + "z" * 500 + '\\u001b"}', (True, "z" * 200)),
        ('{"agent": "true", "reason": "x"}', None),
        ('{"agent": 1}', None),
        ('{"reason": "x"}', None),
        ("[true]", None),
        ("Yes, it is an agent.", None),
        ('{"agent": true', None),
        ("", None),
    ],
)
def test_parse_reply(reply: str, expected: tuple[bool, str] | None) -> None:
    assert parse_reply(reply) == expected


def _finding(path: str, line: int = 2, *, verdict: Verdict = Verdict.NOT_SURE, tag: Tag | None = None) -> Finding:
    return Finding(
        f"r:{path}:agent",
        "agent",
        "r",
        path,
        line,
        "crewai",
        verdict,
        tag,
        {"framework": "crewai"},
        (Evidence("d2_agent_import", path, {"imports": 1}),),
    )


def test_the_prompt_marks_the_code_as_untrusted_data() -> None:
    system, user = build_prompt(_finding("src/a.py"), ">>     2 | x = Agent()", "abc123")
    assert "<untrusted-code-abc123>" in system and "</untrusted-code-abc123>" in system
    assert "ignore" in system.lower() and '{"agent": true' in system
    assert user.index("<untrusted-code-abc123>") < user.index("File: src/a.py") < user.index("x = Agent()")
    assert user.rstrip().endswith("</untrusted-code-abc123>")
    assert "crewai" in user and "line 2" in user


class FakeLlm:
    def __init__(self, *answers: str | Exception) -> None:
        self.answers = list(answers)
        self.calls: list[tuple[str, str, str, int]] = []

    def complete(self, system: str, user: str, *, model: str, max_tokens: int) -> str:
        self.calls.append((system, user, model, max_tokens))
        answer = self.answers.pop(0) if len(self.answers) > 1 else self.answers[0]
        if isinstance(answer, Exception):
            raise answer
        return answer


YES = '{"agent": true, "reason": "Creates an agent with tools."}'
NO = '{"agent": false, "reason": "Only a chat completion."}'


def _tree(tmp_path: Path, count: int) -> list[Finding]:
    for i in range(count):
        (tmp_path / f"a{i}.py").write_text(f"from crewai import Agent\nagent_{i} = Agent()\n")
    return [_finding(f"a{i}.py") for i in range(count)]


def _reviewer(
    llm: FakeLlm,
    *,
    now: float = 0.0,
    deadline: float = 100.0,
    limit: int = MAX_AI_REVIEWS,
    notes: list[str] | None = None,
) -> Reviewer:
    say = (lambda _line: None) if notes is None else notes.append
    return Reviewer(llm, "gpt-x", deadline=deadline, clock=lambda: now, notify=say, limit=limit)


def _outcomes(findings: tuple[Finding, ...]) -> list[ReviewOutcome | None]:
    return [review_outcome(f) for f in findings]


def test_confirmed_findings_are_published_as_ai_review_and_rejected_ones_are_not(tmp_path: Path) -> None:
    llm = FakeLlm(YES, NO)
    progress: list[tuple[int, int]] = []
    reviewed = _reviewer(llm).review(_tree(tmp_path, 2), tmp_path, progress=lambda n, due: progress.append((n, due)))
    assert _outcomes(reviewed) == [ReviewOutcome.CONFIRMED, ReviewOutcome.REJECTED]
    assert [reviewed_by(f) for f in reviewed] == ["ai_review", None]
    assert reviewed[0].attributes[AI_REVIEW_ATTRIBUTE] == {
        "outcome": "confirmed",
        "reason": "Creates an agent with tools.",
    }
    assert progress == [(1, 2), (2, 2)]
    [(_, user, model, max_tokens), _] = llm.calls
    assert "agent_0 = Agent()" in user and model == "gpt-x" and max_tokens <= 1024


def test_proven_and_test_findings_are_never_sent(tmp_path: Path) -> None:
    findings = _tree(tmp_path, 3)
    findings[0] = _finding("a0.py", verdict=Verdict.PROVEN)
    findings[1] = _finding("a1.py", tag="test")
    llm = FakeLlm(YES)
    reviewed = _reviewer(llm).review(findings, tmp_path)
    assert _outcomes(reviewed) == [None, None, ReviewOutcome.CONFIRMED]
    assert len(llm.calls) == 1 and "agent_2" in llm.calls[0][1]
    assert reviewed[:2] == tuple(findings[:2])


def test_a_bad_reply_or_a_transient_error_fails_only_that_finding(tmp_path: Path) -> None:
    llm = FakeLlm("not json", LlmError("The LLM at api.example.com answered HTTP 500.", status=500), YES)
    reviewer = _reviewer(llm)
    reviewed = reviewer.review(_tree(tmp_path, 3), tmp_path)
    assert _outcomes(reviewed) == [ReviewOutcome.FAILED, ReviewOutcome.FAILED, ReviewOutcome.CONFIRMED]
    assert reviewed[1].attributes[AI_REVIEW_ATTRIBUTE] == {
        "outcome": "failed",
        "reason": "The LLM at api.example.com answered HTTP 500.",
    }
    assert reviewer.stopped is None


@pytest.mark.parametrize("status", [401, 403, 404])
def test_a_refused_key_or_unknown_model_stops_review_for_the_scan(tmp_path: Path, status: int) -> None:
    llm = FakeLlm(LlmError(f"The LLM at api.example.com answered HTTP {status}.", status=status))
    notes: list[str] = []
    reviewer = _reviewer(llm, notes=notes)
    findings = _tree(tmp_path, 3)
    first = reviewer.review(findings[:2], tmp_path)
    second = reviewer.review(findings[2:], tmp_path)
    assert _outcomes(first + second) == [ReviewOutcome.FAILED, ReviewOutcome.NOT_REVIEWED, ReviewOutcome.NOT_REVIEWED]
    assert len(llm.calls) == 1
    assert notes == [
        f"AI review stopped: The LLM at api.example.com answered HTTP {status}. "
        "The other not-sure agents are not reviewed."
    ]


def test_failures_in_a_row_stop_review(tmp_path: Path) -> None:
    llm = FakeLlm(LlmError("Could not get an answer from the LLM at api.example.com: timed out"))
    notes: list[str] = []
    reviewer = _reviewer(llm, notes=notes)
    reviewed = reviewer.review(_tree(tmp_path, MAX_FAILURES_IN_A_ROW + 2), tmp_path)
    assert len(llm.calls) == MAX_FAILURES_IN_A_ROW
    assert _outcomes(reviewed)[-2:] == [ReviewOutcome.NOT_REVIEWED] * 2
    assert len(notes) == 1


def test_replies_in_the_wrong_format_in_a_row_stop_review(tmp_path: Path) -> None:
    llm = FakeLlm("Yes, this looks like an agent to me.")
    notes: list[str] = []
    reviewer = _reviewer(llm, notes=notes)
    reviewed = reviewer.review(_tree(tmp_path, MAX_FAILURES_IN_A_ROW + 2), tmp_path)
    assert len(llm.calls) == MAX_FAILURES_IN_A_ROW
    assert _outcomes(reviewed)[-2:] == [ReviewOutcome.NOT_REVIEWED] * 2
    assert len(notes) == 1


def test_at_most_the_limit_of_reviews_across_repositories(tmp_path: Path) -> None:
    assert MAX_AI_REVIEWS == 200
    llm = FakeLlm(YES)
    reviewer = _reviewer(llm, limit=3)
    findings = _tree(tmp_path, 5)
    reviewed = reviewer.review(findings[:2], tmp_path) + reviewer.review(findings[2:], tmp_path)
    assert _outcomes(reviewed) == [ReviewOutcome.CONFIRMED] * 3 + [ReviewOutcome.NOT_REVIEWED] * 2
    assert reviewed[3].attributes[AI_REVIEW_ATTRIBUTE] == {
        "outcome": "not_reviewed",
        "reason": "the scan reached its limit of 3 AI reviews",
    }
    assert review_counts(reviewed)[ReviewOutcome.NOT_REVIEWED] == 2


def test_no_review_starts_after_the_deadline(tmp_path: Path) -> None:
    llm = FakeLlm(YES)
    reviewed = _reviewer(llm, now=100.0, deadline=100.0).review(_tree(tmp_path, 2), tmp_path)
    assert _outcomes(reviewed) == [ReviewOutcome.NOT_REVIEWED] * 2
    assert llm.calls == []
    assert REVIEW_ALLOWANCE_SECONDS == 480


def test_an_unreadable_file_fails_its_review_without_a_request(tmp_path: Path) -> None:
    llm = FakeLlm(YES)
    [reviewed] = _reviewer(llm).review([_finding("missing.py")], tmp_path)
    assert review_outcome(reviewed) == ReviewOutcome.FAILED
    assert llm.calls == []


def test_a_client_bug_fails_the_review_without_showing_its_text(tmp_path: Path) -> None:
    llm = FakeLlm(RuntimeError("x = Agent() from the code"), YES)
    reviewed = _reviewer(llm).review(_tree(tmp_path, 2), tmp_path)
    assert _outcomes(reviewed) == [ReviewOutcome.FAILED, ReviewOutcome.CONFIRMED]
    assert reviewed[0].attributes[AI_REVIEW_ATTRIBUTE] == {
        "outcome": "failed",
        "reason": "the runner could not review it (an internal error)",
    }


def test_an_unknown_outcome_in_a_report_is_none() -> None:
    finding = _finding("a.py")
    assert review_outcome(finding) is None
    odd = Finding(**{**finding.__dict__, "attributes": {AI_REVIEW_ATTRIBUTE: {"outcome": "maybe"}}})
    assert review_outcome(odd) is None
