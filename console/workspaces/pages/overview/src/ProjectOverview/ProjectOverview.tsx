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

import React, { useEffect, useLayoutEffect, type ComponentType } from "react";
import { Box, Button, Stack } from "@wso2/oxygen-ui";
import { Settings, Box as ProjectIcon } from "@wso2/oxygen-ui-icons-react";
import {
  PageLayout,
  useExternalComponentModulesByMountPoint,
  type ProjectSettingsActionProps,
} from "@agent-management-platform/views";
import { AgentList } from "@agent-management-platform/level-resource-listing";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  getExternalProjectManageUrl,
} from "@agent-management-platform/types";
import { useFeatureAccess, useResourceAccess } from "@agent-management-platform/feature-management";
import {
  useListAgents,
  useGetProject,
} from "@agent-management-platform/api-client";
import { AgentTypeSummary } from "./subComponents/AgentTypeSummary";
import { DeploymentPipelineCard } from "./subComponents/DeploymentPipelineCard";

const projectRoutes = absoluteRouteMap.children.org.children.projects;

// Where a deployment can inject its own Settings action; matches
// MountPoints.ProjectSettingsAction (core-ui is not a dependency of this workspace).
const PROJECT_SETTINGS_ACTION_MOUNT_POINT = "project-settings-action";

// Same box icon as the project rows on the organization overview.
const PROJECT_AVATAR = { children: <ProjectIcon size={24} /> };

export const ProjectOverview: React.FC = () => {
  const { orgId = "", projectId = "" } = useParams<{
    orgId: string;
    projectId: string;
  }>();
  const navigate = useNavigate();
  const { canWrite: canWriteProject } = useResourceAccess("project");
  const canCreateAgent = useFeatureAccess("agent_create");
  const { data: project, isLoading } = useGetProject({
    orgName: orgId,
    projName: projectId,
  });
  const {
    data: agents,
    isSuccess: isAgentsLoaded,
    isFetching: isAgentsFetching,
    isFetchedAfterMount: isAgentsFetchedAfterMount,
    refetch: refetchAgents,
  } = useListAgents({
    orgName: orgId,
    projName: projectId,
  });
  const SettingsAction = useExternalComponentModulesByMountPoint(
    PROJECT_SETTINGS_ACTION_MOUNT_POINT,
  )[0]?.component as unknown as ComponentType<ProjectSettingsActionProps> | undefined;
  // An external app may own project management; send Settings there.
  const externalManageUrl = getExternalProjectManageUrl(projectId);
  const manageProject = () => {
    if (externalManageUrl) {
      window.location.assign(externalManageUrl);
      return;
    }
    navigate(generatePath(projectRoutes.children.edit.path, { orgId, projectId }));
  };

  // A project with no agents has nothing to overview; send the user to create
  // one. The page renders as usual meanwhile (so its other queries run in
  // parallel), with the agent list on its skeleton until the outcome is known,
  // so the "no agents" state never flashes before the redirect.
  //
  // Only a successful, empty response counts; a failed load must not redirect.
  // And only one fetched during this visit: an empty list from the cache may be
  // out of date (an agent created in another tab), so it is fetched again first.
  const mayRedirect = canCreateAgent && !!orgId && !!projectId;
  const hasNoAgents = isAgentsLoaded && !agents?.agents?.length;
  const isEmptyConfirmed = hasNoAgents && isAgentsFetchedAfterMount && !isAgentsFetching;
  const shouldRedirect = mayRedirect && isEmptyConfirmed;
  const isCheckingEmpty = mayRedirect && hasNoAgents && !isEmptyConfirmed;

  // A fresh cached empty list is not refetched on mount (staleTime); fetch it.
  useEffect(() => {
    if (isCheckingEmpty && !isAgentsFetchedAfterMount && !isAgentsFetching) {
      void refetchAgents();
    }
  }, [isCheckingEmpty, isAgentsFetchedAfterMount, isAgentsFetching, refetchAgents]);

  // Layout effect: navigate before the browser paints this render.
  useLayoutEffect(() => {
    if (shouldRedirect) {
      // Replace so browser Back doesn't land here and redirect again.
      navigate(generatePath(projectRoutes.children.newAgent.path, { orgId, projectId }), {
        replace: true,
      });
    }
  }, [shouldRedirect, navigate, orgId, projectId]);

  return (
    <PageLayout
      title={project?.displayName ?? "Project Overview"}
      description={project?.description}
      avatar={PROJECT_AVATAR}
      isLoading={isLoading}
      actions={
        canWriteProject &&
        (SettingsAction ? (
          <SettingsAction orgId={orgId} projectId={projectId} disabled={!project} />
        ) : (
          <Button
            variant="outlined"
            size="small"
            startIcon={<Settings size={16} />}
            onClick={manageProject}
            disabled={!project}
          >
            Settings
          </Button>
        ))
      }
    >
      <Stack direction={{ xs: "column", lg: "row" }} gap={3} alignItems="flex-start">
        <Box sx={{ flex: 1, minWidth: 0, width: "100%" }}>
          <AgentList showCreateAction loading={isCheckingEmpty || shouldRedirect} />
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
