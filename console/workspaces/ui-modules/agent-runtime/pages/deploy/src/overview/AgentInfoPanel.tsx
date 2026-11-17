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

import { useGetAgent, useGetAgentBuilds } from "@agent-management-platform/api-client";
import type { AgentOverviewPanelProps } from "@agent-management-platform/shared-component";
import { AgentInfoCard } from "./AgentInfoCard";

/**
 * The agent overview's "built from" card for an agent built from source: its
 * repository and latest build (a panel at agentOverviewInfo). Both queries are
 * shared with the rest of the overview.
 */
export function AgentInfoPanel({ orgId, projectId, agentId }: AgentOverviewPanelProps) {
  const params = { orgName: orgId, projName: projectId, agentName: agentId };
  const { data: agent } = useGetAgent(params);
  const { data: buildList, isLoading: isBuildsLoading } = useGetAgentBuilds(params);
  return (
    <AgentInfoCard
      orgId={orgId}
      projectId={projectId}
      agentId={agentId}
      repository={agent?.provisioning?.repository}
      latestBuild={buildList?.builds[0]}
      isBuildsLoading={isBuildsLoading}
      build={agent?.build}
    />
  );
}
