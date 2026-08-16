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
  IconButton,
  Skeleton,
  ListingTable,
  MenuItem,
  Select,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Edit, Layers, Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { DrawerWrapper, DrawerHeader, DrawerContent, TextInput, PageLayout, DescriptionCard } from "@agent-management-platform/views";
import {
  absoluteRouteMap,
  type AgentKindConfigSchemaItem,
  type AgentKindVersionResponse,
  type BuildResponse,
} from "@agent-management-platform/types";
import { LabelsEditor, useConfirmationDialog } from "@agent-management-platform/shared-component";
import { RuntimeConfigEditor, createRuntimeConfigRow, type RuntimeConfigRow } from "./RuntimeConfigEditor";
import { KindDescriptionField } from "./KindDescriptionField";
import { DangerZoneCard } from "./DangerZoneCard";
import { useDeleteVersionAction } from "./useDeleteVersionAction";
import { useDeleteAgentKind, useGetAgent, useGetAgentBuilds, useGetAgentKind, useListAgentKindVersions, usePublishAgentKind, useUpdateAgentKind } from "@agent-management-platform/api-client";

/** Order-independent equality for label maps — key insertion order shouldn't count as a change. */
const labelsEqual = (a: Record<string, string>, b: Record<string, string>): boolean => {
  const aKeys = Object.keys(a);
  const bKeys = Object.keys(b);
  return aKeys.length === bKeys.length && aKeys.every((key) => a[key] === b[key]);
};

