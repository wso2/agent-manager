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

import React, { type ReactNode, useCallback, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Form,
  Grid,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import {
  PageLayout,
  TextInput,
  useFormValidation,
  LoadErrorNotice,
} from "@agent-management-platform/views";
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

export interface EditMCPProxyFormValues {
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

export interface EditMCPProxyFormProps {
  /** The server's id (its handle) and the fields the form edits. */
  proxy: { id: string; name: string; version: string; context?: string; description?: string };
  /** The endpoints the editor starts with. */
  initialEndpoints: EndpointDraft[];
  orgId: string;
  environments: Environment[];
  viewHref: string;
  /** Saves the form; a rejection is shown as `error` and keeps the user on the page. */
  onSave: (values: EditMCPProxyFormValues, endpoints: EndpointDraft[]) => Promise<void>;
  isPending: boolean;
  error: unknown;
  contextPlaceholder?: string;
  contextHelperText?: string;
  /** The most endpoints the server takes; see EndpointsEditorSection. */
  maxEndpoints?: number;
  /** Hides the endpoint timeouts, for a server that does not keep them. */
  hideResilience?: boolean;
  /** Shown in place of the endpoint editor, for a server whose endpoint is not edited here. */
  endpointsNotice?: ReactNode;
  /** Shown under the Endpoints heading in place of the org wording. */
  endpointsDescription?: string;
}

/** The MCP server edit form: its details and its endpoints. Saving is the caller's. */
export function EditMCPProxyForm({
  proxy,
  initialEndpoints,
  orgId,
  environments,
  viewHref,
  onSave,
  isPending,
  error: updateError,
  contextPlaceholder = "/default/my-mcp-proxy",
  contextHelperText,
  maxEndpoints,
  hideResilience,
  endpointsNotice,
  endpointsDescription,
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

  // --- Endpoints section state ---
  // Seeded once; a draft seeded from a backend endpoint keeps its handle as its id.
  const [endpoints, setEndpoints] = useState<EndpointDraft[]>(initialEndpoints);
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
      try {
        await onSave(formData, endpoints);
        allowNavigation(() => navigate(viewHref));
      } catch {
        // Error is surfaced via updateError below.
      }
    },
    [formData, validateForm, endpoints, onSave, allowNavigation, navigate, viewHref],
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

        <Form.Stack spacing={3}>
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, md: 6, xl: 4 }}>
              <TextInput
                label="Name"
                maxLength={INPUT_LIMITS.NAME}
                hideCharacterCount={formData.name.length < INPUT_LIMITS.NAME}
                value={formData.name}
                onChange={(e) => handleFieldChange("name", e.target.value)}
                error={Boolean(errors.name)}
                helperText={errors.name}
                disabled={isPending}
              />
            </Grid>
            <Grid size={{ xs: 12, md: 6, xl: 4 }}>
              <TextInput
                label="Handle"
                value={proxy.id}
                disabled
                helperText="Unique, immutable identifier for the MCP Server. Cannot be changed after the server is registered."
              />
            </Grid>
            <Grid
              size={{ xs: 12, xl: 4 }}
              sx={{ display: { xs: "none", xl: "block" } }}
            />
            <Grid size={{ xs: 12, md: 6, xl: 4 }}>
              <TextInput
                label="Version"
                maxLength={INPUT_LIMITS.SHORT_TEXT}
                hideCharacterCount={
                  formData.version.length < INPUT_LIMITS.SHORT_TEXT
                }
                value={formData.version}
                onChange={(e) => handleFieldChange("version", e.target.value)}
                error={Boolean(errors.version)}
                helperText={errors.version}
                disabled={isPending}
              />
            </Grid>
            <Grid size={{ xs: 12, md: 6, xl: 4 }}>
              <TextInput
                label="Context (optional)"
                maxLength={INPUT_LIMITS.PATH}
                hideCharacterCount={
                  (formData.context?.length ?? 0) < INPUT_LIMITS.PATH
                }
                placeholder={contextPlaceholder}
                value={formData.context}
                onChange={(e) => handleFieldChange("context", e.target.value)}
                error={Boolean(errors.context)}
                helperText={errors.context ?? contextHelperText}
                disabled={isPending}
              />
            </Grid>
            <Grid
              size={{ xs: 12, xl: 4 }}
              sx={{ display: { xs: "none", xl: "block" } }}
            />
            <Grid size={{ xs: 12, md: 12, xl: 8 }}>
              <TextInput
                label="Description (optional)"
                maxLength={INPUT_LIMITS.DESCRIPTION}
                hideCharacterCount={
                  (formData.description?.length ?? 0) < INPUT_LIMITS.DESCRIPTION
                }
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
            </Grid>
          </Grid>

          <Stack spacing={2}>
            <Typography variant="subtitle1">Endpoints</Typography>
            <Typography variant="body2" color="text.secondary">
              {endpointsDescription ??
                (environments.length > 1
                  ? "Endpoints map backend MCP servers to environments. Editing an endpoint updates the upstream URL, authentication, and capabilities for the environments it serves. Environments left without an endpoint become unconfigured."
                  : "Editing an endpoint updates the upstream URL, authentication, and capabilities for this MCP Server.")}
            </Typography>

            {endpointsNotice ?? (
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
              maxEndpoints={maxEndpoints}
              hideResilience={hideResilience}
            />
            )}
          </Stack>
        </Form.Stack>

        {addOpen || editingId !== null ? null : (
          <Box display="flex" gap={1} mt={2}>
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

/** The org MCP server's save: details and endpoints in one update. */
function OrgEditMCPProxyForm({
  proxy,
  orgId,
  environments,
  viewHref,
}: {
  proxy: MCPProxy;
  orgId: string;
  environments: Environment[];
  viewHref: string;
}) {
  const { mutateAsync: updateMCPProxy, isPending, error } = useUpdateMCPProxy();
  const initialEndpoints = useMemo(
    () => (proxy.endpoints ?? []).map(endpointToDraft),
    [proxy.endpoints],
  );

  const handleSave = useCallback(
    async (formData: EditMCPProxyFormValues, endpoints: EndpointDraft[]) => {
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
    },
    [orgId, proxy, updateMCPProxy],
  );

  return (
    <EditMCPProxyForm
      proxy={proxy}
      initialEndpoints={initialEndpoints}
      orgId={orgId}
      environments={environments}
      viewHref={viewHref}
      onSave={handleSave}
      isPending={isPending}
      error={error}
    />
  );
}

export function EditMCPProxy() {
  const { orgId = "", proxyId = "" } = useParams<{ orgId: string; proxyId: string }>();
  const { data: proxy, isLoading, error, refetch } = useGetMCPProxy({ orgName: orgId, proxyId });
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
      {isLoading || isEnvironmentsLoading ? (
        <Skeleton variant="rounded" width="100%" height={400} />
      ) : !proxy ? (
        <LoadErrorNotice
          entityLabel="MCP server"
          error={error}
          onRetry={() => refetch()}
          backHref={generatePath(absoluteRouteMap.children.org.children.mcpProxies.path, { orgId })}
          backLabel="Back to MCP servers"
        />
      ) : (
        <OrgEditMCPProxyForm
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
