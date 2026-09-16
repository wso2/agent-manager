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

import { useEffect, useId, useMemo, useState } from "react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  PageLayout,
  TextInput,
} from "@agent-management-platform/views";
import {
  CodeBlock,
  AGENTID_ENV_VAR_ROWS,
  copyToClipboard,
  EnvironmentVariablesReference,
  getErrorMessage,
  monospaceInputSx,
  resolveAuthenticationType,
  useAgentIdentityCredentials,
  usePipelineEnvironmentsState,
  useThunderInstanceForEnv,
} from "@agent-management-platform/shared-component";
import {
  Alert,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Divider,
  Form,
  FormControl,
  IconButton,
  ListingTable,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  BookOpen,
  Copy,
  ExternalLink,
  RotateCcwKey,
  Wrench,
} from "@wso2/oxygen-ui-icons-react";
import {
  useGetAgent,
  useGetAgentMCPConfig,
  useGetMCPProxy,
  useListEnvironments,
  useListMCPProxies,
  useListMCPProxyScopes,
  useUpdateAgentMCPConfig,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type EnvironmentVariableConfig,
  type EnvProviderConfigMappings,
  type MCPProxyEndpoint,
} from "@agent-management-platform/types";
import {
  getCapabilityId,
  isToolBlockedByAcl,
} from "@agent-management-platform/mcp-proxies";
import {
  generatePath,
  useLocation,
  useNavigate,
  useParams,
} from "react-router-dom";
import { EnvironmentVariablesGuideDrawer } from "./Configure/subComponents/EnvironmentVariablesGuideDrawer";
import { MCPServerDisplay } from "./Configure/subComponents/MCPServerDisplay";
import { MCPProxyAPIKeysSection } from "./Configure/subComponents/MCPProxyAPIKeysSection";
import { CONFIGURE_TAB_PARAM } from "./configureTabs";

type AuthInfoEntry = {
  type: string;
  in: string;
  name: string;
  value?: string;
};

// Mirrors how buildMCPPythonSnippet resolves the (possibly renamed) URL var,
// so both guides stay consistent if that name changes.
function buildAgentIDPythonSnippet(urlEnvVar: string): string {
  return [
    "import os",
    "from typing import Any",
    "import requests",
    "from langchain_mcp_adapters.client import MultiServerMCPClient",
    "",
    "# 1. This tool's URL is also the OAuth 2.0 target resource (RFC 8707) —",
    "# it must be known before minting the token, not just before calling the tool.",
    `mcp_server_url = os.environ.get("${urlEnvVar}", "").strip()`,
    "",
    "# 2. Request a token using the injected AgentID credentials, scoped to",
    "# this specific tool via the resource parameter.",
    'client_id = os.environ["AMP_AGENTID_CLIENT_ID"]',
    'client_secret = os.environ["AMP_AGENTID_CLIENT_SECRET"]',
    'token_endpoint = os.environ["AMP_AGENTID_TOKEN_ENDPOINT"]',
    'scopes = os.environ["AMP_AGENTID_SCOPES"]',
    "",
    "token_response = requests.post(",
    "    token_endpoint,",
    "    auth=(client_id, client_secret),",
    "    data={",
    '        "grant_type": "client_credentials",',
    '        "scope": scopes,',
    '        "resource": mcp_server_url,',
    "    },",
    "    timeout=30,",
    ")",
    "token_response.raise_for_status()",
    'access_token = token_response.json()["access_token"]',
    "",
    "# 3. Call this tool's URL with the token as a normal Bearer header.",
    "server_configs: dict[str, dict[str, Any]] = {",
    '    "mcp_server": {',
    '        "url": mcp_server_url,',
    '        "transport": "streamable_http",',
    '        "headers": {',
    '            "Authorization": f"Bearer {access_token}",',
    "        },",
    "    }",
    "} if mcp_server_url else {}",
    "",
    "mcp_client = MultiServerMCPClient(server_configs)",
    "tools = await mcp_client.get_tools()",
  ].join("\n");
}

type ToolRow = { id: string; blocked: boolean; scopes: string[] };

// The endpoint bound to a given environment (matched by UUID; at most one per
// environment) — shared by the security/API-key lookup and the tools lookup
// below, which otherwise would each re-derive this same lookup.
function findEndpointForEnvUuid(
  endpoints: MCPProxyEndpoint[] | undefined,
  envUuid: string | undefined,
): MCPProxyEndpoint | undefined {
  if (!envUuid) return undefined;
  return endpoints?.find((endpoint) =>
    endpoint.environments?.some((binding) => binding.environmentUuid === envUuid),
  );
}

/**
 * Compact loading/error fallback shared by ConnectIdentityCredentials and
 * ConnectIdentityEndpoints below — lighter-weight than the Agent ID page's
 * (@agent-management-platform/env-thunders) ListingTable.EmptyState-based
 * QueryStateFallback, since these render as stacked sections inside an
 * already content-dense drawer rather than each owning their own dedicated
 * panel.
 */
