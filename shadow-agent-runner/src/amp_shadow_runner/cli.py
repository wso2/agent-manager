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

from amp_shadow_runner._version import __version__
from amp_shadow_runner.engine import ENGINE_VERSION, RULE_PACK_ID
from amp_shadow_runner.exitcodes import ExitCode

PROG = "amp-shadow-runner"

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


def _cmd_not_implemented(args: argparse.Namespace) -> int:
    print(f"{PROG} {args.command}: not implemented yet", file=sys.stderr)
    return ExitCode.RUNTIME_ERROR


# (name, help, handler). Every command except ``version`` is a stub for now.
_COMMANDS: tuple[tuple[str, str, Handler], ...] = (
    ("scan", "Scan repositories and publish the confirmed agents to Agent Manager.", _cmd_not_implemented),
    ("upload", "Publish a saved scan report to Agent Manager.", _cmd_not_implemented),
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
        sub = subparsers.add_parser(name, help=help_text, description=help_text)
        sub.set_defaults(handler=handler)
        if name == "version":
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
