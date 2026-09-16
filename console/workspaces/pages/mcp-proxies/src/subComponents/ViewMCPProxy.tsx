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

import {
  type SyntheticEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  useGetMCPProxy,
  useListEnvironments,
  useUpdateMCPProxy,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type Environment,
  type MCPEndpointConfig,
  type MCPProxyEndpoint,
} from "@agent-management-platform/types";
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
import { generatePath, useParams, useSearchParams } from "react-router-dom";
import { normalizeVersion } from "@agent-management-platform/shared-component";
import {
  CreatedMetadata,
  PageLayout,
} from "@agent-management-platform/views";
import { MCPProxyManageToolsTab } from "./MCPProxyManageToolsTab";
import { MCPProxyConnectionTab } from "./MCPProxyConnectionTab";
import { MCPProxyOverviewTab } from "./MCPProxyOverviewTab";
import { MCPProxyPoliciesTab } from "./MCPProxyPoliciesTab";
import { MCPProxyRewriteTab } from "./MCPProxyRewriteTab";
import { MCPProxySecurityTab } from "./MCPProxySecurityTab";
import { EditMCPProxyDrawer } from "./EditMCPProxyDrawer";
import { useCopyWithFeedback } from "./useCopyWithFeedback";

// slug is a URL-safe stand-in for each tab, so the selected tab (and
// environment, below) are shareable/deep-linkable and survive a page reload
// instead of resetting to Overview/first-environment.
const TAB_DEFS = [
  { label: "Overview", slug: "overview" },
  { label: "Connection", slug: "connection" },
  { label: "Manage Tools", slug: "manage-tools" },
  { label: "Security", slug: "security" },
  { label: "Rewrite", slug: "rewrite" },
  { label: "Policies", slug: "policies" },
] as const;

