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

"""amp_shadow_runner.gitsource.github against local HTTP servers that answer like the GitHub REST API."""

import io
import json
import tarfile
import threading
from collections.abc import Callable, Iterator
from datetime import UTC, datetime
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any

import pytest

from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.fetch import extract
from amp_shadow_runner.gitsource import FailedRepository, GitSourceError, Repository, RepositoryError
from amp_shadow_runner.gitsource.github import (
    MOVED_REASON,
    NOT_FOUND_REASON,
    PRIVATE_REASON,
    GitHubSource,
    token_check,
)
from amp_shadow_runner.httpclient import HttpClient

TOKEN = "github_pat_s3cr3t-token-value"
SHA = "0123456789abcdef0123456789abcdef01234567"
PREFIX = "/api/v3"  # A GitHub Enterprise Server API path, so the prefix is kept on every request.
ORG_REPOS = f"{PREFIX}/orgs/acme/repos?type=all&per_page=100"
Reply = tuple[int, dict[str, str], bytes]


class _Handler(BaseHTTPRequestHandler):
    server: "_Server"

    def do_GET(self) -> None:  # noqa: N802 (the base class's name)
        self.server.seen.append((self.path, {k.lower(): v for k, v in self.headers.items()}))
        replies = self.server.routes.get(self.path)
        if replies is None:
            status, headers, body = _json(404, {"message": "Not Found"})
        else:
            status, headers, body = replies.pop(0) if len(replies) > 1 else replies[0]
        self.send_response(status)
        for name, value in headers.items():
            self.send_header(name, value)
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args: Any) -> None:  # noqa: A002 (the base class's name)
        pass


class _Server(ThreadingHTTPServer):
    """Answers each path and query with its replies in turn (repeating the last), and 404 otherwise."""

    def __init__(self) -> None:
        super().__init__(("127.0.0.1", 0), _Handler)
        self.routes: dict[str, list[Reply]] = {}
        self.seen: list[tuple[str, dict[str, str]]] = []

    def url(self, path: str = "") -> str:
        return f"http://127.0.0.1:{self.server_address[1]}{path}"

    def route(self, path: str, *replies: Reply) -> None:
        self.routes[path] = list(replies)

    def paths(self) -> list[str]:
        return [path for path, _ in self.seen]


def _start() -> Iterator[_Server]:
    server = _Server()
    thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.01}, daemon=True)
    thread.start()
    yield server
    server.shutdown()
    server.server_close()
    thread.join()


@pytest.fixture
def api() -> Iterator[_Server]:
    yield from _start()


@pytest.fixture
def other() -> Iterator[_Server]:
    """A second server on another port, so another origin (like codeload.github.com)."""
    yield from _start()


