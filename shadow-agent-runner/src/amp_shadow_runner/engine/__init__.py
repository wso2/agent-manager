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

"""The scan engine: a pure library that turns a local source tree into findings.

The engine never opens a network connection, never starts a process and never
reads secrets. ``tests/engine/test_purity.py`` enforces this, so keep fetching,
uploading and any LLM call in the runner, outside this package.
"""

from amp_shadow_runner.engine.version import ENGINE_VERSION, RULE_PACK_ID

__all__ = ["ENGINE_VERSION", "RULE_PACK_ID"]
