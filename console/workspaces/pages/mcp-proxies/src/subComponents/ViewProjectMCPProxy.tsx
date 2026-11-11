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

import { type SyntheticEvent, useCallback, useMemo } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
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
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, Copy, Edit } from "@wso2/oxygen-ui-icons-react";
import {
  generatePath,
  useLocation,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";
import {
  useGetProjectMCPProxy,
  useListEnvironments,
  usePutProjectMCPProxyEnvironment,
  useRefreshProjectMCPProxyCapabilities,
} from "@agent-management-platform/api-client";
import {
  environmentLabel,
  normalizeVersion,
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
  checkBindingRequest,
  proxyAsOrgProxy,
  type ProjectProxyViewState,
  readsAPIKeyFromQuery,
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
  const navigate = useNavigate();
  const location = useLocation();
  // Environments the register form could not deploy the new server to.
  const bindingFailures =
    (location.state as ProjectProxyViewState | null)?.bindingFailures ?? [];
  // Dismissing drops the state, so the notice does not come back on a reload.
  const dismissBindingFailures = () =>
    navigate(
      { pathname: location.pathname, search: location.search },
      { replace: true, state: null },
    );
  const {
    data: proxy,
    isLoading,
    error,
  } = useGetProjectMCPProxy({ orgName: orgId, projName: projectId, proxyId });
  const { data: environments = [] } = useListEnvironments({ orgName: orgId });
  const putBinding = usePutProjectMCPProxyEnvironment();
  const refreshTools = useRefreshProjectMCPProxyCapabilities();
  const { canWrite } = useResourceAccess("mcp_server");
  const handleCopy = useCopyWithFeedback();
  // A server registered by URL is managed here; one from a component is managed by its publisher.
  const isURLProxy = !!proxy && !proxy.source;

  const tabSlug = searchParams.get("tab");
  const tabIndex = Math.max(
    0,
    TAB_DEFS.findIndex((tab) => tab.slug === tabSlug),
  );
  const activeTabSlug = TAB_DEFS[tabIndex]?.slug;
  // The tabs that list the binding's tools, where an unfinished discovery is shown.
  const showsTools =
    activeTabSlug === "overview" || activeTabSlug === "manage-tools";

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


  const saveBinding = useCallback(
    async (request: ProjectMCPProxyEnvironmentRequest) => {
      // A binding still reading its API key from the query is refused by the service; say
      // why, and where to fix it, rather than send it.
      checkBindingRequest(request, selected?.environmentName ?? "");
      return putBinding.mutateAsync({
        params: {
          orgName: orgId,
          projName: projectId,
          proxyId,
          envName: selected?.environmentName,
        },
        body: request,
      });
    },
    [putBinding, orgId, projectId, proxyId, selected?.environmentName],
  );

  // The selected binding as the org tabs read an endpoint, and their edits written back to it.
  // Memoized: the tabs reset their form when the config changes, so a rerender for save state
  // alone must hand them the same one.
  const selectedConfig: MCPEndpointConfig | undefined = useMemo(
    () => (proxy && selected ? bindingEndpointConfig(proxy, selected) : undefined),
    [proxy, selected],
  );
  const updateSelectedConfig = useCallback(
    async (fields: Partial<MCPEndpointConfig>) => {
      if (!selected) return undefined;
      // The URL is read-only here, so an edit changes only this environment's binding.
      return saveBinding(applyBindingConfigChange(selected, fields));
    },
    [selected, saveBinding],
  );
  const isUpdating = putBinding.isPending;

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
              navigate(
                generatePath(
                  absoluteRouteMap.children.org.children.projects.children
                    .mcpProxies.children.view.children.edit.path,
                  { orgId: orgId ?? "", projectId: projectId ?? "", proxyId },
                ),
              )
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
          {bindingFailures.length > 0 && (
            <Alert severity="warning" onClose={dismissBindingFailures}>
              The MCP server was registered, but it could not be deployed to{" "}
              {bindingFailures.length === 1
                ? "this environment"
                : "these environments"}
              . Use &quot;Edit MCP server&quot; to deploy it to them again.
              <Box component="ul" sx={{ m: 0, mt: 1, pl: 2.5 }}>
                {bindingFailures.map((failure) => (
                  <li key={failure.environment}>
                    <strong>{failure.environment}</strong>: {failure.message}
                  </li>
                ))}
              </Box>
            </Alert>
          )}
          {bindings.length > 0 && (
            <Stack
              direction="row"
              spacing={1}
              alignItems="center"
              justifyContent="flex-end"
              flexWrap="wrap"
              useFlexGap
            >
              {selected && <BindingStatusChip env={selected} />}
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
                {readsAPIKeyFromQuery(selected.security) && (
                  <Alert
                    severity="warning"
                    sx={{ mb: 3 }}
                    action={
                      activeTabSlug !== "security" ? (
                        <Button
                          color="inherit"
                          size="small"
                          onClick={() => setParam("tab", "security")}
                        >
                          Go to Security
                        </Button>
                      ) : undefined
                    }
                  >
                    This environment reads its API key from the query, which the gateway does
                    not support. Change it to a header on the Security tab; until then, other
                    changes to this environment cannot be saved.
                  </Alert>
                )}
                {/* Agent Manager lists the tools through the gateway after each deploy. Until it
                    has, or once it gave up, an empty tool list is not the answer yet. */}
                {showsTools &&
                  selected.capabilityDiscovery?.status === "Pending" && (
                    <Alert severity="info" sx={{ mb: 3 }}>
                      Discovering this MCP server&apos;s tools through its
                      gateway. They appear here once found.
                    </Alert>
                  )}
                {showsTools &&
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
                    // The tab is one environment's, but the URL is every environment's: it is
                    // shown here and changed from the edit page.
                    readOnlyURLText={
                      proxy.source
                        ? `Resolved from ${proxy.source.componentName} / ${proxy.source.endpointName} in this environment`
                        : (proxy.upstream?.url ?? "")
                    }
                    readOnlyURLHelperText={
                      proxy.source
                        ? undefined
                        : 'Shared by every environment. Change it with "Edit MCP server".'
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
                  ? 'This MCP Server is not deployed to any environment yet. Use "Edit MCP server" to assign its endpoint to an environment.'
                  : "This MCP Server is not deployed to any environment yet. It appears here once its component is published to one."}
              </Alert>
            </Card>
          )}
        </Stack>
      )}
    </PageLayout>
  );
}

export default ViewProjectMCPProxy;
