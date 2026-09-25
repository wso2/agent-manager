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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { generatePath, useNavigate } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Form,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  Edit,
  Link,
  Search,
  ServerCog,
} from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  TextInput,
} from "@agent-management-platform/views";
import { absoluteRouteMap, INPUT_LIMITS } from "@agent-management-platform/types";
import {
  useCreateAgentMCPConfig,
  useGetAgent,
  useListAgentMCPConfigs,
  useListMCPProxies,
} from "@agent-management-platform/api-client";
import {
  AGENTID_ENV_VAR_ROWS,
  EnvironmentVariablesReference,
  usePipelineEnvironmentsState,
  useMCPProxySecurity,
} from "@agent-management-platform/shared-component";
import { ConfigNameSection } from "./ConfigNameSection";
import { MCPServerDisplay } from "./MCPServerDisplay";
import {
  generateEnvVarNames,
  generateUniqueConfigName,
  type EnvVarKey,
} from "../../utils/envConfig";

const ENV_VAR_DESCRIPTIONS: Record<EnvVarKey, string> = {
  url: "Base URL of the MCP server endpoint",
  apikey: "API key for authenticating with the MCP server endpoint",
};

export interface AddMCPToolConfigPanelProps {
  /** Controls the right-side drawer's visibility. */
  open: boolean;
  orgId?: string;
  projectId?: string;
  agentId?: string;
  /** Called on Cancel/close and after a successful create. */
  onClose: () => void;
}

/**
 * The "Add Tool Configuration" flow rendered as a single right-side drawer. It is a
 * two-step flow within that one panel: first pick an MCP proxy from the list, then the
 * same drawer shows the selected server plus the configuration name and env var names.
 * Selection is environment-agnostic — on save every deployment-pipeline environment is
 * mapped to the one selected proxy; the backend deploys where the proxy is configured
 * and injects empty env vars everywhere else.
 */
