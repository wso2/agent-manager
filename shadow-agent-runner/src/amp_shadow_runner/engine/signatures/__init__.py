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

"""Agent signatures: the framework symbols whose use proves an agent, kept as data.

A ``Signature`` names one symbol (by the module it lives in, as ``imports.Symbol``) and the shape of code
that uses it:

- ``CALL``: the symbol is called, ``Agent(...)`` or ``create_react_agent(...)``.
- ``FACTORY``: a classmethod of the symbol's class is called, ``ReActAgent.from_tools(...)``. Its symbol is
  the method, as ``ImportTable.resolve`` gives it: ``Symbol("llama_index.core.agent.ReActAgent", "from_tools")``.
- ``SUBCLASS``: a class derives from the symbol, ``class MyAgent(Agent)``.

The symbols are matched only through imports, so a local class with the same name never matches. A
``RuleSet`` holds the signatures of one language and the frameworks they belong to.
"""

import functools
from collections.abc import Iterable
from dataclasses import dataclass
from enum import StrEnum

from amp_shadow_runner.engine.imports import Symbol


class Shape(StrEnum):
    """How code uses a signature's symbol."""

    CALL = "call"
    FACTORY = "factory"
    SUBCLASS = "subclass"


@dataclass(frozen=True)
class Framework:
    """An agent framework."""

    id: str
    """Lower case with hyphens, such as ``google-adk``; published as the ``framework`` attribute."""
    modules: tuple[str, ...]
    """Module roots whose imports count as the framework's (``google.adk`` covers ``google.adk.agents``)."""
    packages: tuple[str, ...]
    """Package names a dependency manifest declares for the framework."""


@dataclass(frozen=True)
class Signature:
    """One symbol whose use, in ``shape``, proves an agent of ``framework``."""

    framework: str
    symbol: Symbol
    shape: Shape
    absorbs: str | None = None
    """A keyword argument that passes in an agent this one runs, as ``AgentExecutor(agent=...)`` does. That
    agent is the same agent, so it is not reported on its own."""
    absorbs_position: int | None = None
    """The positional argument (0-based) that may pass that agent instead of the keyword."""
    requires: str | None = None
    """A keyword argument the call must pass, with a value other than a literal ``False`` or ``None``, to
    prove an agent (AutoGen ``ConversableAgent`` needs ``llm_config``). Only for ``CALL`` signatures."""


def agent_class(
    framework: str, module: str, name: str, *, factories: Iterable[str] = (), absorbs: str | None = None
) -> tuple[Signature, ...]:
    """Signatures for an agent class: constructed (``CALL``), derived from (``SUBCLASS``), and built by each
    classmethod in ``factories`` (``FACTORY``). With ``absorbs``, a factory may also take that agent as its
    first positional argument (``AgentExecutor.from_agent_and_tools(agent, tools)``)."""
    symbol = Symbol(module, name)
    position = None if absorbs is None else 0
    return (
        Signature(framework, symbol, Shape.CALL, absorbs),
        Signature(framework, symbol, Shape.SUBCLASS),
        *(
            Signature(framework, Symbol(f"{module}.{name}", method), Shape.FACTORY, absorbs, position)
            for method in factories
        ),
    )


def classmethod_factories(framework: str, module: str, name: str, *methods: str) -> tuple[Signature, ...]:
    """Signatures for classmethods that build an agent, on a class that is not an agent itself."""
    return tuple(Signature(framework, Symbol(f"{module}.{name}", m), Shape.FACTORY) for m in methods)


def conditional_class(framework: str, module: str, name: str, *, requires: str) -> tuple[Signature, ...]:
    """A class that proves an agent only when constructed with the keyword ``requires`` (see
    ``Signature.requires``). Deriving from it proves nothing, since the keyword cannot be seen there."""
    return (Signature(framework, Symbol(module, name), Shape.CALL, requires=requires),)


def agent_classes(framework: str, module: str, *names: str) -> tuple[Signature, ...]:
    """``agent_class`` for several classes of one module, with no factories."""
    return tuple(sig for name in names for sig in agent_class(framework, module, name))


def factories(framework: str, module: str, *names: str) -> tuple[Signature, ...]:
    """Signatures for functions that build an agent when called."""
    return tuple(Signature(framework, Symbol(module, name), Shape.CALL) for name in names)


def base_classes(framework: str, module: str, *names: str) -> tuple[Signature, ...]:
    """Signatures for classes that prove an agent only when derived from (an abstract base agent)."""
    return tuple(Signature(framework, Symbol(module, name), Shape.SUBCLASS) for name in names)


@dataclass(frozen=True)
class RuleSet:
    """The signatures of one language, and the frameworks they belong to."""

    frameworks: tuple[Framework, ...]
    signatures: tuple[Signature, ...]

    def __post_init__(self) -> None:
        known = {f.id for f in self.frameworks}
        if len(known) != len(self.frameworks):
            raise ValueError("a framework is listed twice")
        seen: set[tuple[Symbol, Shape]] = set()
        for sig in self.signatures:
            if sig.framework not in known:
                raise ValueError(f"signature {sig.symbol} names an unknown framework {sig.framework}")
            key = (sig.symbol, sig.shape)
            if key in seen:
                raise ValueError(f"signature {sig.symbol} ({sig.shape}) is listed twice")
            seen.add(key)

    def find(self, symbol: Symbol, shapes: Iterable[Shape]) -> Signature | None:
        """The signature for ``symbol`` used in one of ``shapes``, or None."""
        for shape in shapes:
            sig = self._index.get((symbol, shape))
            if sig is not None:
                return sig
        return None

    def framework(self, framework_id: str) -> Framework:
        """The framework with this id. Raises KeyError for an unknown id."""
        return self._frameworks[framework_id]

    @functools.cached_property
    def _index(self) -> dict[tuple[Symbol, Shape], Signature]:
        return {(sig.symbol, sig.shape): sig for sig in self.signatures}

    @functools.cached_property
    def _frameworks(self) -> dict[str, Framework]:
        return {f.id: f for f in self.frameworks}


__all__ = [
    "Framework",
    "RuleSet",
    "Shape",
    "Signature",
    "agent_class",
    "agent_classes",
    "base_classes",
    "classmethod_factories",
    "conditional_class",
    "factories",
]
