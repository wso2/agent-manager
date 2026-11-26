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

import { useCallback } from "react";
import { generatePath, useLocation, useNavigate } from "react-router-dom";
import { displayAgentSubType, returnToState } from "@agent-management-platform/views";
import {
  absoluteRouteMap,
  type AgentRegistryDiscovery,
  type AgentRegistryEntry,
} from "@agent-management-platform/types";

// Shared by the table and the tile views.

/**
 * Hosting, interface and kind in one line, e.g. "Platform · A2A · Kind reviewer 0.4.1".
 * For an unmanaged agent: where it was found, e.g. "Found on OpenAI · proj_cs".
 */
export function agentTypeText(agent: AgentRegistryEntry): string {
  if (agent.management === "unmanaged") {
    const found = agent.discovery
      ? [`Found on ${agent.discovery.providerDisplayName}`, agent.discovery.scope]
      : ["Found outside Agent Manager"];
    return found.filter(Boolean).join(" · ");
  }
  const hosting = agent.hosting === "platform" ? "Platform" : "External";
  const kind = agent.kindName
    ? `Kind ${agent.kindName}${agent.kindVersion ? ` ${agent.kindVersion}` : ""}`
    : undefined;
  const agentInterface = displayAgentSubType(agent.interface ?? undefined);
  return [hosting, agentInterface, kind].filter(Boolean).join(" · ");
}

/** The connections that found a shadow agent: one name, or "A, B" for one found twice. */
export function shadowSourceText(discovery: AgentRegistryDiscovery): string {
  const names = discovery.foundBy.map((s) => s.connectionName);
  return names.length > 0 ? names.join(", ") : discovery.connectionName;
}

const registryRoutes = absoluteRouteMap.children.org.children.agentRegistry.children;

/**
 * The agent's page in the registry: a managed agent's overview, by project and name; an
 * unmanaged agent's page, by its discovery finding.
 */
export function agentOverviewPath(orgId: string, agent: AgentRegistryEntry): string {
  if (agent.management === "unmanaged") {
    return generatePath(registryRoutes.unmanagedAgent.path, {
      orgId,
      findingId: agent.discovery?.findingId ?? agent.name,
    });
  }
  return generatePath(registryRoutes.agent.path, {
    orgId,
    registryProjectId: agent.projectName,
    registryAgentId: agent.name,
  });
}

/** Opens the agent's overview page. */
export function useOpenAgent(orgId: string) {
  const navigate = useNavigate();
  // Back on the overview returns to the list as it was: filters, search and view.
  const location = useLocation();
  return useCallback(
    (agent: AgentRegistryEntry) =>
      navigate(agentOverviewPath(orgId, agent), { state: returnToState(location) }),
    [navigate, orgId, location],
  );
}
