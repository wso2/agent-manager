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
  useCreateProjectMCPProxy,
  useListEnvironments,
  usePutProjectMCPProxyEnvironment,
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
import {
  environmentLabel,
  useUnsavedChangesGuard,
} from "@agent-management-platform/shared-component";
import { z } from "zod";
import { type EndpointDraft } from "./EndpointFormFields";
import { EndpointsEditorSection } from "./EndpointsEditorSection";
import { draftToEndpoint } from "./mcpEndpoints";
import {
  type BindingFailure,
  type ProjectProxyViewState,
} from "./projectBinding";

const DEFAULT_PROXY_VERSION = "1.0.0";
// Matches the service's handle length limit (agent-manager-service/services/mcp_proxy_service.go).
const MAX_HANDLE_LENGTH = 100;

const SEMVER_PATTERN =
  /^\d+\.\d+\.\d+(?:-[0-9A-Za-z-.]+)?(?:\+[0-9A-Za-z-.]+)?$/;

interface AddMCPProxyFormValues {
  version: string;
}

const addMCPProxySchema = z.object({
  version: z
    .string()
    .trim()
    .min(1, "Version is required")
    .regex(SEMVER_PATTERN, "Use a semantic version, e.g. 0.0.1"),
});

interface AddMCPProxyFormProps {
  onCancel: () => void;
  /**
   * Registers the server for this project instead of the organization. A project server
   * fronts one URL, deployed to each environment picked for it, and its context is
   * relative to the project.
   */
  projectId?: string;
}

