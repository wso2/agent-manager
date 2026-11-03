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

import React, { useEffect } from "react";
import { Box, Button, Stack } from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import { PageLayout } from "@agent-management-platform/views";
import { AgentList } from "@agent-management-platform/level-resource-listing";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  useListAgents,
  useGetProject,
} from "@agent-management-platform/api-client";
import { AgentTypeSummary } from "./subComponents/AgentTypeSummary";
import { DeploymentPipelineCard } from "./subComponents/DeploymentPipelineCard";

const projectRoutes = absoluteRouteMap.children.org.children.projects;

export const ProjectOverview: React.FC = () => {
  const { orgId = "", projectId = "" } = useParams<{
    orgId: string;
    projectId: string;
  }>();
  const navigate = useNavigate();
  const { data: project, isLoading } = useGetProject({
    orgName: orgId,
    projName: projectId,
  });
  const { data: agents, isSuccess: isAgentsLoaded, isFetching: isAgentsFetching } = useListAgents({
    orgName: orgId,
    projName: projectId,
  });
  // Only a successful, empty response counts; a failed load must not redirect.
  const hasNoAgents = isAgentsLoaded && !agents?.agents?.length;

  // A project with no agents has nothing to overview; send the user to create one.
  useEffect(() => {
    if (orgId && projectId && hasNoAgents && !isAgentsFetching) {
      navigate(generatePath(projectRoutes.children.newAgent.path, { orgId, projectId }));
    }
  }, [orgId, projectId, hasNoAgents, isAgentsFetching, navigate]);

  return (
    <PageLayout
      title={project?.displayName ?? "Project Overview"}
      description={project?.description}
      isLoading={isLoading}
      actions={
          <Button
            variant="outlined"
            size="small"
            startIcon={<Edit size={16} />}
            onClick={() =>
              navigate(generatePath(projectRoutes.children.edit.path, { orgId, projectId }))
            }
            disabled={!project}
          >
            Edit Project
          </Button>
      }
    >
      <Stack direction={{ xs: "column", lg: "row" }} gap={3} alignItems="flex-start">
        <Box sx={{ flex: 1, minWidth: 0, width: "100%" }}>
          <AgentList showCreateAction />
        </Box>
        <Stack gap={2} sx={{ width: { xs: "100%", lg: 380 }, flexShrink: 0 }}>
          <AgentTypeSummary />
          <DeploymentPipelineCard />
        </Stack>
      </Stack>
    </PageLayout>
  );
};

export default ProjectOverview;
