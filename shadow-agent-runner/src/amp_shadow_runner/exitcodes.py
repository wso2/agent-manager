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

"""Process exit codes.

Scripts that run the command or its container rely on these values, so they
are a public contract: never renumber an existing code.
"""

from enum import IntEnum


class ExitCode(IntEnum):
    """Exit status of an ``amp-shadow-runner`` invocation."""

    OK = 0
    """The command succeeded."""

    RUNTIME_ERROR = 1
    """An unexpected failure while running."""

    CONFIG_ERROR = 2
    """Invalid configuration or command-line usage."""

    AUTH_ERROR = 3
    """Agent Manager rejected the runner's credentials."""

    UPGRADE_REQUIRED = 4
    """Agent Manager requires a newer runner version."""

    PARTIAL_FAILURE = 5
    """The command finished, but some of its work failed."""
