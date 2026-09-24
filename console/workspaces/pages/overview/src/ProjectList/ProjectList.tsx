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

import { NoDataFound, PageLayout } from "@agent-management-platform/views";
import {
  useDeleteProject,
  useListDeploymentPipelines,
  useListProjects,
} from "@agent-management-platform/api-client";
import { generatePath, Link, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  type DeploymentPipelineResponse,
  type ProjectResponse,
} from "@agent-management-platform/types";
import {
  Avatar,
  Box,
  Button,
  Divider,
  Form,
  formatRelativeTime,
  IconButton,
  SearchBar,
  Skeleton,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  GitBranch,
  Package,
  Plus,
  Clock as TimerOutlined,
  Trash2 as TrashOutline,
} from "@wso2/oxygen-ui-icons-react";
import { type MouseEvent, useCallback, useMemo, useState } from "react";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";

const CARD_HEIGHT = 116;

const projectGridTemplate = {
  xs: "repeat(1, minmax(0, 1fr))",
  md: "repeat(2, minmax(0, 1fr))",
  lg: "repeat(3, minmax(0, 1fr))",
  xl: "repeat(4, minmax(0, 1fr))",
  xxl: "repeat(5, minmax(0, 1fr))",
};


function ProjectCard(props: {
  project: ProjectResponse;
  pipeline: DeploymentPipelineResponse | undefined;
  handleDeleteProject: (project: ProjectResponse) => void;
}) {
  const { project, pipeline, handleDeleteProject } = props;
  const { orgId } = useParams();

  const projectPath = generatePath(
    absoluteRouteMap.children.org.children.projects.path,
    { orgId, projectId: project.name }
  );

  const createdAtText = project.createdAt
    ? formatRelativeTime(new Date(project.createdAt))
    : "—";

  const handleDeleteClick = useCallback(
    (event: MouseEvent<HTMLButtonElement>) => {
      event.preventDefault();
      event.stopPropagation();
      handleDeleteProject(project);
    },
    [handleDeleteProject, project]
  );

  return (
    <Link to={projectPath} style={{ textDecoration: "none" }}>
      <Form.CardButton
        sx={{
          position: "relative",
          width: "100%",
          height: CARD_HEIGHT,
          textAlign: "left",
          display: "flex",
          flexDirection: "column",
          p: 2,
          pt: 3,
          boxSizing: "border-box",
          overflow: "hidden",
        }}
      >
        {/* Top: avatar + name + pipeline */}
        {/* Form.CardButton's own base style defaults alignItems to
            "flex-start", so its flex children are never stretched to the
            card's full width — without an explicit width here, this row
            (and the title inside it) would size to fit its content and get
            hard-clipped by the card's overflow:hidden instead of the title's
            own ellipsis ever kicking in. */}
        <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1.5, width: "100%", minWidth: 0 }}>
          <Avatar
            sx={{
              bgcolor: "secondary.main",
              color: "primary.light",
              height: 44,
              width: 44,
              flexShrink: 0,
            }}
          >
            <Package size={26} />
          </Avatar>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Tooltip title={project.displayName} placement="top-start">
              <Typography
                variant="h5"
                sx={{
                  lineHeight: 1.3,
                  mb: 0.4,
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap",
                  minWidth: 0,
                }}
              >
                {project.displayName}
              </Typography>
            </Tooltip>
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, overflow: "hidden" }}>
              <GitBranch size={13} style={{ opacity: 0.5, flexShrink: 0 }} />
              {pipeline ? (
                <Typography
                  variant="caption"
                  color="text.secondary"
                  noWrap
                  sx={{ minWidth: 0 }}
                >
                  {pipeline.displayName || pipeline.name}
                </Typography>
              ) : (
                <Typography variant="caption" color="text.disabled" sx={{ fontStyle: "italic" }}>
                  No pipeline
                </Typography>
              )}
            </Box>
          </Box>
        </Box>

        <Box sx={{ flex: 1 }} />

        <Divider sx={{ mb: 1 }} />

        {/* Bottom: time */}
        <Box sx={{ width: "100%", minWidth: 0 }}>
          {/* Time — always visible */}
          <Typography
            variant="caption"
            color="text.disabled"
            sx={{ display: "flex", alignItems: "center", gap: 0.4 }}
          >
            <TimerOutlined size={12} />
            {createdAtText}
          </Typography>
        </Box>

        {/* Delete — bottom-right, hover-reveal */}
        <Form.DisappearingCardButtonContent sx={{ position: "absolute", bottom: 8, right: 8 }}>
          <Tooltip title="Delete Project">
            <IconButton size="small" color="error" onClick={handleDeleteClick}>
              <TrashOutline size={15} />
            </IconButton>
          </Tooltip>
        </Form.DisappearingCardButtonContent>
      </Form.CardButton>
    </Link>
  );
}

