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

"""List and download the repositories of a GitHub organization or user.

Works with github.com, GHE.com (``https://api.<name>.ghe.com``) and GitHub
Enterprise Server (``https://<host>/api/v3``), which share the REST paths. The
token goes only to the API's origin: a next-page link elsewhere is refused, and
the HTTP client drops it on the redirect to the archive download host.
"""

import re
import time
from collections.abc import Callable, Iterable
from dataclasses import replace
from datetime import UTC, datetime
from pathlib import Path
from urllib.parse import quote, urlsplit

from amp_shadow_runner.config import MAX_REPOSITORIES, ScanConfig
from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.gitsource import FailedRepository, GitSourceError, Repository, RepositoryError, RepositoryList
from amp_shadow_runner.httpclient import HttpClient, HttpError, JsonResponse, origin

API_VERSION = "2022-11-28"
PER_PAGE = 100
MAX_PAGES = 55
"""Pages of a repository list (100 each): MAX_REPOSITORIES plus a margin for the repositories filtered out."""
MAX_TARBALL_BYTES = 1024**3
"""The largest compressed archive downloaded for one repository (1 GiB)."""
SECONDARY_RETRIES = 2
"""Retries after a secondary rate limit whose ``Retry-After`` is at most MAX_SECONDARY_WAIT seconds."""
MAX_SECONDARY_WAIT = 60.0

TOKEN_REFUSED = "GitHub refused the token (it is wrong, expired or revoked)."
PRIVATE_REASON = "private repository; scanning it needs --include-private"
NOT_FOUND_REASON = (
    "not found, or private and not visible to this scan; to scan private repositories "
    "use --include-private with a token that can read it"
)
MOVED_REASON = "the repository moved to another owner"
_PERMISSIONS = (
    "A fine-grained token needs the Contents (read-only) and Metadata (read-only) repository permissions, "
    "and the organization may need to approve the token."
)
_NAME = re.compile(r"[A-Za-z0-9._-]{1,100}")
_SHA = re.compile(r"[0-9a-f]{40}|[0-9a-f]{64}")
_NEXT = re.compile(r'<([^>]*)>\s*;\s*rel="?next"?')


