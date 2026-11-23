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

"""The configuration of a ``scan``: its command-line flags and environment.

The Agent Manager console builds the command, so the flags arrive in the form
it emits. The checks mirror what Agent Manager accepted when the platform was
saved, so the runner refuses what the server would.

No error message contains the scan token or the environment.
"""

import argparse
import os
import re
import time
from collections.abc import Callable, Mapping
from dataclasses import dataclass, field
from datetime import datetime
from typing import ClassVar, Literal, get_args
from urllib.parse import urlsplit

from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.scantoken import DEFAULT_MIN_SECONDS, require_lifetime, token_expiry

Provider = Literal["github"]
"""The Git hosts the runner scans. Any other provider is refused as unsupported."""

DEFAULT_GITHUB_API_URL = "https://api.github.com"
MAX_REPOSITORIES = 5000
"""The most GitHub repositories ``--repos`` lists (the server's limit on a published repository list)."""

_COPY_AGAIN = "Copy the whole command from Agent Manager again."
_SCAN_ID = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}", re.IGNORECASE)
_GITHUB_ORG = re.compile(r"[A-Za-z0-9](?:[A-Za-z0-9]|-[A-Za-z0-9]){0,38}")
_GITHUB_REPO = re.compile(r"[A-Za-z0-9._-]{1,100}")


class ConfigError(Exception):
    """The command or its environment is unusable. The message is safe to show."""

    exit_code: ClassVar[ExitCode] = ExitCode.CONFIG_ERROR


@dataclass(frozen=True)
class ScanConfig:
    """Everything one scan needs. ``repr`` never shows the token.

    ``amp_url``, ``token`` and ``token_expires_at`` are all set, or all None: the command uploads the results
    directly only when Agent Manager put ``AMP_URL`` and ``AMP_SCAN_TOKEN`` in it.
    """

    amp_url: str | None
    token: str | None = field(repr=False)
    token_expires_at: datetime | None
    """When the token expires; it may already have expired."""
    scan_id: str
    provider: Provider
    organization: str
    all_repositories: bool
    repositories: tuple[str, ...]
    """Repository names. Empty with ``all_repositories``."""
    include_private: bool
    read_ci: bool
    api_url: str
    """The GitHub API address, with no trailing slash."""
    ai_review: bool
    judge: bool

    def __post_init__(self) -> None:
        if self.provider not in get_args(Provider):
            raise ConfigError(f"{self.provider!r} is not a supported provider. This runner scans GitHub only.")


def _url(raw: str, label: str) -> str:
    """Return the http(s) URL without trailing slashes, or raise ConfigError. The value is never echoed.

    Plain http is accepted, as Agent Manager accepts it; callers can tell by the ``http://`` prefix
    (the scheme is returned in lowercase).
    """
    value = raw.strip()
    if any(c.isspace() or c == "\\" or not c.isprintable() for c in value):
        raise ConfigError(f"{label} is not a valid URL.")
    try:
        parts = urlsplit(value)
        _ = parts.port  # Raises ValueError for a port that is not a number in range.
    except ValueError:
        raise ConfigError(f"{label} is not a valid URL.") from None
    if parts.scheme not in ("http", "https"):
        raise ConfigError(f"{label} must start with http:// or https://.")
    if not parts.hostname:
        raise ConfigError(f"{label} is not a valid URL.")
    if "@" in parts.netloc:
        raise ConfigError(f"{label} must not contain a user name or password.")
    if "?" in value or "#" in value:
        raise ConfigError(f"{label} must not contain a query or fragment.")
    value = parts.scheme + value[len(parts.scheme) :]  # urlsplit lowercases the scheme: HTTP:// becomes http://.
    return value.rstrip("/")


def _names(
    raw: str, flag: str, noun: str, *, name_of: Callable[[str], str], fold_case: bool, limit: int | None = None
) -> tuple[str, ...]:
    """Split a comma-separated list: trim, drop empties, check each with ``name_of`` and de-duplicate.

    ``name_of`` returns the name to keep or raises ConfigError. The first spelling of a duplicate is kept.
    """
    seen: set[str] = set()
    names: list[str] = []
    for item in raw.split(","):
        if not item.strip():
            continue
        name = name_of(item.strip())
        key = name.casefold() if fold_case else name
        if key not in seen:
            seen.add(key)
            names.append(name)
    if not names:
        raise ConfigError(f"{flag} lists no {noun} names.")
    if limit is not None and len(names) > limit:
        raise ConfigError(
            f"{flag} lists {len(names)} {noun} names, and a scan takes at most {limit}. "
            f"Scan them all instead, or split them across platforms."
        )
    return tuple(names)


