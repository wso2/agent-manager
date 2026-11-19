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

"""Git hosts the runner lists and downloads repositories from (GitHub now, Azure DevOps later).

Every message here is safe to show: it never holds a token or request headers.
"""

from dataclasses import dataclass

from amp_shadow_runner.exitcodes import ExitCode


@dataclass(frozen=True)
class Repository:
    """One repository to scan."""

    name: str
    """The name the Git host reports, as published to Agent Manager."""
    private: bool
    """True for private and internal repositories."""
    default_branch: str | None
    """None when the host reports none (an empty repository)."""
    commit_sha: str | None = None
    """The default branch's commit, once resolved. The archive is downloaded at this commit."""
    empty: bool = False
    """True when the repository has no commits: it counts as scanned, with zero agents, and is not downloaded."""


@dataclass(frozen=True)
class FailedRepository:
    """A repository the scan could not read, with a short reason that is safe to publish."""

    name: str
    reason: str


@dataclass(frozen=True)
class RepositoryList:
    """The result of listing repositories."""

    repositories: tuple[Repository, ...]
    """The repositories to scan, in the host's order."""
    gone: tuple[str, ...] = ()
    """Named repositories that no longer exist: they count as scanned, with zero agents."""
    failed: tuple[FailedRepository, ...] = ()
    """Named repositories that could not be read, or that the scan may not read."""
    complete: bool = True
    """False when the full list may be missing repositories (a cap, unreadable entries, or repositories the
    token could not see)."""


class GitSourceError(Exception):
    """The Git host refused or failed, so the scan cannot go on. The message is safe to show."""

    def __init__(self, message: str, *, exit_code: ExitCode = ExitCode.RUNTIME_ERROR) -> None:
        super().__init__(message)
        self.exit_code = exit_code


class RepositoryError(GitSourceError):
    """One repository failed; the scan can go on and publish it as failed with ``reason``."""

    def __init__(self, name: str, reason: str) -> None:
        super().__init__(f"{name}: {reason}")
        self.name = name
        self.reason = reason


__all__ = ["FailedRepository", "GitSourceError", "Repository", "RepositoryError", "RepositoryList"]