class GitHubSource:
    """The repositories of one GitHub organization or user account.

    ``token``, when given, is sent as ``Authorization: Bearer``. Without
    ``include_private`` only public repositories are kept, even when the token
    can see more. ``sleep`` (default ``time.sleep``) waits out a short secondary
    rate limit; the HttpClient already retries 429 and 5xx responses itself.
    """

    def __init__(
        self,
        api_url: str,
        organization: str,
        *,
        token: str | None = None,
        include_private: bool = False,
        http: HttpClient | None = None,
        sleep: Callable[[float], None] = time.sleep,
        max_pages: int = MAX_PAGES,
    ) -> None:
        self._api_url = api_url.rstrip("/")
        self._org = organization
        self._token = token
        self._include_private = include_private
        self._http = HttpClient() if http is None else http
        self._sleep = sleep
        self._max_pages = max(1, max_pages)

    @classmethod
    def from_config(cls, config: ScanConfig, token: str | None, *, http: HttpClient | None = None) -> "GitHubSource":
        return cls(config.api_url, config.organization, token=token, include_private=config.include_private, http=http)

    def list_all(self) -> RepositoryList:
        """List every repository of the organization, or of the user account when no organization has the name.

        Raises GitSourceError when the list cannot be read.
        """
        org = quote(self._org, safe="")
        try:
            first = self._get(f"/orgs/{org}/repos?type=all&per_page={PER_PAGE}")
        except HttpError as exc:
            if exc.status != 404:
                raise self._listing_error(exc) from None
            path = (
                f"/user/repos?affiliation=owner&visibility=all&per_page={PER_PAGE}"
                if self._is_token_owner()
                else f"/users/{org}/repos?type=owner&per_page={PER_PAGE}"
            )
            try:
                first = self._get(path)
            except HttpError as exc2:
                if exc2.status == 404:
                    raise GitSourceError(
                        f"No GitHub organization or user named {self._org} at {_host(self._api_url)}.",
                        exit_code=ExitCode.CONFIG_ERROR,
                    ) from None
                raise self._listing_error(exc2) from None
        return self._collect(first)

    def list_named(self, names: Iterable[str]) -> RepositoryList:
        """Look up each named repository; one that cannot be scanned is failed with a reason.

        A 404 counts as gone only with a token and ``include_private``: otherwise the repository may be
        private and hidden from this scan, so it is failed. A repository GitHub redirects to another owner
        (after a transfer) is failed, so another owner's code is never scanned.

        Raises GitSourceError only for what stops the whole scan (a refused token, a rate limit).
        """
        repos: list[Repository] = []
        gone: list[str] = []
        failed: list[FailedRepository] = []
        for name in names:
            try:
                response = self._get(f"/repos/{quote(self._org, safe='')}/{quote(name, safe='')}")
            except HttpError as exc:
                fatal = self._account_error(exc)
                if fatal is not None:
                    raise fatal from None
                if exc.status == 404 and self._token is not None and self._include_private:
                    gone.append(name)
                elif exc.status == 404:
                    failed.append(FailedRepository(name, NOT_FOUND_REASON))
                else:
                    failed.append(FailedRepository(name, _reason(exc)))
                continue
            data = response.data
            repo = _repository(data)
            owner = data.get("owner") if isinstance(data, dict) else None
            login = owner.get("login") if isinstance(owner, dict) else None
            if repo is None or not isinstance(login, str):
                failed.append(FailedRepository(name, "GitHub sent an unexpected response"))
            elif login.casefold() != self._org.casefold():
                failed.append(FailedRepository(name, MOVED_REASON))
            elif repo.private and not self._include_private:
                failed.append(FailedRepository(repo.name, PRIVATE_REASON))
            else:
                repos.append(repo)
        return RepositoryList(tuple(repos), tuple(gone), tuple(failed), complete=True)

    def resolve_commit(self, repo: Repository) -> Repository:
        """Return ``repo`` with its default branch's commit, or marked empty when it has no commits.

        Raises RepositoryError when this repository cannot be read, or GitSourceError for a refused token
        or a rate limit.
        """
        if repo.default_branch is None:
            return replace(repo, empty=True)
        path = f"/repos/{quote(self._org, safe='')}/{quote(repo.name, safe='')}/git/ref/heads/"
        try:
            # The ref, not the commit: its response is small and holds no author names or emails.
            response = self._get(path + quote(repo.default_branch, safe="/"))
        except HttpError as exc:
            if exc.status == 409:  # "Git Repository is empty."
                return replace(repo, empty=True)
            if exc.status == 404:  # The branch went away since the listing; keep its agents.
                raise RepositoryError(repo.name, "the default branch was not found; run the scan again") from None
            raise self._repository_error(repo.name, exc) from None
        data = response.data
        target = data.get("object") if isinstance(data, dict) else None
        sha = target.get("sha") if isinstance(target, dict) else None
        if not isinstance(sha, str) or not _SHA.fullmatch(sha):
            raise RepositoryError(repo.name, "GitHub sent an unexpected response")
        return replace(repo, commit_sha=sha)

    def download_tarball(self, repo: Repository, dest_dir: Path) -> Path:
        """Download ``repo`` at its resolved commit into ``dest_dir`` and return the archive's path.

        The archive is for ``fetch.extract(kind="tar.gz")``. Raises RepositoryError when this repository
        cannot be downloaded (including when it is over MAX_TARBALL_BYTES), or GitSourceError for a refused
        token or a rate limit. An error writing the file is raised as the OSError itself.
        """
        if repo.commit_sha is None:
            raise ValueError("resolve the repository's commit before downloading it")
        url = f"{self._api_url}/repos/{quote(self._org, safe='')}/{quote(repo.name, safe='')}/tarball/{repo.commit_sha}"
        dest = dest_dir / f"{repo.commit_sha}.tar.gz"
        # GitHub redirects to a signed codeload URL; the client drops Authorization on that redirect.
        headers = {k: v for k, v in self._headers().items() if k != "Accept"}
        try:
            self._call(lambda: self._http.download(url, dest, max_bytes=MAX_TARBALL_BYTES, headers=headers))
        except HttpError as exc:
            raise self._repository_error(repo.name, exc) from None
        return dest

    def _collect(self, response: JsonResponse) -> RepositoryList:
        found: dict[str, Repository] = {}
        complete = True
        pages = 1
        while True:
            if not isinstance(response.data, list):
                raise GitSourceError("GitHub sent an unexpected repository list.")
            if response.headers.get("X-GitHub-SSO", "").strip().startswith("partial-results"):
                complete = False  # Repositories of an SSO organization the token is not authorized for are left out.
            for item in response.data:
                repo = _repository(item)
                if repo is None:
                    complete = False
                elif self._include_private or not repo.private:
                    found.setdefault(repo.name.casefold(), repo)  # A repository created mid-listing can repeat.
            next_url = _next_link(response.headers.get("Link"))
            if next_url is None:
                break
            if pages >= self._max_pages or len(found) > MAX_REPOSITORIES:
                complete = False
                break
            if not _same_origin(next_url, self._api_url):
                raise GitSourceError(
                    f"GitHub's next page of repositories is not at {_host(self._api_url)}, "
                    "so the runner stopped rather than send the token there."
                )
            try:
                response = self._get_url(next_url)
            except HttpError as exc:
                raise self._listing_error(exc) from None
            pages += 1
        repos = tuple(found.values())
        if len(repos) > MAX_REPOSITORIES:
            repos, complete = repos[:MAX_REPOSITORIES], False
        if self._include_private and not any(r.private for r in repos):
            complete = False  # Most likely a token that cannot see the private repositories, such as one not approved.
        return RepositoryList(repos, complete=complete)

    def _is_token_owner(self) -> bool:
        """True when the token belongs to the account being scanned, so its private repositories can be listed."""
        if self._token is None:
            return False
        try:
            data = self._get("/user").data
        except HttpError as exc:
            fatal = self._account_error(exc)
            if fatal is not None:
                raise fatal from None
            return False  # Such as a GitHub App token, which cannot read /user.
        login = data.get("login") if isinstance(data, dict) else None
        return isinstance(login, str) and login.casefold() == self._org.casefold()

    def _headers(self) -> dict[str, str]:
        return _api_headers(self._token)

    def _get(self, path: str) -> JsonResponse:
        return self._get_url(self._api_url + path)

    def _get_url(self, url: str) -> JsonResponse:
        return self._call(lambda: self._http.get_json(url, headers=self._headers()))

    def _call[T](self, request: Callable[[], T]) -> T:
        """Run ``request``, waiting out a short secondary rate limit (HTTP 403 with ``Retry-After``)."""
        retries = 0
        while True:
            try:
                return request()
            except HttpError as exc:
                wait = _secondary_wait(exc)
                if wait is None or retries >= SECONDARY_RETRIES:
                    raise
            self._sleep(wait)
            retries += 1

    def _account_error(self, exc: HttpError) -> GitSourceError | None:
        """The error that stops the whole scan (a refused token or a rate limit), or None."""
        if exc.status == 401:
            if self._token is None:
                return GitSourceError(
                    f"{_host(self._api_url)} needs a GitHub token. Pass one with -e GITHUB_TOKEN.",
                    exit_code=ExitCode.CONFIG_ERROR,
                )
            return GitSourceError(
                f"{TOKEN_REFUSED} Create a new fine-grained token and run the command again.",
                exit_code=ExitCode.CONFIG_ERROR,
            )
        if exc.status not in (403, 429):
            return None
        headers = exc.headers
        if headers is not None and headers.get("x-ratelimit-remaining", "").strip() == "0":
            until = _reset_time(headers.get("x-ratelimit-reset"))
            who = "this token" if self._token is not None else "requests without a token"
            message = f"GitHub's API rate limit for {who} is used up"
            message += f" until {until} UTC." if until else "."
            if self._token is None:
                message += " Pass a token with -e GITHUB_TOKEN for a higher limit."
            return GitSourceError(message + " Run the command again after that.")
        secondary = "secondary rate limit" in (exc.server_message or "").lower()
        if exc.status == 429 or secondary or (headers is not None and headers.get("Retry-After") is not None):
            return GitSourceError(
                "GitHub asked the runner to slow down (a secondary rate limit). "
                "Wait a few minutes, then run the command again."
            )
        return None

    def _listing_error(self, exc: HttpError) -> GitSourceError:
        fatal = self._account_error(exc)
        if fatal is not None:
            return fatal
        if exc.status == 403:
            message = f"GitHub refused access to the repositories of {self._org} (HTTP 403). {_PERMISSIONS}"
            if exc.server_message:
                message += f" GitHub said: {exc.server_message}"
            return GitSourceError(message, exit_code=ExitCode.CONFIG_ERROR)
        return GitSourceError(str(exc))

    def _repository_error(self, name: str, exc: HttpError) -> GitSourceError:
        fatal = self._account_error(exc)
        return fatal if fatal is not None else RepositoryError(name, _reason(exc))


