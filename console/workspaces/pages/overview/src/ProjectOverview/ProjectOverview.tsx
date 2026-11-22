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

import React, { useEffect, useState, type ComponentType } from "react";
import { Box, Button, PageContent, Skeleton, Stack } from "@wso2/oxygen-ui";
import { Settings, Box as ProjectIcon } from "@wso2/oxygen-ui-icons-react";
import {
  PageLayout,
  useDocumentTitle,
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
import { ProjectLanding } from "./subComponents/ProjectLanding";

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
    isLoading: isAgentsLoading,
    isSuccess: isAgentsLoaded,
    dataUpdatedAt: agentsUpdatedAt,
    refetch: refetchAgents,
  } = useListAgents({
    orgName: orgId,
    projName: projectId,
  });
  // A cached empty list may be out of date (an agent created elsewhere), and within
  // the stale time nothing refetches it. So an empty list counts only once a request
  // made since this page opened confirms it; until then the page shows the skeleton.
  // Switching project restarts the clock, as the router may keep this component mounted.
  const [opened, setOpened] = useState(() => ({ projectId, at: Date.now() }));
  if (opened.projectId !== projectId) setOpened({ projectId, at: Date.now() });
  // For the one render before the reset applies, nothing counts as confirmed.
  const openedAt = opened.projectId === projectId ? opened.at : Infinity;
  const hasNoAgents = isAgentsLoaded && !agents?.agents?.length;
  const emptyUnconfirmed = hasNoAgents && agentsUpdatedAt < openedAt;
  useEffect(() => {
    if (emptyUnconfirmed) void refetchAgents();
  }, [emptyUnconfirmed, refetchAgents]);
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

  const settingsAction = !canWriteProject ? undefined : SettingsAction ? (
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
  );

  // Until the agent list is known the page could be either the overview or the
  // landing, so it shows neither.
  if (isAgentsLoading || emptyUnconfirmed) {
    return <ProjectOverviewSkeleton title={project?.displayName} />;
  }

  // A project with no agents has nothing to overview, so it shows where to start.
  // Only a successful, empty response counts; a failed load shows the overview,
  // whose agent list reports the error.
  if (hasNoAgents) {
    return (
      <ProjectLanding
        orgId={orgId}
        projectId={projectId}
        projectName={project?.displayName}
        canCreateAgent={canCreateAgent}
        settingsAction={settingsAction}
      />
    );
  }

  return (
    <PageLayout
      title={project?.displayName ?? "Project Overview"}
      description={project?.description}
      avatar={PROJECT_AVATAR}
      isLoading={isLoading}
      actions={settingsAction}
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

/** Neutral placeholder while it is not yet known whether the project has agents. */
function ProjectOverviewSkeleton({ title }: { title?: string }) {
  useDocumentTitle(title ?? "Project Overview");
  return (
    <PageContent fullWidth>
      <Stack spacing={{ xs: 3, md: 4 }}>
        <Stack spacing={1}>
          <Skeleton variant="text" width={280} height={48} sx={{ maxWidth: "100%" }} />
          <Skeleton variant="text" width={420} sx={{ maxWidth: "100%" }} />
        </Stack>
        <Skeleton variant="rounded" height={360} />
      </Stack>
    </PageContent>
  );
}

export default ProjectOverview;
