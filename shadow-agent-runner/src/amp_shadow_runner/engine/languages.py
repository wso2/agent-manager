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

"""The languages the engine scans, their file extensions, and their tree-sitter parsers.

Ballerina has no pinned grammar, so it is scanned as text only: ``parser_for``
and ``parse`` return None for it.
"""

import functools
from collections.abc import Callable, Mapping
from enum import StrEnum
from pathlib import PurePath
from types import MappingProxyType

import tree_sitter
import tree_sitter_c_sharp
import tree_sitter_javascript
import tree_sitter_python
import tree_sitter_typescript


class Language(StrEnum):
    """A language the engine recognises by file extension."""

    PYTHON = "python"
    JAVASCRIPT = "javascript"
    TYPESCRIPT = "typescript"
    TSX = "tsx"
    C_SHARP = "c_sharp"
    BALLERINA = "ballerina"


EXTENSIONS: Mapping[str, Language] = MappingProxyType(
    {
        ".py": Language.PYTHON,
        # The javascript grammar parses JSX too.
        ".js": Language.JAVASCRIPT,
        ".mjs": Language.JAVASCRIPT,
        ".cjs": Language.JAVASCRIPT,
        ".jsx": Language.JAVASCRIPT,
        ".ts": Language.TYPESCRIPT,
        ".mts": Language.TYPESCRIPT,
        ".cts": Language.TYPESCRIPT,
        ".tsx": Language.TSX,
        ".cs": Language.C_SHARP,
        ".bal": Language.BALLERINA,
    }
)
"""Lower-case file extension to language."""

_GRAMMARS: Mapping[Language, Callable[[], object]] = MappingProxyType(
    {
        Language.PYTHON: tree_sitter_python.language,
        Language.JAVASCRIPT: tree_sitter_javascript.language,
        Language.TYPESCRIPT: tree_sitter_typescript.language_typescript,
        Language.TSX: tree_sitter_typescript.language_tsx,
        Language.C_SHARP: tree_sitter_c_sharp.language,
    }
)


def language_for(path: str | PurePath) -> Language | None:
    """The language of a file, from its extension (case-insensitive), or None when it is not one the engine scans."""
    return EXTENSIONS.get(PurePath(path).suffix.lower())


@functools.cache
def parser_for(language: Language) -> tree_sitter.Parser | None:
    """The parser for ``language``, created once and reused, or None for a language with no grammar (Ballerina).

    Parsers are not thread-safe; the engine runs on one thread.
    """
    grammar = _GRAMMARS.get(language)
    if grammar is None:
        return None
    return tree_sitter.Parser(tree_sitter.Language(grammar()))


def parse(language: Language, source: bytes) -> tree_sitter.Tree | None:
    """Parse ``source`` into a syntax tree, or return None for a language with no grammar (Ballerina).

    tree-sitter recovers from syntax errors, so a broken file still gives a tree (with ``has_error`` set).
    """
    parser = parser_for(language)
    return None if parser is None else parser.parse(source)
