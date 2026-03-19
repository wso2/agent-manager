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
  xxl: "repeat(5, minmax(0, 1fr))",
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
        sx={{ width: "100%", textAlign: "left", pt: 1.5, textDecoration: "none" }}
      >
        <Form.CardHeader
          title={
            <Form.Stack direction="row" spacing={1.5} alignItems="center">
              <Avatar sx={{ bgcolor: "secondary.main", color: "primary.light", height: 52, width: 52 }}>
                <Package size={32} />
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
                </Form.Stack>
                <Typography variant="caption" color="textPrimary">
                  {projectDescription}
                </Typography>
              </Form.Stack>
            </Form.Stack>
          }
        />
        <Form.CardContent sx={{ width: "100%" }}>
          <Form.CardActions sx={{ justifyContent: "space-between", p: 0, width: "100%" }}>
            <Typography
              variant="caption"
              color="textSecondary"
              sx={{ display: "flex", alignItems: "center", gap: 0.5 }}
            >
              <TimerOutlined size={16} opacity={0.5} />
              {createdAtText}
            </Typography>
            <Form.DisappearingCardButtonContent>

              <Tooltip title="Delete Project">
                <IconButton
                  size="small"
                  color="error"
                  onClick={handleDeleteClick}
                >
                  <TrashOutline size={16} />
                </IconButton>
              </Tooltip>
            </Form.DisappearingCardButtonContent>
          </Form.CardActions>
        </Form.CardContent>

      </Form.CardButton>
    </Link>
  );
}

function SkeletonPageLayout() {
  // Show 4 skeleton cards for loading state
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
        <Box
          key={index}
          sx={{
            width: "100%",
            borderRadius: 2,
            boxShadow: 1,
            bgcolor: "background.paper",
            p: 2,
            minHeight: 160,
            display: "flex",
            flexDirection: "column",
            justifyContent: "space-between",
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
            <Skeleton variant="circular" width={52} height={52} />
            <Box sx={{ flex: 1 }}>
              <Skeleton variant="text" width="60%" height={28} sx={{ mb: 1 }} />
              <Skeleton variant="text" width="80%" height={18} />
            </Box>
          </Box>
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mt: 2 }}>
            <Skeleton variant="rectangular" width={80} height={16} />
            <Skeleton variant="circular" width={32} height={32} />
          </Box>
        </Box>
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