function SkeletonCard() {
  return (
    <Form.CardButton
      tabIndex={-1}
      sx={{
        width: "100%",
        height: CARD_HEIGHT,
        textAlign: "left",
        display: "flex",
        flexDirection: "column",
        p: 2,
        pt: 3,
        boxSizing: "border-box",
        pointerEvents: "none",
      }}
    >
      <Box sx={{ display: "flex", alignItems: "flex-start", width: '100%', gap: 1.5 }}>
        <Skeleton variant="circular" width={44} height={44} />
        <Box sx={{ flex: 1 }}>
          <Skeleton variant="text" width="55%" height={22} sx={{ mb: 0.4 }} />
          <Skeleton variant="text" width="70%" height={14} />
        </Box>
      </Box>
      <Box sx={{ flex: 1 }} />
      <Divider sx={{ mb: 1 }} />
      <Skeleton variant="text" width={90} height={16} />
    </Form.CardButton>
  );
}

export function ProjectList() {
  const { orgId } = useParams();

  const {
    data: projects,
    isPending: isLoadingProjects,
  } = useListProjects({ orgName: orgId });

  const { data: pipelinesData } = useListDeploymentPipelines({ orgName: orgId });

  const pipelineMap = useMemo(
    () =>
      new Map<string, DeploymentPipelineResponse>(
        pipelinesData?.deploymentPipelines?.map((p) => [p.name, p]) ?? []
      ),
    [pipelinesData]
  );

  const { addConfirmation } = useConfirmationDialog();
  const { mutate: deleteProject, isPending: isDeletingProject } = useDeleteProject();

  const handleDeleteProject = useCallback(
    (project: ProjectResponse) => {
      addConfirmation({
        title: "Delete Project?",
        description: `Are you sure you want to delete the project "${project.displayName}"? This action cannot be undone.`,
        onConfirm: () => {
          deleteProject({ orgName: orgId, projName: project.name });
        },
        confirmButtonColor: "error",
        confirmButtonIcon: <TrashOutline size={16} />,
        confirmButtonText: "Delete",
      });
    },
    [addConfirmation, deleteProject, orgId]
  );

  const [search, setSearch] = useState("");

  const filteredProjects = useMemo(
    () =>
      projects?.projects?.filter((p) =>
        p.displayName.toLowerCase().includes(search.toLowerCase())
      ) ?? [],
    [projects, search]
  );

  return (
    <PageLayout docs={["project", "organization", "deploymentPipeline"]}
      title="All Projects"
      disableIcon
    >
      <Box sx={{ display: "flex", flexDirection: "column", gap: 4 }}>
        <Box display="flex" gap={2}>
          <Box flexGrow={1}>
            <SearchBar
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search Projects"
              disabled={!projects?.projects?.length}
              size="small"
              fullWidth
            />
          </Box>
          <Button
            variant="contained"
            color="primary"
            size="small"
            startIcon={<Plus size={16} />}
            component={Link}
            to={generatePath(absoluteRouteMap.children.org.children.newProject.path, { orgId })}
          >
            Add Project
          </Button>
        </Box>

        {filteredProjects.length === 0 && !isLoadingProjects && (
          <NoDataFound
            message="No Projects Found"
            subtitle={
              search
                ? "Looks like there are no projects matching your search."
                : "Create a New Project to Get Started"
            }
            iconElement={Package}
          />
        )}

        <Box sx={{ display: "grid", gridTemplateColumns: projectGridTemplate, gap: 2, width: "100%" }}>
          {isLoadingProjects || isDeletingProject
            ? Array.from({ length: 4 }).map((_, i) => <SkeletonCard key={i} />)
            : filteredProjects.map((project) => (
              <ProjectCard
                key={project.name}
                project={project}
                pipeline={
                  project.deploymentPipeline
                    ? pipelineMap.get(project.deploymentPipeline)
                    : undefined
                }
                handleDeleteProject={handleDeleteProject}
              />
            ))}
        </Box>
      </Box>
    </PageLayout>
  );
}
