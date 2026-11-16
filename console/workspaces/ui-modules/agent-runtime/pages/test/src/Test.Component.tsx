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
import { EnvironmentSelector } from "@agent-management-platform/shared-component";
import { PageLayout, StateNotice } from "@agent-management-platform/views";
import { Box, Button, Skeleton } from "@wso2/oxygen-ui";
import { Network, RefreshCw, Rocket, ShieldAlert } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { Swagger } from "./AgentTest/Swagger";
import {
  useGetAgent,
  useGetAgentConfigurations,
  useListAgentDeployments,
} from "@agent-management-platform/api-client";

const SkeletonTestPageLayout: React.FC = () => {
  return (
    <Box
      display="flex"
      flexDirection="column"
      alignItems="center"
      justifyContent="center"
      gap={2}
      height="60vh"
    >
      <Skeleton variant="circular" width={80} height={80} />
      <Skeleton variant="text" width={250} height={32} />
      <Skeleton variant="text" width={350} height={20} />
      <Skeleton variant="rounded" width={500} height={48} sx={{ mt: 2 }} />
    </Box>
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
  const isA2AAgent = agent?.agentType?.subType === "a2a-agent";

  const { data: deployments, isLoading: isDeploymentsLoading } =
    useListAgentDeployments({
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
    });
  const currentDeployment = deployments?.[envId ?? ""];

  // Children must not render as testable before this environment's auth mode is known.
  const { isLoading: isEnvConfigLoading, isError: isEnvConfigError } =
    useGetAgentConfigurations(
      {
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
      },
      {
        environment: envId ?? "",
      },
    );

  const isDeploymentLoading = isDeploymentsLoading || isAgentLoading;
  const isLoading = isDeploymentLoading || isEnvConfigLoading;

  if (!isDeploymentLoading && !isA2AAgent && currentDeployment?.status !== "active") {
    const deployPath =
      orgId && projectId && agentId
        ? generatePath(
            absoluteRouteMap.children.org.children.projects.children.agents.children
              .deployment.path,
            { orgId, projectId, agentId },
          )
        : undefined;
    return (
      <PageLayout title="Try your agent" disableIcon actions={<EnvironmentSelector />}>
        <StateNotice
          icon={Rocket}
          title="Deploy your agent to try it"
          description="This agent isn't running in this environment yet. Deploy it, then come back to chat with it or call its API."
          steps={[
            { label: "Build", state: "done" },
            { label: "Deploy", state: "current" },
            { label: "Try it", state: "todo" },
          ]}
          actions={
            deployPath && (
              <Button variant="contained" component={Link} to={deployPath}>
                Go to Deploy
              </Button>
            )
          }
        />
      </PageLayout>
    );
  }

  return (
    <PageLayout title={"Try your agent"} disableIcon isLoading={isLoading} actions={<EnvironmentSelector />}>
      {isLoading ? (
        <SkeletonTestPageLayout />
      ) : isEnvConfigError ? (
        <StateNotice
          icon={ShieldAlert}
          tone="error"
          title="Couldn't load the security settings"
          description="Testing needs this environment's security settings to know how to sign requests. Retry in a moment or reload the page."
          actions={
            <Button
              variant="outlined"
              startIcon={<RefreshCw size={16} />}
              onClick={() => window.location.reload()}
            >
              Reload page
            </Button>
          }
        />
      ) : (
        <>
          {isChatAgent ? (
            <AgentChat key={`${agentId}-${envId}`} />
          ) : isA2AAgent ? (
            <StateNotice
              icon={Network}
              title="Testing isn't available for A2A agents yet"
              description="This agent speaks the A2A protocol directly. Use an A2A-compatible client with its agent card to try it out."
            />
          ) : (
            <Swagger key={`${agentId}-${envId}`} />
          )}
        </>
      )}
    </PageLayout>
  );
};

export default TestComponent;
