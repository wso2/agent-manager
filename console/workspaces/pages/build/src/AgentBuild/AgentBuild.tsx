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

import React from "react";
import { Box, Skeleton, Stack } from "@wso2/oxygen-ui";
import { TopCards } from "./subComponents/TopCards";
import { BuildTable } from "./subComponents/BuildTable";
import { FadeIn } from "@agent-management-platform/views";
import { useParams } from "react-router-dom";
import { useGetAgentBuilds } from "@agent-management-platform/api-client";

export function AgentBuildSkeleton() {
  return (
    <Box display="flex" flexDirection="column" gap={4} pt={1}>
      <Box display="flex" justifyContent="space-between" gap={4}>
        <Skeleton variant="rounded" width="100%" height={120} />
        <Skeleton variant="rounded" width="100%" height={120} />
        <Skeleton variant="rounded" width="100%" height={120} />
      </Box>
      <Skeleton variant="rounded" width="100%" height={500} />
    </Box>
  );
}

export const AgentBuild: React.FC = () => {
  const { agentId, projectId, orgId } = useParams();
  const { isLoading } = useGetAgentBuilds({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  if (isLoading) {
    return <AgentBuildSkeleton />;
  }

  return (
    <FadeIn>
      <Stack gap={4} flexDirection="column">
        <TopCards />
        <BuildTable />
      </Stack>
    </FadeIn>
  );
};
