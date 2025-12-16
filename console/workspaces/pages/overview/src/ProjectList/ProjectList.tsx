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
  FadeIn,
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
  ButtonBase,
  Card,
  CardContent,
  CircularProgress,
  IconButton,
  Skeleton,
  TextField,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import {
  Package,
  Plus,
  RefreshCcw,
  Search as SearchRounded,
  Clock as TimerOutlined,
  Trash2,
} from "@wso2/oxygen-ui-icons-react";
import { useCallback, useMemo, useState } from "react";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";

dayjs.extend(relativeTime);

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
  const theme = useTheme();
  const { orgId } = useParams();
  return (
    <FadeIn>
      <ButtonBase
        sx={{
          width: "100%",
        }}
        component={Link}
        to={generatePath(absoluteRouteMap.children.org.children.projects.path, {
          orgId: orgId,
          projectId: project.name,
        })}
      >
        <Card
          sx={{
            width: "100%",
            transition: theme.transitions.create(["all"], {
              duration: theme.transitions.duration.short,
            }),
            p: 1,
            pb: 0,
            pt: 4,
            "& .delete-project-button": {
              opacity: 0,
              transition: theme.transitions.create(["all"], {
                duration: theme.transitions.duration.short,
              }),
            },
            "&.MuiCard-root": {
              backgroundColor: "background.paper",
            },
            "&:hover": {
              "& .delete-project-button": {
                opacity: 1,
              },
              borderColor: "primary.main",
              backgroundColor: "background.main",
              transform: "translateY(-2px)",
            },
          }}
        >
          <CardContent>
            <Box display="flex" alignItems="center" gap={2}>
              <Avatar
                sx={{
                  height: 40,
                  width: 40,
                  "&.MuiAvatar-root": {
                    transition: theme.transitions.create(["all"], {
                      duration: theme.transitions.duration.short,
                    }),
                    bgcolor: "primary.light",
                    color: "background.paper",
                  },
                }}
              >
                <Package fontSize="inherit" size={24} />
              </Avatar>
              <Box
                display="flex"
                flexDirection="column"
                alignItems="flex-start"
              >
                <Typography variant="h5" noWrap textOverflow="ellipsis" maxWidth="70%">
                  {project.displayName}
                </Typography>
                <Typography variant="caption">
                  {project.description ? project.description : "No description"}
                </Typography>
              </Box>
            </Box>
            <Box
              display="flex"
              alignItems="center"
              justifyContent="space-between"
              mt={4}
            >
              <Typography
                variant="body2"
                color="textSecondary"
                sx={{
   
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "flex-start",
                }}
              >
                <TimerOutlined size={16} opacity={0.5} />
                &nbsp;
                {dayjs(project.createdAt).fromNow()}
              </Typography>
              <IconButton
                size="small"
                color="error"
                className="delete-project-button"
                onClick={(e) => {
                  e.stopPropagation();
                  e.preventDefault();
                  handleDeleteProject(project);
                }}
              >
                <Trash2 size={16} />
              </IconButton>
            </Box>
          </CardContent>
        </Card>
      </ButtonBase>
    </FadeIn>
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
        confirmButtonIcon: <Trash2 size={16} />,
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
            <TextField
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              slotProps={{
                input: { endAdornment: <SearchRounded size={16} /> },
              }}
              fullWidth
              variant="outlined"
              placeholder="Search Projects"
              disabled={!projects?.projects?.length}
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
