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

import { type ReactNode, useCallback, useMemo, useState } from "react";
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
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, Edit, Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useParams } from "react-router-dom";
import {
  useDeleteProjectMCPProxyEnvironment,
  useGetProjectMCPProxy,
  useListEnvironments,
  usePutProjectMCPProxyEnvironment,
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
import { MCPProxyPoliciesTab } from "./MCPProxyPoliciesTab";
import {
  BindingAPIKeysTab,
  BindingOverviewTab,
  BindingSecurityTab,
  BindingStatusChip,
  BindingUpstreamAuthTab,
} from "./ProjectBindingTabs";
import { bindingRequestFrom, proxyRequestFrom } from "./projectBinding";

const TABS = [
  { slug: "overview", label: "Overview" },
  { slug: "security", label: "Security" },
  { slug: "api-keys", label: "API Keys" },
  { slug: "upstream", label: "Upstream Auth" },
  { slug: "policies", label: "Policies" },
] as const;
type TabSlug = (typeof TABS)[number]["slug"];

function InfoCard({
  label,
  value,
  action,
}: {
  label: string;
  value?: string;
  action?: ReactNode;
}) {
  return (
    <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
      <Stack spacing={0.5}>
        <Stack direction="row" alignItems="center" justifyContent="space-between">
          <Typography variant="caption" color="text.secondary" fontWeight={500}>
            {label}
          </Typography>
          {action}
        </Stack>
        <Typography
          variant="body2"
          sx={{ fontFamily: "monospace", wordBreak: "break-all" }}
        >
          {value || "—"}
        </Typography>
      </Stack>
    </Card>
  );
}

/**
 * A project MCP server: one binding per environment it is published to. Each
 * binding is configured on its own (security, API keys, upstream credential,
 * policies) and every save replaces that binding's configuration.
 */
export function ViewProjectMCPProxy() {
  const { orgId, projectId, proxyId = "" } = useParams<{
    orgId: string;
    projectId: string;
    proxyId: string;
  }>();
  const {
    data: proxy,
    isLoading,
    error,
  } = useGetProjectMCPProxy({ orgName: orgId, projName: projectId, proxyId });
  const { data: environments = [] } = useListEnvironments({ orgName: orgId });
  const putBinding = usePutProjectMCPProxyEnvironment();
  const removeBinding = useDeleteProjectMCPProxyEnvironment();
  const updateProxy = useUpdateProjectMCPProxy();
  const { addConfirmation } = useConfirmationDialog();
  const { canWrite } = useResourceAccess("mcp_server");
  const [urlDraft, setUrlDraft] = useState<string>();
  const [addEnvOpen, setAddEnvOpen] = useState(false);
  const [envToAdd, setEnvToAdd] = useState("");
  // A server registered by URL is managed here; one from a component is managed by its publisher.
  const isURLProxy = !!proxy && !proxy.source;

  const bindings = useMemo(() => proxy?.environments ?? [], [proxy]);
  const [selectedEnvName, setSelectedEnvName] = useState<string>();
  const [tab, setTab] = useState<TabSlug>("overview");
  const selected =
    bindings.find((env) => env.environmentName === selectedEnvName) ??
    bindings[0];

  const envLabel = useCallback(
    (uuid: string, name: string) =>
      environmentLabel(environments.find((env) => env.id === uuid)) || name,
    [environments],
  );

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

  // The policies tab edits an endpoint config; a binding's policies are saved through its PUT.
  const policiesConfig: MCPEndpointConfig | undefined = selected
    ? { policies: selected.policies ?? [] }
    : undefined;
  const savePolicies = useCallback(
    (fields: Partial<MCPEndpointConfig>) =>
      selected
        ? saveBinding({
            ...bindingRequestFrom(selected),
            policies: fields.policies ?? [],
          })
        : Promise.resolve(undefined),
    [saveBinding, selected],
  );

  const proxyParams = { orgName: orgId, projName: projectId, proxyId };
  const unboundEnvironments = environments.filter(
    (env) => !bindings.some((binding) => binding.environmentName === env.name),
  );

  const saveURL = async () => {
    if (!proxy || !urlDraft?.trim()) return;
    await updateProxy.mutateAsync({
      params: proxyParams,
      body: { ...proxyRequestFrom(proxy), upstream: { url: urlDraft.trim() } },
    });
    setUrlDraft(undefined);
  };

  const addEnvironment = async () => {
    if (!envToAdd) return;
    // An empty body takes the defaults: the binding is secured with an API key.
    await putBinding.mutateAsync({ params: { ...proxyParams, envName: envToAdd }, body: {} });
    setSelectedEnvName(envToAdd);
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
        removeBinding.mutate({ ...proxyParams, envName: selected.environmentName });
        setSelectedEnvName(undefined);
      },
    });
  };

  const addEnvironmentButton =
    isURLProxy && canWrite && unboundEnvironments.length > 0 ? (
      <Button
        size="small"
        variant="outlined"
        startIcon={<Plus size={16} />}
        onClick={() => setAddEnvOpen(true)}
      >
        Add environment
      </Button>
    ) : null;

  const backHref = generatePath(
    absoluteRouteMap.children.org.children.projects.children.mcpProxies.path,
    { orgId: orgId ?? "", projectId: projectId ?? "" },
  );

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
            : "Failed to load the MCP server. Please try again."}
        </Alert>
      ) : null}

      {proxy && !error && (
        <Stack spacing={4}>
          {proxy.source ? (
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 4 }}>
                <InfoCard label="Component" value={proxy.source.componentName} />
              </Grid>
              <Grid size={{ xs: 12, sm: 4 }}>
                <InfoCard label="Endpoint" value={proxy.source.endpointName} />
              </Grid>
              <Grid size={{ xs: 12, sm: 4 }}>
                <InfoCard label="Context" value={proxy.context} />
              </Grid>
            </Grid>
          ) : (
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 8 }}>
                <InfoCard
                  label="MCP server URL"
                  value={proxy.upstream?.url}
                  action={
                    canWrite ? (
                      <Button
                        size="small"
                        startIcon={<Edit size={14} />}
                        onClick={() => setUrlDraft(proxy.upstream?.url ?? "")}
                      >
                        Edit
                      </Button>
                    ) : undefined
                  }
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 4 }}>
                <InfoCard label="Context" value={proxy.context} />
              </Grid>
            </Grid>
          )}

          {selected ? (
            <Card variant="outlined">
              <Stack
                direction="row"
                alignItems="center"
                justifyContent="space-between"
                spacing={2}
                sx={{ pr: 2 }}
              >
                <Tabs value={tab} onChange={(_, value: TabSlug) => setTab(value)}>
                  {TABS.map((item) => (
                    <Tab key={item.slug} value={item.slug} label={item.label} />
                  ))}
                </Tabs>
                <Stack direction="row" alignItems="center" spacing={1}>
                  {addEnvironmentButton}
                  {isURLProxy && canWrite && (
                    <Button
                      size="small"
                      color="error"
                      startIcon={<Trash size={16} />}
                      onClick={confirmRemoveEnvironment}
                    >
                      Remove
                    </Button>
                  )}
                  <BindingStatusChip env={selected} />
                  <FormControl size="small" sx={{ minWidth: 180 }}>
                    <Select
                      value={selected.environmentName}
                      onChange={(event) =>
                        setSelectedEnvName(event.target.value as string)
                      }
                    >
                      {bindings.map((env) => (
                        <MenuItem key={env.environmentName} value={env.environmentName}>
                          {envLabel(env.environmentUuid, env.environmentName)}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                </Stack>
              </Stack>
              <Divider />
              {/* Keyed by environment, so unsaved edits never carry over to another one. */}
              <Box sx={{ p: 3 }} key={selected.environmentName}>
                {tab === "overview" && <BindingOverviewTab env={selected} />}
                {tab === "security" && (
                  <BindingSecurityTab
                    env={selected}
                    onSave={saveBinding}
                    isSaving={putBinding.isPending}
                  />
                )}
                {tab === "api-keys" && (
                  <BindingAPIKeysTab
                    env={selected}
                    orgName={orgId}
                    projName={projectId}
                    proxyId={proxyId}
                    onGoToSecurity={() => setTab("security")}
                  />
                )}
                {tab === "upstream" && (
                  <BindingUpstreamAuthTab
                    env={selected}
                    onSave={saveBinding}
                    isSaving={putBinding.isPending}
                  />
                )}
                {tab === "policies" && (
                  <MCPProxyPoliciesTab
                    config={policiesConfig}
                    selectedEndpointId={selected.environmentName}
                    orgName={orgId}
                    onUpdate={savePolicies}
                    isUpdating={putBinding.isPending}
                  />
                )}
              </Box>
            </Card>
          ) : (
            <Card variant="outlined" sx={{ p: 3 }}>
              <Stack spacing={2} alignItems="flex-start">
                <Alert severity="info" sx={{ alignSelf: "stretch" }}>
                  {isURLProxy
                    ? "This MCP server is not deployed to any environment yet. Add one to let the project's agents call it there."
                    : "This MCP server is not deployed to any environment yet. It appears here once its component is published to one."}
                </Alert>
                {addEnvironmentButton}
              </Stack>
            </Card>
          )}
        </Stack>
      )}

      <Dialog open={urlDraft !== undefined} onClose={() => setUrlDraft(undefined)} fullWidth>
        <DialogTitle>Edit MCP server URL</DialogTitle>
        <DialogContent>
          <TextField
            sx={{ mt: 1 }}
            fullWidth
            size="small"
            label="MCP server URL"
            value={urlDraft ?? ""}
            helperText="Every environment the server is deployed to is redeployed with the new URL."
            onChange={(event) => setUrlDraft(event.target.value)}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setUrlDraft(undefined)}>Cancel</Button>
          <Button
            variant="contained"
            disabled={
              !urlDraft?.trim() ||
              urlDraft.trim() === proxy?.upstream?.url ||
              updateProxy.isPending
            }
            onClick={() => void saveURL()}
          >
            Save
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={addEnvOpen} onClose={() => setAddEnvOpen(false)} fullWidth maxWidth="xs">
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
          <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: "block" }}>
            The server is deployed there secured with an API key.
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
    </PageLayout>
  );
}

export default ViewProjectMCPProxy;
