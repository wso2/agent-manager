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

import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Form,
  FormControl,
  FormLabel,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
  useFormValidation,
} from "@agent-management-platform/views";
import { useUpdateMCPProxy } from "@agent-management-platform/api-client";
import { type Environment, type MCPProxy, INPUT_LIMITS } from "@agent-management-platform/types";
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

interface EditMCPProxyDrawerProps {
  open: boolean;
  onClose: () => void;
  proxy: MCPProxy;
  orgId: string;
  environments: Environment[];
}

export function EditMCPProxyDrawer({
  open,
  onClose,
  proxy,
  orgId,
  environments,
}: EditMCPProxyDrawerProps) {
  const [formData, setFormData] = useState<EditMCPProxyFormValues>({
    name: proxy.name,
    version: proxy.version,
    context: proxy.context ?? "",
    description: proxy.description ?? "",
  });

  const { errors, validateField, validateForm, clearErrors, setFieldError } =
    useFormValidation<EditMCPProxyFormValues>(editMCPProxySchema);

  const {
    mutateAsync: updateMCPProxy,
    isPending,
    error: updateError,
    reset: resetMutation,
  } = useUpdateMCPProxy();

  // --- Endpoints section state ---
  const [endpoints, setEndpoints] = useState<EndpointDraft[]>([]);
  const [addOpen, setAddOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

  const wasOpenRef = useRef(false);

  useEffect(() => {
    const justOpened = open && !wasOpenRef.current;
    wasOpenRef.current = open;
    // Only (re)seed on the closed→open transition — proxy can change while the
    // drawer is already open (e.g. a background refetch), and reseeding then
    // would clobber whatever the user is mid-editing.
    if (!justOpened) return;

    setFormData({
      name: proxy.name,
      version: proxy.version,
      context: proxy.context ?? "",
      description: proxy.description ?? "",
    });
    clearErrors();
    resetMutation();

    // Seed the working list from the proxy's native endpoints each time the drawer is
    // opened. A draft seeded from a backend endpoint keeps its handle as its id.
    setEndpoints((proxy.endpoints ?? []).map(endpointToDraft));
    setAddOpen(false);
    setEditingId(null);
  }, [proxy, open, clearErrors, resetMutation]);

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
        onClose();
      } catch {
        // Error is surfaced via updateError below.
      }
    },
    [formData, validateForm, endpoints, updateMCPProxy, orgId, proxy, onClose],
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
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<Edit size={24} />}
        title="Edit MCP Server"
        onClose={onClose}
      />
      <DrawerContent>
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
                />
              </Form.Stack>
            </Form.Section>

            {addOpen || editingId !== null ? null : (
              <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
                <Button
                  variant="outlined"
                  color="inherit"
                  onClick={onClose}
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
      </DrawerContent>
    </DrawerWrapper>
  );
}
