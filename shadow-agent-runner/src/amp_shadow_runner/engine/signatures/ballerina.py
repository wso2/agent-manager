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

"""Ballerina agent signatures.

Ballerina has no grammar, so its files are matched as text (``match_bal``) and every match is Not sure
(``Condition.TEXT``): published only when AI review confirms it. A signature's symbol is the module
(``ballerina/ai``) and the agent class constructed from it.
"""

from amp_shadow_runner.engine.imports import Symbol
from amp_shadow_runner.engine.signatures import Condition, Framework, RuleSet, Shape, Signature

_MODULES = ("ballerina/ai", "ballerinax/ai", "ballerinax/ai.agent")

BALLERINA = RuleSet(
    frameworks=(Framework("ballerina-ai", _MODULES, _MODULES),),
    signatures=(
        *(Signature("ballerina-ai", Symbol(m, "Agent"), Shape.CALL, condition=Condition.TEXT) for m in _MODULES),
        # The agent classes of the earlier ballerinax/ai.agent module.
        *(
            Signature("ballerina-ai", Symbol("ballerinax/ai.agent", name), Shape.CALL, condition=Condition.TEXT)
            for name in ("FunctionCallAgent", "ReActAgent")
        ),
    ),
)
"""The Ballerina signatures."""