export function AddMCPProxyForm({ onCancel, projectId }: AddMCPProxyFormProps) {
  const navigate = useNavigate();
  const { orgId } = useParams<{ orgId: string }>();
  const createMCPProxy = useCreateMCPProxy();
  const createProjectMCPProxy = useCreateProjectMCPProxy();
  const putProjectBinding = usePutProjectMCPProxyEnvironment();
  const [projectError, setProjectError] = useState<string>();
  // An org server's context is under /default; a project server's is served under the project.
  const contextFor = useCallback(
    (name: string) =>
      projectId ? `/${toHandle(name)}` : `/default/${toHandle(name)}`,
    [projectId],
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
    useFormValidation<AddMCPProxyFormValues>(addMCPProxySchema);

  // Derived from the handle actually submitted (toHandle(handle || name)), so a handle
  // auto-generated from a long name is caught too, not just a manually-typed one.
  const effectiveHandle = toHandle(handle || proxyName);
  const handleLengthError =
    effectiveHandle.length > MAX_HANDLE_LENGTH
      ? `Handle must be at most ${MAX_HANDLE_LENGTH} characters (currently ${effectiveHandle.length}).`
      : undefined;

  const isCreating =
    createMCPProxy.isPending ||
    createProjectMCPProxy.isPending ||
    putProjectBinding.isPending;

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

  // A project server: the proxy fronts the endpoint's URL, then each environment the
  // endpoint is assigned gets its own binding with the endpoint's credential. Agent Manager
  // discovers each binding's tools through its gateway once it is deployed.
  const handleCreateForProject = useCallback(async () => {
    if (!orgId || !projectId || endpoints.length === 0) return;
    if (!validateForm({ version: proxyVersion })) return;
    setProjectError(undefined);

    const proxyId = toHandle(handle || proxyName.trim());
    const endpoint = draftToEndpoint(endpoints[0], 0);
    const params = { orgName: orgId, projName: projectId, proxyId };
    try {
      await createProjectMCPProxy.mutateAsync({
        params,
        body: {
          name: proxyName.trim(),
          version: proxyVersion.trim(),
          description: proxyDescription.trim() || undefined,
          context: proxyContext.trim() || undefined,
          upstream: { url: endpoint.upstream?.main?.url ?? "" },
        },
      });
    } catch (err) {
      // Registering only creates: an id the project already has is refused and left as it is.
      setProjectError(
        (err as { status?: number })?.status === 412
          ? `An MCP server with the id "${proxyId}" already exists in this project.`
          : ((err as { message?: string })?.message ??
              "Failed to register the MCP server."),
      );
      return;
    }
    const auth = endpoint.upstream?.main?.auth;
    // The server is registered either way: environments it could not be deployed to are
    // named on its page, where Edit deploys it to them again.
    const bindingFailures: BindingFailure[] = [];
    for (const binding of endpoint.environments) {
      const env = environments.find((e) => e.id === binding.environmentUuid);
      const envName = env?.name;
      if (!envName) {
        bindingFailures.push({
          environment: binding.environmentUuid,
          message: "The environment was not found.",
        });
        continue;
      }
      try {
        await putProjectBinding.mutateAsync({
          params: { ...params, envName },
          body: {
            ...(binding.gatewayId ? { gatewayId: binding.gatewayId } : {}),
            ...(auth?.header
              ? {
                  upstreamAuth: {
                    type: "api-key",
                    header: auth.header,
                    value: auth.value,
                  },
                }
              : {}),
            ...(endpoint.resilience ? { resilience: endpoint.resilience } : {}),
          },
        });
      } catch (err) {
        bindingFailures.push({
          environment: environmentLabel(env),
          message:
            (err as { message?: string })?.message ?? "Deployment failed.",
        });
      }
    }
    const state: ProjectProxyViewState | undefined = bindingFailures.length
      ? { bindingFailures }
      : undefined;
    allowNavigation(() =>
      navigate(
        generatePath(
          absoluteRouteMap.children.org.children.projects.children.mcpProxies
            .children.view.path,
          { orgId, projectId, proxyId },
        ),
        { state },
      ),
    );
  }, [
    allowNavigation,
    createProjectMCPProxy,
    endpoints,
    environments,
    handle,
    navigate,
    orgId,
    projectId,
    proxyContext,
    proxyDescription,
    proxyName,
    proxyVersion,
    putProjectBinding,
    validateForm,
  ]);

  const handleCreate = useCallback(async () => {
    if (projectId) {
      await handleCreateForProject();
      return;
    }
    if (!orgId || endpoints.length === 0) return;
    if (!validateForm({ version: proxyVersion })) return;

    const name = proxyName.trim();

    // Each endpoint carries its own upstream, fetched capabilities and default security,
    // and is bound to one or more environments. The org-level proxy is a grouping and
    // deploys nothing itself.
    const body: MCPProxy = {
      id: toHandle(handle || name),
      name,
      version: proxyVersion.trim(),
      description: proxyDescription.trim() || undefined,
      context: proxyContext.trim() || undefined,
      endpoints: (() => {
        const usedHandles = new Set<string>();
        return endpoints.map((endpoint, index) =>
          draftToEndpoint(endpoint, index, undefined, usedHandles),
        );
      })(),
    };

    await createMCPProxy.mutateAsync({ params: { orgName: orgId }, body });
    allowNavigation(() =>
      navigate(
        generatePath(absoluteRouteMap.children.org.children.mcpProxies.path, {
          orgId,
        }),
      ),
    );
  }, [
    allowNavigation,
    createMCPProxy,
    handleCreateForProject,
    projectId,
    endpoints,
    navigate,
    orgId,
    proxyContext,
    proxyDescription,
    handle,
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
    !isCreating;

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
              helperText={
                handleLengthError ??
                (projectId
                  ? "Generated from the name. Unique in the organization and immutable once registered."
                  : "Generated from the name. Unique and immutable once registered.")
              }
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
              helperText={
                projectId
                  ? `Served under the project, at /${projectId}${proxyContext || "/<handle>"}.`
                  : undefined
              }
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
          <Typography variant="subtitle1">
            {projectId ? "Endpoint" : "Endpoints"}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {projectId
              ? "Add the MCP server's public URL and assign it to the environments to deploy it to. Each environment is secured with an API key."
              : environments.length > 1
                ? "Add a backend endpoint and assign it to one or more environments. Environments without an endpoint are simply left unconfigured."
                : "Add a backend endpoint for this MCP Server."}
          </Typography>
          {projectError && <Alert severity="error">{projectError}</Alert>}

          <EndpointsEditorSection
            orgId={orgId ?? ""}
            environments={environments}
            endpoints={endpoints}
            onEndpointsChange={setEndpoints}
            addOpen={addOpen}
            onAddOpenChange={setAddOpen}
            editingId={editingId}
            onEditingIdChange={setEditingId}
            emptyStateText={
              projectId ? "No endpoint added yet." : "No endpoints added yet."
            }
            onEndpointAdded={handleEndpointAdded}
            maxEndpoints={projectId ? 1 : undefined}
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
              isCreating ? (
                <CircularProgress size={16} color="inherit" />
              ) : undefined
            }
          >
            {isCreating ? "Creating" : "Create"}
          </Button>
        </Stack>
      )}
    </Stack>
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
