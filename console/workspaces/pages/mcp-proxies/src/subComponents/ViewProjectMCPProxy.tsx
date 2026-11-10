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

import { type SyntheticEvent, useCallback, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  Grid,
  IconButton,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  Copy,
  Edit,
  Plus,
  Trash,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, useParams, useSearchParams } from "react-router-dom";
import {
  useDeleteProjectMCPProxyEnvironment,
  useGetProjectMCPProxy,
  useListEnvironments,
  usePutProjectMCPProxyEnvironment,
  useRefreshProjectMCPProxyCapabilities,
  useUpdateProjectMCPProxy,
} from "@agent-management-platform/api-client";
import {
  environmentLabel,
  normalizeVersion,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import {
  absoluteRouteMap,
  type MCPEndpointConfig,
  type ProjectMCPProxyEnvironmentRequest,
} from "@agent-management-platform/types";
import { CreatedMetadata, PageLayout } from "@agent-management-platform/views";
import { MCPProxyConnectionTab } from "./MCPProxyConnectionTab";
import { MCPProxyManageToolsTab } from "./MCPProxyManageToolsTab";
import { MCPProxyOverviewTab } from "./MCPProxyOverviewTab";
import { MCPProxyPoliciesTab } from "./MCPProxyPoliciesTab";
import { MCPProxyRewriteTab } from "./MCPProxyRewriteTab";
import {
  BindingAPIKeysTab,
  BindingSecurityTab,
  BindingStatusChip,
} from "./ProjectBindingTabs";
import {
  applyBindingConfigChange,
  bindingEndpointConfig,
  proxyAsOrgProxy,
  proxyRequestFrom,
} from "./projectBinding";
import { useCopyWithFeedback } from "./useCopyWithFeedback";

// The org MCP server tabs, with the API keys a project binding has of its own. As on the
// org page, the tab and environment are in the URL so they survive a reload.
const TAB_DEFS = [
  { label: "Overview", slug: "overview" },
  { label: "Connection", slug: "connection" },
  { label: "Manage Tools", slug: "manage-tools" },
  { label: "Security", slug: "security" },
  { label: "API Keys", slug: "api-keys" },
  { label: "Rewrite", slug: "rewrite" },
  { label: "Policies", slug: "policies" },
] as const;

function InfoCard({
  label,
  value,
  onCopy,
}: {
  label: string;
  value?: string;
  onCopy?: () => void;
}) {
  return (
    <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
      <Stack spacing={0.5}>
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ fontWeight: 500 }}
        >
          {label}
        </Typography>
        <Stack direction="row" alignItems="center" spacing={0.5}>
          <Typography
            variant="body2"
            sx={{ fontFamily: "monospace", wordBreak: "break-all" }}
          >
            {value || "—"}
          </Typography>
          {value && onCopy && (
            <Tooltip title={`Copy ${label}`}>
              <IconButton
                size="small"
                aria-label={`Copy ${label}`}
                onClick={onCopy}
              >
                <Copy size={14} />
              </IconButton>
            </Tooltip>
          )}
        </Stack>
      </Stack>
    </Card>
  );
}

interface EditDraft {
  name: string;
  version: string;
  context: string;
  description: string;
}

/**
 * A project MCP server, laid out as the org MCP server page. Where an org server has
 * endpoints, a project server has one binding per environment it is deployed to, each
 * configured on its own; the environment picker chooses the binding the tabs show.
 */
