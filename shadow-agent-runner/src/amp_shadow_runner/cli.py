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

"""Command-line entry point for ``amp-shadow-runner``."""

import argparse
import json
import sys
from collections.abc import Callable, Sequence
from pathlib import Path

from amp_shadow_runner._version import __version__
from amp_shadow_runner.config import (
    DEFAULT_GITHUB_API_URL,
    ConfigError,
    agent_manager_env,
    build_scan_config,
    check_scan_id,
)
from amp_shadow_runner.engine import ENGINE_VERSION, RULE_PACK_ID
from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.pipeline import YES_ENV, PublishInterrupted, assume_yes, run_scan
from amp_shadow_runner.publish import Publisher, PublishError, published_line, report_exit_code
from amp_shadow_runner.report import DEFAULT_REPORT_PATH, ReportError, read_report
from amp_shadow_runner.scantoken import ScanTokenError

PROG = "amp-shadow-runner"
UPLOAD_MIN_SECONDS = 60
"""``upload`` only publishes, so it needs a minute of token lifetime, not a scan's ten."""

Handler = Callable[[argparse.Namespace], int]


def version_info() -> dict[str, str]:
    """Return the versions the runner reports to Agent Manager and to ``version``."""
    return {
        "runner_version": __version__,
        "engine_version": ENGINE_VERSION,
        "rule_pack_id": RULE_PACK_ID,
    }


def _cmd_version(args: argparse.Namespace) -> int:
    info = version_info()
    if args.json:
        print(json.dumps(info, sort_keys=True))
    else:
        print(f"runner:    {info['runner_version']}")
        print(f"engine:    {info['engine_version']}")
        print(f"rule pack: {info['rule_pack_id']}")
    return ExitCode.OK


def _cmd_scan(args: argparse.Namespace) -> int:
    try:
        config = build_scan_config(args)
        return run_scan(config, yes=assume_yes(args.yes))
    except (ConfigError, ScanTokenError, ReportError) as exc:  # Their messages never contain a token.
        print(f"{PROG}: {exc}", file=sys.stderr)
        return exc.exit_code
    except PublishInterrupted:
        print(
            f"\n{PROG}: Cancelled while publishing. The results may already have been published; "
            "check the scan in Agent Manager.",
            file=sys.stderr,
        )
        return ExitCode.RUNTIME_ERROR
    except KeyboardInterrupt:  # The workspace is already deleted.
        print(f"\n{PROG}: Cancelled. Nothing was published.", file=sys.stderr)
        return ExitCode.RUNTIME_ERROR


def _cmd_upload(args: argparse.Namespace) -> int:
    path = Path(args.report)
    try:
        scan_id = check_scan_id(args.scan_id)
        amp_url, token, expires_at = agent_manager_env(min_seconds=UPLOAD_MIN_SECONDS)
        report = read_report(path)
        if report.scan_id.lower() != scan_id:
            raise ConfigError(
                f"{path} is the report of scan {report.scan_id}, not of scan {scan_id}. "
                "Upload a report with the scan ID it was made for."
            )
        result = Publisher(amp_url, token, expires_at=expires_at).publish(report, report_location=str(path))
    except (ConfigError, ScanTokenError, ReportError, PublishError) as exc:  # Their messages never hold a token.
        print(f"{PROG}: {exc}", file=sys.stderr)
        return exc.exit_code
    print(published_line(result), file=sys.stderr)
    return report_exit_code(report)


def _add_upload_arguments(sub: argparse.ArgumentParser) -> None:
    sub.epilog = "AMP_URL and AMP_SCAN_TOKEN must be set in the environment, as for scan."
    sub.add_argument("--scan-id", required=True, help="The scan ID the report was made for.")
    sub.add_argument(
        "report",
        nargs="?",
        default=str(DEFAULT_REPORT_PATH),
        help=f"The saved scan report (default {DEFAULT_REPORT_PATH}).",
    )


def _add_scan_arguments(sub: argparse.ArgumentParser) -> None:
    sub.epilog = (
        "AMP_URL and AMP_SCAN_TOKEN must be set in the environment. "
        "Agent Manager builds the whole command; copy it from there."
    )
    sub.add_argument("--scan-id", required=True, help="The scan ID that Agent Manager issued.")
    sub.add_argument(
        "--github-org", metavar="ORG", required=True, help="Scan this GitHub organization or user account."
    )

    github = sub.add_argument_group("GitHub")
    repos = github.add_mutually_exclusive_group()
    repos.add_argument("--all-repos", action="store_true", help="Scan every repository.")
    repos.add_argument("--repos", metavar="NAMES", help="Scan these repositories (comma-separated names).")
    github.add_argument("--include-private", action="store_true", help="Also scan private repositories.")
    github.add_argument(
        "--github-api-url",
        metavar="URL",
        help=f"The GitHub API address, for GitHub Enterprise (default {DEFAULT_GITHUB_API_URL}).",
    )

    sub.add_argument("--read-ci", action="store_true", help="Also read CI pipeline definitions.")
    sub.add_argument(
        "--ai-review",
        action="store_true",
        help="Use AI-enhanced detection: an LLM checks the agents the rules are not sure of.",
    )
    sub.add_argument(
        "--judge", action="store_true", help="Have a second LLM check the AI-enhanced detection (needs --ai-review)."
    )
    sub.add_argument(
        "--yes",
        action="store_true",
        help=f"Scan the listed repositories without asking to confirm (also {YES_ENV}=1). Needed without a terminal.",
    )


# (name, help, handler).
_COMMANDS: tuple[tuple[str, str, Handler], ...] = (
    ("scan", "Scan repositories and publish the confirmed agents to Agent Manager.", _cmd_scan),
    ("upload", "Publish a saved scan report to Agent Manager.", _cmd_upload),
    ("version", "Print the runner version, engine version and rule-pack id.", _cmd_version),
)


def build_parser() -> argparse.ArgumentParser:
    """Build the argument parser. argparse exits with 2 on usage errors, which is ExitCode.CONFIG_ERROR."""
    parser = argparse.ArgumentParser(
        prog=PROG,
        description="Find AI agents in source code repositories and publish them to WSO2 Agent Manager.",
    )
    parser.add_argument("--version", action="version", version=f"{PROG} {__version__}")
    subparsers = parser.add_subparsers(dest="command", metavar="COMMAND")
    for name, help_text, handler in _COMMANDS:
        sub = subparsers.add_parser(name, help=help_text, description=help_text, allow_abbrev=False)
        sub.set_defaults(handler=handler)
        if name == "scan":
            _add_scan_arguments(sub)
        elif name == "upload":
            _add_upload_arguments(sub)
        elif name == "version":
            sub.add_argument("--json", action="store_true", help="Print the versions as a JSON object.")
    return parser


def _status_from_system_exit(code: str | int | None) -> int:
    if code is None:
        return ExitCode.OK
    if isinstance(code, int):
        return code
    return ExitCode.CONFIG_ERROR


def main(argv: Sequence[str] | None = None) -> int:
    """Run the CLI and return its exit code (see ``ExitCode``)."""
    parser = build_parser()
    try:
        args = parser.parse_args(argv)
    except SystemExit as exc:  # --help and --version exit 0; usage errors exit 2.
        return _status_from_system_exit(exc.code)
    if args.command is None:
        parser.print_usage(sys.stderr)
        print(f"{PROG}: error: a command is required", file=sys.stderr)
        return ExitCode.CONFIG_ERROR
    handler: Handler = args.handler
    return int(handler(args))
