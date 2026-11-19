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

"""amp_shadow_runner.engine.languages: extensions, cached parsers, and Ballerina as text only."""

import pytest

from amp_shadow_runner.engine.languages import EXTENSIONS, Language, language_for, parse, parser_for


@pytest.mark.parametrize(
    ("name", "language"),
    [
        ("agent.py", Language.PYTHON),
        ("index.js", Language.JAVASCRIPT),
        ("index.mjs", Language.JAVASCRIPT),
        ("index.cjs", Language.JAVASCRIPT),
        ("App.jsx", Language.JAVASCRIPT),
        ("agent.ts", Language.TYPESCRIPT),
        ("agent.mts", Language.TYPESCRIPT),
        ("agent.cts", Language.TYPESCRIPT),
        ("App.tsx", Language.TSX),
        ("Agent.cs", Language.C_SHARP),
        ("main.bal", Language.BALLERINA),
        ("LEGACY.PY", Language.PYTHON),
        ("src/pkg/agent.py", Language.PYTHON),
    ],
)
def test_language_for_known_extensions(name: str, language: Language) -> None:
    assert language_for(name) == language


@pytest.mark.parametrize("name", ["README.md", "Makefile", "agent.pyi", "types.d", "py", ".py.txt"])
def test_language_for_unknown_extensions(name: str) -> None:
    assert language_for(name) is None


def test_every_language_has_an_extension() -> None:
    assert set(EXTENSIONS.values()) == set(Language)


SOURCES = {
    Language.PYTHON: (b"import openai\n", "module"),
    Language.JAVASCRIPT: (b"const v = <Panel title='a' />;\n", "program"),
    Language.TYPESCRIPT: (b"const n: number = 1;\n", "program"),
    Language.TSX: (b"const v = <Panel title='a' />;\n", "program"),
    Language.C_SHARP: (b"using Microsoft.SemanticKernel;\n", "compilation_unit"),
}


@pytest.mark.parametrize("language", list(SOURCES), ids=str)
def test_parse_each_grammar(language: Language) -> None:
    source, root_type = SOURCES[language]
    tree = parse(language, source)
    assert tree is not None
    assert tree.root_node.type == root_type
    assert not tree.root_node.has_error


@pytest.mark.parametrize("language", list(SOURCES), ids=str)
def test_parser_is_cached(language: Language) -> None:
    parser = parser_for(language)
    assert parser is not None
    assert parser_for(language) is parser


def test_parse_recovers_from_syntax_errors() -> None:
    tree = parse(Language.PYTHON, b"def broken(:\n")
    assert tree is not None and tree.root_node.has_error


def test_ballerina_has_no_parser() -> None:
    assert parser_for(Language.BALLERINA) is None
    assert parse(Language.BALLERINA, b"import ballerina/http;\n") is None
