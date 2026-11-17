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

import { useGetAgent, useListAgentDeployments } from "@agent-management-platform/api-client";
import type { AgentOverviewPanelProps } from "@agent-management-platform/shared-component";
import { KindInfoCard } from "./KindInfoCard";

/**
 * The agent overview's "built from" card for an agent created from a catalog
 * kind (a panel at agentOverviewInfo). Shows the kind version running in the
 * selected environment, not the one the agent was created from: a redeploy
 * moves the former and leaves the latter behind.
 */
export function KindInfoPanel({ orgId, projectId, agentId, envId }: AgentOverviewPanelProps) {
  const params = { orgName: orgId, projName: projectId, agentName: agentId };
  const { data: agent } = useGetAgent(params);
  const { data: deployments } = useListAgentDeployments(params, {
    enabled: !!orgId && !!projectId && !!agentId,
  });
  if (!agent?.kindName) return null;
  return (
    <KindInfoCard
      orgId={orgId}
      kindName={agent.kindName}
      kindVersion={deployments?.[envId]?.kindVersion}
    />
  );
}
