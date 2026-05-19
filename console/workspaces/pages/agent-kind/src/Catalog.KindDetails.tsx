/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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


import React, { useMemo, useState } from "react";
import { formatDistanceToNow } from "date-fns";
import { Link, generatePath, useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Divider,
  ListingTable,
  Menu,
  MenuItem,
  Select,
  SelectChangeEvent,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { SwaggerSpecViewer } from "@agent-management-platform/shared-component";
import { useGetAgentKind, useGetAgentEndpoints, useListKindAgents, useListProjects, useGetAgentKindVersion } from "@agent-management-platform/api-client";
import { ExternalLink, Plus } from "@wso2/oxygen-ui-icons-react";

export const CatalogKindDetails: React.FC = () => {
  const navigate = useNavigate();
  const { kindId, orgId } = useParams<{ kindId: string; orgId: string }>();

  const { data: kind, isLoading } = useGetAgentKind({ orgName: orgId, kindName: kindId ?? "" });

  const sortedVersions = useMemo(
    () =>
      [...(kind?.versions ?? [])].sort(
        (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
      ),
    [kind],
  );

  const [searchParams, setSearchParams] = useSearchParams();
  const defaultVersion = sortedVersions[0]?.version ?? "";
  const selectedVersionTag = searchParams.get("version") ?? defaultVersion;
  const selectedVersion =
    sortedVersions.find((v) => v.version === selectedVersionTag) ?? sortedVersions[0];

  const { data: kindVersion, isLoading: isVersionLoading } = useGetAgentKindVersion({
    orgName: orgId ?? "",
    kindName: kindId ?? "",
    versionTag: selectedVersionTag,
  });

  const { data: endpointsData, isFetching: isEndpointsLoading } = useGetAgentEndpoints(
    {
      orgName: orgId ?? "",
      projName: kindVersion?.sourceProjectName,
      agentName: kindVersion?.sourceAgentName,
    },
    { environment: "default" },
  );

  const { data: kindAgents, isLoading: isKindAgentsLoading } = useListKindAgents({
    orgName: orgId ?? "",
    kindName: kindId ?? "",
  });

  const { data: projectsData, isLoading: isProjectsLoading } = useListProjects({
    orgName: orgId,
  });

  const [addInstanceAnchorEl, setAddInstanceAnchorEl] = useState<null | HTMLElement>(null);

  const endpointKey = useMemo(() => Object.keys(endpointsData ?? {})[0] ?? "", [endpointsData]);
  const apiSpec = useMemo(
    () => endpointsData?.[endpointKey]?.schema?.content as Record<string, unknown> | undefined,
    [endpointsData, endpointKey],
  );

  const backHref = generatePath(absoluteRouteMap.children.org.children.catalog.path, {
    orgId: orgId ?? "",
  });

  const sortedProjects = useMemo(
    () =>
      [...(projectsData?.projects ?? [])].sort((a, b) =>
        (a.displayName || a.name).localeCompare(b.displayName || b.name),
      ),
    [projectsData],
  );

  const handleOpenAddInstanceMenu = (event: React.MouseEvent<HTMLElement>) => {
    setAddInstanceAnchorEl(event.currentTarget);
  };

  const handleCloseAddInstanceMenu = () => {
    setAddInstanceAnchorEl(null);
  };

  const handleSelectProjectForInstance = (projectName: string) => {
    const targetPath = generatePath(
      absoluteRouteMap.children.org.children.projects.children.newAgent
        .children.create.children.catalog.children.withKind.path,
      {
        orgId: orgId ?? "",
        projectId: projectName,
        kindId: kindId ?? "",
      },
    );
    const target = selectedVersionTag ?
      `${targetPath}?version=${encodeURIComponent(selectedVersionTag)}` : targetPath;
    handleCloseAddInstanceMenu();
    navigate(target);
  };

  const versionSelector = sortedVersions.length > 1 && (
    <Select
      size="small"
      value={selectedVersionTag}
      onChange={(e: SelectChangeEvent<string>) =>
        setSearchParams((prev) => {
          const next = new URLSearchParams(prev);
          next.set("version", e.target.value);
          return next;
        })
      }
      sx={{ minWidth: 120 }}
    >
      {sortedVersions.map((v) => (
        <MenuItem key={v.version} value={v.version}>
          v{v.version}
        </MenuItem>
      ))}
    </Select>
  );

  if (isLoading) {
    return (
      <PageLayout title={kindId ?? "Agent Kind Details"} backHref={backHref}
        backLabel="Back to Agent Catalog" disableIcon>
        <Box sx={{ p: 2 }}>
          <Skeleton variant="rounded" height={32} sx={{ mb: 2, maxWidth: 320 }} />
          <Skeleton variant="rounded" height={48} sx={{ mb: 1 }} />
          <Skeleton variant="rounded" height={48} sx={{ mb: 1 }} />
          <Skeleton variant="rounded" height={200} />
        </Box>
      </PageLayout>
    );
  }

  if (!kind) {
    return (
      <PageLayout title="Agent Kind Details" backHref={backHref}
        backLabel="Back to Agent Catalog" disableIcon>
        <Alert severity="error">Agent kind &quot;{kindId}&quot; was not found.</Alert>
      </PageLayout>
    );
  }

  const releasedLabel = selectedVersion
    ? `Released on ${new Date(selectedVersion.createdAt).toLocaleDateString(undefined,
      { year: "numeric", month: "long", day: "numeric" })}`
    : undefined;

  return (
    <PageLayout
      title={kind.displayName}
      description={releasedLabel ?? "View details of this Agent Kind."}
      backHref={backHref}
      backLabel="Back to Agent Catalog"
      titleTail={
        kindVersion?.sourceProjectName && kindVersion?.sourceAgentName ? (
          <Button
            size="small"
            startIcon={<ExternalLink size={14} />}
            variant="outlined"
            component={Link}
            to={generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents.path,
              {
                orgId,
                projectId: kindVersion.sourceProjectName,
                agentId: kindVersion.sourceAgentName,
              },
            )}
            target="_blank"
            rel="noopener noreferrer"
          >
            View Source Agent
          </Button>
        ) : undefined
      }
      actions={[
        versionSelector || undefined,
        <React.Fragment key="add-instance-action">
          <Button
            startIcon={<Plus />}
            variant="contained"
            color="primary"
            onClick={handleOpenAddInstanceMenu}
            aria-controls={addInstanceAnchorEl ? "add-instance-menu" : undefined}
            aria-haspopup="true"
            aria-expanded={Boolean(addInstanceAnchorEl)}
          >
            Add Instance
          </Button>
          <Menu
            id="add-instance-menu"
            anchorEl={addInstanceAnchorEl}
            open={Boolean(addInstanceAnchorEl)}
            onClose={handleCloseAddInstanceMenu}
            anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
            transformOrigin={{ vertical: "top", horizontal: "right" }}
          >
            {isProjectsLoading ? (
              <MenuItem disabled>Loading projects...</MenuItem>
            ) : sortedProjects.length > 0 ? (
              sortedProjects.map((project) => (
                <MenuItem key={project.name} onClick={() =>
                  handleSelectProjectForInstance(project.name)}>
                  {project.displayName || project.name}
                </MenuItem>
              ))
            ) : (
              <MenuItem disabled>No projects found</MenuItem>
            )}
          </Menu>
        </React.Fragment>,
      ]}
      disableIcon
    >
      <Stack spacing={3}>
        {/* Description */}
        {kind.description && (
          <Box>
            <Typography variant="overline" color="text.secondary">
              Description
            </Typography>
            <Typography variant="body1">{kind.description}</Typography>
          </Box>
        )}

        <Divider />

        {/* Configuration Schema */}
        <Stack spacing={1.5}>
          <Typography variant="overline" color="text.secondary">
            Configuration Schema
          </Typography>
          {selectedVersion && selectedVersion.configSchema.length > 0 ? (
            <ListingTable.Container>
              <ListingTable>
                <ListingTable.Head>
                  <ListingTable.Row>
                    <ListingTable.Cell width="25%">Name</ListingTable.Cell>
                    <ListingTable.Cell width="35%">Description</ListingTable.Cell>
                    <ListingTable.Cell width="15%">Mandatory</ListingTable.Cell>
                    <ListingTable.Cell width="15%">Secret</ListingTable.Cell>
                    <ListingTable.Cell width="10%">Default</ListingTable.Cell>
                  </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                  {selectedVersion.configSchema.map((item) => (
                    <ListingTable.Row key={item.name}>
                      <ListingTable.Cell>
                        <Typography variant="body2" fontWeight={500}>{item.name}</Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="body2" color="text.secondary">
                          {item.description ?? "—"}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="body2" color="text.secondary">
                          {item.isMandatory ? "Yes" : "No"}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="body2" color="text.secondary">
                          {item.isSecret ? "Yes" : "No"}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="body2" color="text.secondary">
                          {item.defaultValue ?? "—"}
                        </Typography>
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  ))}
                </ListingTable.Body>
              </ListingTable>
            </ListingTable.Container>
          ) : (
            <Alert severity="info">No configuration schema defined for this version.</Alert>
          )}
        </Stack>

        <Divider />

        {/* Usage — agents deployed from this kind */}
        <Stack spacing={1.5}>
          <Typography variant="overline" color="text.secondary">
            Usage
          </Typography>
          {isKindAgentsLoading ? (
            <Skeleton variant="rounded" height={120} />
          ) : kindAgents && kindAgents.length > 0 ? (
            <ListingTable.Container>
              <ListingTable>
                <ListingTable.Head>
                  <ListingTable.Row>
                    <ListingTable.Cell>Agent</ListingTable.Cell>
                    <ListingTable.Cell>Project</ListingTable.Cell>
                    <ListingTable.Cell>Created</ListingTable.Cell>
                    <ListingTable.Cell />
                  </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                  {kindAgents.map((agent) => {
                    const agentHref = generatePath(
                      absoluteRouteMap.children.org.children.projects.children.agents.path,
                      { orgId: orgId ?? "", projectId: agent.projectName, agentId: agent.name },
                    );
                    return (
                      <ListingTable.Row
                        key={`${agent.projectName}/${agent.name}`}
                        onClick={() => window.open(agentHref, "_blank", "noopener,noreferrer")}
                        sx={{ cursor: "pointer", "&:hover": { bgcolor: "action.hover" } }}
                      >
                        <ListingTable.Cell>
                          <Typography variant="body2" fontWeight={500}>
                            {agent.displayName || agent.name}
                          </Typography>
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          <Typography variant="body2" color="text.secondary">
                            {agent.projectName}
                          </Typography>
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          {agent.createdAt ? (
                            <Typography variant="body2" color="text.secondary">
                              {formatDistanceToNow(new Date(agent.createdAt), { addSuffix: true })}
                            </Typography>
                          ) : (
                            <Typography variant="body2" color="text.secondary">—</Typography>
                          )}
                        </ListingTable.Cell>
                        <ListingTable.Cell align="right">
                          <Button
                            size="small"
                            variant="text"
                            endIcon={<ExternalLink size={16} />}
                            onClick={(e) => { e.stopPropagation(); window.open(agentHref, "_blank", "noopener,noreferrer"); }}
                          >
                            View Agent
                          </Button>
                        </ListingTable.Cell>
                      </ListingTable.Row>
                    );
                  })}
                </ListingTable.Body>
              </ListingTable>
            </ListingTable.Container>
          ) : (
            <Alert severity="info">No agents are currently using this kind.</Alert>
          )}
        </Stack>

        <Divider />
        {/* API Specification */}
        <Stack spacing={1.5}>
          <Typography variant="overline" color="text.secondary">
            API Specification
          </Typography>
          {isVersionLoading || isEndpointsLoading ? (
            <Skeleton variant="rounded" height={300} />
          ) : (apiSpec ? (
            <SwaggerSpecViewer
              spec={apiSpec}
              docExpansion="list"
              hideInfoSection
              hideServers
              hideAuthorizeButton
            />
          ) : (
            <Alert severity="info">No API specification available for this version.</Alert>
          ))}
        </Stack>
      </Stack>
    </PageLayout>
  );
};

export default CatalogKindDetails;