const ConnectQueryFallback: React.FC<{
  isLoading: boolean;
  error: unknown;
}> = ({ isLoading, error }) => {
  if (isLoading) {
    return <Skeleton variant="rounded" height={96} />;
  }
  return (
    <Alert severity="error" icon={<AlertTriangle size={18} />}>
      {getErrorMessage(error)}
    </Alert>
  );
};

/**
 * Client ID (always visible once provisioned) + one-time secret reveal, for
 * the "Connect to MCP Server" drawer's OAuth (AgentID) branch — an external
 * agent needs the actual credential values here, not an env-var name/
 * description pair, since nothing is injected into its runtime for it.
 */
const ConnectIdentityCredentials: React.FC<{
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
}> = ({ orgId, projectId, agentId, envId }) => {
  const {
    binding, provisioned, isLoading, isError, error, revealed, isRegenerating, regenerate,
  } = useAgentIdentityCredentials({ orgId, projectId, agentId, envId });

  if (isLoading || isError) {
    return <ConnectQueryFallback isLoading={isLoading} error={error} />;
  }

  if (binding?.status === "failed") {
    return (
      <Typography variant="body2" color="text.secondary">
        Provisioning failed — check the identity settings for details.
      </Typography>
    );
  }

  if (!binding?.clientId && !revealed) {
    return (
      <Typography variant="body2" color="text.secondary">
        Provisioning in progress…
      </Typography>
    );
  }

  return (
    <Stack spacing={1.5}>
      <TextInput
        label="Client ID"
        value={revealed?.clientId ?? binding?.clientId ?? ""}
        copyable
        fullWidth
        size="small"
        slotProps={{ input: { readOnly: true } }}
        sx={monospaceInputSx}
      />
      <TextInput
        label="Client Secret"
        value={revealed?.clientSecret ?? "••••••••"}
        type={revealed ? "password" : "text"}
        showPasswordToggle={Boolean(revealed)}
        copyable={Boolean(revealed)}
        fullWidth
        size="small"
        slotProps={{ input: { readOnly: true } }}
        sx={monospaceInputSx}
      />
      <Stack direction="row" alignItems="center" justifyContent="space-between" gap={1}>
        <Typography variant="body2" color="text.secondary">
          {revealed
            ? "This secret will not be shown again — copy it now."
            : "The client secret is only shown once, right after regenerating."}
        </Typography>
        {provisioned && (
          <Button
            variant="text"
            size="small"
            onClick={() => void regenerate()}
            disabled={isRegenerating}
            startIcon={
              isRegenerating ? <CircularProgress size={16} /> : <RotateCcwKey size={16} />
            }
            sx={{ flexShrink: 0 }}
          >
            {isRegenerating ? "Regenerating..." : "Regenerate Secret"}
          </Button>
        )}
      </Stack>
    </Stack>
  );
};

/**
 * Issuer/token/JWKS endpoints for the environment's identity provider —
 * alongside ConnectIdentityCredentials, this is everything an external agent
 * needs to request and use a token, without visiting a separate page.
 */
const ConnectIdentityEndpoints: React.FC<{ orgId: string; envId: string }> = ({
  orgId, envId,
}) => {
  const { thunderInstance, isLoading, isError, error } = useThunderInstanceForEnv({
    orgId, envId,
  });

  if (isLoading || isError) {
    return <ConnectQueryFallback isLoading={isLoading} error={error} />;
  }

  if (!thunderInstance) {
    return (
      <Typography variant="body2" color="text.secondary">
        No identity provider found for this environment.
      </Typography>
    );
  }

  return (
    <Stack spacing={1.5}>
      <TextInput
        label="Issuer URL"
        value={thunderInstance.issuerUrl}
        copyable
        fullWidth
        size="small"
        slotProps={{ input: { readOnly: true } }}
      />
      <TextInput
        label="Token Endpoint"
        value={thunderInstance.tokenUrl}
        copyable
        fullWidth
        size="small"
        slotProps={{ input: { readOnly: true } }}
      />
      <TextInput
        label="JWKS Endpoint"
        value={thunderInstance.jwksUrl}
        copyable
        fullWidth
        size="small"
        slotProps={{ input: { readOnly: true } }}
      />
    </Stack>
  );
};

