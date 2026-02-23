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

import {
  NoDataFound,
  PageLayout,
} from "@agent-management-platform/views";
import {
  useDeleteProject,
  useListProjects,
} from "@agent-management-platform/api-client";
import { generatePath, Link, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  ProjectResponse,
} from "@agent-management-platform/types";
import {
  Avatar,
  Box,
  Button,
  CircularProgress,
  Form,
  IconButton,
  SearchBar,
  Skeleton,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Package,
  Plus,
  RefreshCcw,
  Clock as TimerOutlined,
  Trash2 as TrashOutline,
} from "@wso2/oxygen-ui-icons-react";
import { type MouseEvent, useCallback, useMemo, useState } from "react";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { formatDistanceToNow } from "date-fns";

const projectGridTemplate = {
  xs: "repeat(1, minmax(0, 1fr))",
  md: "repeat(2, minmax(0, 1fr))",
  lg: "repeat(3, minmax(0, 1fr))",
  xl: "repeat(4, minmax(0, 1fr))",
};

function ProjectCard(props: {
  project: ProjectResponse;
  handleDeleteProject: (project: ProjectResponse) => void;
}) {
  const { project, handleDeleteProject } = props;
  const { orgId } = useParams();
  const projectPath = generatePath(
    absoluteRouteMap.children.org.children.projects.path,
    {
      orgId: orgId,
      projectId: project.name,
    }
  );

  const projectDescription = project.description?.trim()
    ? project.description
    : "No description provided";

  const handleDeleteClick = useCallback(
    (event: MouseEvent<HTMLButtonElement>) => {
      event.preventDefault();
      event.stopPropagation();
      handleDeleteProject(project);
    },
    [handleDeleteProject, project]
  );

  const createdAtText = project.createdAt
    ? formatDistanceToNow(new Date(project.createdAt), { addSuffix: true })
    : "—";

  return (
    <Link to={projectPath} style={{ textDecoration: "none" }}>
      <Form.CardButton
        sx={{ width: "100%", textAlign: "left", textDecoration: "none" }}
      >
        <Form.CardContent>
          <Form.CardHeader
            title={
              <Form.Stack direction="row" spacing={1.5} alignItems="center">
                <Avatar sx={{ bgcolor: "primary.main", color: "primary.contrastText", height: 42, width: 42 }}>
                  <Package size={24} />
                </Avatar>
                <Form.Stack
                  direction="column"
                  spacing={0.5}
                  flex={1}
                  minWidth={0}
                >
                  <Form.Stack direction="row" spacing={1} alignItems="center">
                    <Typography
                      variant="h5"
                      noWrap
                      textOverflow="ellipsis"
                      sx={{ maxWidth: "90%" }}
                    >
                      {project.displayName}
                    </Typography>
                    <Form.DisappearingCardButtonContent>
                      <Tooltip title="Delete project">
                        <IconButton
                          size="small"
                          color="error"
                          onClick={handleDeleteClick}
                        >
                          <TrashOutline size={16} />
                        </IconButton>
                      </Tooltip>
                    </Form.DisappearingCardButtonContent>
                  </Form.Stack>
                  <Typography variant="caption" color="textPrimary">
                    {projectDescription}
                  </Typography>
                </Form.Stack>
              </Form.Stack>
            }
          />
                <Form.CardActions sx={{flexWrap: "wrap" }}>
          <Typography
            variant="caption"
            color="textSecondary"
            sx={{ display: "flex", alignItems: "center", gap: 0.5 }}
          >
            <TimerOutlined size={16} opacity={0.5} />
            {createdAtText}
          </Typography>
        </Form.CardActions>
        </Form.CardContent>
  
      </Form.CardButton>
    </Link>
  );
}

function SkeletonPageLayout() {
  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: projectGridTemplate,
        gap: 2,
        width: "100%",
      }}
    >
      {Array.from({ length: 4 }).map((_, index) => (
        <Skeleton
          key={index}
          variant="rounded"
          height={160}
          sx={{ width: "100%" }}
        />
      ))}
    </Box>
  );
}

export function ProjectList() {
  const { orgId } = useParams();
  const {
    data: projects,
    isRefetching,
    refetch: refetchProjects,
    isPending: isLoadingProjects,
  } = useListProjects({
    orgName: orgId,
  });
  const { addConfirmation } = useConfirmationDialog();
  const { mutate: deleteProject, isPending: isDeletingProject } =
    useDeleteProject();

  const handleDeleteProject = useCallback(
    (project: ProjectResponse) => {
      addConfirmation({
        title: "Delete Project?",
        description: `Are you sure you want to delete the project "${project.displayName}"? This action cannot be undone.`,
        onConfirm: () => {
          deleteProject({
            orgName: orgId,
            projName: project.name,
          });
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
      projects?.projects?.filter((project) =>
        project.displayName.toLowerCase().includes(search.toLowerCase())
      ) || [],
    [projects, search]
  );

  const handleRefresh = useCallback(() => {
    refetchProjects();
  }, [refetchProjects]);

  return (
    <PageLayout
      title="Projects"
      description="List of projects"
      titleTail={
        <Box
          display="flex"
          alignItems="center"
          minWidth={32}
          justifyContent="center"
        >
          {isRefetching ? (
            <CircularProgress size={18} color="primary" />
          ) : (
            <IconButton size="small" color="primary" onClick={handleRefresh}>
              <RefreshCcw size={18} />
            </IconButton>
          )}
        </Box>
      }
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
            to={generatePath(
              absoluteRouteMap.children.org.children.newProject.path,
              {
                orgId: orgId,
              }
            )}
          >
            Add Project
          </Button>
        </Box>
        {filteredProjects?.length === 0 && !isLoadingProjects && (
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
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: projectGridTemplate,
            gap: 2,
            width: "100%",
          }}
        >
          {!isDeletingProject &&
            filteredProjects?.map((project) => (
              <ProjectCard
                key={project.name}
                project={project}
                handleDeleteProject={handleDeleteProject}
              />
            ))}
        </Box>
      </Box>
      {(isLoadingProjects || isDeletingProject) && <SkeletonPageLayout />}
    </PageLayout>
  );
}
