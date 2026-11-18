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

"""The package exposes its version and the engine identity."""

import amp_shadow_runner
from amp_shadow_runner.engine import ENGINE_VERSION, RULE_PACK_ID


def test_runner_version_is_set() -> None:
    assert isinstance(amp_shadow_runner.__version__, str)
    assert amp_shadow_runner.__version__


def test_engine_identity() -> None:
    assert ENGINE_VERSION == "0.1.0"
    assert RULE_PACK_ID == "builtin-1"
