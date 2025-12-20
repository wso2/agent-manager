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
import { AgentChat } from "./AgentTest/AgentChat";
import {
  FadeIn,
  NoDataFound,
  PageLayout,
} from "@agent-management-platform/views";
import { Box, Skeleton, Stack } from "@wso2/oxygen-ui";
import { Rocket } from "@wso2/oxygen-ui-icons-react";
import { useParams } from "react-router-dom";
import { Swagger } from "./AgentTest/Swagger";
import {
  useGetAgent,
  useListAgentDeployments,
} from "@agent-management-platform/api-client";

const SkeletonTestPageLayout: React.FC = () => {
  return (
    <Stack spacing={3} sx={{ padding: 3 }}>
      {/* Page Title Skeleton */}
      <Box
        display="flex"
        alignItems="center"
        justifyContent="space-between"
        gap={1}
      >
        <Stack spacing={1}>
          <Skeleton variant="rounded" width={200} height={36} />
        </Stack>
      </Box>

      {/* Content Area Skeleton */}
      <Box
        display="flex"
        flexDirection="column"
        alignItems="center"
        justifyContent="center"
        gap={2}
      >
        <Skeleton variant="rounded" width="100%" height="70vh" />
      </Box>
    </Stack>
  );
};

export const TestComponent: React.FC = () => {
  const { orgId, projectId, agentId, envId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
  }>();

  const { data: agent, isLoading: isAgentLoading } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  const isChatAgent = agent?.agentType?.subType === "chat-api";

  const { data: deployments, isLoading: isDeploymentsLoading } =
    useListAgentDeployments({
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
    });
  const currentDeployment = deployments?.[envId ?? ""];

  if (isDeploymentsLoading || isAgentLoading) {
    return <SkeletonTestPageLayout />;
  }

  if (currentDeployment?.status !== "active") {
    return (
      <Box
        height="50vh"
        display="flex"
        justifyContent="center"
        alignItems="center"
      >
        <NoDataFound
          iconElement={Rocket}
          disableBackground
          message="Agent is not deployed"
          subtitle="Deploy your agent to try it out. You can deploy your agent by clicking the deploy button in the deploy tab."
        />
      </Box>
    );
  }

  return (
    <FadeIn>
      <PageLayout title={"Try your agent"} disableIcon>
        {isChatAgent ? <AgentChat /> : <Swagger />}
      </PageLayout>
    </FadeIn>
  );
};

export default TestComponent;