export function ViewProjectMCPProxy() {
  const {
    orgId,
    projectId,
    proxyId = "",
  } = useParams<{
    orgId: string;
    projectId: string;
    proxyId: string;
  }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const {
    data: proxy,
    isLoading,
    error,
  } = useGetProjectMCPProxy({ orgName: orgId, projName: projectId, proxyId });
  const { data: environments = [] } = useListEnvironments({ orgName: orgId });
  const putBinding = usePutProjectMCPProxyEnvironment();
  const removeBinding = useDeleteProjectMCPProxyEnvironment();
  const updateProxy = useUpdateProjectMCPProxy();
  const refreshTools = useRefreshProjectMCPProxyCapabilities();
  const { addConfirmation } = useConfirmationDialog();
  const { canWrite } = useResourceAccess("mcp_server");
  const handleCopy = useCopyWithFeedback();
  const [addEnvOpen, setAddEnvOpen] = useState(false);
  const [envToAdd, setEnvToAdd] = useState("");
  const [editDraft, setEditDraft] = useState<EditDraft>();
  // A server registered by URL is managed here; one from a component is managed by its publisher.
  const isURLProxy = !!proxy && !proxy.source;

  const tabSlug = searchParams.get("tab");
  const tabIndex = tabSlug
    ? Math.max(
        0,
        TAB_DEFS.findIndex((tab) => tab.slug === tabSlug),
      )
    : 0;
  const activeTabSlug = TAB_DEFS[tabIndex]?.slug;

  const bindings = useMemo(() => proxy?.environments ?? [], [proxy]);
  const selected =
    bindings.find((env) => env.environmentName === searchParams.get("env")) ??
    bindings[0];

  const setParam = useCallback(
    (key: string, value: string | undefined) =>
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (value) next.set(key, value);
          else next.delete(key);
          return next;
        },
        { replace: true },
      ),
    [setSearchParams],
  );

  const envLabel = useCallback(
    (uuid: string, name: string) =>
      environmentLabel(environments.find((env) => env.id === uuid)) || name,
    [environments],
  );

  const proxyParams = { orgName: orgId, projName: projectId, proxyId };

  const saveBinding = useCallback(
    (request: ProjectMCPProxyEnvironmentRequest) =>
      putBinding.mutateAsync({
        params: {
          orgName: orgId,
          projName: projectId,
          proxyId,
          envName: selected?.environmentName,
        },
        body: request,
      }),
    [putBinding, orgId, projectId, proxyId, selected?.environmentName],
  );

  // The selected binding as the org tabs read an endpoint, and their edits written back to it.
  const selectedConfig: MCPEndpointConfig | undefined =
    proxy && selected ? bindingEndpointConfig(proxy, selected) : undefined;
  const updateSelectedConfig = useCallback(
    async (fields: Partial<MCPEndpointConfig>) => {
      if (!proxy || !selected) return undefined;
      const change = applyBindingConfigChange(proxy, selected, fields);
      if (change.proxyURL) {
        // The URL is the proxy's: changing it redeploys every environment with it.
        await updateProxy.mutateAsync({
          params: proxyParams,
          body: {
            ...proxyRequestFrom(proxy),
            upstream: { url: change.proxyURL },
          },
        });
      }
      return saveBinding(change.binding);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- proxyParams is from these params
    [proxy, selected, saveBinding, updateProxy, orgId, projectId, proxyId],
  );
  const isUpdating = putBinding.isPending || updateProxy.isPending;

  const unboundEnvironments = environments.filter(
    (env) => !bindings.some((binding) => binding.environmentName === env.name),
  );

  const addEnvironment = async () => {
    if (!envToAdd) return;
    // The new binding starts as the selected one is: same credential and policies. Its tools
    // are discovered there, unless the selected binding's were given by its caller.
    const body: ProjectMCPProxyEnvironmentRequest = selected
      ? { ...applyBindingConfigChange(proxy!, selected, {}).binding }
      : {};
    delete body.gatewayId;
    await putBinding.mutateAsync({
      params: { ...proxyParams, envName: envToAdd },
      body,
    });
    setParam("env", envToAdd);
    setAddEnvOpen(false);
    setEnvToAdd("");
  };

  const confirmRemoveEnvironment = () => {
    if (!selected) return;
    const label = envLabel(selected.environmentUuid, selected.environmentName);
    addConfirmation({
      title: "Remove from environment",
      description: `Remove ${proxy?.name ?? proxyId} from ${label}? Its API keys there are revoked and agents in ${label} can no longer call it.`,
      confirmButtonText: "Remove",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () => {
        removeBinding.mutate({
          ...proxyParams,
          envName: selected.environmentName,
        });
        setParam("env", undefined);
      },
    });
  };

  const saveEdit = async () => {
    if (!proxy || !editDraft) return;
    await updateProxy.mutateAsync({
      params: proxyParams,
      body: {
        ...proxyRequestFrom(proxy),
        name: editDraft.name.trim(),
        version: editDraft.version.trim(),
        context: editDraft.context.trim(),
        description: editDraft.description.trim() || undefined,
      },
    });
    setEditDraft(undefined);
  };

  const backHref = generatePath(
    absoluteRouteMap.children.org.children.projects.children.mcpProxies.path,
    { orgId: orgId ?? "", projectId: projectId ?? "" },
  );

  const selectedEnvChips = selected
    ? [
        {
          id: selected.environmentUuid,
          label: envLabel(selected.environmentUuid, selected.environmentName),
          status:
            selected.deploymentStatus === "Deployed"
              ? ("Deployed" as const)
              : ("Undeployed" as const),
        },
      ]
    : [];

  return (
    <PageLayout
      title={proxy?.name ?? proxyId}
      backHref={backHref}
      backLabel="Back to MCP servers"
      isLoading={isLoading}
      titleTail={
        proxy?.version ? (
          <Chip
            label={normalizeVersion(proxy.version)}
            size="small"
            variant="outlined"
            sx={{ ml: 1 }}
          />
        ) : undefined
      }
      description={
        proxy?.description ? (
          <Typography variant="body2" color="text.secondary">
            {proxy.description}
          </Typography>
        ) : undefined
      }
      meta={<CreatedMetadata createdAt={proxy?.createdAt} />}
      actions={
        proxy && canWrite ? (
          <Button
            variant="outlined"
            size="small"
            startIcon={<Edit size={16} />}
            onClick={() =>
              setEditDraft({
                name: proxy.name,
                version: proxy.version,
                context: proxy.context,
                description: proxy.description ?? "",
              })
            }
          >
            Edit MCP server
          </Button>
        ) : undefined
      }
    >
      {isLoading && (
        <Stack spacing={3}>
          <Skeleton variant="rounded" height={56} />
          <Skeleton variant="rounded" height={360} />
        </Stack>
      )}

      {error ? (
        <Alert severity="error" icon={<AlertTriangle size={18} />}>
          {error instanceof Error
            ? error.message
            : "Failed to load MCP Server. Please try again."}
        </Alert>
      ) : null}

      {proxy && !error && (
        <Stack spacing={4}>
          {(bindings.length > 0 || (isURLProxy && canWrite)) && (
            <Stack
              direction="row"
              spacing={1}
              alignItems="center"
              justifyContent="flex-end"
              flexWrap="wrap"
              useFlexGap
            >
              {isURLProxy && canWrite && unboundEnvironments.length > 0 && (
                <Button
                  size="small"
                  variant="outlined"
                  startIcon={<Plus size={16} />}
                  onClick={() => setAddEnvOpen(true)}
                >
                  Add environment
                </Button>
              )}
              {isURLProxy && canWrite && selected && (
                <Button
                  size="small"
                  color="error"
                  startIcon={<Trash size={16} />}
                  onClick={confirmRemoveEnvironment}
                >
                  Remove from environment
                </Button>
              )}
              {selected && <BindingStatusChip env={selected} />}
              {bindings.length > 0 && (
                <FormControl size="small" sx={{ minWidth: 260 }}>
                  <Select
                    value={selected?.environmentName ?? ""}
                    onChange={(event) =>
                      setParam("env", event.target.value as string)
                    }
                    renderValue={(value) => {
                      const env = bindings.find(
                        (item) => item.environmentName === value,
                      );
                      return `${env ? envLabel(env.environmentUuid, env.environmentName) : value} Environment`;
                    }}
                  >
                    {bindings.map((env) => (
                      <MenuItem
                        key={env.environmentName}
                        value={env.environmentName}
                      >
                        {envLabel(env.environmentUuid, env.environmentName)}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              )}
            </Stack>
          )}

          <Grid container spacing={2}>
            <Grid size={{ xs: 12, sm: proxy.source ? 4 : 6 }}>
              <InfoCard
                label="Context"
                value={proxy.context}
                onCopy={() => handleCopy(proxy.context, "Context")}
              />
            </Grid>
            <Grid size={{ xs: 12, sm: proxy.source ? 4 : 6 }}>
              <InfoCard
                label="MCP Spec Versions"
                value={proxy.mcpSpecVersion}
              />
            </Grid>
            {proxy.source && (
              <Grid size={{ xs: 12, sm: 4 }}>
                <InfoCard
                  label="Source"
                  value={`${proxy.source.componentName} / ${proxy.source.endpointName}`}
                />
              </Grid>
            )}
          </Grid>

          {selected && selectedConfig ? (
            <Card variant="outlined">
              <Stack
                direction="row"
                alignItems="center"
                justifyContent="space-between"
                sx={{ pr: 2 }}
              >
                <Tabs
                  value={tabIndex}
                  onChange={(_: SyntheticEvent, index: number) =>
                    setParam("tab", TAB_DEFS[index]?.slug)
                  }
                >
                  {TAB_DEFS.map((tab) => (
                    <Tab key={tab.slug} label={tab.label} />
                  ))}
                </Tabs>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ whiteSpace: "nowrap" }}
                >
                  Showing{" "}
                  <Typography
                    component="span"
                    variant="caption"
                    sx={{ fontWeight: 600 }}
                  >
                    {envLabel(
                      selected.environmentUuid,
                      selected.environmentName,
                    )}
                  </Typography>
                </Typography>
              </Stack>
              <Divider />
              {/* Keyed by environment, so unsaved edits never carry over to another one. */}
              <Box sx={{ p: 3 }} key={selected.environmentName}>
                {/* Agent Manager lists the tools through the gateway after each deploy. Until it
                    has, or once it gave up, an empty tool list is not the answer yet. */}
                {(activeTabSlug === "overview" ||
                  activeTabSlug === "manage-tools") &&
                  selected.capabilityDiscovery?.status === "Pending" && (
                    <Alert severity="info" sx={{ mb: 3 }}>
                      Discovering this MCP server&apos;s tools through its
                      gateway. They appear here once found.
                    </Alert>
                  )}
                {(activeTabSlug === "overview" ||
                  activeTabSlug === "manage-tools") &&
                  selected.capabilityDiscovery?.status === "Failed" && (
                    <Alert
                      severity="warning"
                      sx={{ mb: 3 }}
                      action={
                        canWrite ? (
                          <Button
                            color="inherit"
                            size="small"
                            disabled={refreshTools.isPending}
                            onClick={() =>
                              refreshTools.mutate({
                                orgName: orgId,
                                projName: projectId,
                                proxyId,
                                envName: selected.environmentName,
                              })
                            }
                          >
                            Retry
                          </Button>
                        ) : undefined
                      }
                    >
                      {selected.capabilityDiscovery.lastError
                        ? `Tool discovery failed: ${selected.capabilityDiscovery.lastError}`
                        : "Tool discovery failed."}
                    </Alert>
                  )}
                {activeTabSlug === "overview" && (
                  <MCPProxyOverviewTab
                    proxy={proxyAsOrgProxy(proxy)}
                    config={selectedConfig}
                    envChips={selectedEnvChips}
                  />
                )}
                {activeTabSlug === "connection" && (
                  <MCPProxyConnectionTab
                    config={selectedConfig}
                    selectedEndpointId={selected.environmentName}
                    onUpdate={updateSelectedConfig}
                    isUpdating={isUpdating}
                    hideResilience
                    readOnlyURLText={
                      proxy.source
                        ? `Resolved from ${proxy.source.componentName} / ${proxy.source.endpointName} in this environment`
                        : undefined
                    }
                  />
                )}
                {activeTabSlug === "manage-tools" && (
                  <MCPProxyManageToolsTab
                    config={selectedConfig}
                    selectedEndpointId={selected.environmentName}
                    orgName={orgId}
                    onUpdate={updateSelectedConfig}
                    isUpdating={isUpdating}
                  />
                )}
                {activeTabSlug === "security" && (
                  <BindingSecurityTab
                    env={selected}
                    onSave={saveBinding}
                    isSaving={isUpdating}
                  />
                )}
                {activeTabSlug === "api-keys" && (
                  <BindingAPIKeysTab
                    env={selected}
                    orgName={orgId}
                    projName={projectId}
                    proxyId={proxyId}
                    onGoToSecurity={() => setParam("tab", "security")}
                  />
                )}
                {activeTabSlug === "rewrite" && (
                  <MCPProxyRewriteTab
                    config={selectedConfig}
                    selectedEndpointId={selected.environmentName}
                    orgName={orgId}
                    onUpdate={updateSelectedConfig}
                    isUpdating={isUpdating}
                  />
                )}
                {activeTabSlug === "policies" && (
                  <MCPProxyPoliciesTab
                    config={selectedConfig}
                    selectedEndpointId={selected.environmentName}
                    orgName={orgId}
                    onUpdate={updateSelectedConfig}
                    isUpdating={isUpdating}
                  />
                )}
              </Box>
            </Card>
          ) : (
            <Card variant="outlined" sx={{ p: 3 }}>
              <Alert severity="info">
                {isURLProxy
                  ? 'This MCP Server is not deployed to any environment yet. Use "Add environment" above to deploy it.'
                  : "This MCP Server is not deployed to any environment yet. It appears here once its component is published to one."}
              </Alert>
            </Card>
          )}
        </Stack>
      )}

      <Dialog
        open={addEnvOpen}
        onClose={() => setAddEnvOpen(false)}
        fullWidth
        maxWidth="xs"
      >
        <DialogTitle>Add environment</DialogTitle>
        <DialogContent>
          <FormControl fullWidth size="small" sx={{ mt: 1 }}>
            <Select
              value={envToAdd}
              displayEmpty
              onChange={(event) => setEnvToAdd(event.target.value as string)}
            >
              <MenuItem value="" disabled>
                Choose an environment
              </MenuItem>
              {unboundEnvironments.map((env) => (
                <MenuItem key={env.name} value={env.name}>
                  {environmentLabel(env) || env.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ mt: 1, display: "block" }}
          >
            {selected
              ? "The server is deployed there with the selected environment's credential and policies."
              : "The server is deployed there secured with an API key."}
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setAddEnvOpen(false)}>Cancel</Button>
          <Button
            variant="contained"
            disabled={!envToAdd || putBinding.isPending}
            onClick={() => void addEnvironment()}
          >
            Add
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={!!editDraft}
        onClose={() => setEditDraft(undefined)}
        fullWidth
      >
        <DialogTitle>Edit MCP Server</DialogTitle>
        <DialogContent>
          {editDraft && (
            <Stack spacing={2} sx={{ mt: 1 }}>
              {proxy?.source && (
                <Alert severity="info">
                  This MCP Server is published from the{" "}
                  {proxy.source.componentName} component. Its publisher may set
                  these fields again when it next publishes it.
                </Alert>
              )}
              <TextField
                label="Name"
                size="small"
                value={editDraft.name}
                onChange={(event) =>
                  setEditDraft({ ...editDraft, name: event.target.value })
                }
              />
              <TextField
                label="Version"
                size="small"
                value={editDraft.version}
                onChange={(event) =>
                  setEditDraft({ ...editDraft, version: event.target.value })
                }
              />
              <TextField
                label="Context"
                size="small"
                value={editDraft.context}
                helperText={`Served under the project, at /${projectId}${editDraft.context}.`}
                onChange={(event) =>
                  setEditDraft({ ...editDraft, context: event.target.value })
                }
              />
              <TextField
                label="Description (optional)"
                size="small"
                multiline
                minRows={3}
                value={editDraft.description}
                onChange={(event) =>
                  setEditDraft({
                    ...editDraft,
                    description: event.target.value,
                  })
                }
              />
            </Stack>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditDraft(undefined)}>Cancel</Button>
          <Button
            variant="contained"
            disabled={
              !editDraft?.name.trim() ||
              !editDraft?.context.trim().startsWith("/") ||
              updateProxy.isPending
            }
            onClick={() => void saveEdit()}
          >
            Save
          </Button>
        </DialogActions>
      </Dialog>
    </PageLayout>
  );
}

export default ViewProjectMCPProxy;
