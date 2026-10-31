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

import { PageLayout } from "@agent-management-platform/views";
import {
  useDeleteProject,
  useListProjects,
} from "@agent-management-platform/api-client";
import { generatePath, Link, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  type ProjectResponse,
} from "@agent-management-platform/types";
import {
  Box,
  Button,
  ListingCard,
  ListingTable,
  SearchBar,
  Skeleton,
} from "@wso2/oxygen-ui";
import {
  Package,
  Plus,
  Search,
  Trash2 as TrashOutline,
} from "@wso2/oxygen-ui-icons-react";
import { type MouseEvent, useCallback, useMemo, useState } from "react";
import {
  formatRelativeTime,
  isListEmpty,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";

/** Matches ListingCard's own minimum so the loading skeletons line up. */
const CARD_HEIGHT = 160;

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
    { orgId, projectId: project.name }
  );

  const handleDeleteClick = useCallback(
    (event: MouseEvent<HTMLButtonElement>) => {
      event.preventDefault();
      event.stopPropagation();
      handleDeleteProject(project);
    },
    [handleDeleteProject, project]
  );

  return (
    // ListingCard's own `href` renders a plain anchor, which would reload the
    // app; the router Link keeps navigation client-side.
    <Link to={projectPath} style={{ textDecoration: "none" }}>
      <ListingCard
        title={project.displayName}
        description={project.description}
        icon={<Package size={20} />}
        updatedLabel="Created"
        updatedTime={formatRelativeTime(project.createdAt)}
        updatedDateTime={project.createdAt}
        onDelete={handleDeleteClick}
        deleteLabel="Delete project"
      />
    </Link>
  );
}

function SkeletonCard() {
  return <Skeleton variant="rounded" height={CARD_HEIGHT} />;
}

export function ProjectList() {
  const { orgId } = useParams();

  const {
    data: projects,
    isPending: isLoadingProjects,
  } = useListProjects({ orgName: orgId });

  const { addConfirmation } = useConfirmationDialog();
  const { mutate: deleteProject, isPending: isDeletingProject } = useDeleteProject();

  const handleDeleteProject = useCallback(
    (project: ProjectResponse) => {
      addConfirmation({
        analytics: { entity: "project", action: "delete" },
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

  const createButton = (
    <Button
      variant="contained"
      color="primary"
      startIcon={<Plus size={16} />}
      component={Link}
      to={generatePath(absoluteRouteMap.children.org.children.newProject.path, { orgId })}
    >
      Add Project
    </Button>
  );

  const isEmpty = isListEmpty({
    isLoading: isLoadingProjects,
    count: projects?.projects?.length ?? 0,
    isFiltered: Boolean(search),
  });

  return (
    <PageLayout title="All Projects" disableIcon actions={createButton}>
      {isEmpty ? (
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<Package size={64} />}
            title="No projects yet"
            description="Create a project to group your agents and their configuration."
            action={createButton}
          />
        </ListingTable.Container>
      ) : (
        <Box sx={{ display: "flex", flexDirection: "column", gap: 4 }}>
          <SearchBar
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search Projects"
            size="small"
            fullWidth
          />

          {filteredProjects.length === 0 && !isLoadingProjects ? (
            <ListingTable.Container>
              <ListingTable.EmptyState
                illustration={<Search size={64} />}
                title="No projects match your search"
                description="Try a different keyword or clear the search filter."
              />
            </ListingTable.Container>
          ) : (
            <Box sx={{ display: "grid", gridTemplateColumns: projectGridTemplate, gap: 2, width: "100%" }}>
              {isLoadingProjects || isDeletingProject
                ? Array.from({ length: 4 }).map((_, i) => <SkeletonCard key={i} />)
                : filteredProjects.map((project) => (
                  <ProjectCard
                    key={project.name}
                    project={project}
                    handleDeleteProject={handleDeleteProject}
                  />
                ))}
            </Box>
          )}
        </Box>
      )}
    </PageLayout>
  );
}
