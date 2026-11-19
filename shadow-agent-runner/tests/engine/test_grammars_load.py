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

"""Every pinned grammar loads in the pinned tree-sitter binding and parses one line.

This also proves the grammars' ABI versions are compatible with the binding.
"""

from collections.abc import Callable

import pytest
import tree_sitter
import tree_sitter_c_sharp
import tree_sitter_javascript
import tree_sitter_python
import tree_sitter_typescript

GRAMMARS: list[tuple[str, Callable[[], object], bytes, str]] = [
    ("python", tree_sitter_python.language, b"def handler(event):\n    return event\n", "module"),
    ("javascript", tree_sitter_javascript.language, b"const agent = new Agent({ name: 'a' });\n", "program"),
    ("typescript", tree_sitter_typescript.language_typescript, b"const n: number = 1;\n", "program"),
    ("tsx", tree_sitter_typescript.language_tsx, b"const view = <Panel title='a' />;\n", "program"),
    ("c_sharp", tree_sitter_c_sharp.language, b"class Agent { void Run() {} }\n", "compilation_unit"),
]


@pytest.mark.parametrize(("name", "language_fn", "source", "root_type"), GRAMMARS, ids=[g[0] for g in GRAMMARS])
def test_grammar_loads_and_parses(name: str, language_fn: Callable[[], object], source: bytes, root_type: str) -> None:
    language = tree_sitter.Language(language_fn())
    assert tree_sitter.MIN_COMPATIBLE_LANGUAGE_VERSION <= language.abi_version <= tree_sitter.LANGUAGE_VERSION
    tree = tree_sitter.Parser(language).parse(source)
    assert tree.root_node.type == root_type
    assert not tree.root_node.has_error, f"{name} grammar failed to parse {source!r}"
