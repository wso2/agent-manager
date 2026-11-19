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
import { absoluteRouteMap, type AgentRegistryEntry } from "@agent-management-platform/types";

// Shared by the table and the tile views.

/**
 * Hosting, interface and kind in one line, e.g. "Platform · A2A · Kind reviewer 0.4.1".
 * For an unregistered agent: where it was found, e.g. "Found on OpenAI · proj_cs".
 */
export function agentTypeText(agent: AgentRegistryEntry): string {
  if (agent.registration === "unregistered") {
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

/** True when the row can open a page. */
export function canOpenAgent(agent: AgentRegistryEntry): boolean {
  // Unregistered agents will open their Shadow Agent Discovery finding once that
  // page is on this branch; until then they open nothing.
  return agent.registration === "registered";
}

/** Opens the agent's org-level overview page, which links on to its project page. */
export function useOpenAgent(orgId: string) {
  const navigate = useNavigate();
  // Back on the overview returns to the list as it was: filters, search and view.
  const location = useLocation();
  return useCallback(
    (agent: AgentRegistryEntry) =>
      canOpenAgent(agent) &&
      navigate(
        generatePath(absoluteRouteMap.children.org.children.agentRegistry.children.agent.path, {
          orgId,
          registryProjectId: agent.projectName,
          registryAgentId: agent.name,
        }),
        { state: returnToState(location) },
      ),
    [navigate, orgId, location],
  );
}
