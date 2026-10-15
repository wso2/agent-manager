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

import type { HealthChecks, InputInterfaceType } from "@agent-management-platform/types";

/** The agentType.subType sent for each agent type in the create wizard. */
export const subTypeByInterface = {
  CUSTOM: "custom-api",
  A2A: "a2a-agent",
  DEFAULT: "chat-api",
} as const satisfies Record<InputInterfaceType, string>;

export type AgentSubType = (typeof subTypeByInterface)[InputInterfaceType];

/**
 * Whether the create wizard offers the readiness check for an agent type, and the
 * path an HTTP GET check starts with. null hides the choice (a future type with no
 * endpoint to check). Every other health check setting is in the build configuration.
 */
export const READINESS_BY_SUBTYPE: Record<AgentSubType, { httpPath: string } | null> = {
  "chat-api": { httpPath: "/health" },
  "custom-api": { httpPath: "/health" },
  // Every A2A server serves its agent card here; the protocol has no health endpoint.
  "a2a-agent": { httpPath: "/.well-known/agent-card.json" },
};

/** The readiness choice for an agent type, or null when the wizard does not offer it. */
export function readinessOptionFor(interfaceType: InputInterfaceType) {
  return READINESS_BY_SUBTYPE[subTypeByInterface[interfaceType]];
}

/**
 * The readiness choice after the agent type changes. A type without the choice goes
 * back to TCP. The path follows the agent type's default until the user edits it.
 */
export function readinessOnTypeChange(
  prev: { interfaceType: InputInterfaceType; readinessType?: "tcp" | "http"; readinessPath?: string },
  nextType: InputInterfaceType,
): { readinessType?: "tcp" | "http"; readinessPath: string } {
  const prevPath = readinessOptionFor(prev.interfaceType)?.httpPath;
  const nextOption = readinessOptionFor(nextType);
  const edited = !!prev.readinessPath && prev.readinessPath !== prevPath;
  return {
    readinessType: nextOption ? prev.readinessType : "tcp",
    readinessPath: edited ? prev.readinessPath! : nextOption?.httpPath ?? "",
  };
}

/**
 * The create request's healthChecks: only an HTTP GET readiness check is sent. TCP is
 * the platform default, so an agent left on TCP sends nothing and keeps the defaults.
 */
export function buildReadinessHealthChecks(data: {
  interfaceType: InputInterfaceType;
  readinessType?: "tcp" | "http";
  readinessPath?: string;
}): HealthChecks | undefined {
  if (!readinessOptionFor(data.interfaceType) || data.readinessType !== "http") return undefined;
  return { readiness: { type: "http", path: data.readinessPath?.trim() } };
}
