/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { ShadowProviderSpec } from "@agent-management-platform/types";

/**
 * What a source host connection scans: its settings and endpoints as the
 * service stores them (or as the form would save them).
 */
export interface ScanCommandInput {
  settings: Record<string, unknown> | undefined;
  endpoints: Record<string, string> | undefined;
}

export interface ScanCommandOptions {
  aiReview: boolean;
  /**
   * The command uploads its results with a scan token. Off, it has no Agent
   * Manager address or token and only saves the results file.
   */
  directUpload: boolean;
}

/** The parts the service issues for one scan; the token only with direct upload. */
export interface IssuedScan {
  ampUrl: string;
  token?: string;
  scanId: string;
  image: string;
}

/** A value as a shell word: simple values as they are, others in single quotes. */
export function shellQuote(v: string): string {
  if (/^[A-Za-z0-9._,:/@=-]+$/.test(v)) return v;
  return `'${v.replace(/'/g, `'\\''`)}'`;
}

const text = (v: unknown) => (typeof v === "string" ? v.trim() : "");
const items = (v: unknown) =>
  (Array.isArray(v) ? v : [])
    .map((x) => (typeof x === "string" ? x.trim() : ""))
    .filter(Boolean);

/** The connection's endpoint URL when it is not the official default. */
function customURL(spec: Pick<ShadowProviderSpec, "endpoints">, input: ScanCommandInput) {
  const e = spec.endpoints[0];
  if (!e) return "";
  const url = (input.endpoints?.[e.key] ?? "").trim().replace(/\/+$/, "");
  return url && url !== e.default.replace(/\/+$/, "") ? url : "";
}

/**
 * The docker command that scans a GitHub connection on the
 * customer's machine. Until a scan is issued, the token, scan ID, Agent
 * Manager address and image are placeholders. Without direct upload the
 * command has no Agent Manager address or token lines at all.
 */
export function buildScanCommand(
  spec: Pick<ShadowProviderSpec, "provider" | "endpoints">,
  input: ScanCommandInput,
  options: ScanCommandOptions,
  issued?: IssuedScan,
): string {
  const s = input.settings ?? {};
  const org = text(s.organization);
  // One option per line, so the command reads top to bottom in a narrow panel.
  const lines: string[] = [];
  // "Every repository" is its own flag: a list can hold a repository named "all".
  // The list is joined with "=", so a name starting with "-" is not read as an option.
  const scope = (all: unknown, allFlag: string, list: unknown, listFlag: string, none: string) => {
    if (all === true) return allFlag;
    const names = items(list);
    return `${listFlag}=${names.length > 0 ? shellQuote(names.join(",")) : none}`;
  };
  const url = customURL(spec, input);
  lines.push(`--github-org ${org ? shellQuote(org) : "<organization>"}`);
  lines.push(scope(s.allRepositories, "--all-repos", s.repositories, "--repos", "<repositories>"));
  if (s.privateRepositories === true) lines.push("--include-private");
  if (s.readCiFiles === true) lines.push("--read-ci");
  if (url) lines.push(`--github-api-url ${shellQuote(url)}`);
  // The LLM judge (--judge) is not offered until the command runs it.
  if (options.aiReview) lines.push("--ai-review");
  const upload = options.directUpload
    ? [
        `-e AMP_URL=${issued ? shellQuote(issued.ampUrl) : "<amp-url>"}`,
        `-e AMP_SCAN_TOKEN=${issued?.token ? shellQuote(issued.token) : "<scan-token>"}`,
      ]
    : [];
  return [
    "docker run --rm -it",
    `--user "$(id -u):$(id -g)"`,
    `-v "$PWD:/out"`,
    ...upload,
    `${issued ? shellQuote(issued.image) : "<scanner-image>"} scan`,
    `--scan-id ${issued ? shellQuote(issued.scanId) : "<scan-id>"}`,
    ...lines,
  ].join(" \\\n  ");
}