@pytest.fixture(autouse=True)
def _no_proxy(monkeypatch: pytest.MonkeyPatch) -> None:
    for name in ("http_proxy", "https_proxy", "all_proxy", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"):
        monkeypatch.delenv(name, raising=False)
    monkeypatch.setenv("no_proxy", "*")


@pytest.fixture
def sleeps() -> list[float]:
    return []


@pytest.fixture
def http(sleeps: list[float]) -> HttpClient:
    return HttpClient(timeout=2, sleep=sleeps.append, jitter=lambda: 0.0)


@pytest.fixture
def make(api: _Server, http: HttpClient, sleeps: list[float]) -> Callable[..., GitHubSource]:
    def make(*, token: str | None = TOKEN, include_private: bool = False, max_pages: int = 55) -> GitHubSource:
        return GitHubSource(
            api.url(PREFIX) + "/",
            "acme",
            token=token,
            include_private=include_private,
            http=http,
            sleep=sleeps.append,
            max_pages=max_pages,
        )

    return make


def _json(status: int, data: object, **headers: str) -> Reply:
    body = json.dumps(data).encode()
    return status, {"Content-Type": "application/json", "Content-Length": str(len(body)), **headers}, body


def _repo(name: str, visibility: str = "public", **extra: object) -> dict[str, object]:
    return {
        "name": name,
        "private": visibility != "public",
        "visibility": visibility,
        "default_branch": "main",
        "owner": {"login": "acme"},
        **extra,
    }


def _fails(call: Callable[[], object], kind: type[GitSourceError] = GitSourceError) -> GitSourceError:
    with pytest.raises(kind) as info:
        call()
    assert TOKEN not in str(info.value)
    assert TOKEN not in repr(info.value)
    return info.value


def _names(repos: tuple[Repository, ...]) -> list[str]:
    return [r.name for r in repos]


def test_org_listing_follows_next_pages_with_the_token(api: _Server, make: Callable[..., GitHubSource]) -> None:
    page2 = f"{PREFIX}/organizations/7/repos?type=all&per_page=100&page=2"
    link = f'<{api.url(page2)}>; rel="next", <{api.url(page2)}>; rel="last"'
    api.route(ORG_REPOS, _json(200, [_repo("app"), _repo("lib")], Link=link))
    api.route(page2, _json(200, [_repo("tool")], Link=f'<{api.url(ORG_REPOS)}>; rel="prev"'))

    listing = make().list_all()

    assert _names(listing.repositories) == ["app", "lib", "tool"]
    assert (listing.gone, listing.failed, listing.complete) == ((), (), True)
    assert api.paths() == [ORG_REPOS, page2]
    for _, headers in api.seen:
        assert headers["authorization"] == f"Bearer {TOKEN}"
        assert headers["accept"] == "application/vnd.github+json"
        assert headers["x-github-api-version"] == "2022-11-28"


def test_next_page_on_another_origin_is_refused(
    api: _Server, other: _Server, make: Callable[..., GitHubSource]
) -> None:
    api.route(ORG_REPOS, _json(200, [_repo("app")], Link=f'<{other.url("/steal?page=2")}>; rel="next"'))
    exc = _fails(make().list_all)
    assert "not at 127.0.0.1" in str(exc)
    assert other.seen == []


def test_page_cap_makes_the_list_incomplete(api: _Server, make: Callable[..., GitHubSource]) -> None:
    api.route(ORG_REPOS, _json(200, [_repo("app")], Link=f'<{api.url(ORG_REPOS)}>; rel="next"'))
    listing = make(max_pages=2).list_all()
    assert listing.complete is False
    assert _names(listing.repositories) == ["app"]  # The repeated entry is kept once.
    assert len(api.seen) == 2


def test_user_account_is_listed_when_no_org_has_the_name(api: _Server, make: Callable[..., GitHubSource]) -> None:
    api.route(f"{PREFIX}/users/acme/repos?type=owner&per_page=100", _json(200, [_repo("dotfiles")]))
    listing = make(token=None).list_all()
    assert _names(listing.repositories) == ["dotfiles"]
    assert api.paths() == [ORG_REPOS, f"{PREFIX}/users/acme/repos?type=owner&per_page=100"]


def test_token_of_the_user_lists_its_private_repositories(api: _Server, make: Callable[..., GitHubSource]) -> None:
    own = f"{PREFIX}/user/repos?affiliation=owner&visibility=all&per_page=100"
    api.route(f"{PREFIX}/user", _json(200, {"login": "Acme"}))
    api.route(own, _json(200, [_repo("secret", "private")]))
    listing = make(include_private=True).list_all()
    assert _names(listing.repositories) == ["secret"]
    assert listing.complete is True
    assert api.paths() == [ORG_REPOS, f"{PREFIX}/user", own]


def test_token_of_another_user_lists_the_public_user_repositories(
    api: _Server, make: Callable[..., GitHubSource]
) -> None:
    api.route(f"{PREFIX}/user", _json(200, {"login": "someone-else"}))
    api.route(f"{PREFIX}/users/acme/repos?type=owner&per_page=100", _json(200, [_repo("dotfiles")]))
    assert _names(make().list_all().repositories) == ["dotfiles"]


@pytest.mark.parametrize(("include_private", "expected"), [(False, ["open"]), (True, ["open", "closed", "inner"])])
def test_visibility_filter(
    api: _Server, make: Callable[..., GitHubSource], include_private: bool, expected: list[str]
) -> None:
    repos = [_repo("open"), _repo("closed", "private"), _repo("inner", "internal")]
    api.route(ORG_REPOS, _json(200, repos))
    listing = make(include_private=include_private).list_all()
    assert _names(listing.repositories) == expected
    assert [r.private for r in listing.repositories] == [name != "open" for name in expected]
    assert listing.complete is True


def test_sso_partial_results_make_the_list_incomplete(api: _Server, make: Callable[..., GitHubSource]) -> None:
    sso = 'partial-results; organizations="21955855"'
    api.route(ORG_REPOS, _json(200, [_repo("open"), _repo("closed", "private")], **{"X-GitHub-SSO": sso}))
    listing = make(include_private=True).list_all()
    assert (_names(listing.repositories), listing.complete) == (["open", "closed"], False)


def test_private_scan_that_sees_no_private_repository_is_incomplete(
    api: _Server, make: Callable[..., GitHubSource]
) -> None:
    api.route(ORG_REPOS, _json(200, [_repo("open")]))
    listing = make(include_private=True).list_all()
    assert (_names(listing.repositories), listing.complete) == (["open"], False)


def test_entries_without_visibility_use_private(api: _Server, make: Callable[..., GitHubSource]) -> None:
    old = [{"name": "closed", "private": True, "default_branch": "main"}, {"name": "open", "private": False}]
    api.route(ORG_REPOS, _json(200, old))  # Older GitHub Enterprise Server has no visibility field.
    listing = make(include_private=True).list_all()
    assert [(r.name, r.private, r.default_branch) for r in listing.repositories] == [
        ("closed", True, "main"),
        ("open", False, None),
    ]
    assert listing.complete is True


@pytest.mark.parametrize(
    "entry",
    [
        {"name": "odd", "private": "yes", "visibility": "public"},
        {"name": "odd", "visibility": "public"},
        {"name": "..", "private": False, "visibility": "public"},
        "not an object",
    ],
)
def test_unreadable_entry_makes_the_list_incomplete(
    api: _Server, make: Callable[..., GitHubSource], entry: object
) -> None:
    api.route(ORG_REPOS, _json(200, [_repo("open"), entry]))
    listing = make().list_all()
    assert (_names(listing.repositories), listing.complete) == (["open"], False)


def test_named_repositories_with_private_access_are_found_gone_or_failed(
    api: _Server, make: Callable[..., GitHubSource], sleeps: list[float]
) -> None:
    api.route(f"{PREFIX}/repos/acme/App", _json(200, _repo("app")))
    api.route(f"{PREFIX}/repos/acme/closed", _json(200, _repo("closed", "private")))
    api.route(f"{PREFIX}/repos/acme/broken", _json(500, {"message": "Server Error"}))

    listing = make(include_private=True).list_named(["App", "closed", "deleted", "broken"])

    assert _names(listing.repositories) == ["app", "closed"]  # GitHub's spelling.
    assert listing.gone == ("deleted",)
    assert listing.failed == (FailedRepository("broken", "GitHub answered HTTP 500: Server Error"),)
    assert listing.complete is True
    assert len(sleeps) == 3  # The client retried the 500.


@pytest.mark.parametrize(("token", "include_private"), [(None, False), (TOKEN, False), (None, True)])
def test_named_repositories_without_private_access_are_never_gone(
    api: _Server, make: Callable[..., GitHubSource], token: str | None, include_private: bool
) -> None:
    api.route(f"{PREFIX}/repos/acme/closed", _json(200, _repo("closed", "private")))
    listing = make(token=token, include_private=include_private).list_named(["missing", "closed"])
    assert listing.gone == ()
    expected = [FailedRepository("missing", NOT_FOUND_REASON)]
    if not include_private:
        expected.append(FailedRepository("closed", PRIVATE_REASON))
    assert list(listing.failed) == expected


def test_named_repository_moved_to_another_owner_is_failed(api: _Server, make: Callable[..., GitHubSource]) -> None:
    api.route(
        f"{PREFIX}/repos/acme/old",
        (301, {"Location": api.url(f"{PREFIX}/repositories/42"), "Content-Length": "0"}, b""),
    )
    api.route(f"{PREFIX}/repositories/42", _json(200, _repo("old", owner={"login": "someone-else"})))
    listing = make(include_private=True).list_named(["old"])
    assert (listing.repositories, listing.failed) == ((), (FailedRepository("old", MOVED_REASON),))


def test_commit_is_resolved_from_the_default_branch(api: _Server, make: Callable[..., GitHubSource]) -> None:
    api.route(f"{PREFIX}/repos/acme/app/git/ref/heads/release/2.x", _json(200, {"object": {"sha": SHA}}))
    repo = make().resolve_commit(Repository("app", False, "release/2.x"))
    assert (repo.commit_sha, repo.empty) == (SHA, False)


def test_empty_repositories_are_marked_empty(api: _Server, make: Callable[..., GitHubSource]) -> None:
    api.route(f"{PREFIX}/repos/acme/new/git/ref/heads/main", _json(409, {"message": "Git Repository is empty."}))
    source = make()
    assert source.resolve_commit(Repository("new", False, "main")).empty is True
    assert source.resolve_commit(Repository("bare", False, None)).empty is True
    assert len(api.seen) == 1  # No default branch: nothing to ask.


def test_missing_default_branch_fails_the_repository(api: _Server, make: Callable[..., GitHubSource]) -> None:
    exc = _fails(lambda: make().resolve_commit(Repository("app", False, "main")), RepositoryError)
    assert isinstance(exc, RepositoryError)
    assert exc.name == "app"
    assert "default branch was not found" in exc.reason


def test_tarball_is_downloaded_through_a_redirect_without_the_token(
    api: _Server, other: _Server, make: Callable[..., GitHubSource], tmp_path: Path
) -> None:
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w:gz") as tar:
        data = b"from openai import OpenAI\n"
        info = tarfile.TarInfo(f"acme-app-{SHA[:7]}/agent.py")
        info.size = len(data)
        tar.addfile(info, io.BytesIO(data))
    archive = buffer.getvalue()
    signed = f"/acme/app/legacy.tar.gz/{SHA}?token=signed-query"
    api.route(
        f"{PREFIX}/repos/acme/app/tarball/{SHA}", (302, {"Location": other.url(signed), "Content-Length": "0"}, b"")
    )
    other.route(signed, (200, {"Content-Length": str(len(archive))}, archive))

    path = make().download_tarball(Repository("app", False, "main", commit_sha=SHA), tmp_path)

    assert api.seen[0][1]["authorization"] == f"Bearer {TOKEN}"
    assert "authorization" not in other.seen[0][1]
    result = extract(path, tmp_path / "tree", kind="tar.gz")
    assert (result.files, result.stripped) == (1, True)
    assert (tmp_path / "tree" / "agent.py").read_bytes() == b"from openai import OpenAI\n"


def test_refused_token(api: _Server, make: Callable[..., GitHubSource]) -> None:
    api.route(ORG_REPOS, _json(401, {"message": "Bad credentials"}))
    exc = _fails(make().list_all)
    assert str(exc) == (
        "GitHub refused the token (it is wrong, expired or revoked). "
        "Create a new fine-grained token and run the command again."
    )
    assert exc.exit_code is ExitCode.CONFIG_ERROR


def test_refused_token_stops_named_repositories(api: _Server, make: Callable[..., GitHubSource]) -> None:
    api.route(f"{PREFIX}/repos/acme/app", _json(401, {"message": "Bad credentials"}))
    exc = _fails(lambda: make().list_named(["app"]))
    assert not isinstance(exc, RepositoryError)
    assert exc.exit_code is ExitCode.CONFIG_ERROR


RESET = int(datetime(2026, 11, 19, 14, 5, tzinfo=UTC).timestamp())


@pytest.mark.parametrize(
    ("token", "expected"),
    [
        (TOKEN, "GitHub's API rate limit for this token is used up until 14:05 UTC. Run the command again after that."),
        (
            None,
            "GitHub's API rate limit for requests without a token is used up until 14:05 UTC. "
            "Pass a token with -e GITHUB_TOKEN for a higher limit. Run the command again after that.",
        ),
    ],
)
def test_primary_rate_limit(
    api: _Server, make: Callable[..., GitHubSource], sleeps: list[float], token: str | None, expected: str
) -> None:
    limited = _json(403, {"message": "API rate limit exceeded"}, **{"x-ratelimit-remaining": "0"})
    limited[1]["x-ratelimit-reset"] = str(RESET)
    api.route(ORG_REPOS, limited)
    exc = _fails(make(token=token).list_all)
    assert str(exc) == expected
    assert exc.exit_code is ExitCode.RUNTIME_ERROR
    assert sleeps == []


def test_secondary_rate_limit_is_waited_out(
    api: _Server, make: Callable[..., GitHubSource], sleeps: list[float]
) -> None:
    slow = _json(403, {"message": "You have exceeded a secondary rate limit."}, **{"Retry-After": "5"})
    api.route(ORG_REPOS, slow, _json(200, [_repo("app")]))
    assert _names(make().list_all().repositories) == ["app"]
    assert sleeps == [5.0]


def test_long_secondary_rate_limit_fails(api: _Server, make: Callable[..., GitHubSource], sleeps: list[float]) -> None:
    api.route(ORG_REPOS, _json(403, {"message": "secondary rate limit"}, **{"Retry-After": "600"}))
    exc = _fails(make().list_all)
    assert "secondary rate limit" in str(exc)
    assert sleeps == []


def test_permission_refusal_names_the_permissions(api: _Server, make: Callable[..., GitHubSource]) -> None:
    api.route(ORG_REPOS, _json(403, {"message": "Resource not accessible by personal access token"}))
    exc = _fails(make().list_all)
    assert "Contents (read-only) and Metadata (read-only)" in str(exc)
    assert "organization may need to approve the token" in str(exc)
    assert "GitHub said: Resource not accessible by personal access token" in str(exc)
    assert exc.exit_code is ExitCode.CONFIG_ERROR


def test_permission_refusal_on_a_download_fails_only_that_repository(
    api: _Server, make: Callable[..., GitHubSource], tmp_path: Path
) -> None:
    api.route(f"{PREFIX}/repos/acme/app/tarball/{SHA}", _json(403, {"message": "Resource not accessible"}))
    exc = _fails(lambda: make().download_tarball(Repository("app", False, "main", SHA), tmp_path), RepositoryError)
    assert isinstance(exc, RepositoryError)
    assert "Contents and Metadata read-only" in exc.reason
    assert list(tmp_path.iterdir()) == []


def test_unknown_org_and_user(api: _Server, make: Callable[..., GitHubSource]) -> None:
    exc = _fails(make(token=None).list_all)
    assert str(exc) == f"No GitHub organization or user named acme at {api.url().removeprefix('http://')}."
    assert exc.exit_code is ExitCode.CONFIG_ERROR


def test_unexpected_repository_list(api: _Server, make: Callable[..., GitHubSource]) -> None:
    api.route(ORG_REPOS, _json(200, {"not": "a list"}))
    exc = _fails(make().list_all)
    assert str(exc) == "GitHub sent an unexpected repository list."


def test_token_check(api: _Server, http: HttpClient) -> None:
    check = token_check(api.url(PREFIX), http=http)
    api.route(f"{PREFIX}/user", _json(401, {"message": "Bad credentials"}), _json(200, {"login": "acme"}))
    message = check(TOKEN)
    assert message == "GitHub refused the token (it is wrong, expired or revoked)."
    assert check(TOKEN) is None
    assert api.seen[0][1]["authorization"] == f"Bearer {TOKEN}"