export function ViewMCPProxy() {
  const { orgId, proxyId } = useParams<{ orgId: string; proxyId: string }>();
  const routeProxyId = proxyId ?? "";
  const [searchParams, setSearchParams] = useSearchParams();

  const tabSlug = searchParams.get("tab");
  const tabIndex = tabSlug
    ? Math.max(0, TAB_DEFS.findIndex((tab) => tab.slug === tabSlug))
    : 0;
  // Render blocks below key off the slug, not the raw index, so reordering or
  // removing a tab in TAB_DEFS doesn't require manually renumbering every block.
  const activeTabSlug = TAB_DEFS[tabIndex]?.slug;
  const selectedEndpointId = searchParams.get("endpoint") ?? "";

  const setSelectedEndpointId = useCallback(
    (endpointId: string) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.set("endpoint", endpointId);
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const handleTabChange = useCallback(
    (_event: SyntheticEvent, value: number) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.set("tab", TAB_DEFS[value].slug);
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const [editDrawerOpen, setEditDrawerOpen] = useState(false);
  const handleCopy = useCopyWithFeedback();

  const {
    data: proxy,
    isLoading,
    error,
  } = useGetMCPProxy({
    orgName: orgId,
    proxyId: routeProxyId,
  });
  const { data: environments = [] } = useListEnvironments({
    orgName: orgId ?? "",
  });
  const updateMCPProxy = useUpdateMCPProxy();

  const endpoints = useMemo<MCPProxyEndpoint[]>(
    () => proxy?.endpoints ?? [],
    [proxy?.endpoints],
  );

  // Options for the endpoint dropdown, labelled with the endpoint name (falling back to
  // its handle).
  const endpointOptions = useMemo(() => {
    return endpoints.map((endpoint) => ({
      id: endpoint.id,
      label: endpoint.name || endpoint.id,
    }));
  }, [endpoints]);

  // Keep the selected endpoint valid: default to the first endpoint and reset when the
  // current selection is no longer present.
  useEffect(() => {
    if (endpoints.length === 0) {
      return;
    }
    if (!endpoints.some((endpoint) => endpoint.id === selectedEndpointId)) {
      setSelectedEndpointId(endpoints[0].id);
    }
  }, [endpoints, selectedEndpointId, setSelectedEndpointId]);

  const selectedEndpoint = useMemo<MCPProxyEndpoint | undefined>(
    () => endpoints.find((endpoint) => endpoint.id === selectedEndpointId),
    [endpoints, selectedEndpointId],
  );

  // The selected endpoint's flat config, consumed by every config tab. Environment
  // bindings and per-env deployment status are surfaced separately as chips.
  const selectedConfig: MCPEndpointConfig | undefined = selectedEndpoint;

  // Joins the selected endpoint's environment bindings against the org's full
  // environment list once — chips and the resolved Environment objects below
  // both derive from this instead of each re-doing the same lookup.
  const selectedEnvironmentBindings = useMemo(() => {
    return (selectedEndpoint?.environments ?? []).map((binding) => ({
      binding,
      env: environments.find((item) => item.id === binding.environmentUuid),
    }));
  }, [selectedEndpoint, environments]);

  // Chips describing each environment the selected endpoint is bound to, with its
  // per-environment deployment status.
  const selectedEnvChips = useMemo(
    () =>
      selectedEnvironmentBindings.map(({ binding, env }) => ({
        id: binding.environmentUuid,
        label: env?.displayName ?? env?.name ?? binding.environmentUuid,
        status: binding.deploymentStatus,
      })),
    [selectedEnvironmentBindings],
  );

  // The full Environment objects (name/displayName) the selected endpoint is
  // bound to — used by the Security tab's Create Scope panel to offer roles
  // from every environment this MCP Server is actually reachable from.
  const selectedEnvironments = useMemo(
    () =>
      selectedEnvironmentBindings
        .filter(({ binding }) => binding.deploymentStatus === "Deployed")
        .map(({ env }) => env)
        .filter((env): env is Environment => !!env),
    [selectedEnvironmentBindings],
  );

  // Merge-and-save callback used by every config tab. It merges a partial into the
  // selected endpoint's flat config and PUTs the whole proxy with that one endpoint
  // replaced.
  const updateSelectedEndpointConfig = useCallback(
    async (fields: Partial<MCPEndpointConfig>) => {
      if (!orgId || !proxy?.id || !selectedEndpointId) {
        throw new Error("MCP proxy or endpoint is not loaded.");
      }
      const nextEndpoints = (proxy.endpoints ?? []).map((endpoint) =>
        endpoint.id === selectedEndpointId
          ? { ...endpoint, ...fields }
          : endpoint,
      );
      return updateMCPProxy.mutateAsync({
        params: { orgName: orgId, proxyId: proxy.id },
        body: { ...proxy, endpoints: nextEndpoints },
      });
    },
    [orgId, proxy, selectedEndpointId, updateMCPProxy],
  );

  const displayName = proxy?.name ?? proxy?.id ?? proxyId ?? "MCP Server";
  const hasEndpoints = endpoints.length > 0;
  // The proxy fetch's own isLoading flips to false as soon as `proxy` arrives,
  // but selecting the first endpoint (when the URL doesn't already name one)
  // happens in a follow-up effect — so for a render or two, endpoints exist
  // but selectedEndpoint/selectedConfig is still undefined. Tabs that treat
  // that as "loaded, and there's nothing here" (e.g. Manage Tools) flash an
  // empty list before the real config shows up; folding this into the
  // isLoading passed to every tab keeps them on their loading skeleton until
  // the endpoint actually resolves.
  const isResolvingEndpoint = hasEndpoints && !selectedEndpoint;
  const isTabContentLoading = isLoading || isResolvingEndpoint;
  const backHref = generatePath(
    absoluteRouteMap.children.org.children.mcpProxies.path,
    { orgId: orgId ?? "" },
  );

  return (
    <>
      <PageLayout docs={["mcpProxy", "registerMcpProxy", "authorizeAgentMcpTools"]}
        variant="card"
        title={displayName}
        backHref={backHref}
        backLabel="Back to MCP Servers"
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
        meta={
          <CreatedMetadata createdAt={proxy?.createdAt} />
        }
        actions={
          proxy ? (
            <Button
              variant="outlined"
              size="small"
              startIcon={<Edit size={16} />}
              onClick={() => setEditDrawerOpen(true)}
            >
              Edit MCP Server
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
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 6 }}>
                <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                  <Stack spacing={0.5}>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ fontWeight: 500 }}
                    >
                      Context
                    </Typography>
                    <Stack direction="row" alignItems="center" spacing={0.5}>
                      <Typography
                        variant="body2"
                        sx={{
                          fontFamily: "monospace",
                          wordBreak: "break-all",
                        }}
                      >
                        {proxy.context || "—"}
                      </Typography>
                      {proxy.context && (
                        <Tooltip title="Copy Context">
                          <IconButton
                            size="small"
                            aria-label="Copy Context"
                            onClick={() =>
                              handleCopy(proxy.context as string, "Context")
                            }
                          >
                            <Copy size={14} />
                          </IconButton>
                        </Tooltip>
                      )}
                    </Stack>
                  </Stack>
                </Card>
              </Grid>

              <Grid size={{ xs: 12, sm: 6 }}>
                <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                  <Stack spacing={0.5}>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ fontWeight: 500 }}
                    >
                      MCP Spec Version
                    </Typography>
                    <Typography
                      variant="body2"
                      sx={{ fontFamily: "monospace" }}
                    >
                      {proxy.mcpSpecVersion || "—"}
                    </Typography>
                  </Stack>
                </Card>
              </Grid>
            </Grid>

            {endpoints.length > 1 && (
              <Stack
                direction="row"
                spacing={1}
                alignItems="center"
                justifyContent="flex-end"
                flexWrap="wrap"
                useFlexGap
              >
                <FormControl size="small" sx={{ minWidth: 260 }}>
                  <Select
                    value={selectedEndpointId}
                    onChange={(event) =>
                      setSelectedEndpointId(event.target.value as string)
                    }
                    renderValue={(value) => {
                      const option = endpointOptions.find(
                        (o) => o.id === value,
                      );
                      return `${option?.label ?? value} Endpoint`;
                    }}
                  >
                    {endpointOptions.map((option) => (
                      <MenuItem key={option.id} value={option.id}>
                        {option.label}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              </Stack>
            )}

            {hasEndpoints ? (
              <Card variant="outlined">
                <Stack
                  direction="row"
                  alignItems="center"
                  justifyContent="space-between"
                  sx={{ pr: 2 }}
                >
                  <Tabs value={tabIndex} onChange={handleTabChange}>
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
                      {selectedEndpoint?.name || selectedEndpoint?.id}
                    </Typography>
                  </Typography>
                </Stack>
                <Divider />
                <Box sx={{ p: 3 }}>
                  {activeTabSlug === "overview" && (
                    <MCPProxyOverviewTab
                      proxy={proxy}
                      config={selectedConfig}
                      envChips={selectedEnvChips}
                      isLoading={isTabContentLoading}
                    />
                  )}
                  {activeTabSlug === "connection" && (
                    <MCPProxyConnectionTab
                      config={selectedConfig}
                      selectedEndpointId={selectedEndpointId}
                      isLoading={isTabContentLoading}
                      onUpdate={updateSelectedEndpointConfig}
                      isUpdating={updateMCPProxy.isPending}
                    />
                  )}
                  {activeTabSlug === "manage-tools" && (
                    <MCPProxyManageToolsTab
                      config={selectedConfig}
                      selectedEndpointId={selectedEndpointId}
                      orgName={orgId}
                      isLoading={isTabContentLoading}
                      onUpdate={updateSelectedEndpointConfig}
                      isUpdating={updateMCPProxy.isPending}
                    />
                  )}
                  {activeTabSlug === "security" && (
                    <MCPProxySecurityTab
                      config={selectedConfig}
                      selectedEndpointId={selectedEndpointId}
                      orgName={orgId}
                      proxyId={routeProxyId}
                      environments={selectedEnvironments}
                      isLoading={isTabContentLoading}
                      onUpdate={updateSelectedEndpointConfig}
                      isUpdating={updateMCPProxy.isPending}
                    />
                  )}
                  {activeTabSlug === "rewrite" && (
                    <MCPProxyRewriteTab
                      config={selectedConfig}
                      selectedEndpointId={selectedEndpointId}
                      orgName={orgId}
                      isLoading={isTabContentLoading}
                      onUpdate={updateSelectedEndpointConfig}
                      isUpdating={updateMCPProxy.isPending}
                    />
                  )}
                  {activeTabSlug === "policies" && (
                    <MCPProxyPoliciesTab
                      config={selectedConfig}
                      selectedEndpointId={selectedEndpointId}
                      orgName={orgId}
                      onUpdate={updateSelectedEndpointConfig}
                      isUpdating={updateMCPProxy.isPending}
                    />
                  )}
                </Box>
              </Card>
            ) : (
              <Card variant="outlined" sx={{ p: 3 }}>
                <Alert severity="info">
                  This MCP Server has no endpoints configured. Use &quot;Edit MCP
                  Server&quot; above to add one.
                </Alert>
              </Card>
            )}
          </Stack>
        )}
      </PageLayout>

      {proxy && orgId && (
        <EditMCPProxyDrawer
          open={editDrawerOpen}
          onClose={() => setEditDrawerOpen(false)}
          proxy={proxy}
          orgId={orgId}
          environments={environments}
        />
      )}
    </>
  );
}

export default ViewMCPProxy;
