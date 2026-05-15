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

import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Form,
  Skeleton,
  ListingTable,
  MenuItem,
  Select,
  Typography,
} from "@wso2/oxygen-ui";
import { Package, Plus } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { DrawerWrapper, DrawerHeader, DrawerContent, TextInput, PageLayout } from "@agent-management-platform/views";
import {
  absoluteRouteMap,
  type AgentKindConfigSchemaItem,
  type AgentKindVersionResponse,
  type BuildResponse,
} from "@agent-management-platform/types";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { RuntimeConfigEditor, createRuntimeConfigRow, type RuntimeConfigRow } from "./RuntimeConfigEditor";
import { useGetAgent, useGetAgentBuilds, useGetAgentKind, useListAgentKindVersions, usePublishAgentKind } from "@agent-management-platform/api-client";


export const PublishedList: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  const {data: agentKindVersions, isLoading: isAgentKindVersionsLoading} =
    useListAgentKindVersions({
    orgName: orgId,
    kindName: agentId!,
  });

  const { data:agent } = useGetAgent({
    orgName: orgId, 
    projName: projectId,
    agentName: agentId,
  });
  const { data: existingKind } = useGetAgentKind({ orgName: orgId!, kindName: agentId! });

  const listPath = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.children.publish.path,
    { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "" },
  );

  const createVersionPath = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents
      .children.publish.children.createNewVersion.path,
    { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "" },
  );

  const isCreateOpen = location.pathname.endsWith("/create-new-version");

  // Create drawer state
  const [versionName, setVersionName] = useState("");
  const [selectedBuildName, setSelectedBuildName] = useState("");
  const [kindDisplayName, setKindDisplayName] = useState("");
  const [kindDescription, setKindDescription] = useState("");
  const [createRows, setCreateRows] = useState<RuntimeConfigRow[]>([createRuntimeConfigRow()]);

  const { addConfirmation } = useConfirmationDialog();

  // Pre-fill display name & description from existing kind when drawer opens
  useEffect(() => {
    if (isCreateOpen && existingKind) {
      setKindDisplayName(existingKind.displayName ?? "");
      setKindDescription(existingKind.description ?? "");
    } else if (!existingKind && agent) {
      setKindDisplayName(agent.displayName ?? "");
      setKindDescription(agent.description ?? "");
    }
  }, [isCreateOpen, existingKind, agent]);

  const { mutateAsync: publishAgentKind, isPending: isCreating } = usePublishAgentKind();

  const isDirty = useMemo(
    () => versionName.trim() !== "" || selectedBuildName !== "" || kindDisplayName.trim() !== "" || kindDescription.trim() !== "" || createRows.some((r) => r.key.trim() !== ""),
    [versionName, selectedBuildName, kindDisplayName, kindDescription, createRows],
  );

  const resetCreateForm = useCallback(() => {
    setVersionName("");
    setSelectedBuildName("");
    setKindDisplayName("");
    setKindDescription("");
    setCreateRows([createRuntimeConfigRow()]);
  }, []);

  const handleDrawerClose = useCallback(() => {
    if (isDirty) {
      addConfirmation({
        title: "Discard Changes?",
        description: "You have unsaved changes. Are you sure you want to close without saving?",
        confirmButtonText: "Discard",
        confirmButtonColor: "error",
        onConfirm: () => {
          resetCreateForm();
          navigate(listPath);
        },
      });
    } else {
      navigate(listPath);
    }
  }, [isDirty, addConfirmation, resetCreateForm, navigate, listPath]);

  const handleCreate = useCallback(async () => {
    const configSchema: AgentKindConfigSchemaItem[] = createRows
      .filter((r) => r.key.trim() !== "")
      .map((r) => ({
        name: r.key.trim(),
        isSecret: r.isSecret,
        isMandatory: r.isMandatory ?? false,
        defaultValue: r.defaultValue?.trim() || null,
      }));

    await publishAgentKind({
      params: { orgName: orgId, projName: projectId, agentName: agentId },
      body: {
        kindName: agentId ?? "",
        kindDisplayName: kindDisplayName.trim() || undefined,
        kindDescription: kindDescription.trim() || undefined,
        version: versionName.trim(),
        buildName: selectedBuildName,
        configSchema,
      },
    });

    resetCreateForm();
    navigate(listPath);
  }, [orgId, projectId, agentId, versionName, selectedBuildName, kindDisplayName, kindDescription,
    createRows, publishAgentKind, resetCreateForm, navigate, listPath]);

  const { data: buildsData, isLoading: isBuildsLoading } = useGetAgentBuilds({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  const succeededBuilds = useMemo(
    () => (buildsData?.builds ?? []).filter((b: BuildResponse) => b.status === "Completed" || b.status === "Succeeded"),
    [buildsData],
  );

  const versions = useMemo(
    () =>
      (agentKindVersions ?? []).slice().sort(
        (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
      ),
    [agentKindVersions],
  );

  const latestVersionKey = useMemo(() => versions[0]?.version, [versions]);

  const handleRowClick = (versionKey: string) => {
    navigate(
      generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents
          .children.publish.children.versionDetails.path,
        { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "", versionId: versionKey },
      ),
    );
  };

  return (
    <>
      <PageLayout
        title="Publish"
        description="Manage and publish versions of this agent kind to the catalog."
        disableIcon
        actions={
          <Button
            variant="contained"
            component={Link}
            to={createVersionPath}
            startIcon={<Plus />}
            color="primary"
          >
            Create Version
          </Button>
        }
      >
        <ListingTable.Container>
          {isAgentKindVersionsLoading ? (
            <Box sx={{ m: 2 }}>
              <Skeleton variant="rounded" height={48} sx={{ mb: 1 }} />
              <Skeleton variant="rounded" height={48} sx={{ mb: 1 }} />
              <Skeleton variant="rounded" height={48} sx={{ mb: 1 }} />
              <Skeleton variant="rounded" height={48} />
            </Box>
          ) : versions.length === 0 ? (
            <ListingTable.EmptyState
              illustration={<Package size={64} />}
              title="No versions published yet"
              description="Publish a build as a version to make this agent kind available in the catalog."
            />
          ) : (
            <ListingTable>
              <ListingTable.Head>
                <ListingTable.Row>
                  <ListingTable.Cell width="20%">Version</ListingTable.Cell>
                  <ListingTable.Cell width="18%">Release Date</ListingTable.Cell>
                  <ListingTable.Cell>Build Name</ListingTable.Cell>
                </ListingTable.Row>
              </ListingTable.Head>
              <ListingTable.Body>
                {versions.map((version: AgentKindVersionResponse) => (
                  <ListingTable.Row
                    key={version.version}
                    hover
                    clickable
                    onClick={() => handleRowClick(version.version)}
                  >
                    <ListingTable.Cell>
                      <Typography variant="body2" fontWeight={600}>
                        {version.version}
                        {version.version === latestVersionKey && (
                          <Chip
                            label="Latest"
                            size="small"
                            color="primary"
                            sx={{ ml: 1, height: 18, fontSize: "0.65rem" }}
                          />
                        )}
                      </Typography>
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Typography variant="body2" color="text.secondary">
                        {new Date(version.createdAt).toLocaleDateString(undefined, {
                          year: "numeric",
                          month: "short",
                          day: "numeric",
                        })}
                      </Typography>
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Typography variant="body2" color="text.secondary">
                        {version.buildName ?? "—"}
                      </Typography>
                    </ListingTable.Cell>
                  </ListingTable.Row>
                ))}
              </ListingTable.Body>
            </ListingTable>
          )}
        </ListingTable.Container>
      </PageLayout>

      {/* Create Version Drawer */}
      <DrawerWrapper open={isCreateOpen} onClose={handleDrawerClose} minWidth={700} maxWidth={700}>
        <DrawerHeader title="Create New Version" icon={<Plus size={24} />} onClose={handleDrawerClose} />
        <DrawerContent>
          <Form.Stack spacing={3}>
            {!existingKind && (
              <Form.Section>
                <Form.Subheader>Kind Details</Form.Subheader>
                <Form.Stack spacing={2}>
                  <Form.ElementWrapper label="Display Name" name="kindDisplayName">
                    <TextInput
                      id="kindDisplayName"
                      placeholder="e.g. My Agent Kind"
                      value={kindDisplayName}
                      onChange={(e) => setKindDisplayName(e.target.value)}
                      fullWidth
                      size="small"
                    />
                  </Form.ElementWrapper>
                  <Form.ElementWrapper label="Description" name="kindDescription">
                    <TextInput
                      id="kindDescription"
                      placeholder="Describe this agent kind"
                      value={kindDescription}
                      onChange={(e) => setKindDescription(e.target.value)}
                      fullWidth
                      size="small"
                      multiline
                      rows={2}
                    />
                  </Form.ElementWrapper>
                </Form.Stack>
              </Form.Section>
            )}

            <Form.Section>
              <Form.Subheader>Version Details</Form.Subheader>
              <Form.Stack spacing={2}>
                <Form.ElementWrapper label="Version Name" name="versionName">
                  <TextInput
                    id="versionName"
                    placeholder="e.g. 1.2.0"
                    value={versionName}
                    onChange={(e) => setVersionName(e.target.value)}
                    fullWidth
                    size="small"
                  />
                </Form.ElementWrapper>
                <Form.ElementWrapper label="Build" name="selectedBuildName">
                  <Select
                    id="selectedBuildName"
                    fullWidth
                    size="small"
                    displayEmpty
                    value={selectedBuildName}
                    onChange={(e) => setSelectedBuildName(e.target.value)}
                    disabled={isBuildsLoading}
                    renderValue={(value) => {
                      if (!value) return (
                        <Typography variant="body2" color="text.secondary">Select a build</Typography>
                      );
                      const build = succeededBuilds.find(
                        (b: BuildResponse) => b.buildName === value,
                      );
                      return build ? build.buildName : value;
                    }}
                    endAdornment={
                      isBuildsLoading ? <CircularProgress size={16} sx={{ mr: 3 }} /> : undefined
                    }
                  >
                    {succeededBuilds.length === 0 && !isBuildsLoading && (
                      <MenuItem disabled value="">
                        <Typography variant="body2" color="text.secondary">No succeeded builds available</Typography>
                      </MenuItem>
                    )}
                    {succeededBuilds.map((build: BuildResponse) => (
                      <MenuItem key={build.buildName} value={build.buildName}>
                        <Box>
                          <Typography variant="body2" fontWeight={500}>{build.buildName}</Typography>
                          <Typography variant="caption" color="text.secondary">
                            {build.buildParameters.branch}
                            {build.buildParameters.commitId ? ` · ${build.buildParameters.commitId.slice(0, 7)}` : ""}
                            {" · "}{new Date(build.startedAt).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" })}
                          </Typography>
                        </Box>
                      </MenuItem>
                    ))}
                  </Select>
                </Form.ElementWrapper>
              </Form.Stack>
            </Form.Section>

            <Form.Section>
              <Form.Subheader>Runtime Configuration</Form.Subheader>
              <RuntimeConfigEditor rows={createRows} onChange={setCreateRows} />
            </Form.Section>

            <Box display="flex" justifyContent="flex-end" gap={1}>
              <Button variant="outlined" color="inherit" onClick={handleDrawerClose} disabled={isCreating}>
                Cancel
              </Button>
              <Button
                variant="contained"
                color="primary"
                onClick={handleCreate}
                disabled={isCreating || !versionName.trim() || !selectedBuildName}
              >
                {isCreating ? "Creating..." : "Create Version"}
              </Button>
            </Box>
          </Form.Stack>
        </DrawerContent>
      </DrawerWrapper>
    </>
  );
};

export default PublishedList;
