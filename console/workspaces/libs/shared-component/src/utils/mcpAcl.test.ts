/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { MCPEndpointConfig } from "@agent-management-platform/types";
import { describe, expect, it } from "vitest";
import {
  MCP_ACL_POLICY_NAME,
  getCapabilityId,
  isToolBlockedByAcl,
  parseAclSection,
} from "./mcpAcl";

describe("getCapabilityId", () => {
  it.each([
    ["tool", undefined, null],
    ["tool", { name: " search " }, "search"],
    ["tool", { uri: "file://a" }, "file://a"],
    ["resource", { uri: "file://a", name: "A" }, "file://a"],
    ["resource", { name: "A" }, "A"],
    ["prompt", { name: "   " }, null],
    ["prompt", { name: 5 }, null],
  ] as const)("%s %j -> %j", (kind, raw, expected) => {
    expect(getCapabilityId(kind, raw as Record<string, unknown> | undefined)).toBe(expected);
  });
});

describe("parseAclSection", () => {
  it("returns no mode when the section is missing", () => {
    expect(parseAclSection(undefined, "tools")).toEqual({ mode: null, exceptions: [] });
    expect(parseAclSection({}, "tools")).toEqual({ mode: null, exceptions: [] });
  });

  it("normalizes mode and keeps only non-blank string exceptions", () => {
    expect(
      parseAclSection({ tools: { mode: "DENY", exceptions: [" a ", "", 3, "b"] } }, "tools"),
    ).toEqual({ mode: "deny", exceptions: ["a", "b"] });
  });

  it("rejects an unknown mode and non-array exceptions", () => {
    expect(parseAclSection({ tools: { mode: "maybe", exceptions: "a" } }, "tools")).toEqual({
      mode: null,
      exceptions: [],
    });
  });
});

describe("isToolBlockedByAcl", () => {
  const config = (mode: string, exceptions: string[]) =>
    ({
      policies: [{ name: MCP_ACL_POLICY_NAME, params: { tools: { mode, exceptions } } }],
    }) as unknown as MCPEndpointConfig;

  it("is never blocked without an ACL policy", () => {
    expect(isToolBlockedByAcl(undefined, "x")).toBe(false);
    expect(isToolBlockedByAcl({ policies: [] } as unknown as MCPEndpointConfig, "x")).toBe(false);
  });

  it.each([
    ["allow", ["x"], "x", true],
    ["allow", ["x"], "y", false],
    ["deny", ["x"], "x", false],
    ["deny", ["x"], "y", true],
  ])("mode %s with exceptions %j blocks %s: %s", (mode, exceptions, tool, expected) => {
    expect(isToolBlockedByAcl(config(mode, exceptions), tool)).toBe(expected);
  });
});