def token_check(api_url: str, *, http: HttpClient | None = None) -> Callable[[str], str | None]:
    """Return a ``check`` for ``prompts.ask_secret`` that asks GitHub whether a token is accepted.

    The check returns a message when GitHub refuses the token (HTTP 401), so it can be typed again,
    and None otherwise: other failures surface when the repositories are listed.
    """
    client = HttpClient() if http is None else http
    url = api_url.rstrip("/") + "/user"

    def check(token: str) -> str | None:
        try:
            client.get_json(url, headers=_api_headers(token))
        except HttpError as exc:
            return TOKEN_REFUSED if exc.status == 401 else None
        return None

    return check


def _api_headers(token: str | None) -> dict[str, str]:
    headers = {"Accept": "application/vnd.github+json", "X-GitHub-Api-Version": API_VERSION}
    if token is not None:
        headers["Authorization"] = f"Bearer {token}"
    return headers


def _repository(data: object) -> Repository | None:
    """A Repository from GitHub's JSON, or None when the entry is not readable."""
    if not isinstance(data, dict):
        return None
    name = data.get("name")
    if not isinstance(name, str) or not _NAME.fullmatch(name) or name in (".", ".."):
        return None
    private = data.get("private")
    if not isinstance(private, bool):
        return None
    visibility = data.get("visibility")  # Absent on older GitHub Enterprise Server: ``private`` alone decides.
    if isinstance(visibility, str):
        private = private or visibility != "public"  # Internal repositories count as private.
    branch = data.get("default_branch")
    return Repository(name, private, branch if isinstance(branch, str) and branch else None)