def _github(args: argparse.Namespace) -> tuple[str, bool, tuple[str, ...], str]:
    org: str = args.github_org.strip()
    if not _GITHUB_ORG.fullmatch(org):
        raise ConfigError(f"{org!r} is not a GitHub organization or user name, such as my-org.")
    all_repos: bool = args.all_repos
    repos: tuple[str, ...] = ()
    if args.repos is not None:

        def repo_name(item: str) -> str:
            # Like Agent Manager: owner/name is accepted for the scanned owner, and kept as the name.
            owner, slash, name = item.partition("/")
            if not slash:
                name = owner
            elif owner.casefold() != org.casefold():
                raise ConfigError(f"Repository {item!r} is not in {org}.")
            if not _GITHUB_REPO.fullmatch(name) or name in (".", ".."):  # GitHub forbids . and ..
                raise ConfigError(f"{item!r} is not a GitHub repository name.")
            return name

        repos = _names(args.repos, "--repos", "repository", name_of=repo_name, fold_case=True, limit=MAX_REPOSITORIES)
    elif not all_repos:
        raise ConfigError("Choose what to scan: --all-repos, or --repos with a comma-separated list of names.")
    api_url = DEFAULT_GITHUB_API_URL
    if args.github_api_url is not None:
        api_url = _url(args.github_api_url, "--github-api-url")
    return org, all_repos, repos, api_url


def check_scan_id(raw: str) -> str:
    """Return ``--scan-id`` trimmed and in lowercase, or raise ConfigError when it is not a scan ID."""
    scan_id = raw.strip()
    if not _SCAN_ID.fullmatch(scan_id):
        raise ConfigError(f"--scan-id must be the scan ID that Agent Manager issued. {_COPY_AGAIN}")
    return scan_id.lower()


def agent_manager_env(
    env: Mapping[str, str] | None = None,
    *,
    min_seconds: int = DEFAULT_MIN_SECONDS,
    now: Callable[[], float] = time.time,
) -> tuple[str, str, datetime]:
    """Return ``AMP_URL`` (checked, no trailing slash), ``AMP_SCAN_TOKEN`` and the token's expiry.

    ``env`` defaults to ``os.environ``. Raises ConfigError for an unset or unusable variable, and
    ScanTokenError when the token has under ``min_seconds`` left or cannot be read.
    """
    env = os.environ if env is None else env
    amp_url = env.get("AMP_URL", "")
    if not amp_url.strip():
        raise ConfigError(f"AMP_URL is not set. {_COPY_AGAIN}")
    amp_url = _url(amp_url, "AMP_URL")
    token = env.get("AMP_SCAN_TOKEN", "").strip()
    if not token:
        raise ConfigError(f"AMP_SCAN_TOKEN is not set. {_COPY_AGAIN}")
    return amp_url, token, require_lifetime(token, min_seconds, now=now)


def scan_agent_manager_env(env: Mapping[str, str] | None = None) -> tuple[str | None, str | None, datetime | None]:
    """Return ``AMP_URL``, ``AMP_SCAN_TOKEN`` and the token's expiry for a scan, or three Nones without a token.

    The token is optional: without it the results are only saved. With it, ``AMP_URL`` is required and the
    token must be readable, but it may have expired (the scan then saves the results for the console).
    Without a token ``AMP_URL`` is ignored. Raises ConfigError or ScanTokenError.
    """
    env = os.environ if env is None else env
    token = env.get("AMP_SCAN_TOKEN", "").strip()
    if not token:
        return None, None, None
    amp_url = env.get("AMP_URL", "")
    if not amp_url.strip():
        raise ConfigError(f"AMP_URL is not set. {_COPY_AGAIN}")
    return _url(amp_url, "AMP_URL"), token, token_expiry(token)


def build_scan_config(
    args: argparse.Namespace,
    env: Mapping[str, str] | None = None,
) -> ScanConfig:
    """Build the scan configuration from the parsed ``scan`` flags and the environment.

    ``env`` defaults to ``os.environ``. Raises ConfigError for an unusable flag
    or variable, and ScanTokenError when a given token cannot be read. Both
    have exit code CONFIG_ERROR. A scan does not check the token's lifetime
    (see ``scan_agent_manager_env``).
    """
    scan_id = check_scan_id(args.scan_id)
    provider: Provider = "github"
    org, all_repos, repos, api_url = _github(args)
    ai_review: bool = args.ai_review
    judge: bool = args.judge
    if judge and not ai_review:
        raise ConfigError("--judge needs --ai-review.")

    amp_url, token, expires_at = scan_agent_manager_env(env)

    return ScanConfig(
        amp_url=amp_url,
        token=token,
        token_expires_at=expires_at,
        scan_id=scan_id,
        provider=provider,
        organization=org,
        all_repositories=all_repos,
        repositories=repos,
        include_private=bool(args.include_private),
        read_ci=bool(args.read_ci),
        api_url=api_url,
        ai_review=ai_review,
        judge=judge,
    )
