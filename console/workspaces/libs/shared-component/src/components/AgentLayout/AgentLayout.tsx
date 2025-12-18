/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { Outlet, useMatch, useParams } from "react-router-dom";
// import { EnvBuildSelector } from "./EnvBuildSelector";
import {
  displayProvisionTypes,
  PageLayout,
} from "@agent-management-platform/views";
import { useGetAgent } from "@agent-management-platform/api-client";
import { Box, Chip, Skeleton } from "@wso2/oxygen-ui";
import { absoluteRouteMap } from "@agent-management-platform/types";

const AgentLayoutSkeleton = () => {
  return (
    <Box display="flex" flexDirection="column" p={3} gap={4} width="100%">
      <Box display="flex" flexDirection="row" gap={2} alignItems="center">
        <Skeleton variant="rounded" width={70} height={70} />
        <Box display="flex" gap={2} flexDirection="column">
        <Skeleton variant="rounded" width={400} height={30} />
        <Skeleton variant="rounded" width={500} height={20} />
        </Box>
      </Box>
      <Skeleton variant="rounded" width="100%" height="40vh" />
    </Box>
  );
};

export function AgentLayout() {
  return <Outlet />;
}

export interface AgentInfoPageLayoutProps {
  children: React.ReactNode;
}
export function AgentInfoPageLayout({ children }: AgentInfoPageLayoutProps) {
  const { orgId, agentId, projectId } = useParams();
  const { data: agent, isLoading: isAgentLoading } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const isOverview = !!useMatch(
    absoluteRouteMap.children.org.children.projects.children.agents.path
  );

  if (isAgentLoading) {
    return <AgentLayoutSkeleton />;
  }
  return (
    <PageLayout
      title={agent?.displayName ?? "Agent"}
      description={
        agent?.description && isOverview
          ? agent.description
          : "No description provided."
      }
      titleTail={
        <Chip
          label={displayProvisionTypes(agent?.provisioning?.type)}
          color="default"
          size="small"
          variant="outlined"
        />
      }
    >
      {children}
    </PageLayout>
  );
}
