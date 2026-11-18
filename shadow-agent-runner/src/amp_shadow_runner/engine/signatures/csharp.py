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

"""C# agent signatures: Semantic Kernel agents and the Microsoft Agent Framework.

A class counts as the framework's only when its namespace is imported (see ``Condition.NAMESPACE``): the
file's ``using`` directives, the ``global using`` directives of every ``.cs`` file of its project, and the
``<Using Include="..."/>`` items of its ``.csproj``, or a fully qualified name. The same class name with
none of these is Not sure: a lookalike class of the codebase's own. The .NET SDK's implicit usings never
include these namespaces, so they are not added.

Extension methods cannot be resolved by type, so ``CreateAIAgent`` and ``AsAIAgent`` (Agent Framework,
on a chat client) count only with a ``using`` of ``Microsoft.Agents.AI`` (or a namespace under it) in
scope, and are no match without one.

Left out on purpose: orchestration (Semantic Kernel ``AgentGroupChat`` and the orchestration classes, Agent
Framework workflows), whose agents are found on their own; and classes deriving from an agent class.
"""

from amp_shadow_runner.engine.imports import Symbol
from amp_shadow_runner.engine.signatures import Condition, Framework, RuleSet, Shape, Signature, conditional_calls

_SK_OPENAI = "Microsoft.SemanticKernel.Agents.OpenAI"
_MAF = "Microsoft.Agents.AI"

CSHARP = RuleSet(
    frameworks=(
        Framework(
            "semantic-kernel", ("Microsoft.SemanticKernel",), ("Microsoft.SemanticKernel", "Microsoft.SemanticKernel.*")
        ),
        Framework("microsoft-agent-framework", (_MAF,), (_MAF, f"{_MAF}.*")),
    ),
    signatures=(
        # Semantic Kernel agents (Microsoft.SemanticKernel.Agents.Core, .OpenAI and .AzureAI packages).
        *conditional_calls(
            "semantic-kernel", "Microsoft.SemanticKernel.Agents", "ChatCompletionAgent", condition=Condition.NAMESPACE
        ),
        *conditional_calls(
            "semantic-kernel", _SK_OPENAI, "OpenAIAssistantAgent", "OpenAIResponseAgent", condition=Condition.NAMESPACE
        ),
        Signature(
            "semantic-kernel",
            Symbol(f"{_SK_OPENAI}.OpenAIAssistantAgent", "CreateAsync"),
            Shape.FACTORY,
            condition=Condition.NAMESPACE,
        ),
        *conditional_calls(
            "semantic-kernel", "Microsoft.SemanticKernel.Agents.AzureAI", "AzureAIAgent", condition=Condition.NAMESPACE
        ),
        # Microsoft Agent Framework: the agent class, and the extension methods that build one from a chat client.
        *conditional_calls("microsoft-agent-framework", _MAF, "ChatClientAgent", condition=Condition.NAMESPACE),
        Signature("microsoft-agent-framework", Symbol(_MAF, "CreateAIAgent"), Shape.EXTENSION),
        Signature("microsoft-agent-framework", Symbol(_MAF, "AsAIAgent"), Shape.EXTENSION),
    ),
)
"""The C# signatures."""
