/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import { useMemo } from "react";
import type { OrgAgentDisplayResolver } from "@agent-management-platform/api-client";
import type { AgentIdentityAgentResponse } from "@agent-management-platform/types";

const RAW_NAME_RESOLVER: OrgAgentDisplayResolver = {
  resolveAgentName: (_projectName, name) => name,
  resolveProjectName: (projectName) => projectName,
};

/**
 * Agents without a Thunder binding yet can't be added as a group member or
 * role assignee, and can't be looked up by agent ID, so both the
 * picker options and the lookup map are restricted to bound agents.
 *
 * `resolver` (from `useOrgAgentDisplayNames`) resolves each bound agent's
 * real display name; omit it to render raw names.
 */
export function useAgentLookup(
  agents: AgentIdentityAgentResponse[],
  resolver: OrgAgentDisplayResolver = RAW_NAME_RESOLVER,
) {
  const boundAgents = useMemo(() => agents.filter((a) => !!a.thunderAgentId), [agents]);
  const agentsByThunderId = useMemo(
    () => new Map(boundAgents.map((a) => [a.thunderAgentId as string, a])),
    [boundAgents],
  );

  const displayNameForAgent = (agent: AgentIdentityAgentResponse) =>
    resolver.resolveAgentName(agent.projectName, agent.agentName);

  // Agent names (and their resolved display names) are only unique within a
  // project, so the project name disambiguates two agents that otherwise
  // look identical in a picker or list.
  const projectDisplayNameForAgent = (agent: AgentIdentityAgentResponse) =>
    resolver.resolveProjectName(agent.projectName, agent.agentName);

  const displayName = (thunderAgentId: string) => {
    const agent = agentsByThunderId.get(thunderAgentId);
    return agent ? displayNameForAgent(agent) : thunderAgentId;
  };

  const projectDisplayName = (thunderAgentId: string) => {
    const agent = agentsByThunderId.get(thunderAgentId);
    return agent ? projectDisplayNameForAgent(agent) : undefined;
  };

  return {
    agents: boundAgents,
    agentsByThunderId,
    displayName,
    displayNameForAgent,
    projectDisplayName,
    projectDisplayNameForAgent,
  };
}
