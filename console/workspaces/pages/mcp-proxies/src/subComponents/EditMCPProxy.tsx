/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useCallback, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Form,
  FormControl,
  FormLabel,
  Skeleton,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { PageLayout, useFormValidation } from "@agent-management-platform/views";
import {
  useGetMCPProxy,
  useListEnvironments,
  useUpdateMCPProxy,
} from "@agent-management-platform/api-client";
import { useUnsavedChangesGuard } from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  type Environment,
  type MCPProxy,
  INPUT_LIMITS,
} from "@agent-management-platform/types";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { z } from "zod";
import { type EndpointDraft } from "./EndpointFormFields";
import { EndpointsEditorSection } from "./EndpointsEditorSection";
import { draftToEndpoint, endpointToDraft } from "./mcpEndpoints";

interface EditMCPProxyFormValues {
  name: string;
  version: string;
  context?: string;
  description?: string;
}

const editMCPProxySchema = z.object({
  name: z.string().trim().min(1, "Name is required"),
  version: z.string().trim().min(1, "Version is required"),
  context: z.string().trim().optional(),
  description: z.string().trim().optional(),
});

interface EditMCPProxyFormProps {
  proxy: MCPProxy;
  orgId: string;
  environments: Environment[];
  viewHref: string;
}

function EditMCPProxyForm({
  proxy,
  orgId,
  environments,
  viewHref,
}: EditMCPProxyFormProps) {
  const navigate = useNavigate();
  const [formData, setFormData] = useState<EditMCPProxyFormValues>({
    name: proxy.name,
    version: proxy.version,
    context: proxy.context ?? "",
    description: proxy.description ?? "",
  });

  const { errors, validateField, validateForm, setFieldError } =
    useFormValidation<EditMCPProxyFormValues>(editMCPProxySchema);

  const {
    mutateAsync: updateMCPProxy,
    isPending,
    error: updateError,
  } = useUpdateMCPProxy();

  // --- Endpoints section state ---
  // Seeded once from the proxy's native endpoints; a draft seeded from a backend
  // endpoint keeps its handle as its id.
  const [endpoints, setEndpoints] = useState<EndpointDraft[]>(() =>
    (proxy.endpoints ?? []).map(endpointToDraft),
  );
  const [addOpen, setAddOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

  const [seededSnapshot] = useState(() => JSON.stringify({ formData, endpoints }));
  // An endpoint being added/edited inline isn't in `endpoints` until the user
  // applies it, so its form reports its own dirty state.
  const [isEndpointFormDirty, setIsEndpointFormDirty] = useState(false);
  const isDirty =
    isEndpointFormDirty || JSON.stringify({ formData, endpoints }) !== seededSnapshot;
  const { allowNavigation } = useUnsavedChangesGuard(isDirty);

  const handleCancel = useCallback(() => {
    allowNavigation(() => navigate(viewHref));
  }, [allowNavigation, navigate, viewHref]);

  const handleFieldChange = useCallback(
    (field: keyof EditMCPProxyFormValues, value: string) => {
      const error = validateField(field, value);
      setFieldError(field, error);
      setFormData((prev) => ({ ...prev, [field]: value }));
    },
    [validateField, setFieldError],
  );

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!validateForm(formData)) return;

      // Match each draft back to its source endpoint by handle so an edited endpoint
      // keeps its policies, security and tool-scope bindings; drafts added this session
      // have no match and get freshly derived handles + default security.
      const existingByHandle = new Map(
        (proxy.endpoints ?? []).map((endpoint) => [endpoint.id, endpoint]),
      );
      const usedHandles = new Set<string>();
      const nextEndpoints = endpoints.map((draft, index) =>
        draftToEndpoint(
          draft,
          index,
          existingByHandle.get(draft.id),
          usedHandles,
        ),
      );

      try {
        await updateMCPProxy({
          params: { orgName: orgId, proxyId: proxy.id },
          body: {
            ...proxy,
            name: formData.name.trim(),
            version: formData.version.trim(),
            context: formData.context?.trim() || undefined,
            description: formData.description?.trim() || undefined,
            endpoints: nextEndpoints,
          },
        });
        allowNavigation(() => navigate(viewHref));
      } catch {
        // Error is surfaced via updateError below.
      }
    },
    [
      formData, validateForm, endpoints, updateMCPProxy, orgId, proxy,
      allowNavigation, navigate, viewHref,
    ],
  );

  const errorMessage = useMemo(() => {
    if (!updateError) return null;
    return (updateError as Error)?.message ?? "Failed to update MCP Server";
  }, [updateError]);

  const isValid =
    !errors.name &&
    !errors.version &&
    formData.name.trim().length > 0 &&
    formData.version.trim().length > 0;

  return (
    <form onSubmit={handleSubmit}>
      <Stack spacing={3}>
        {errorMessage && (
          <Alert severity="error">
            <Typography variant="body2">{errorMessage}</Typography>
          </Alert>
        )}

        <Form.Section>
          <Form.Header>MCP Server Details</Form.Header>
          <Form.Stack spacing={2}>
            <FormControl fullWidth>
              <FormLabel>Handle</FormLabel>
              <TextField fullWidth size="small" value={proxy.id} disabled />
              <Typography variant="caption" color="text.secondary">
                Unique, immutable identifier for the MCP Server.
                Cannot be changed after the server is registered.
              </Typography>
            </FormControl>
            <FormControl fullWidth error={Boolean(errors.name)}>
              <FormLabel required>Name</FormLabel>
              <TextField
                slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.NAME } }}
                fullWidth
                size="small"
                value={formData.name}
                onChange={(e) => handleFieldChange("name", e.target.value)}
                error={Boolean(errors.name)}
                helperText={errors.name}
                disabled={isPending}
              />
            </FormControl>
            <FormControl fullWidth error={Boolean(errors.version)}>
              <FormLabel required>Version</FormLabel>
              <TextField
                slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.SHORT_TEXT } }}
                fullWidth
                size="small"
                value={formData.version}
                onChange={(e) =>
                  handleFieldChange("version", e.target.value)
                }
                error={Boolean(errors.version)}
                helperText={errors.version}
                disabled={isPending}
              />
            </FormControl>
            <FormControl fullWidth error={Boolean(errors.context)}>
              <FormLabel>Context</FormLabel>
              <TextField
                slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.PATH } }}
                fullWidth
                size="small"
                placeholder="/default/my-mcp-proxy"
                value={formData.context}
                onChange={(e) =>
                  handleFieldChange("context", e.target.value)
                }
                error={Boolean(errors.context)}
                helperText={errors.context}
                disabled={isPending}
              />
            </FormControl>
            <FormControl fullWidth error={Boolean(errors.description)}>
              <FormLabel>Description</FormLabel>
              <TextField
                slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.DESCRIPTION } }}
                fullWidth
                size="small"
                multiline
                minRows={3}
                value={formData.description}
                onChange={(e) =>
                  handleFieldChange("description", e.target.value)
                }
                error={Boolean(errors.description)}
                helperText={errors.description}
                disabled={isPending}
              />
            </FormControl>
          </Form.Stack>
        </Form.Section>

        <Form.Section>
          <Form.Header>Endpoints</Form.Header>
          <Form.Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">
              {environments.length > 1
                ? "Endpoints map backend MCP servers to environments. Editing an endpoint updates the upstream URL, authentication, and capabilities for the environments it serves. Environments left without an endpoint become unconfigured."
                : "Editing an endpoint updates the upstream URL, authentication, and capabilities for this MCP Server."}
            </Typography>

            <EndpointsEditorSection
              orgId={orgId}
              environments={environments}
              endpoints={endpoints}
              onEndpointsChange={setEndpoints}
              addOpen={addOpen}
              onAddOpenChange={setAddOpen}
              editingId={editingId}
              onEditingIdChange={setEditingId}
              emptyStateText="No endpoints configured yet."
              onEndpointFormDirtyChange={setIsEndpointFormDirty}
            />
          </Form.Stack>
        </Form.Section>

        {addOpen || editingId !== null ? null : (
          <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
            <Button
              variant="outlined"
              color="inherit"
              onClick={handleCancel}
              disabled={isPending}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              variant="contained"
              color="primary"
              disabled={!isValid || isPending}
            >
              {isPending ? "Saving..." : "Save"}
            </Button>
          </Box>
        )}
      </Stack>
    </form>
  );
}

export function EditMCPProxy() {
  const { orgId = "", proxyId = "" } = useParams<{ orgId: string; proxyId: string }>();
  const { data: proxy, isLoading } = useGetMCPProxy({ orgName: orgId, proxyId });
  const { data: environments = [], isLoading: isEnvironmentsLoading } =
    useListEnvironments({ orgName: orgId });
  const viewHref = generatePath(
    absoluteRouteMap.children.org.children.mcpProxies.children.view.path,
    { orgId, proxyId },
  );

  return (
    <PageLayout
      title="Edit MCP Server"
      disableIcon
      backHref={viewHref}
      backLabel="Back to MCP server"
    >
      {isLoading || isEnvironmentsLoading || !proxy ? (
        <Skeleton variant="rounded" width="100%" height={400} />
      ) : (
        <EditMCPProxyForm
          proxy={proxy}
          orgId={orgId}
          environments={environments}
          viewHref={viewHref}
        />
      )}
    </PageLayout>
  );
}

export default EditMCPProxy;