def _reason(exc: HttpError) -> str:
    """A short reason for a failed repository, safe to publish."""
    said = f": {exc.server_message}" if exc.server_message else ""
    if exc.status == 403:
        return (
            "GitHub refused access (HTTP 403); the token needs Contents and Metadata read-only, "
            f"and the organization may need to approve it{said}"
        )
    if exc.status is not None and exc.status >= 400:
        return f"GitHub answered HTTP {exc.status}{said}"
    return str(exc)


def _secondary_wait(exc: HttpError) -> float | None:
    """Seconds to wait before retrying a secondary rate limit, or None when it is not one to wait out."""
    if exc.status != 403 or exc.headers is None or exc.headers.get("x-ratelimit-remaining", "").strip() == "0":
        return None
    value = (exc.headers.get("Retry-After") or "").strip()
    if not (value.isdecimal() and value.isascii()):
        return None
    wait = float(value)
    return wait if wait <= MAX_SECONDARY_WAIT else None


def _reset_time(value: str | None) -> str | None:
    """``x-ratelimit-reset`` (epoch seconds) as UTC HH:MM, or None when unreadable."""
    value = (value or "").strip()
    if not (value.isdecimal() and value.isascii()):
        return None
    try:
        return datetime.fromtimestamp(int(value), UTC).strftime("%H:%M")
    except (OverflowError, OSError, ValueError):
        return None


def _next_link(header: str | None) -> str | None:
    match = _NEXT.search(header or "")
    return match.group(1).strip() if match else None


def _same_origin(url: str, base: str) -> bool:
    try:
        return origin(url) == origin(base)
    except ValueError:  # An invalid address.
        return False


def _host(api_url: str) -> str:
    """The host to name in messages: github.com for api.github.com, the host otherwise."""
    host = urlsplit(api_url).netloc
    return "github.com" if host == "api.github.com" else host