export function AddMCPToolConfigPanel({
  open,
  orgId,
  projectId,
  agentId,
  onClose,
}: AddMCPToolConfigPanelProps) {
  const navigate = useNavigate();

  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const isExternal = agent?.provisioning?.type === "external";

  const { environments, isLoading: isLoadingEnvironments } =
    usePipelineEnvironmentsState(orgId, projectId);
  const { data: proxiesData, isLoading: isLoadingProxies } = useListMCPProxies(
    { orgName: orgId },
    { limit: 50, offset: 0 },
  );
  const { data: existingConfigsList } = useListAgentMCPConfigs(
    { orgName: orgId, projName: projectId, agentName: agentId },
    { limit: 1000, offset: 0 },
  );
  const servers = useMemo(() => proxiesData?.list ?? [], [proxiesData]);

  const [selectedProxyId, setSelectedProxyId] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [configName, setConfigName] = useState("");
  const configNameEditedRef = useRef(false);
  const [envVarNames, setEnvVarNames] = useState<Record<EnvVarKey, string>>(() =>
    generateEnvVarNames(""),
  );
  const envVarNamesEditedRef = useRef(false);

  const createConfig = useCreateAgentMCPConfig();

  // Reset to a clean slate every time the drawer opens.
  useEffect(() => {
    if (!open) return;
    setSelectedProxyId(null);
    setSearch("");
    configNameEditedRef.current = false;
    envVarNamesEditedRef.current = false;
    setConfigName("");
    setEnvVarNames(generateEnvVarNames(""));
  }, [open]);

  const selectedProxy = useMemo(
    () => servers.find((s) => s.id === selectedProxyId) ?? null,
    [servers, selectedProxyId],
  );

  // The list item has no security info, so the hook fetches the full proxy. No
  // environmentUuid is passed on purpose: this proxy maps to every environment at
  // once, so the hook's every-endpoint rule applies and a mixed-security proxy
  // keeps both fields.
  const {
    authenticationType,
    usesIdentitySecurity,
    spec,
    isLoading: isSecurityLoading,
    isResolved: isSecurityResolved,
  } = useMCPProxySecurity({
    orgName: orgId,
    proxyId: selectedProxyId,
  });
  // A failed fetch reports "" (unsecured), which is indistinguishable from a
  // genuinely open endpoint — acting on it would drop an API key variable the
  // agent needs. Hold the section until the answer is real.
  const isSecurityUnknown = !isSecurityLoading && !isSecurityResolved;
  const isSecurityPending = isSecurityLoading || isSecurityUnknown;
  const relevantEnvVarKeys: EnvVarKey[] = spec.editableKeys;

  const filteredServers = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (!query) return servers;
    return servers.filter(
      (s) =>
        (s.name ?? "").toLowerCase().includes(query) ||
        (s.description ?? "").toLowerCase().includes(query) ||
        (s.context ?? "").toLowerCase().includes(query),
    );
  }, [servers, search]);

  // Auto-generate env var names from the selected proxy until the user edits them.
  useEffect(() => {
    if (envVarNamesEditedRef.current) return;
    setEnvVarNames(generateEnvVarNames(selectedProxyId ?? ""));
  }, [selectedProxyId]);

  const suggestedConfigName = useMemo(() => {
    if (!selectedProxyId) return "";
    const basis = selectedProxy?.name ?? selectedProxyId;
    const existingNames = (existingConfigsList?.configs ?? []).map((c) => c.name);
    return generateUniqueConfigName(basis, "mcp", existingNames);
  }, [selectedProxyId, selectedProxy, existingConfigsList]);

  // Auto-populate the config name until the user renames it.
  useEffect(() => {
    if (configNameEditedRef.current) return;
    setConfigName(suggestedConfigName);
  }, [suggestedConfigName]);

  const handleSave = useCallback(() => {
    if (!orgId || !projectId || !agentId || !selectedProxyId) return;

    // Environment-agnostic: map EVERY pipeline environment to the SAME proxy.
    const envMappings: Record<
      string,
      { proxyId?: string; configuration: Record<string, never> }
    > = {};
    for (const env of environments) {
      envMappings[env.name] = { proxyId: selectedProxyId, configuration: {} };
    }

    const environmentVariables = !isExternal
      ? relevantEnvVarKeys
          .map((key) => ({
            key,
            name: (envVarNames[key] ?? "").trim(),
          }))
          .filter((envVar) => envVar.name.length > 0)
      : [];

    const name = configName.trim() || suggestedConfigName;

    createConfig.mutate(
      {
        params: { orgName: orgId, projName: projectId, agentName: agentId },
        body: {
          name,
          type: "mcp" as const,
          envMappings,
          environmentVariables:
            environmentVariables.length > 0 ? environmentVariables : undefined,
        },
      },
      {
        onSuccess: (data) => {
          onClose();
          if (!orgId || !projectId || !agentId || !data?.uuid) return;
          // Collect the one-time API key returned per env mapping so ViewMCPServer
          // can surface it for copying — it is only present in the create response.
          const authInfoByEnv: Record<
            string,
            { type: string; in: string; name: string; value?: string }
          > = {};
          for (const [envName, mapping] of Object.entries(
            data.envMappings ?? {},
          )) {
            if (mapping.configuration?.authInfo) {
              authInfoByEnv[envName] = mapping.configuration.authInfo;
            }
          }
          // Land on the newly created configuration and open the "Connect to MCP
          // Server" panel straight away (ViewMCPServer reads state.openEnvPanel /
          // state.authInfoByEnv).
          navigate(
            generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.configure.children.mcpProxies.children.view.path,
              {
                orgId,
                projectId,
                agentId,
                proxyId: encodeURIComponent(data.uuid),
              },
            ),
            { state: { openEnvPanel: true, authInfoByEnv } },
          );
        },
      },
    );
  }, [
    orgId,
    projectId,
    agentId,
    selectedProxyId,
    environments,
    isExternal,
    relevantEnvVarKeys,
    envVarNames,
    configName,
    suggestedConfigName,
    createConfig,
    onClose,
    navigate,
  ]);

  const canSave =
    Boolean(selectedProxyId) &&
    !createConfig.isPending &&
    !isLoadingEnvironments &&
    environments.length > 0 &&
    // Saving before the proxy's security resolves would write whichever env vars
    // the unknown default implies (issue #1597). External agents get no env vars
    // at all, so they are unaffected.
    (isExternal || !isSecurityPending);

  return (
    <DrawerWrapper open={open} onClose={onClose} minWidth={740}>
      <DrawerHeader
        icon={<ServerCog size={24} />}
        title="Add Tool Configuration"
        onClose={onClose}
      />
      <DrawerContent>
        <Stack spacing={3}>
          {createConfig.isError ? (
            <Alert
              severity="error"
              icon={<AlertTriangle size={18} />}
              onClose={createConfig.reset}
            >
              {String(
                createConfig.error instanceof Error
                  ? createConfig.error.message
                  : "Failed to create MCP configuration. Please try again.",
              )}
            </Alert>
          ) : null}

          {!selectedProxyId ? (
            /* Step 1 — pick an MCP server from the list. */
            <Stack spacing={1}>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                Choose the MCP server for this agent.
              </Typography>
              <SearchBar
                placeholder="Search MCP servers"
                size="small"
                fullWidth
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                sx={{ mb: 1 }}
              />
              <Stack spacing={1} sx={{ overflowY: "auto" }}>
                {isLoadingProxies ? (
                  Array.from({ length: 3 }).map((_, i) => (
                    <Skeleton key={i} variant="rounded" height={72} />
                  ))
                ) : servers.length === 0 ? (
                  <ListingTable.Container>
                    <ListingTable.EmptyState
                      illustration={<ServerCog size={64} />}
                      title="No MCP servers available"
                      description="No MCP servers found. Add MCP servers from the organization MCP Servers page first."
                      action={
                        orgId ? (
                          <Button
                            variant="contained"
                            size="small"
                            startIcon={<Link size={16} />}
                            onClick={() =>
                              navigate(
                                generatePath(
                                  absoluteRouteMap.children.org.children
                                    .mcpProxies.children.add.path,
                                  { orgId },
                                ),
                              )
                            }
                          >
                            Register MCP Server
                          </Button>
                        ) : undefined
                      }
                    />
                  </ListingTable.Container>
                ) : filteredServers.length === 0 ? (
                  <ListingTable.Container>
                    <ListingTable.EmptyState
                      illustration={<Search size={64} />}
                      title="No MCP servers match your search"
                      description="Try a different keyword or clear the search filter."
                    />
                  </ListingTable.Container>
                ) : (
                  filteredServers.map((server) => (
                    <Form.CardButton
                      key={server.id}
                      selected={false}
                      onClick={() => setSelectedProxyId(server.id ?? null)}
                      aria-label={`${server.name}. Click to select`}
                    >
                      <Form.CardContent>
                        <MCPServerDisplay server={server} isSelected={false} />
                      </Form.CardContent>
                    </Form.CardButton>
                  ))
                )}
              </Stack>
            </Stack>
          ) : (
            /* Step 2 — the same drawer shows the selection + configuration. */
            <Stack spacing={3}>
              <Form.Section>
                <Form.Subheader>MCP Server</Form.Subheader>
                <Form.CardButton
                  onClick={() => setSelectedProxyId(null)}
                  selected
                  aria-label={`Selected: ${selectedProxy?.name}. Click to change.`}
                  sx={{ position: "relative" }}
                >
                  <Tooltip title="Change MCP server" placement="top" arrow>
                    <Box
                      sx={{
                        position: "absolute",
                        top: 8,
                        right: 8,
                        display: "inline-flex",
                        color: "text.secondary",
                      }}
                    >
                      <Edit size={16} />
                    </Box>
                  </Tooltip>
                  <Form.CardContent>
                    <MCPServerDisplay
                      server={selectedProxy}
                      isSelected={false}
                      hideCheckbox
                    />
                  </Form.CardContent>
                </Form.CardButton>
              </Form.Section>

              <ConfigNameSection
                value={configName}
                onChange={(value) => {
                  configNameEditedRef.current = true;
                  setConfigName(value);
                }}
                description="A name for this MCP configuration."
                placeholder="my-mcp-configuration"
              />

              {!isExternal && isSecurityUnknown ? (
                <Form.Section>
                  <Form.Subheader>Environment Variable Names</Form.Subheader>
                  <Alert severity="error">
                    Couldn&apos;t load this MCP server&apos;s security settings, so
                    the runtime variables it needs are unknown. Reselect the server
                    or retry once it is reachable — saving now could leave the agent
                    without a credential it needs.
                  </Alert>
                </Form.Section>
              ) : null}

              {!isExternal && isSecurityLoading ? (
                <Form.Section>
                  <Form.Subheader>Environment Variable Names</Form.Subheader>
                  <Skeleton variant="rounded" height={120} />
                </Form.Section>
              ) : null}

              {!isExternal && !isSecurityPending ? (
                <Form.Section>
                  <Form.Subheader>Environment Variable Names</Form.Subheader>
                  <Typography
                    variant="body2"
                    color="text.secondary"
                    sx={{ mb: 2 }}
                  >
                    {authenticationType === "identity"
                      ? "Your agent still needs this tool's URL, even with OAuth. Shared across all environments; edit only if your code uses a different name."
                      : authenticationType === ""
                        ? "This MCP server is unsecured, so your agent needs only its URL. Shared across all environments; edit only if your code uses a different name."
                        : "These names are shared across all environments. The platform injects the MCP server URL and API key values at runtime per environment (empty in environments the proxy is not configured for). Edit only if your code uses different names."}
                  </Typography>
                  <ListingTable.Container>
                    <ListingTable density="compact">
                      <ListingTable.Head>
                        <ListingTable.Row>
                          <ListingTable.Cell>
                            Variable Name{" "}
                            <Typography
                              component="span"
                              variant="caption"
                              color="text.secondary"
                            >
                              (editable)
                            </Typography>
                          </ListingTable.Cell>
                          <ListingTable.Cell>Description</ListingTable.Cell>
                        </ListingTable.Row>
                      </ListingTable.Head>
                      <ListingTable.Body>
                        {relevantEnvVarKeys.map((key) => (
                          <ListingTable.Row key={key}>
                            <ListingTable.Cell>
                              <TextInput
                                maxLength={INPUT_LIMITS.KEY}
                                value={envVarNames[key] ?? ""}
                                onChange={(event) => {
                                  envVarNamesEditedRef.current = true;
                                  setEnvVarNames((prev) => ({
                                    ...prev,
                                    [key]: event.target.value,
                                  }));
                                }}
                                copyable
                                copyTooltipText={`Copy ${envVarNames[key] || key}`}
                                size="small"
                              />
                            </ListingTable.Cell>
                            <ListingTable.Cell>
                              <Typography variant="body2" color="text.secondary">
                                {ENV_VAR_DESCRIPTIONS[key]}
                              </Typography>
                            </ListingTable.Cell>
                          </ListingTable.Row>
                        ))}
                      </ListingTable.Body>
                    </ListingTable>
                  </ListingTable.Container>
                </Form.Section>
              ) : null}

              {!isExternal && usesIdentitySecurity ? (
                <Form.Section>
                  <Form.Subheader>AgentID Variables</Form.Subheader>
                  <Alert severity="info" sx={{ mb: 2 }}>
                    <Typography variant="body2">
                      This tool uses OAuth (AgentID) security. These values
                      are injected into the agent&apos;s pod at runtime, use
                      them in your code to request a token. Scopes are
                      configured on this MCP Server&apos;s own security
                      settings.
                    </Typography>
                  </Alert>
                  <EnvironmentVariablesReference
                    variant="plain"
                    title="Injected at runtime"
                    description="These names are fixed, only their values change per environment, and they're injected automatically at runtime alongside the URL above."
                    rows={AGENTID_ENV_VAR_ROWS}
                  />
                </Form.Section>
              ) : null}
            </Stack>
          )}

          <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
            <Button variant="outlined" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="contained" onClick={handleSave} disabled={!canSave}>
              {createConfig.isPending ? "Saving..." : "Save"}
            </Button>
          </Box>
        </Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}

export default AddMCPToolConfigPanel;
