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
import { generatePath, useNavigate } from "react-router-dom";
import { displayAgentSubType } from "@agent-management-platform/views";
import { absoluteRouteMap, type AgentRegistryEntry } from "@agent-management-platform/types";

// Shared by the table and the tile views.

/** Hosting, interface and kind in one line, e.g. "Platform · A2A · Kind reviewer 0.4.1". */
export function agentTypeText(agent: AgentRegistryEntry): string {
  const hosting = agent.hosting === "platform" ? "Platform" : "External";
  const kind = agent.kindName
    ? `Kind ${agent.kindName}${agent.kindVersion ? ` ${agent.kindVersion}` : ""}`
    : undefined;
  return [hosting, displayAgentSubType(agent.interface), kind].filter(Boolean).join(" · ");
}

/** Opens the agent's own page in its project. */
export function useOpenAgent(orgId: string) {
  const navigate = useNavigate();
  return useCallback(
    (agent: AgentRegistryEntry) =>
      navigate(
        generatePath(absoluteRouteMap.children.org.children.projects.children.agents.path, {
          orgId,
          projectId: agent.projectName,
          agentId: agent.name,
        }),
      ),
    [navigate, orgId],
  );
}
