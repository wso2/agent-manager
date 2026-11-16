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

import { useCallback, useState } from "react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useCreateMCPProxy,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type MCPProxy,
  INPUT_LIMITS,
} from "@agent-management-platform/types";
import {
  Alert,
  Button,
  CircularProgress,
  Form,
  Grid,
  IconButton,
  InputAdornment,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import { TextInput, useFormValidation } from "@agent-management-platform/views";
import { useUnsavedChangesGuard } from "@agent-management-platform/shared-component";
import { z } from "zod";
import { type EndpointDraft } from "./EndpointFormFields";
import { EndpointsEditorSection } from "./EndpointsEditorSection";
import { draftToEndpoint } from "./mcpEndpoints";

const DEFAULT_PROXY_VERSION = "1.0.0";
// Matches the service's handle length limit (agent-manager-service/services/mcp_proxy_service.go).
const MAX_HANDLE_LENGTH = 100;

const SEMVER_PATTERN =
  /^\d+\.\d+\.\d+(?:-[0-9A-Za-z-.]+)?(?:\+[0-9A-Za-z-.]+)?$/;

interface VersionFormValues {
  version: string;
}

const addMCPProxySchema = z.object({
  version: z
    .string()
    .trim()
    .min(1, "Version is required")
    .regex(SEMVER_PATTERN, "Use a semantic version, e.g. 0.0.1"),
});

/** The server's details as the register form submits them, trimmed. */
export interface AddMCPProxyFormValues {
  /** The handle the server is registered under. */
  id: string;
  name: string;
  version: string;
  description?: string;
  context?: string;
}

/** Where the form goes once the server is registered. */
export interface AddMCPProxyNavigation {
  to: string;
  state?: unknown;
}

export interface AddMCPProxyFormProps {
  onCancel: () => void;
  /**
   * Registers the server. It resolves with where to go next, or rejects to keep the user on
   * the form; the caller shows the reason (as `error`, or the mutation's own message).
   */
  onSubmit: (
    values: AddMCPProxyFormValues,
    endpoints: EndpointDraft[],
  ) => Promise<AddMCPProxyNavigation>;
  isSubmitting: boolean;
  /** Shown above the endpoints, when the last submit failed. */
  error?: string;
  /** The context a name suggests, from its handle. */
  defaultContext: (handle: string) => string;
  handleHelperText: string;
  /** Shown under the context field, given the context typed so far. */
  contextHelperText?: (context: string) => string | undefined;
  endpointsTitle?: string;
  /** Shown under the endpoints title; defaults to the org wording. */
  endpointsDescription?: string;
  emptyStateText?: string;
  /** The most endpoints the server takes; see EndpointsEditorSection. */
  maxEndpoints?: number;
}

/** The MCP server register form: its details and its endpoints. Registering is the caller's. */
export function AddMCPProxyForm({
  onCancel,
  onSubmit,
  isSubmitting,
  error,
  defaultContext,
  handleHelperText,
  contextHelperText,
  endpointsTitle = "Endpoints",
  endpointsDescription,
  emptyStateText = "No endpoints added yet.",
  maxEndpoints,
}: AddMCPProxyFormProps) {
  const navigate = useNavigate();
  const { orgId } = useParams<{ orgId: string }>();
  const contextFor = useCallback(
    (name: string) => defaultContext(toHandle(name)),
    [defaultContext],
  );
  const { data: environments = [] } = useListEnvironments({
    orgName: orgId ?? "",
  });

  const [proxyName, setProxyName] = useState("");
  const [proxyVersion, setProxyVersion] = useState(DEFAULT_PROXY_VERSION);
  const [proxyDescription, setProxyDescription] = useState("");
  const [proxyContext, setProxyContext] = useState("");
  const [handle, setHandle] = useState("");
  const [handleEdited, setHandleEdited] = useState(false);
  const [isHandleEditable, setIsHandleEditable] = useState(false);
  const [endpoints, setEndpoints] = useState<EndpointDraft[]>([]);
  const [addOpen, setAddOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

  const { errors, validateField, validateForm, setFieldError } =
    useFormValidation<VersionFormValues>(addMCPProxySchema);

  // Derived from the handle actually submitted (toHandle(handle || name)), so a handle
  // auto-generated from a long name is caught too, not just a manually-typed one.
  const effectiveHandle = toHandle(handle || proxyName);
  const handleLengthError =
    effectiveHandle.length > MAX_HANDLE_LENGTH
      ? `Handle must be at most ${MAX_HANDLE_LENGTH} characters (currently ${effectiveHandle.length}).`
      : undefined;

  const isDirty =
    Boolean(proxyName) ||
    proxyVersion !== DEFAULT_PROXY_VERSION ||
    Boolean(proxyDescription) ||
    Boolean(proxyContext) ||
    Boolean(handle) ||
    endpoints.length > 0;
  const { allowNavigation } = useUnsavedChangesGuard(isDirty);

  const handleVersionChange = useCallback(
    (value: string) => {
      setProxyVersion(value);
      setFieldError("version", validateField("version", value));
    },
    [validateField, setFieldError],
  );

  const handleNameChange = useCallback(
    (value: string) => {
      const previousContext = proxyName ? contextFor(proxyName) : "";
      setProxyName(value);
      if (!proxyContext || proxyContext === previousContext) {
        setProxyContext(value ? contextFor(value) : "");
      }
      if (!handleEdited) {
        setHandle(toHandle(value));
      }
    },
    [proxyContext, proxyName, handleEdited, contextFor],
  );

  const handleHandleChange = useCallback((value: string) => {
    setHandleEdited(true);
    setHandle(toHandle(value));
  }, []);

  // Convenience: seed the proxy name/version/context from the first fetched
  // server when the user hasn't typed them yet. They remain fully editable.
  const handleEndpointAdded = useCallback(
    (draft: Omit<EndpointDraft, "id">) => {
      if (!proxyName && draft.serverName) {
        setProxyName(draft.serverName);
        if (!proxyContext) {
          setProxyContext(contextFor(draft.serverName));
        }
        if (!handleEdited) {
          setHandle(toHandle(draft.serverName));
        }
      }
      if (proxyVersion === DEFAULT_PROXY_VERSION && draft.serverVersion) {
        handleVersionChange(draft.serverVersion);
      }
    },
    [
      proxyContext,
      proxyName,
      proxyVersion,
      handleEdited,
      handleVersionChange,
      contextFor,
    ],
  );

  const handleCreate = useCallback(async () => {
    if (endpoints.length === 0) return;
    if (!validateForm({ version: proxyVersion })) return;
    const name = proxyName.trim();
    let next: AddMCPProxyNavigation;
    try {
      next = await onSubmit(
        {
          id: toHandle(handle || name),
          name,
          version: proxyVersion.trim(),
          description: proxyDescription.trim() || undefined,
          context: proxyContext.trim() || undefined,
        },
        endpoints,
      );
    } catch {
      // The caller shows why; the form stays as it is.
      return;
    }
    allowNavigation(() => navigate(next.to, { state: next.state }));
  }, [
    allowNavigation,
    endpoints,
    handle,
    navigate,
    onSubmit,
    proxyContext,
    proxyDescription,
    proxyName,
    proxyVersion,
    validateForm,
  ]);

  const canCreate =
    Boolean(proxyName.trim()) &&
    Boolean(handle.trim()) &&
    Boolean(proxyVersion.trim()) &&
    !errors.version &&
    !handleLengthError &&
    endpoints.length > 0 &&
    !isSubmitting;

  return (
    <Stack spacing={3}>
      <Form.Stack spacing={3}>
        <Grid container spacing={2}>
          <Grid size={{ xs: 12, md: 6, xl: 4 }}>
            <TextInput
              label="Name"
              placeholder="e.g. GitHub MCP Server"
              maxLength={INPUT_LIMITS.NAME}
              hideCharacterCount={proxyName.length < INPUT_LIMITS.NAME}
              value={proxyName}
              onChange={(event) => handleNameChange(event.target.value)}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 6, xl: 4 }}>
            <TextInput
              label="Handle"
              placeholder="e.g. github-mcp-server"
              maxLength={MAX_HANDLE_LENGTH}
              hideCharacterCount={
                !isHandleEditable || handle.length < MAX_HANDLE_LENGTH
              }
              value={handle}
              onChange={(event) => handleHandleChange(event.target.value)}
              error={Boolean(handleLengthError)}
              helperText={handleLengthError ?? handleHelperText}
              disabled={!isHandleEditable && !handle}
              slotProps={{
                htmlInput: { readOnly: !isHandleEditable },
                input: {
                  endAdornment: !isHandleEditable && (
                    <InputAdornment position="end">
                      <IconButton
                        size="small"
                        aria-label="Edit handle"
                        onClick={() => setIsHandleEditable(true)}
                      >
                        <Edit size={16} />
                      </IconButton>
                    </InputAdornment>
                  ),
                },
              }}
            />
          </Grid>
          <Grid
            size={{ xs: 12, xl: 4 }}
            sx={{ display: { xs: "none", xl: "block" } }}
          />
          <Grid size={{ xs: 12, md: 6, xl: 4 }}>
            <TextInput
              label="Version"
              placeholder="1.0.0"
              maxLength={INPUT_LIMITS.SHORT_TEXT}
              hideCharacterCount={proxyVersion.length < INPUT_LIMITS.SHORT_TEXT}
              value={proxyVersion}
              onChange={(event) => handleVersionChange(event.target.value)}
              error={Boolean(errors.version)}
              helperText={errors.version}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 6, xl: 4 }}>
            <TextInput
              label="Context (optional)"
              placeholder="/github"
              helperText={contextHelperText?.(proxyContext)}
              maxLength={INPUT_LIMITS.PATH}
              hideCharacterCount={proxyContext.length < INPUT_LIMITS.PATH}
              value={proxyContext}
              onChange={(event) => setProxyContext(event.target.value)}
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
                proxyDescription.length < INPUT_LIMITS.DESCRIPTION
              }
              multiline
              minRows={3}
              value={proxyDescription}
              onChange={(event) => setProxyDescription(event.target.value)}
              placeholder="What tools does this server provide?"
            />
          </Grid>
        </Grid>

        <Stack spacing={2}>
          <Typography variant="subtitle1">{endpointsTitle}</Typography>
          <Typography variant="body2" color="text.secondary">
            {endpointsDescription ??
              (environments.length > 1
                ? "Add a backend endpoint and assign it to one or more environments. Environments without an endpoint are simply left unconfigured."
                : "Add a backend endpoint for this MCP Server.")}
          </Typography>
          {error && <Alert severity="error">{error}</Alert>}

          <EndpointsEditorSection
            orgId={orgId ?? ""}
            environments={environments}
            endpoints={endpoints}
            onEndpointsChange={setEndpoints}
            addOpen={addOpen}
            onAddOpenChange={setAddOpen}
            editingId={editingId}
            onEditingIdChange={setEditingId}
            emptyStateText={emptyStateText}
            onEndpointAdded={handleEndpointAdded}
            maxEndpoints={maxEndpoints}
          />
        </Stack>
      </Form.Stack>

      {addOpen || editingId !== null ? null : (
        <Stack direction="row" spacing={1}>
          <Button variant="outlined" onClick={() => allowNavigation(onCancel)}>
            Cancel
          </Button>
          <Button
            variant="contained"
            disabled={!canCreate}
            onClick={handleCreate}
            startIcon={
              isSubmitting ? (
                <CircularProgress size={16} color="inherit" />
              ) : undefined
            }
          >
            {isSubmitting ? "Creating" : "Create"}
          </Button>
        </Stack>
      )}
    </Stack>
  );
}

