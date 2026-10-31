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
import { Edit, Users } from "@wso2/oxygen-ui-icons-react";
import {
  PageLayout,
  DescriptionCard,
  CreatedMetadata,
} from "@agent-management-platform/views";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  useListAgents,
  useGetProject,
} from "@agent-management-platform/api-client";
import { AgentTypeSummery } from "./subComponents/AgentTypeSummery";
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
  const { data: agents, isFetching: isAgentsFetching } = useListAgents({
    orgName: orgId,
    projName: projectId,
  });
  const hasNoAgents = !agents?.agents?.length;

  // A project with no agents has nothing to overview; send the user to create one.
  useEffect(() => {
    if (orgId && projectId && hasNoAgents && !isAgentsFetching) {
      navigate(generatePath(projectRoutes.children.newAgent.path, { orgId, projectId }));
    }
  }, [orgId, projectId, hasNoAgents, isAgentsFetching, navigate]);

  return (
    <PageLayout
      title={project?.displayName ?? "Project Overview"}
      meta={project ? <CreatedMetadata createdAt={project.createdAt} /> : undefined}
      isLoading={isLoading}
      actions={
        <Stack direction="row" gap={1}>
          <Button
            variant="outlined"
            size="small"
            startIcon={<Users size={16} />}
            onClick={() =>
              navigate(generatePath(projectRoutes.children.agentListing.path, { orgId, projectId }))
            }
          >
            View Agents
          </Button>
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
        </Stack>
      }
    >
      {project?.description && (
        <DescriptionCard content={project.description} sx={{ mb: 4 }} />
      )}
      <Box display="flex" flexDirection="column" gap={2} maxWidth={450}>
        <AgentTypeSummery />
        <DeploymentPipelineCard />
      </Box>
    </PageLayout>
  );
};

export default ProjectOverview;
