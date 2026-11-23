/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it, vi } from "vitest";

// The api-client entry point reads the browser runtime config at import time
// and cannot load under this package's node test environment. The function
// under test is a pure helper, so load the real one from its own source file.
vi.mock("@agent-management-platform/api-client", async () => ({
  normalizePolicyVersionToMajor: (
    await import("../../../../libs/api-client/src/utils/policyVersion")
  ).normalizePolicyVersionToMajor,
}));

import { applyPolicySelection, resolveRequiredMCPPolicy } from "./mcpPolicySelection";

describe("applyPolicySelection", () => {
  const saved = [
    { name: "pii-masking", version: "v1", params: { word: "a" } },
    { name: "other", version: "v1" },
  ];

  it("edits an applied v1 policy against catalog v1.0.2 in place, keeping v1", () => {
    const result = applyPolicySelection(saved, {
      name: "pii-masking",
      version: "v1.0.2",
      displayName: "PII Masking",
      settings: { word: "b" },
    });
    expect(result).toHaveLength(2);
    expect(result[0]).toEqual({
      name: "pii-masking",
      version: "v1",
      displayName: "PII Masking",
      params: { word: "b" },
    });
    expect(result[1]).toBe(saved[1]);
  });

  it("appends a new policy stored under its major version", () => {
    const result = applyPolicySelection(saved, {
      name: "new-policy",
      version: "v2.1.0",
    });
    expect(result).toHaveLength(3);
    expect(result[2]).toMatchObject({ name: "new-policy", version: "v2" });
  });

  it("replaces a stored full-version entry in place, writing back the major", () => {
    const stored = [
      { name: "x", version: "v1.0.2", params: { a: 1 } },
      { name: "other", version: "v1" },
    ];
    const result = applyPolicySelection(stored, {
      name: "x",
      version: "v1.0.5",
      settings: { a: 2 },
    });
    expect(result).toHaveLength(2);
    expect(result[0]).toMatchObject({ name: "x", version: "v1", params: { a: 2 } });
    expect(result[1]).toBe(stored[1]);
  });

  it("appends when the stored major differs", () => {
    const result = applyPolicySelection([{ name: "x", version: "v1.0.2" }], {
      name: "x",
      version: "v2.0.0",
    });
    expect(result).toHaveLength(2);
  });
});

describe("resolveRequiredMCPPolicy", () => {
  const entry = (status: "installed" | "not_installed") => ({
    name: "mcp-rewrite",
    version: "v1.0.2",
    savedVersion: "v1",
    displayName: "MCP Rewrite",
    status,
  });
  const catalog = (
    status: "installed" | "not_installed",
    availabilityKnown: boolean,
  ) =>
    ({
      count: 1,
      data: [entry(status)],
      hubAvailable: true,
      availabilityScope: "deployed",
      availabilityKnown,
    }) as never;
  const saved = [{ name: "mcp-rewrite", version: "v1.0.0", displayName: "Saved Rewrite" }];

  it("uses an installed entry", () => {
    expect(resolveRequiredMCPPolicy(catalog("installed", true), "mcp-rewrite", [])).toEqual({
      version: "v1",
      displayName: "MCP Rewrite",
    });
  });

  it("refuses a not-installed entry when availability is known", () => {
    expect(resolveRequiredMCPPolicy(catalog("not_installed", true), "mcp-rewrite", saved)).toBeUndefined();
  });

  it("uses a not-installed catalog entry when availability is unknown", () => {
    expect(resolveRequiredMCPPolicy(catalog("not_installed", false), "mcp-rewrite", [])).toEqual({
      version: "v1",
      displayName: "MCP Rewrite",
    });
  });

  it("falls back to the saved version when availability is unknown and the catalog lacks the name", () => {
    const empty = { count: 0, data: [], hubAvailable: false, availabilityScope: "deployed", availabilityKnown: false } as never;
    expect(resolveRequiredMCPPolicy(empty, "mcp-rewrite", saved)).toEqual({
      version: "v1",
      displayName: "Saved Rewrite",
    });
    expect(resolveRequiredMCPPolicy(empty, "mcp-rewrite", [])).toBeUndefined();
  });

  it("does not fall back to the saved version when availability is known", () => {
    const empty = { count: 0, data: [], hubAvailable: true, availabilityScope: "deployed", availabilityKnown: true } as never;
    expect(resolveRequiredMCPPolicy(empty, "mcp-rewrite", saved)).toBeUndefined();
  });
});