/**
 * Registers an org MCP server: one request with every endpoint, each bound to its
 * environments. The org-level proxy is a grouping and deploys nothing itself.
 */
export function OrgAddMCPProxyForm({ onCancel }: { onCancel: () => void }) {
  const { orgId = "" } = useParams<{ orgId: string }>();
  const createMCPProxy = useCreateMCPProxy();

  const handleSubmit = useCallback(
    async (values: AddMCPProxyFormValues, endpoints: EndpointDraft[]) => {
      const usedHandles = new Set<string>();
      const body: MCPProxy = {
        ...values,
        endpoints: endpoints.map((endpoint, index) =>
          draftToEndpoint(endpoint, index, undefined, usedHandles),
        ),
      };
      // A failure is shown by the mutation's own error message.
      await createMCPProxy.mutateAsync({ params: { orgName: orgId }, body });
      return {
        to: generatePath(absoluteRouteMap.children.org.children.mcpProxies.path, {
          orgId,
        }),
      };
    },
    [createMCPProxy, orgId],
  );

  return (
    <AddMCPProxyForm
      onCancel={onCancel}
      onSubmit={handleSubmit}
      isSubmitting={createMCPProxy.isPending}
      defaultContext={(handle) => `/default/${handle}`}
      handleHelperText="Generated from the name. Unique and immutable once registered."
    />
  );
}

function toHandle(value: string): string {
  const handle = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return handle || "mcp-proxy";
}
