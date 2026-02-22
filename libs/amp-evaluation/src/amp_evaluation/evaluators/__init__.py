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
Evaluators subpackage.

Import Strategy:
----------------

1. Base classes and parameter descriptors (always available, no dependencies):
   >>> from amp_evaluation.evaluators import BaseEvaluator, LLMAsJudgeEvaluator, Param

2. Standard evaluators (no external dependencies):
   >>> from amp_evaluation.evaluators.builtin.standard import LatencyEvaluator, TokenEfficiencyEvaluator

3. DeepEval evaluators (requires deepeval package):
   >>> from amp_evaluation.evaluators.builtin.deepeval import DeepEvalPlanQualityEvaluator

4. Built-in evaluator factory (recommended):
   >>> from amp_evaluation import builtin
   >>> evaluator = builtin("latency", max_latency_ms=5000)

The base classes are always imported. Built-in evaluators are in the builtin/
subpackage.
"""

# Base classes - always available
from .base import BaseEvaluator, LLMAsJudgeEvaluator, FunctionEvaluator, FunctionLLMJudge, JudgeOutput
from .config import Param, EvalMode

__all__ = [
    # Base classes
    "BaseEvaluator",
    "LLMAsJudgeEvaluator",
    "FunctionEvaluator",
    "FunctionLLMJudge",
    "JudgeOutput",
    # Parameter descriptor
    "Param",
    # Enums
    "EvalMode",
]