export const ViewMCPServerComponent = () => {
  const { orgId, projectId, agentId, proxyId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    proxyId: string;
  }>();
  const decodedConfigId = useMemo(() => decodeRouteParam(proxyId), [proxyId]);
  const environmentSelectId = useId();
  const environmentSelectLabelId = useId();
  const navigate = useNavigate();
  const location = useLocation();

  const authInfoByEnv = (
    location.state as {
      authInfoByEnv?: Record<string, AuthInfoEntry>;
      openEnvPanel?: boolean;
    } | null
  )?.authInfoByEnv;

  const [panelOpen, setPanelOpen] = useState(() =>
    Boolean(
      (location.state as { openEnvPanel?: boolean } | null)?.openEnvPanel ||
        authInfoByEnv,
    ),
  );
  // Selected environment drives only the external connect / API-key panels; the
  // deployment-status list itself is not a per-env editor.
  const [selectedEnvName, setSelectedEnvName] = useState("");
  const [envVarNames, setEnvVarNames] = useState<Record<string, string>>({});
  const [scopesCopied, setScopesCopied] = useState(false);

  const {
    data: config,
    isLoading,
    isError,
    error,
  } = useGetAgentMCPConfig({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
    configId: decodedConfigId,
  });

  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const isExternal = agent?.provisioning?.type === "external";

  const { data: environments = [], isError: isEnvironmentsError } = useListEnvironments({
    orgName: orgId,
  });
  const getEnvDisplayName = (name: string) =>
    environments.find((env) => env.name === name)?.displayName ?? name;
  const { environments: pipelineEnvs } = usePipelineEnvironmentsState(
    orgId,
    projectId,
  );
  const { data: proxiesData } = useListMCPProxies(
    { orgName: orgId },
    { limit: 50, offset: 0 },
  );
  const servers = useMemo(() => proxiesData?.list ?? [], [proxiesData]);
  const updateConfig = useUpdateAgentMCPConfig();

  const backHref =
    orgId && projectId && agentId
      ? `${generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.configure.path,
          { orgId, projectId, agentId },
        )}?${CONFIGURE_TAB_PARAM}=tools`
      : "#";

  // Show every environment the agent deploys to (pipeline order), plus any mapped
  // envs no longer in the pipeline, so each carries a deployment status.
  const envNames = useMemo(() => {
    const configured = Object.keys(config?.envMappings ?? {});
    const ordered = pipelineEnvs.map((env) => env.name);
    const extras = configured.filter((name) => !ordered.includes(name));
    const union = [...ordered, ...extras];
    return union.length > 0 ? union : configured;
  }, [config, pipelineEnvs]);

  // The tool config references a single, environment-agnostic MCP proxy. Derive it
  // from any environment that has a mapping.
  const configProxyName = useMemo(() => {
    for (const mapping of Object.values(config?.envMappings ?? {})) {
      const name = getMCPProxyName(mapping.configuration);
      if (name) return name;
    }
    return undefined;
  }, [config]);
  const configProxy = useMemo(
    () => servers.find((s) => s.id === configProxyName),
    [servers, configProxyName],
  );
  const configProxyHref =
    orgId && configProxyName
      ? generatePath(
          absoluteRouteMap.children.org.children.mcpProxies.children.view.path,
          { orgId, proxyId: configProxyName },
        )
      : undefined;

  // Default the selected environment (external connect / API-key panels) to the
  // first environment once names resolve, and keep it valid.
  useEffect(() => {
    if (envNames.length === 0) return;
    if (!envNames.includes(selectedEnvName)) {
      setSelectedEnvName(envNames[0]);
    }
  }, [envNames, selectedEnvName]);

  const providerConfig = config?.envMappings?.[selectedEnvName]?.configuration;

  const {
    data: sourceProxyDetails,
    isLoading: isLoadingProxyDetails,
    isError: isProxyDetailsError,
  } = useGetMCPProxy({
    orgName: orgId,
    proxyId: configProxyName ?? "",
  });
  const selectedEnvUuid = environments.find(
    (env) => env.name === selectedEnvName,
  )?.id;
  const sourceProxyEndpoint = findEndpointForEnvUuid(
    sourceProxyDetails?.endpoints,
    selectedEnvUuid,
  );
  const apiKeyHeaderName = getMCPAPIKeyHeaderName(sourceProxyEndpoint?.security);
  // Same rule the MCP Servers Security tab renders and the creation flow derives
  // its variables from, so all three surfaces agree on API key vs OAuth vs None.
  const authenticationType = resolveAuthenticationType(sourceProxyEndpoint);
  const usesIdentitySecurity = authenticationType === "identity";
  const usesAPIKeySecurity = authenticationType === "apiKey";
  // A failed or in-flight proxy fetch leaves sourceProxyEndpoint undefined,
  // which resolveAuthenticationType reads as "None" — indistinguishable from a
  // genuinely unsecured endpoint. Treating that as real would hide whichever
  // fields/rows the endpoint's actual security needs and show misleading
  // copy, the same #1597 failure mode with the polarity reversed. Callers must
  // gate on this rather than trusting authenticationType directly while true.
  const isMCPSecurityUnresolved = isLoadingProxyDetails || isProxyDetailsError;

  // Scopes are a proxy-level catalog (action -> tools it authorizes), not
  // per-endpoint, so this fetch doesn't depend on the selected environment.
  const {
    data: scopesData,
    isLoading: isLoadingScopes,
    isError: isScopesError,
  } = useListMCPProxyScopes(
    { orgName: orgId ?? "", proxyId: configProxyName ?? "" },
    { enabled: !!orgId && !!configProxyName },
  );
  // Both feed toolRows below — while either is still in flight, it would
  // compute against stale/empty data and the Tools section would flash "No
  // tools available" before the real list shows up.
  const isLoadingTools = isLoadingProxyDetails || isLoadingScopes;
  // A failure here would otherwise look identical to "this environment
  // genuinely has no tools" — surfaced separately in the Tools section below.
  const isToolsError = isEnvironmentsError || isProxyDetailsError || isScopesError;

  // Tools belong to the endpoint bound to the selected environment.
  const toolRows = useMemo<ToolRow[]>(() => {
    const scopesByTool: Record<string, string[]> = {};
    for (const scope of scopesData?.scopes ?? []) {
      for (const toolId of scope.tools) {
        (scopesByTool[toolId] ??= []).push(scope.scope);
      }
    }

    return (sourceProxyEndpoint?.capabilities?.tools ?? [])
      .map((raw) => {
        const id = getCapabilityId("tool", raw);
        if (!id) return null;
        return {
          id,
          blocked: isToolBlockedByAcl(sourceProxyEndpoint, id),
          scopes: scopesByTool[id] ?? [],
        };
      })
      .filter((row): row is ToolRow => row !== null);
  }, [sourceProxyEndpoint, scopesData]);

  // The proxy's full scope catalog, deduped — shown in the "Connect to MCP
  // Server" drawer's OAuth branch so an external agent knows what to
  // request, independent of which tool(s) it plans to call.
  const uniqueScopes = useMemo(
    () => [...new Set(scopesData?.scopes.map((scope) => scope.scope) ?? [])],
    [scopesData],
  );

  const envVarRows = useMemo<EnvironmentVariableConfig[]>(
    () => config?.environmentVariables ?? [],
    [config],
  );

  // A config may still carry an apikey row from before the proxy's security was
  // switched away from API key; hide it rather than show a stale, irrelevant
  // field. Only an API-key endpoint has one — OAuth mints credentials
  // server-side and an unsecured endpoint needs none (issue #1597).
  const visibleEnvVarRows = useMemo(
    () =>
      usesAPIKeySecurity
        ? envVarRows
        : envVarRows.filter((envVar) => !isAPIKeyEnvVarKey(envVar.key)),
    [envVarRows, usesAPIKeySecurity],
  );

  useEffect(() => {
    const nextNames: Record<string, string> = {};
    for (const envVar of envVarRows) {
      nextNames[envVar.key] = envVar.name;
    }
    setEnvVarNames(nextNames);
  }, [envVarRows]);

  const hasEmptyEnvVarName = visibleEnvVarRows.some(
    (envVar) => (envVarNames[envVar.key] ?? envVar.name).trim() === "",
  );
  const isDirty = visibleEnvVarRows.some(
    (envVar) => (envVarNames[envVar.key] ?? envVar.name) !== envVar.name,
  );

  const handleSave = () => {
    if (
      !orgId ||
      !projectId ||
      !agentId ||
      !decodedConfigId ||
      hasEmptyEnvVarName ||
      isMCPSecurityUnresolved
    ) {
      return;
    }
    // Only the shared env var NAMES are editable here. Deployment is handled at the
    // org-level MCP proxy + gateways, so no server mappings are sent.
    updateConfig.mutate(
      {
        params: {
          orgName: orgId,
          projName: projectId,
          agentName: agentId,
          configId: decodedConfigId,
        },
        body: {
          environmentVariables: envVarRows.map((envVar) => ({
            key: envVar.key,
            name: (envVarNames[envVar.key] ?? envVar.name).trim(),
          })),
        },
      },
      {
        onSuccess: () => setPanelOpen(false),
      },
    );
  };

  const resetEnvVarNames = () => {
    const nextNames: Record<string, string> = {};
    for (const envVar of envVarRows) {
      nextNames[envVar.key] = envVar.name;
    }
    setEnvVarNames(nextNames);
  };

  const envVarReferenceRows = useMemo(
    () =>
      visibleEnvVarRows.map((envVar) => ({
        key: envVar.key,
        name: envVarNames[envVar.key] ?? envVar.name,
        description: describeMCPEnvVar(envVar.key),
      })),
    [visibleEnvVarRows, envVarNames],
  );

  const pythonSnippet = useMemo(
    () => buildMCPPythonSnippet(envVarReferenceRows),
    [envVarReferenceRows],
  );

  const agentIDPythonSnippet = useMemo(() => {
    const urlEnvVar =
      envVarReferenceRows.find((row) => /url/i.test(row.key))?.name ??
      "MCP_SERVER_URL";
    return buildAgentIDPythonSnippet(urlEnvVar);
  }, [envVarReferenceRows]);

  if (isLoading) {
    return (
      <PageLayout docs={["configureAgentMcpProxies", "authorizeAgentMcpTools", "mcpProxy"]}
        title="Tool Configuration"
        backHref={backHref}
        disableIcon
        backLabel="Back to Configure"
      >
        <Stack spacing={2}>
          <Skeleton variant="rounded" height={56} />
          <Skeleton variant="rounded" height={180} />
          <Skeleton variant="rounded" height={240} />
        </Stack>
      </PageLayout>
    );
  }

  if (isError || !config) {
    return (
      <PageLayout docs={["configureAgentMcpProxies", "authorizeAgentMcpTools", "mcpProxy"]}
        title="Tool Configuration"
        backHref={backHref}
        disableIcon
        backLabel="Back to Configure"
      >
        <Alert severity="error" icon={<AlertTriangle size={18} />}>
          {error instanceof Error
            ? error.message
            : "Configuration not found or failed to load."}
        </Alert>
      </PageLayout>
    );
  }

  const pageTitle =
    config.name || configProxy?.name || configProxyName || "Tool Configuration";
  const showPanel =
    (isExternal && !!providerConfig) ||
    (!isExternal && (envVarRows.length > 0 || usesIdentitySecurity));

  const envVarsPanel =
    showPanel &&
    (isExternal && providerConfig ? (
      <DrawerWrapper
        open={panelOpen}
        onClose={() => setPanelOpen(false)}
        minWidth={640}
        maxWidth={640}
      >
        <DrawerHeader
          icon={<BookOpen size={24} />}
          title="Connect to MCP Server"
          onClose={() => setPanelOpen(false)}
        />
        <DrawerContent>
          {isMCPSecurityUnresolved ? (
            isLoadingProxyDetails ? (
              <Skeleton variant="rounded" height={160} />
            ) : (
              <Alert severity="error" icon={<AlertTriangle size={18} />}>
                <Typography variant="body2">
                  Couldn&apos;t load this MCP server&apos;s security settings, so
                  the connection details shown here may not match how this tool
                  is actually secured. Reload the page or try again once the
                  server is reachable.
                </Typography>
              </Alert>
            )
          ) : usesIdentitySecurity ? (
            <Stack spacing={3}>
              <Alert severity="info">
                <Typography variant="body2">
                  This tool uses OAuth (AgentID) security. Use the client
                  credentials and identity provider endpoints below to
                  request a token, passing the MCP URL as the{" "}
                  <code>resource</code> parameter, then call the MCP URL with
                  the token as a Bearer header.
                </Typography>
              </Alert>

              {Boolean(providerConfig.url) && (
                <Form.Section>
                  <Form.Subheader>Connection</Form.Subheader>
                  <TextInput
                    label="MCP URL"
                    value={providerConfig.url ?? ""}
                    copyable
                    copyTooltipText="Copy MCP URL"
                    slotProps={{ input: { readOnly: true } }}
                    size="small"
                  />
                </Form.Section>
              )}

              <Form.Section>
                <Form.Subheader>Client Credentials</Form.Subheader>
                <ConnectIdentityCredentials
                  key={selectedEnvName}
                  orgId={orgId ?? ""}
                  projectId={projectId ?? ""}
                  agentId={agentId ?? ""}
                  envId={selectedEnvName}
                />
              </Form.Section>

              <Form.Section>
                <Form.Subheader>Identity Provider Endpoints</Form.Subheader>
                <ConnectIdentityEndpoints orgId={orgId ?? ""} envId={selectedEnvName} />
              </Form.Section>

              {uniqueScopes.length > 0 && (
                <Form.Section>
                  <Stack direction="row" alignItems="center" justifyContent="space-between">
                    <Form.Subheader>Available Scopes</Form.Subheader>
                    <Tooltip title={scopesCopied ? "Copied!" : "Copy scopes"}>
                      <IconButton
                        size="small"
                        onClick={() =>
                          void copyToClipboard(uniqueScopes.join(" ")).then((succeeded) => {
                            if (!succeeded) return;
                            setScopesCopied(true);
                            setTimeout(() => setScopesCopied(false), 2000);
                          })
                        }
                        aria-label="Copy scopes"
                      >
                        <Copy size={14} />
                      </IconButton>
                    </Tooltip>
                  </Stack>
                  <Typography variant="body2" color="text.secondary">
                    Request a token that includes the scopes required by the
                    tools you plan to call.
                  </Typography>
                  <Stack direction="row" spacing={0.5} flexWrap="wrap">
                    {uniqueScopes.map((scope) => (
                      <Chip key={scope} size="small" variant="outlined" label={scope} />
                    ))}
                  </Stack>
                </Form.Section>
              )}
            </Stack>
          ) : usesAPIKeySecurity ? (() => {
            const authEntry =
              authInfoByEnv?.[selectedEnvName] ?? providerConfig.authInfo;
            const headerName = apiKeyHeaderName || authEntry?.name || "api-key";
            const headerValue = authEntry?.value || "<api-key>";
            const curlCode = [
              `curl -N ${providerConfig.url || "<endpoint-url>"}`,
              `  --header "${headerName}: ${headerValue}"`,
            ].join(" \\\n");
            return (
              <Stack spacing={3}>
                {authEntry?.value ? (
                  <>
                    <Alert severity="info">
                      <Typography variant="body2">
                        Configure your external agent with the endpoint and API
                        key below to call this MCP server through the gateway.
                      </Typography>
                    </Alert>
                    <Alert severity="warning">
                      <Typography variant="body2" fontWeight={600}>
                        Make sure to copy your API key now. You will not be able
                        to see it again.
                      </Typography>
                    </Alert>
                  </>
                ) : (
                  <Alert severity="info">
                    <Typography variant="body2">
                      The endpoint is available below. If the MCP server
                      requires an API key, the key was only displayed when this
                      configuration was created.
                    </Typography>
                  </Alert>
                )}
                <Form.Section>
                  <Form.Subheader>Connection</Form.Subheader>
                  {Boolean(providerConfig.url) && (
                    <TextInput
                      label="Endpoint URL"
                      value={providerConfig.url ?? ""}
                      copyable
                      copyTooltipText="Copy Endpoint URL"
                      slotProps={{ input: { readOnly: true } }}
                      size="small"
                    />
                  )}
                  <TextInput
                    label="Header Name"
                    value={headerName}
                    copyable
                    copyTooltipText="Copy Header Name"
                    slotProps={{ input: { readOnly: true } }}
                    size="small"
                  />
                  {authEntry?.value && (
                    <TextInput
                      label="API Key"
                      type="password"
                      value={authEntry.value}
                      copyable
                      copyTooltipText="Copy API Key"
                      slotProps={{ input: { readOnly: true } }}
                      size="small"
                    />
                  )}
                </Form.Section>
                <Form.Section>
                  <Form.Subheader>Example cURL</Form.Subheader>
                  <CodeBlock
                    code={curlCode}
                    language="bash"
                    fieldId="mcp-curl"
                  />
                </Form.Section>
              </Stack>
            );
          })() : (
            // Unsecured endpoint: no header, no key, and no "the key was only
            // shown at creation" copy — that phrasing implies a credential that
            // was never minted, since this server takes none (issue #1597).
            <Stack spacing={3}>
              <Alert severity="info">
                <Typography variant="body2">
                  This tool has no security configured. Call the endpoint
                  directly — no header or API key is required.
                </Typography>
              </Alert>
              <Form.Section>
                <Form.Subheader>Connection</Form.Subheader>
                {Boolean(providerConfig.url) && (
                  <TextInput
                    label="Endpoint URL"
                    value={providerConfig.url ?? ""}
                    copyable
                    copyTooltipText="Copy Endpoint URL"
                    slotProps={{ input: { readOnly: true } }}
                    size="small"
                  />
                )}
              </Form.Section>
              <Form.Section>
                <Form.Subheader>Example cURL</Form.Subheader>
                <CodeBlock
                  code={`curl -N ${providerConfig.url || "<endpoint-url>"}`}
                  language="bash"
                  fieldId="mcp-curl"
                />
              </Form.Section>
            </Stack>
          )}
        </DrawerContent>
      </DrawerWrapper>
    ) : (
      <EnvironmentVariablesGuideDrawer
        open={panelOpen}
        onClose={() => setPanelOpen(false)}
        onCancel={() => {
          resetEnvVarNames();
          setPanelOpen(false);
        }}
        onSave={handleSave}
        isDirty={isDirty}
        isSaving={updateConfig.isPending}
        hasInvalidNames={hasEmptyEnvVarName || isMCPSecurityUnresolved}
        error={updateConfig.isError ? updateConfig.error : undefined}
        description={
          "These variable names are injected into the agent at runtime with environment-specific values. Rename them here if your code already uses different names, then save."
        }
        rows={envVarReferenceRows}
        onNameChange={(key, value) =>
          setEnvVarNames((prev) => ({
            ...prev,
            [key]: value,
          }))
        }
      >
        <Divider sx={{ my: 2 }} />
        {isMCPSecurityUnresolved ? (
          isLoadingProxyDetails ? (
            <Skeleton variant="rounded" height={160} />
          ) : (
            <Alert severity="error" icon={<AlertTriangle size={18} />}>
              <Typography variant="body2">
                Couldn&apos;t load this MCP server&apos;s security settings, so
                the fields and integration guide shown here may not match how
                this tool is actually secured. Reload the page or try again
                once the server is reachable before saving.
              </Typography>
            </Alert>
          )
        ) : usesIdentitySecurity ? (
          <Stack spacing={3}>
            <Alert severity="info">
              <Typography variant="body2">
                This tool uses OAuth (AgentID) security. These values are
                injected into your agent&apos;s pod at runtime, use them in
                your code to request a token. Scopes are configured on this
                MCP Server&apos;s own security settings.
              </Typography>
            </Alert>
            <Form.Section>
              <EnvironmentVariablesReference
                variant="plain"
                title="AgentID Variables"
                description="These names are fixed, only their values change per environment, and they're injected automatically at runtime alongside the URL above."
                rows={AGENTID_ENV_VAR_ROWS}
              />
            </Form.Section>
            <Form.Section>
              <Form.Subheader>Integration Guide</Form.Subheader>
              <Typography variant="body2" color="text.secondary">
                Copy this pattern into your agent code to request a token
                and call the tool with it.
              </Typography>
              <CodeBlock
                language="python"
                fieldId="mcp-identity-python-snippet"
                code={agentIDPythonSnippet}
              />
            </Form.Section>
          </Stack>
        ) : (
          <Form.Section>
            <Form.Subheader>Integration Guide</Form.Subheader>
            <Typography variant="body2" color="text.secondary">
              {usesAPIKeySecurity
                ? "Copy this pattern into your agent code to load MCP tools through the injected proxy URL and API key."
                : "Copy this pattern into your agent code to load MCP tools through the injected proxy URL. This server requires no API key."}
            </Typography>
            <CodeBlock
              language="python"
              fieldId="mcp-python-snippet"
              code={pythonSnippet}
            />
          </Form.Section>
        )}
      </EnvironmentVariablesGuideDrawer>
    ));

  return (
    <PageLayout docs={["configureAgentMcpProxies", "authorizeAgentMcpTools", "mcpProxy"]}
      title={pageTitle}
      backHref={backHref}
      disableIcon
      backLabel="Back to Configuration Listing"
      actions={
        showPanel ? (
          <Button
            variant="outlined"
            size="small"
            startIcon={<BookOpen size={16} />}
            onClick={() => setPanelOpen(true)}
          >
            {isExternal
              ? "Connect to MCP Server"
              : "Environment Variables & Integration Guide"}
          </Button>
        ) : undefined
      }
    >
      <Stack spacing={3}>
        <Form.Section>
          <Form.Subheader>MCP Server</Form.Subheader>
          {configProxy ? (
            <Card variant="outlined">
              <CardContent sx={{ position: "relative" }}>
                {configProxyHref && (
                  <Tooltip title="View MCP Server" placement="top" arrow>
                    <IconButton
                      size="small"
                      color="primary"
                      onClick={() => navigate(configProxyHref)}
                      aria-label={`View MCP Server ${configProxy.name ?? configProxyName} in the organization`}
                      sx={{ position: "absolute", top: 8, right: 8 }}
                    >
                      <ExternalLink size={16} />
                    </IconButton>
                  </Tooltip>
                )}
                <MCPServerDisplay
                  server={configProxy}
                  isSelected={false}
                  hideCheckbox
                />
              </CardContent>
            </Card>
          ) : (
            <Typography variant="body2" color="text.secondary">
              {configProxyName ?? "No MCP server referenced."}
            </Typography>
          )}
        </Form.Section>

        {envNames.length > 1 && (
          <Stack direction="row" spacing={2} alignItems="center" justifyContent="flex-end">
            <Typography
              id={environmentSelectLabelId}
              variant="body2"
              color="text.secondary"
            >
              Environment
            </Typography>
            <FormControl size="small" sx={{ minWidth: 260 }}>
              <Select
                id={environmentSelectId}
                labelId={environmentSelectLabelId}
                value={selectedEnvName}
                onChange={(event) =>
                  setSelectedEnvName(event.target.value as string)
                }
              >
                {envNames.map((name) => (
                  <MenuItem key={name} value={name}>
                    {getEnvDisplayName(name)}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Stack>
        )}

        <Form.Section>
          <Form.Subheader>Tools</Form.Subheader>
          {isLoadingTools ? (
            <Skeleton variant="rounded" height={160} />
          ) : isToolsError ? (
            <ListingTable.Container>
              <ListingTable.EmptyState
                illustration={<AlertTriangle size={56} />}
                title="Failed to load tools"
                description="Something went wrong while loading this environment's tools. Please try again."
              />
            </ListingTable.Container>
          ) : toolRows.length === 0 ? (
            <ListingTable.Container>
              <ListingTable.EmptyState
                illustration={<Wrench size={56} />}
                title="No tools available"
                description="This environment's endpoint hasn't reported any tools yet."
              />
            </ListingTable.Container>
          ) : (
            <ListingTable.Container>
              <ListingTable variant="table">
                <ListingTable.Head>
                  <ListingTable.Row>
                    <ListingTable.Cell>Tool</ListingTable.Cell>
                    <ListingTable.Cell width="140px">Status</ListingTable.Cell>
                    {usesIdentitySecurity && <ListingTable.Cell>Scopes</ListingTable.Cell>}
                  </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                  {toolRows.map((tool) => (
                    <ListingTable.Row key={tool.id} variant="table">
                      <ListingTable.Cell>
                        <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
                          {tool.id}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Chip
                          size="small"
                          variant="outlined"
                          label={tool.blocked ? "Blocked" : "Allowed"}
                          color={tool.blocked ? "error" : "success"}
                        />
                      </ListingTable.Cell>
                      {usesIdentitySecurity && (
                        <ListingTable.Cell>
                          {tool.scopes.length > 0 ? (
                            <Stack direction="row" spacing={0.5} flexWrap="wrap">
                              {tool.scopes.map((scope) => (
                                <Chip key={scope} size="small" variant="outlined" label={scope} />
                              ))}
                            </Stack>
                          ) : (
                            <Typography variant="body2" color="text.secondary">
                              —
                            </Typography>
                          )}
                        </ListingTable.Cell>
                      )}
                    </ListingTable.Row>
                  ))}
                </ListingTable.Body>
              </ListingTable>
            </ListingTable.Container>
          )}
        </Form.Section>

        {isExternal && providerConfig && usesAPIKeySecurity && (
          <MCPProxyAPIKeysSection
            orgName={orgId}
            projName={projectId}
            agentName={agentId}
            configId={decodedConfigId}
            envName={selectedEnvName}
          />
        )}
      </Stack>

      {envVarsPanel}
    </PageLayout>
  );
};

function decodeRouteParam(value?: string) {
  if (!value) return "";
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function getMCPProxyName(
  config?: EnvProviderConfigMappings["configuration"],
): string | undefined {
  return (
    config?.proxyName ??
    config?.proxyId ??
    config?.mcpProxyName ??
    config?.mcpProxyId ??
    config?.providerName
  );
}

function getMCPAPIKeyHeaderName(security?: {
  enabled?: boolean;
  apiKey?: { enabled?: boolean; key?: string };
}): string | undefined {
  if (security?.enabled === false || security?.apiKey?.enabled === false) {
    return undefined;
  }
  const headerName = security?.apiKey?.key?.trim();
  return headerName || "X-API-Key";
}

function describeMCPEnvVar(key: string): string {
  if (/url/i.test(key)) return "Base URL of the MCP server endpoint";
  if (isAPIKeyEnvVarKey(key))
    return "API key for authenticating with the MCP server endpoint";
  return key
    .replace(/([A-Z])/g, " $1")
    .replace(/^./, (str) => str.toUpperCase());
}

function isAPIKeyEnvVarKey(key: string): boolean {
  return /api[-_]?key/i.test(key);
}

function buildMCPPythonSnippet(rows: { key: string; name: string }[]): string {
  const urlEnvVar =
    rows.find((row) => /url/i.test(row.key))?.name ?? "MCP_SERVER_URL";
  // The caller already filters the apikey row out of `rows` for anything but an
  // API-key-secured endpoint (visibleEnvVarRows, issue #1597), so its absence
  // here is the real signal — not something to paper over with a hardcoded
  // fallback name. An unsecured endpoint has no key to read at all.
  const apiKeyEnvVar = rows.find((row) => /api[-_]?key/i.test(row.key))?.name;

  if (!apiKeyEnvVar) {
    return [
      "import os",
      "from typing import Any",
      "from langchain_mcp_adapters.client import MultiServerMCPClient",
      "",
      `raw_urls = os.environ.get("${urlEnvVar}", "")`,
      'mcp_server_urls = [url.strip() for url in raw_urls.split(",") if url.strip()]',
      "",
      "server_configs: dict[str, dict[str, Any]] = {",
      '    f"mcp_server_{i}": {',
      '        "url": url,',
      '        "transport": "streamable_http",',
      "    }",
      "    for i, url in enumerate(mcp_server_urls)",
      "} if mcp_server_urls else {}",
      "",
      "mcp_client = MultiServerMCPClient(server_configs)",
      "tools = await mcp_client.get_tools()",
    ].join("\n");
  }

  return [
    "import os",
    "from typing import Any",
    "from langchain_mcp_adapters.client import MultiServerMCPClient",
    "",
    `raw_urls = os.environ.get("${urlEnvVar}", "")`,
    'mcp_server_urls = [url.strip() for url in raw_urls.split(",") if url.strip()]',
    `mcp_api_key = os.environ.get("${apiKeyEnvVar}", "").strip()`,
    "",
    "server_configs: dict[str, dict[str, Any]] = {",
    '    f"mcp_server_{i}": {',
    '        "url": url,',
    '        "transport": "streamable_http",',
    '        "headers": {',
    '            "API-Key": mcp_api_key,',
    '            "Authorization": "",',
    "        },",
    "    }",
    "    for i, url in enumerate(mcp_server_urls)",
    "} if mcp_server_urls and mcp_api_key else {}",
    "",
    "mcp_client = MultiServerMCPClient(server_configs)",
    "tools = await mcp_client.get_tools()",
  ].join("\n");
}

export default ViewMCPServerComponent;
