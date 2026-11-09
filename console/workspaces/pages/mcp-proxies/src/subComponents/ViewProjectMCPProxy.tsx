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

import { useCallback, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Card,
  Chip,
  Divider,
  FormControl,
  Grid,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useParams } from "react-router-dom";
import {
  useGetProjectMCPProxy,
  useListEnvironments,
  usePutProjectMCPProxyEnvironment,
} from "@agent-management-platform/api-client";
import {
  environmentLabel,
  normalizeVersion,
} from "@agent-management-platform/shared-component";
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
import { bindingRequestFrom } from "./projectBinding";

const TABS = [
  { slug: "overview", label: "Overview" },
  { slug: "security", label: "Security" },
  { slug: "api-keys", label: "API Keys" },
  { slug: "upstream", label: "Upstream Auth" },
  { slug: "policies", label: "Policies" },
] as const;
type TabSlug = (typeof TABS)[number]["slug"];

function InfoCard({ label, value }: { label: string; value?: string }) {
  return (
    <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
      <Stack spacing={0.5}>
        <Typography variant="caption" color="text.secondary" fontWeight={500}>
          {label}
        </Typography>
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
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, sm: 4 }}>
              <InfoCard label="Component" value={proxy.source?.componentName} />
            </Grid>
            <Grid size={{ xs: 12, sm: 4 }}>
              <InfoCard label="Endpoint" value={proxy.source?.endpointName} />
            </Grid>
            <Grid size={{ xs: 12, sm: 4 }}>
              <InfoCard label="Context" value={proxy.context} />
            </Grid>
          </Grid>

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
              <Alert severity="info">
                This MCP server is not deployed to any environment yet. It
                appears here once its component is published to one.
              </Alert>
            </Card>
          )}
        </Stack>
      )}
    </PageLayout>
  );
}

export default ViewProjectMCPProxy;