export const PublishedList: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();

  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  const { mutate: unpublishAgentKind, isPending: isUnpublishing, isSuccess: hasUnpublished } =
    useDeleteAgentKind();

  const { confirmDeleteVersion } = useDeleteVersionAction({ orgName: orgId, kindName: agentId });

  const { data: agentKindVersions, isLoading: isAgentKindVersionsLoading } =
    useListAgentKindVersions({ orgName: orgId, kindName: agentId! });

  const { data: agent } = useGetAgent({
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

  const editKindPath = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents
      .children.publish.children.editKind.path,
    { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "" },
  );

  const isCreateOpen = location.pathname.endsWith("/create-new-version");
  const isEditKindOpen = location.pathname.endsWith("/edit-kind");

  // Create drawer state
  const [versionName, setVersionName] = useState("");
  const [selectedBuildName, setSelectedBuildName] = useState("");
  const [kindDisplayName, setKindDisplayName] = useState("");
  const [kindDescription, setKindDescription] = useState("");
  const [kindLabels, setKindLabels] = useState<Record<string, string>>({});
  const [createRows, setCreateRows] = useState<RuntimeConfigRow[]>([createRuntimeConfigRow()]);

  const { addConfirmation } = useConfirmationDialog();

  // Shared "close a dirty drawer" confirmation, used by both the Create
  // Version and Edit Kind drawers below.
  const confirmDiscardIfDirty = useCallback((dirty: boolean, onDiscard: () => void) => {
    if (dirty) {
      addConfirmation({
        title: "Discard Changes?",
        description: "You have unsaved changes. Are you sure you want to close without saving?",
        confirmButtonText: "Discard",
        confirmButtonColor: "error",
        onConfirm: onDiscard,
      });
    } else {
      onDiscard();
    }
  }, [addConfirmation]);

  // Edit Kind drawer state — separate from the create-version drawer's
  // kind-detail fields above, so editing an existing kind never interacts
  // with pre-filling a new one.
  const [editDisplayName, setEditDisplayName] = useState("");
  const [editDescription, setEditDescription] = useState("");
  const [editLabels, setEditLabels] = useState<Record<string, string>>({});

  const { mutateAsync: updateKind, isPending: isSavingKind } = useUpdateAgentKind();

  const isEditKindDirty =
    editDisplayName !== (existingKind?.displayName ?? "") ||
    editDescription !== (existingKind?.description ?? "") ||
    !labelsEqual(editLabels, existingKind?.labels ?? {});

  const handleCloseEditKind = useCallback(() => {
    confirmDiscardIfDirty(isEditKindDirty, () => navigate(listPath));
  }, [isEditKindDirty, confirmDiscardIfDirty, navigate, listPath]);

  const handleSaveEditKind = useCallback(async () => {
    if (!orgId || !agentId) return;
    // Labels are always sent so removing the last one clears them ({} =
    // clear, absent = leave unchanged on the backend).
    await updateKind({
      params: { orgName: orgId, kindName: agentId },
      body: {
        displayName: editDisplayName.trim(),
        description: editDescription.trim() || undefined,
        labels: editLabels,
      },
    });
    navigate(listPath);
  }, [orgId, agentId, editDisplayName, editDescription,
    editLabels, updateKind, navigate, listPath]);

  // Pre-fill drawer fields from existing kind data when either drawer opens.
  // Skipped once hasUnpublished is true — the drawers are closed at that
  // point anyway, so there's nothing to pre-fill, only a pointless re-render
  // as `existingKind` settles to null once the invalidated query refetches.
  useEffect(() => {
    if (hasUnpublished) {
      return;
    }
    if (isCreateOpen && existingKind) {
      setKindDisplayName(existingKind.displayName ?? "");
      setKindDescription(existingKind.description ?? "");
    } else if (!existingKind && agent) {
      setKindDisplayName(agent.displayName ?? "");
      setKindDescription(agent.description ?? "");
    }
    if (isEditKindOpen && existingKind) {
      setEditDisplayName(existingKind.displayName ?? "");
      setEditDescription(existingKind.description ?? "");
      setEditLabels(existingKind.labels ?? {});
    }
  }, [isCreateOpen, isEditKindOpen, existingKind, agent, hasUnpublished]);

  const { mutateAsync: publishAgentKind, isPending: isCreating } = usePublishAgentKind();

  const handleUnpublishKind = useCallback(() => {
    addConfirmation({
      title: "Unpublish Kind",
      description: "Are you sure you want to unpublish this Agent Kind? This removes it and all its versions from the catalog. This action cannot be undone.",
      confirmButtonText: "Unpublish",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () => unpublishAgentKind({ orgName: orgId!, kindName: agentId! }),
    });
  }, [addConfirmation, unpublishAgentKind, orgId, agentId]);

  const isDirty = useMemo(
    () => versionName.trim() !== "" || selectedBuildName !== "" || kindDisplayName.trim() !== "" || kindDescription.trim() !== "" || Object.keys(kindLabels).length > 0 || createRows.some((r) => r.key.trim() !== ""),
    [versionName, selectedBuildName, kindDisplayName, kindDescription, kindLabels, createRows],
  );

  const resetCreateForm = useCallback(() => {
    setVersionName("");
    setSelectedBuildName("");
    setKindDisplayName("");
    setKindDescription("");
    setKindLabels({});
    setCreateRows([createRuntimeConfigRow()]);
  }, []);

  const handleDrawerClose = useCallback(() => {
    confirmDiscardIfDirty(isDirty, () => {
      resetCreateForm();
      navigate(listPath);
    });
  }, [isDirty, confirmDiscardIfDirty, resetCreateForm, navigate, listPath]);

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
        kindLabels: Object.keys(kindLabels).length > 0 ? kindLabels : undefined,
        version: versionName.trim(),
        buildName: selectedBuildName,
        configSchema,
      },
    });

    resetCreateForm();
    navigate(listPath);
  }, [orgId, projectId, agentId, versionName, selectedBuildName, kindDisplayName, kindDescription,
    kindLabels, createRows, publishAgentKind, resetCreateForm, navigate, listPath]);

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
        description="Manage and publish versions of this Agent Kind to the catalog."
        disableIcon
        actions={[
          existingKind && (
            <Button
              key="edit-kind"
              variant="outlined"
              component={Link}
              to={editKindPath}
              startIcon={<Edit />}
            >
              Edit Kind
            </Button>
          ),
          <Button
            key="create-version"
            variant="contained"
            component={Link}
            to={createVersionPath}
            startIcon={<Plus />}
            color="primary"
          >
            Create Version
          </Button>,
        ].filter(Boolean)}
      >
        {existingKind?.description && (
          <DescriptionCard content={existingKind.description} sx={{ mb: 2 }} />
        )}

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
              illustration={<Layers size={64} />}
              title="No versions published yet"
              description="Publish a build as a version to make this Agent Kind available in the catalog."
            />
          ) : (
            <ListingTable>
              <ListingTable.Head>
                <ListingTable.Row>
                  <ListingTable.Cell width="20%">Version</ListingTable.Cell>
                  <ListingTable.Cell width="18%">Release Date</ListingTable.Cell>
                  <ListingTable.Cell>Build Name</ListingTable.Cell>
                  <ListingTable.Cell width="5%" align="center" />
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
                      <Typography variant="body2" fontWeight={600} component="span">
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
                    <ListingTable.Cell align="center">
                      <ListingTable.RowActions visibility="hover">
                        <Tooltip title="Delete version">
                          <IconButton
                            color="error"
                            size="small"
                            onClick={(e: React.MouseEvent) => {
                              e.stopPropagation();
                              confirmDeleteVersion(version.version);
                            }}
                          >
                            <Trash size={16} />
                          </IconButton>
                        </Tooltip>
                      </ListingTable.RowActions>
                    </ListingTable.Cell>
                  </ListingTable.Row>
                ))}
              </ListingTable.Body>
            </ListingTable>
          )}
        </ListingTable.Container>

        {existingKind && (
          <Box sx={{ mt: 3 }}>
            <DangerZoneCard
              title="Unpublish this Agent Kind"
              description="Removes it and all its versions from the catalog. This action cannot be undone."
              buttonLabel="Unpublish Kind"
              pendingLabel="Unpublishing..."
              isPending={isUnpublishing}
              onClick={handleUnpublishKind}
            />
          </Box>
        )}
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
                  <KindDescriptionField
                    id="kindDescription"
                    value={kindDescription}
                    onChange={setKindDescription}
                  />
                  <Form.ElementWrapper label="Labels (optional)" name="kindLabels">
                    <LabelsEditor
                      hideTitle
                      description="Attach key/value labels to organize and filter kinds."
                      value={kindLabels}
                      onChange={setKindLabels}
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

      {/* Edit Kind Drawer */}
      <DrawerWrapper
        open={isEditKindOpen}
        onClose={handleCloseEditKind}
        minWidth={700}
        maxWidth={700}
      >
        <DrawerHeader title="Edit Agent Kind" icon={<Edit size={24} />} onClose={handleCloseEditKind} />
        <DrawerContent>
          <Form.Stack spacing={3}>
            <Form.Section>
              <Form.Subheader>Kind Details</Form.Subheader>
              <Form.Stack spacing={2}>
                <Form.ElementWrapper label="Display Name" name="editDisplayName">
                  <TextInput
                    id="editDisplayName"
                    value={editDisplayName}
                    onChange={(e) => setEditDisplayName(e.target.value)}
                    fullWidth
                    size="small"
                  />
                </Form.ElementWrapper>
                <KindDescriptionField
                  id="editDescription"
                  value={editDescription}
                  onChange={setEditDescription}
                />
                <Form.ElementWrapper label="Labels (optional)" name="editLabels">
                  <LabelsEditor
                    hideTitle
                    description="Attach key/value labels to organize and filter kinds."
                    value={editLabels}
                    onChange={setEditLabels}
                    disabled={isSavingKind}
                  />
                </Form.ElementWrapper>
              </Form.Stack>
            </Form.Section>

            <Box display="flex" justifyContent="flex-end" gap={1}>
              <Button variant="outlined" color="inherit" onClick={handleCloseEditKind} disabled={isSavingKind}>
                Cancel
              </Button>
              <Button
                variant="contained"
                color="primary"
                onClick={handleSaveEditKind}
                disabled={isSavingKind || !editDisplayName.trim()}
              >
                {isSavingKind ? "Saving..." : "Save Changes"}
              </Button>
            </Box>
          </Form.Stack>
        </DrawerContent>
      </DrawerWrapper>
    </>
  );
};

export default PublishedList;
