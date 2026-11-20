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

import { describe, expect, it } from "vitest";
import { buildScanCommand, shellQuote } from "./scanCommand";

const github = {
  provider: "github" as const,
  endpoints: [{ key: "api", label: "GitHub API URL", default: "https://api.github.com", placeholders: [] }],
};
const off = { aiReview: false, llmJudge: false };
// The connection's options: the lines after --scan-id, one option per line.
const lastLine = (cmd: string) => {
  const lines = cmd.split("\n").map((l) => l.trim().replace(/ \\$/, ""));
  return lines.slice(lines.findIndex((l) => l.startsWith("--scan-id")) + 1).join(" ");
};

describe("buildScanCommand", () => {
  it("uses placeholders until a scan is issued", () => {
    expect(buildScanCommand(github, { settings: {}, endpoints: {} }, off)).toBe(
      [
        "docker run --rm -it \\",
        `  --user "$(id -u):$(id -g)" \\`,
        `  -v "$PWD:/out" \\`,
        "  -e AMP_URL=<amp-url> \\",
        "  -e AMP_SCAN_TOKEN=<scan-token> \\",
        "  <scanner-image> scan \\",
        "  --scan-id <scan-id> \\",
        "  --github-org <organization> \\",
        "  --repos=<repositories>",
      ].join("\n"),
    );
  });

  it("scans every repository or the listed ones, even one named all", () => {
    const all = { organization: "acme", allRepositories: true, repositories: ["x"] };
    expect(lastLine(buildScanCommand(github, { settings: all, endpoints: {} }, off))).toBe(
      "--github-org acme --all-repos",
    );
    const listed = { organization: "acme", allRepositories: false, repositories: ["a", "b"] };
    expect(lastLine(buildScanCommand(github, { settings: listed, endpoints: {} }, off))).toBe(
      "--github-org acme --repos=a,b",
    );
    const named = { organization: "acme", repositories: ["all"] };
    expect(lastLine(buildScanCommand(github, { settings: named, endpoints: {} }, off))).toBe(
      "--github-org acme --repos=all",
    );
  });

  it("joins the list to its flag, so a name starting with - is not read as an option", () => {
    const dash = { organization: "acme", repositories: ["-foo", "bar"] };
    expect(lastLine(buildScanCommand(github, { settings: dash, endpoints: {} }, off))).toBe(
      "--github-org acme --repos=-foo,bar",
    );
    const spaced = { organization: "acme", repositories: ["my repo"] };
    expect(lastLine(buildScanCommand(github, { settings: spaced, endpoints: {} }, off))).toBe(
      "--github-org acme --repos='my repo'",
    );
  });

  it("adds the private, CI and AI flags", () => {
    const settings = {
      organization: "acme",
      allRepositories: true,
      privateRepositories: true,
      readCiFiles: true,
    };
    const input = { settings, endpoints: {} };
    expect(lastLine(buildScanCommand(github, input, { aiReview: true, llmJudge: true }))).toBe(
      "--github-org acme --all-repos --include-private --read-ci --ai-review --judge",
    );
    // The judge needs AI review.
    expect(lastLine(buildScanCommand(github, input, { aiReview: false, llmJudge: true }))).toBe(
      "--github-org acme --all-repos --include-private --read-ci",
    );
  });

  it("passes the API URL only when it is not the default", () => {
    const settings = { organization: "acme", allRepositories: true };
    expect(
      lastLine(buildScanCommand(github, { settings, endpoints: { api: "https://api.github.com/" } }, off)),
    ).toBe("--github-org acme --all-repos");
    expect(
      lastLine(
        buildScanCommand(github, { settings, endpoints: { api: "https://ghe.acme.com/api/v3" } }, off),
      ),
    ).toBe("--github-org acme --all-repos --github-api-url https://ghe.acme.com/api/v3");
  });

  it("has no Azure DevOps options", () => {
    const cmd = buildScanCommand(github, { settings: { organization: "acme" }, endpoints: {} }, off);
    expect(cmd).not.toMatch(/azure|--projects|--all-projects/i);
  });

  it("quotes values a shell would split or expand", () => {
    expect(shellQuote("my-org")).toBe("my-org");
    expect(shellQuote("My Project")).toBe("'My Project'");
    expect(shellQuote("it's")).toBe(`'it'\\''s'`);
    expect(shellQuote("$(rm -rf /)")).toBe("'$(rm -rf /)'");
    const settings = { organization: "Acme Ltd", repositories: ["my repo", "web"] };
    expect(lastLine(buildScanCommand(github, { settings, endpoints: {} }, off))).toBe(
      "--github-org 'Acme Ltd' --repos='my repo,web'",
    );
  });

  it("fills in the issued values", () => {
    const cmd = buildScanCommand(
      github,
      { settings: { organization: "acme", allRepositories: true }, endpoints: {} },
      off,
      { ampUrl: "https://amp.example.com", token: "tok.en-1", scanId: "s9", image: "ghcr.io/wso2/scanner:1.0" },
    );
    expect(cmd).toContain("-e AMP_URL=https://amp.example.com \\");
    expect(cmd).toContain("-e AMP_SCAN_TOKEN=tok.en-1 \\");
    expect(cmd).toContain("ghcr.io/wso2/scanner:1.0 scan \\");
    expect(cmd).toContain("--scan-id s9 \\");
    expect(cmd).not.toMatch(/<[a-z-]+>/);
  });
});
