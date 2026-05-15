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
import { generatePath, useLocation, useNavigate, useParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Chip,
  Divider,
  Form,
  ListingTable,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import { DrawerWrapper, DrawerHeader, DrawerContent, TextInput, PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useGetAgentKind, useGetAgentKindVersion, useGetAgentEndpoints, useUpdateAgentKind } from "@agent-management-platform/api-client";
import { SwaggerSpecViewer, useConfirmationDialog } from "@agent-management-platform/shared-component";
import { RuntimeConfigEditor, createRuntimeConfigRow, type RuntimeConfigRow } from "./RuntimeConfigEditor";

const deepEqual = (a: unknown, b: unknown): boolean => {
  if (a === b) {
    return true;
  }

  if (a === null || b === null || typeof a !== "object" || typeof b !== "object") {
    return false;
  }

  if (Array.isArray(a) && Array.isArray(b)) {
    if (a.length !== b.length) {
      return false;
    }
    return a.every((value, index) => deepEqual(value, b[index]));
  }

  if (Array.isArray(a) || Array.isArray(b)) {
    return false;
  }

  const aObj = a as Record<string, unknown>;
  const bObj = b as Record<string, unknown>;
  const aKeys = Object.keys(aObj);
  const bKeys = Object.keys(bObj);

  if (aKeys.length !== bKeys.length) {
    return false;
  }

  return aKeys.every((key) => deepEqual(aObj[key], bObj[key]));
};

export const PublishVersionDetails: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { orgId, projectId, agentId, versionId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    versionId: string;
  }>();

  const versionDetailsHref = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents
      .children.publish.children.versionDetails.path,
    { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "", versionId: versionId ?? "" },
  );

  const backHref = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.children.publish.path,
    { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "" },
  );

  const isEditOpen = location.pathname.endsWith("/edit");

  const { data: kind } = useGetAgentKind({ orgName: orgId!, kindName: agentId! });
  const { data: version, isLoading: isVersionLoading } = useGetAgentKindVersion({
    orgName: orgId!,
    kindName: agentId!,
    versionTag: versionId!,
  });

  const { data: endpointsData, isLoading: isEndpointsLoading } = useGetAgentEndpoints(
    { orgName: orgId!, projName: projectId!, agentName: agentId! },
    { environment: "default" },
  );

  const endpointKey = useMemo(() => Object.keys(endpointsData ?? {})[0] ?? "", [endpointsData]);
  const apiSpec = useMemo(
    () => endpointsData?.[endpointKey]?.schema?.content as Record<string, unknown> | undefined,
    [endpointsData, endpointKey],
  );

  const formattedDate = useMemo(
    () =>
      version
        ? new Date(version.createdAt).toLocaleDateString("en-US", {
          year: "numeric",
          month: "long",
          day: "numeric",
        })
        : undefined,
    [version],
  );

  // Edit drawer state — kind fields
  const [displayName, setDisplayName] = useState("");
  const [description, setDescription] = useState("");

  useEffect(() => {
    if (kind) {
      setDisplayName(kind.displayName);
      setDescription(kind.description ?? "");
    }
  }, [kind]);

  // Edit drawer state — config schema
  const [editSchema, setEditSchema] = useState<RuntimeConfigRow[]>([]);

  useEffect(() => {
    if (version) {
      setEditSchema(
        version.configSchema.map((item) => ({
          ...createRuntimeConfigRow({
            key: item.name,
            isSecret: item.isSecret,
            isMandatory: item.isMandatory,
            defaultValue: item.defaultValue ?? "",
          }),
        }))
      );
    }
  }, [version]);

  const initialSchemaRows = useMemo(
    () =>
      version?.configSchema.map((item) => ({
        ...createRuntimeConfigRow({
          key: item.name,

          defaultValue: item.defaultValue ?? "",
        }),
      })) ?? [],
    [version],
  );

  const initialDisplayName = kind?.displayName ?? "";
  const initialDescription = kind?.description ?? "";

  const isSchemaChanged = !deepEqual(
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    editSchema.map(({ id, ...row }) => row),
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    initialSchemaRows.map(({ id, ...row }) => row),
  );
  const isDirty =
    displayName !== initialDisplayName || description !== initialDescription || isSchemaChanged;

  const { mutateAsync: updateKind, isPending: isSaving } = useUpdateAgentKind();
  const { addConfirmation } = useConfirmationDialog();

  const handleDrawerClose = useCallback(() => {
    if (isDirty) {
      addConfirmation({
        title: "Discard Changes?",
        description: "You have unsaved changes. Are you sure you want to close without saving?",
        confirmButtonText: "Discard",
        confirmButtonColor: "error",
        onConfirm: () => {
          setDisplayName(initialDisplayName);
          setDescription(initialDescription);
          setEditSchema(initialSchemaRows.map((r) => ({ ...r })));
          navigate(versionDetailsHref);
        },
      });
    } else {
      navigate(versionDetailsHref);
    }
  }, [isDirty, addConfirmation, initialDisplayName, initialDescription, initialSchemaRows,
    navigate, versionDetailsHref]);

  const handleSave = useCallback(async () => {
    if (agentId && orgId) {
      await updateKind({
        params: { orgName: orgId, kindName: agentId },
        body: { displayName: displayName.trim(), description: description.trim() || undefined },
      });
      navigate(versionDetailsHref);
    }
  }, [orgId, agentId, displayName, description, updateKind, navigate, versionDetailsHref]);

  return (
    <>
      <PageLayout
        title={`${displayName || agentId} ${versionId}`}
        description={version ? `Build Id: ${version.buildName ?? "—"}` : ""}
        disableIcon
        backHref={backHref}
        backLabel="Back to Publish"
        actions={
          <Button
            variant="outlined"
            startIcon={<Edit />}
            onClick={() => navigate(versionDetailsHref + "/edit")}
          >
            Edit
          </Button>
        }
      >
        {isVersionLoading ? (
          <Box sx={{ p: 2 }}>
            <Skeleton variant="rounded" height={32} sx={{ mb: 2, maxWidth: 320 }} />
            <Skeleton variant="rounded" height={48} sx={{ mb: 1 }} />
            <Skeleton variant="rounded" height={48} sx={{ mb: 1 }} />
            <Skeleton variant="rounded" height={48} />
          </Box>
        ) : !version ? (
          <Alert severity="error">Version not found.</Alert>
        ) : (
          <Stack spacing={3}>
            {/* Metadata chips */}
            <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
              {version.agentSubType && (
                <Chip label={version.agentSubType} size="small" variant="outlined" />
              )}
              {formattedDate && (
                <Typography variant="body2" color="text.secondary">
                  Published on {formattedDate}
                </Typography>
              )}
            </Stack>

            <Divider />

            {/* Config Schema */}
            <Stack spacing={1.5}>
              <Typography variant="subtitle1" fontWeight={600}>
                Configuration Schema
              </Typography>
              {version.configSchema.length > 0 ? (
                <ListingTable.Container>
                  <ListingTable>
                    <ListingTable.Head>
                      <ListingTable.Row>
                        <ListingTable.Cell width="25%">Name</ListingTable.Cell>
                        <ListingTable.Cell width="30%">Description</ListingTable.Cell>
                        <ListingTable.Cell width="15%">Mandatory</ListingTable.Cell>
                        <ListingTable.Cell width="15%">Secret</ListingTable.Cell>
                        <ListingTable.Cell width="15%">Default Value</ListingTable.Cell>
                      </ListingTable.Row>
                    </ListingTable.Head>
                    <ListingTable.Body>
                      {version.configSchema.map((item) => (
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

            {/* API Specification */}
            <Stack spacing={1.5}>
              <Typography variant="subtitle1" fontWeight={600}>
                API Specification
              </Typography>
              {isEndpointsLoading ? (
                <Skeleton variant="rounded" height={300} />
              ) : apiSpec ? (
                <SwaggerSpecViewer
                  spec={apiSpec}
                  docExpansion="list"
                  hideInfoSection
                  hideServers
                  hideAuthorizeButton
                />
              ) : (
                <Alert severity="info">No API specification available for this version.</Alert>
              )}
            </Stack>
          </Stack>
        )}
      </PageLayout>

      {/* Edit Drawer */}
      <DrawerWrapper open={isEditOpen} onClose={handleDrawerClose} minWidth={700} maxWidth={700}>
        <DrawerHeader title="Edit Agent Kind" icon={<Edit size={24} />} onClose={handleDrawerClose} />
        <DrawerContent>
          <Form.Stack spacing={3}>
            <Form.Section>
              <Form.Subheader>Kind Details</Form.Subheader>
              <Form.Stack spacing={2}>
                <Form.ElementWrapper label="Display Name" name="displayName">
                  <TextInput
                    id="displayName"
                    value={displayName}
                    onChange={(e) => setDisplayName(e.target.value)}
                    fullWidth
                    size="small"
                  />
                </Form.ElementWrapper>
                <Form.ElementWrapper label="Description" name="description">
                  <TextInput
                    id="description"
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    fullWidth
                    size="small"
                    multiline
                    rows={3}
                  />
                </Form.ElementWrapper>
              </Form.Stack>
            </Form.Section>

            {editSchema.length > 0 && (
              <Form.Section>
                <Form.Subheader>Configuration Schema</Form.Subheader>
                <RuntimeConfigEditor rows={editSchema} onChange={setEditSchema} readonlyKey />
              </Form.Section>
            )}

            <Box display="flex" justifyContent="flex-end" gap={1}>
              <Button variant="outlined" color="inherit" onClick={handleDrawerClose} disabled={isSaving}>
                Cancel
              </Button>
              <Button
                variant="contained"
                color="primary"
                onClick={handleSave}
                disabled={isSaving || !displayName.trim()}
              >
                {isSaving ? "Saving..." : "Save Changes"}
              </Button>
            </Box>
          </Form.Stack>
        </DrawerContent>
      </DrawerWrapper>
    </>
  );
};

export default PublishVersionDetails;

