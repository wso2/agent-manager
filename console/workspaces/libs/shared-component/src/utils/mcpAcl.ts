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

// MCP capability ids and the gateway's ACL (allow/deny list) policy shape.
// Shared because two UI modules read them: AI Gateway's MCP server pages,
// which write the policy, and Agent Runtime's agent configuration, which shows
// which tools that policy blocks.
import type { MCPEndpointConfig } from "@agent-management-platform/types";

/** Name of the gateway policy that holds an MCP endpoint's ACL. */
export const MCP_ACL_POLICY_NAME = "mcp-acl-list";

export type CapabilityKind = "tool" | "resource" | "prompt";

// Policy-params section key for each capability kind (the ACL and Rewrite
// policies both key their per-kind config off these names) — shared so every
// reader/writer of those policies' params agrees on the same section names.
export const CAPABILITY_SECTION_KEY: Record<CapabilityKind, string> = {
  tool: "tools",
  resource: "resources",
  prompt: "prompts",
};

// Backend identifier of a capability entry, matching the resolution used by the
// Rewrite / Manage Tools tabs (resources key on uri, tools/prompts on name).
export function getCapabilityId(
  kind: CapabilityKind,
  raw: Record<string, unknown> | undefined,
): string | null {
  if (!raw) return null;
  const value =
    kind === "resource" ? (raw.uri ?? raw.name) : (raw.name ?? raw.uri);
  if (typeof value !== "string") return null;
  const trimmed = value.trim();
  return trimmed.length ? trimmed : null;
}

// Reads one capability-kind section (e.g. "tools") off an ACL policy's params —
// the single source of truth for the allow/deny + exceptions shape, shared by
// isToolBlockedByAcl below and MCPProxyManageToolsTab's parseExistingAclPolicy.
export function parseAclSection(
  params: Record<string, unknown> | undefined,
  sectionKey: string,
): { mode: "allow" | "deny" | null; exceptions: string[] } {
  const section = params?.[sectionKey] as Record<string, unknown> | undefined;
  if (!section) return { mode: null, exceptions: [] };

  const rawMode = String(section.mode ?? "").toLowerCase();
  const mode = rawMode === "allow" || rawMode === "deny" ? rawMode : null;
  const exceptions = Array.isArray(section.exceptions)
    ? (section.exceptions as unknown[])
        .filter((entry): entry is string => typeof entry === "string" && entry.trim().length > 0)
        .map((entry) => entry.trim())
    : [];

  return { mode, exceptions };
}

// Whether a tool is currently blocked by the Manage Tools tab's ACL policy
// (allow-all-except-exceptions, or deny-all-except-exceptions) — used by the
// Security tab to flag RBAC scope bindings on tools that ACL has shut off.
export function isToolBlockedByAcl(
  config: MCPEndpointConfig | undefined,
  toolIdentifier: string,
): boolean {
  const policy = config?.policies?.find((p) => p.name === MCP_ACL_POLICY_NAME);
  const { mode, exceptions } = parseAclSection(
    policy?.params as Record<string, unknown> | undefined,
    "tools",
  );
  if (!mode) return false;

  const isException = exceptions.includes(toolIdentifier);
  return mode === "deny" ? !isException : isException;
}

