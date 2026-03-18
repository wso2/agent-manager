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

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { PageLayout, TextInput } from "@agent-management-platform/views";
import {
  Alert,
  Button,
  Card,
  CardContent,
  Form,
  ListingTable,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useLocation, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  useGetAgent,
  useGetAgentModelConfig,
  useListCatalogLLMProviders,
  useListEnvironments,
  useListLLMProviderTemplates,
  useUpdateAgentModelConfig,
} from "@agent-management-platform/api-client";
import {
  GuardrailsSection,
  type GuardrailSelection,
} from "@agent-management-platform/llm-providers";
import { ProviderDisplay } from "./AddLLMProvider.Component";

function generateDisplayName(key: string): string {
  switch (key) {
    case "apikey":
      return "API Key for authenticating with the LLM provider";
    case "url":
      return "Base URL of the LLM Provider API endpoint";
    default:
      return key.replace(/([A-Z])/g, " $1").replace(/^./, (str) => str.toUpperCase()); // Add space before capital letters and capitalize the first letter

  }
}

function getClientSetupSnippet(
  templateId: string | undefined,
  varKeys: string[],
): { importLine: string; setup: string } | null {
  if (!templateId) return null;
  const urlKey = varKeys.find((k) => /url/i.test(k));
  const apiKeyKey = varKeys.find((k) => /key/i.test(k));

  const build = (
    importLine: string,
    lines: (string | null)[],
  ) => ({
    importLine,
    setup: lines.filter(Boolean).join("\n"),
  });

  switch (templateId) {
    case "openai":
      return build("from openai import OpenAI", [
        "client = OpenAI(",
        urlKey ? `    base_url=${urlKey},` : null,
        `    api_key="",`,
        `    default_headers={"API-Key": ${apiKeyKey}, "Authorization": ""}`,
        ")",
      ]);
    case "anthropic":
      return build("from anthropic import Anthropic", [
        "client = Anthropic(",
        urlKey ? `    base_url=${urlKey},` : null,
        `    default_headers={"API-Key": ${apiKeyKey}, "Authorization": ""}`,
        ")",
      ]);
    case "azure-openai":
    case "azureai-foundry":
      return build("from openai import AzureOpenAI", [
        "client = AzureOpenAI(",
        urlKey ? `    azure_endpoint=${urlKey},` : null,
        `    api_key="",`,
        `    default_headers={"API-Key": ${apiKeyKey}, "Authorization": ""}`,
        ")",
      ]);
    case "mistralai":
      return build("import httpx\nfrom mistralai import Mistral", [
        `_http_client = httpx.Client(headers={"API-Key": ${apiKeyKey}, "Authorization": ""})`,
        "client = Mistral(",
        urlKey ? `    server_url=${urlKey},` : null,
        "    client=_http_client,",
        ")",
      ]);
    case "gemini":
      return build("from google import genai\nfrom google.genai import types", [
        urlKey
          ? `_http_options = types.HttpOptions(base_url=${urlKey}, client_args={"headers": {"API-Key": ${apiKeyKey}, "Authorization": ""}})`
          : `_http_options = types.HttpOptions(client_args={"headers": {"API-Key": ${apiKeyKey}, "Authorization": ""}})`,
        "client = genai.Client(",
        `    http_options=_http_options`,
        ")",
      ]);
    case "awsbedrock":
      return build("import boto3\nfrom botocore import UNSIGNED\nfrom botocore.config import Config", [
        "client = boto3.client(",
        `    "bedrock-runtime",`,
        urlKey ? `    endpoint_url=${urlKey},` : null,
        `    config=Config(signature_version=UNSIGNED),`,
        ")",
        `def _add_headers(request, **kwargs):`,
        `    request.headers["API-Key"] = ${apiKeyKey}`,
        `    request.headers["Authorization"] = ""`,
        `client.meta.events.register("before-send", _add_headers)`,
      ]);
    default:
      return null;
  }
}

export const ViewLLMProviderComponent: React.FC = () => {
  const { orgId, projectId, agentId, configId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    configId: string;
  }>();
  const navigate = useNavigate();
  const location = useLocation();

  type AuthInfoEntry = {
    type: string;
    in: string;
    name: string;
    value?: string;
  };
  const authInfoByEnv = (
    location.state as {
      authInfoByEnv?: Record<string, AuthInfoEntry>;
    }
  )?.authInfoByEnv;

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [selectedEnvIndex, setSelectedEnvIndex] = useState(0);
  const [guardrailsByEnv, setGuardrailsByEnv] = useState<
    Record<string, GuardrailSelection[]>
  >({});
  const [envVarNames, setEnvVarNames] = useState<Record<string, string>>({});
  const [snippetTab, setSnippetTab] = useState(0);

  const backHref =
    orgId && projectId && agentId
      ? generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents
          .children.configure.path,
        { orgId, projectId, agentId },
      )
      : "#";

  const {
    data: config,
    isLoading,
    isError,
  } = useGetAgentModelConfig({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
    configId,
  });

  const { data: environments = [] } = useListEnvironments({
    orgName: orgId,
  });

  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  const isExternal = agent?.provisioning?.type === "external";

  const { data: catalogData } = useListCatalogLLMProviders(
    { orgName: orgId },
    { limit: 50 },
  );

  const { data: templatesData } = useListLLMProviderTemplates({
    orgName: orgId,
  });

  const updateConfig = useUpdateAgentModelConfig();

  useEffect(() => {
    if (!config) return;
    setName(config.name);
    setDescription(config.description ?? "");

    const nextNames: Record<string, string> = {};
    for (const ev of config.environmentVariables ?? []) {
      nextNames[ev.key] = ev.name;
    }
    setEnvVarNames(nextNames);

    const nextByEnv: Record<string, GuardrailSelection[]> = {};
    for (const [envName, m] of Object.entries(config.envMappings ?? {})) {
      const envPolicies = m.configuration?.policies ?? [];
      const seen = new Set<string>();
      const envGuardrails: GuardrailSelection[] = [];
      for (const p of envPolicies) {
        const key = `${p.name}@${p.version}`;
        if (seen.has(key)) continue;
        seen.add(key);
        const params = p.paths?.[0]?.params;
        envGuardrails.push({
          name: p.name,
          version: p.version,
          settings: (params ?? {}) as Record<string, unknown>,
        });
      }
      nextByEnv[envName] = envGuardrails;
    }
    setGuardrailsByEnv(nextByEnv);
  }, [config]);

  const selectedEnvName = useMemo(
    () => environments[selectedEnvIndex]?.name ?? "",
    [environments, selectedEnvIndex],
  );

  const envMapping = useMemo(
    () => config?.envMappings?.[selectedEnvName],
    [config, selectedEnvName],
  );

  const providerConfig = envMapping?.configuration;

  const catalogProvider = useMemo(() => {
    if (!providerConfig?.providerName || !catalogData?.entries)
      return undefined;
    return catalogData.entries.find(
      (e) => e.handle === providerConfig.providerName,
    );
  }, [providerConfig?.providerName, catalogData]);

  const templateLogo = useMemo(() => {
    if (!catalogProvider?.template || !templatesData?.templates)
      return undefined;
    const tpl = templatesData.templates.find(
      (t) => t.id === catalogProvider.template,
    );
    return tpl?.metadata?.logoUrl;
  }, [catalogProvider, templatesData]);

  const templateDisplayName = useMemo(() => {
    if (!catalogProvider?.template || !templatesData?.templates)
      return undefined;
    const tpl = templatesData.templates.find(
      (t) => t.id === catalogProvider.template,
    );
    return tpl?.name;
  }, [catalogProvider, templatesData]);

  const guardrails = useMemo(
    () => guardrailsByEnv[selectedEnvName] ?? [],
    [guardrailsByEnv, selectedEnvName],
  );

  const isDirty = useMemo(() => {
    if (!config) return false;
    if (name !== config.name) return true;
    if ((description || "") !== (config.description ?? "")) return true;

    // Check env var names
    for (const ev of config.environmentVariables ?? []) {
      if ((envVarNames[ev.key] ?? ev.name) !== ev.name) return true;
    }

    // Check guardrails
    for (const [envName, m] of Object.entries(config.envMappings ?? {})) {
      const origPolicies = m.configuration?.policies ?? [];
      const edited = guardrailsByEnv[envName] ?? [];
      if (origPolicies.length !== edited.length) return true;
      for (let i = 0; i < origPolicies.length; i++) {
        if (
          origPolicies[i].name !== edited[i]?.name ||
          origPolicies[i].version !== edited[i]?.version
        )
          return true;
      }
    }

    return false;
  }, [config, name, description, envVarNames, guardrailsByEnv]);

  const handleAddGuardrail = useCallback(
    (guardrail: GuardrailSelection) => {
      setGuardrailsByEnv((prev) => {
        const envList = prev[selectedEnvName] ?? [];
        if (
          envList.some(
            (g) =>
              g.name === guardrail.name && g.version === guardrail.version,
          )
        )
          return prev;
        return { ...prev, [selectedEnvName]: [...envList, guardrail] };
      });
    },
    [selectedEnvName],
  );

  const handleRemoveGuardrail = useCallback(
    (gName: string, gVersion: string) => {
      setGuardrailsByEnv((prev) => {
        const envList = prev[selectedEnvName] ?? [];
        return {
          ...prev,
          [selectedEnvName]: envList.filter(
            (g) => !(g.name === gName && g.version === gVersion),
          ),
        };
      });
    },
    [selectedEnvName],
  );

  const handleSave = useCallback(() => {
    if (!orgId || !projectId || !agentId || !configId || !config) return;

    const envMappings: Record<
      string,
      {
        providerName?: string;
        configuration: {
          policies?: {
            name: string;
            version: string;
            paths: {
              path: string;
              methods: string[];
              params: Record<string, unknown>;
            }[];
          }[];
        };
      }
    > = {};

    for (const [envName, mapping] of Object.entries(
      config.envMappings ?? {},
    )) {
      const pConfig = mapping.configuration;
      if (pConfig) {
        const envGuardrails = guardrailsByEnv[envName];
        if (envGuardrails !== undefined) {
          // Environment was edited — build policies from edited guardrails
          const envPolicies =
            envGuardrails.length > 0
              ? envGuardrails.map((g) => ({
                name: g.name,
                version: g.version,
                paths: [
                  {
                    path: "/*",
                    methods: ["*"],
                    params: g.settings ?? {},
                  },
                ],
              }))
              : undefined;
          envMappings[envName] = {
            providerName: pConfig.providerName,
            configuration: { policies: envPolicies },
          };
        } else {
          // Environment not loaded — preserve original policies intact
          envMappings[envName] = {
            providerName: pConfig.providerName,
            configuration: {
              policies: pConfig.policies?.map((p) => ({
                name: p.name,
                version: p.version,
                paths: p.paths.map((pp) => ({
                  path: pp.path,
                  methods: pp.methods,
                  params: pp.params ?? {},
                })),
              })),
            },
          };
        }
      }
    }

    updateConfig.mutate(
      {
        params: {
          orgName: orgId,
          projName: projectId,
          agentName: agentId,
          configId,
        },
        body: {
          name: name.trim(),
          description: description.trim() || undefined,
          envMappings,
          environmentVariables: Object.keys(envVarNames).length > 0
            ? Object.entries(envVarNames).map(([key, n]) => ({
              key,
              name: n.trim(),
            }))
            : undefined,
        },
      },
      { onSuccess: () => navigate(backHref) },
    );
  }, [
    orgId,
    projectId,
    agentId,
    configId,
    config,
    name,
    description,
    guardrailsByEnv,
    envVarNames,
    updateConfig,
    navigate,
    backHref,
  ]);

  if (isLoading) {
    return (
      <PageLayout
        title="LLM Provider Configuration"
        backHref={backHref}
        disableIcon
        backLabel="Back to Configure"
      >
        <Stack spacing={2}>
          <Skeleton variant="rounded" height={56} />
          <Skeleton variant="rounded" height={56} />
          <Skeleton variant="rounded" height={120} />
        </Stack>
      </PageLayout>
    );
  }

  if (isError || !config) {
    return (
      <PageLayout
        title="LLM Provider Configuration"
        backHref={backHref}
        disableIcon
        backLabel="Back to Configuration Listing"
      >
        <Alert severity="error" icon={<AlertTriangle size={18} />}>
          Configuration not found or failed to load.
        </Alert>
      </PageLayout>
    );
  }

  const apiKeyValue = providerConfig?.authInfo?.value;

  return (
    <PageLayout
      title={config.name}
      backHref={backHref}
      disableIcon
      backLabel="Back to Configuration Listing"
    >
      {config.description && (
        <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
          {config.description}
        </Typography>
      )}

      <Stack spacing={3}>
        {updateConfig.isError && (
          <Alert
            severity="error"
            icon={<AlertTriangle size={18} />}
            onClose={() => updateConfig.reset()}
          >
            {updateConfig.error instanceof Error
              ? updateConfig.error.message
              : "Failed to update configuration. Please try again."}
          </Alert>
        )}

        {!isExternal && config.environmentVariables?.length > 0 && (
          <Alert severity="info" sx={{ mt: 2 }}>
            <Typography variant="body2" fontWeight={600} sx={{ mb: 1 }}>
              Environment Variables References
            </Typography>
            <Typography variant="body2" sx={{ mb: 2 }}>
              The following environment variables will be injected into
              the agent deployment with environment-specific values.
              If your code already uses different variable names,
              you can update them below to ensure compatibility.
            </Typography>

            <Stack spacing={1}>
              <ListingTable.Container>
                <ListingTable density="compact">
                  <ListingTable.Head>
                    <ListingTable.Row>
                      <ListingTable.Cell>Variable Name</ListingTable.Cell>
                      <ListingTable.Cell>Description</ListingTable.Cell>
                    </ListingTable.Row>
                  </ListingTable.Head>
                  <ListingTable.Body>
                    {config.environmentVariables.map((envVar) => (
                      <ListingTable.Row key={envVar.key}>
                        <ListingTable.Cell>
                          <TextInput
                            value={envVarNames[envVar.key] ?? envVar.name}
                            onChange={(e) =>
                              setEnvVarNames((prev) => ({
                                ...prev,
                                [envVar.key]: e.target.value,
                              }))
                            }
                            copyable
                            copyTooltipText={`Copy ${envVarNames[envVar.key] ?? envVar.name}`}
                            size="small"
                          />
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          <Typography variant="body2" color="text.secondary">
                            {generateDisplayName(envVar.key)}
                          </Typography>
                        </ListingTable.Cell>
                      </ListingTable.Row>
                    ))}
                  </ListingTable.Body>
                </ListingTable>
              </ListingTable.Container>
              <ToggleButtonGroup
                size="small"
                value={snippetTab}
                exclusive
                color="primary"
                onChange={(_, v: number | null) => { if (v !== null) setSnippetTab(v); }}
                sx={{ mb: 2 }}
              >
                <ToggleButton value={0} sx={{ textTransform: "none" }}>Python</ToggleButton>
                <ToggleButton value={1} sx={{ textTransform: "none" }}>AI Prompt</ToggleButton>
              </ToggleButtonGroup>

              {snippetTab === 0 && (
                <TextInput
                  label="Python Code Snippet"
                  value={(() => {
                    const clientSetup = getClientSetupSnippet(
                      catalogProvider?.template,
                      config.environmentVariables.map((ev) => ev.key),
                    );
                    const imports = ["import os"];
                    if (clientSetup) imports.push(clientSetup.importLine);
                    const envVars = config.environmentVariables.map(
                      (envVar) =>
                        `${envVar.key} = os.environ.get('${envVarNames[envVar.key] ?? envVar.name}')`,
                    );
                    const parts = [imports.join("\n"), "", ...envVars];
                    if (clientSetup) parts.push("", clientSetup.setup);
                    return parts.join("\n");
                  })()}
                  copyable
                  copyTooltipText="Copy Code Snippet"
                  slotProps={{
                    input: {
                      sx: { fontFamily: "Source Code Pro, monospace" },
                      readOnly: true,
                      multiline: true,
                      rows: Math.min(
                        config.environmentVariables.length + 8,
                        15,
                      ),
                    },
                  }}
                  size="small"
                />
              )}

              {snippetTab === 1 && (
                <TextInput
                  label="AI Prompt — Update Your Code"
                  value={(() => {
                    const providerName = templateDisplayName
                      ?? catalogProvider?.name
                      ?? providerConfig?.providerName
                      ?? "the LLM provider";
                    const envVarList = config.environmentVariables
                      .map(
                        (ev) =>
                          `- ${generateDisplayName(ev.key)}: \`${envVarNames[ev.key] ?? ev.name}\``,
                      )
                      .join("\n");
                    return [
                      `Update my code to use ${providerName}.`,
                      "",
                      "Environment variables that will be injected at runtime:",
                      envVarList,
                      "",
                      "Requirements:",
                      `- Read the environment variables listed above to configure the client for ${providerName}.`,
                      `- Initialize the ${providerName} client with the URL and API key from those variables.`,
                      "- Do not hardcode any secrets or endpoint URLs.",
                      "- Keep the rest of my code unchanged.",
                    ].join("\n");
                  })()}
                  copyable
                  copyTooltipText="Copy Prompt"
                  slotProps={{
                    input: {
                      readOnly: true,
                      multiline: true,
                      rows: 10,
                    },
                  }}
                  size="small"
                />
              )}
            </Stack>
          </Alert>
        )}

        <Form.Section>
          <Stack spacing={3}>

            {
              environments.length > 1 && (
                <Tabs
                  value={selectedEnvIndex}
                  onChange={(_, v: number) => setSelectedEnvIndex(v)}
                  sx={{ mb: 2 }}
                >
                  {environments.map((enTab, idx) => (
                    <Tab
                      key={enTab.name}
                      label={enTab.displayName ?? enTab.name}
                      value={idx}
                    />
                  ))}
                </Tabs>
              )
            }

            {providerConfig && isExternal && (
              <Form.Section>
                <Form.Header>Connect to your LLM Provider</Form.Header>
                <>
                  {
                    !authInfoByEnv?.[selectedEnvName] && (
                      <>
                        <Alert severity="info" sx={{ mb: 1 }}>
                          <Typography variant="body2">
                            The credentials for this provider were issued during initial
                            setup. To route your agent&apos;s traffic through the
                            governance layer, configure your client with the provided
                            endpoint and API key.
                          </Typography>
                          <Typography
                            variant="body2"
                            sx={{ mt: 1, fontWeight: 600 }}
                          >
                            Security Reminder: Credentials are only displayed once at
                            creation time.
                          </Typography>
                        </Alert>
                      </>
                    )
                  }

                  {authInfoByEnv?.[selectedEnvName] && (
                    <>
                      <Alert severity="info" sx={{ mb: 1 }}>
                        <Typography variant="body2">
                          To route your agent&apos;s llm traffic through the governance layer,
                          configure your client with the credentials below.
                        </Typography>
                        <Typography
                          variant="body2"
                          sx={{ mt: 1, fontWeight: 600 }}
                        >
                          Make sure to copy your API key now as
                          you will not be able to see it again.
                        </Typography>
                      </Alert>
                    </>
                  )}
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


                  {authInfoByEnv?.[selectedEnvName] && (
                    <TextInput
                      label="Header Name"
                      value={authInfoByEnv[selectedEnvName].name}
                      copyable
                      copyTooltipText="Copy Header Name"
                      slotProps={{ input: { readOnly: true } }}
                      size="small"
                    />
                  )}
                  {authInfoByEnv?.[selectedEnvName].value && (
                    <TextInput
                      label="API Key"
                      type="password"
                      value={authInfoByEnv[selectedEnvName].value}
                      copyable
                      copyTooltipText="Copy API Key"
                      slotProps={{ input: { readOnly: true } }}
                      size="small"
                    />
                  )}

                  {apiKeyValue && (
                    <TextInput
                      label="API Key"
                      type="password"
                      value={apiKeyValue}
                      copyable
                      copyTooltipText="Copy API Key"
                      slotProps={{ input: { readOnly: true } }}
                      size="small"
                    />
                  )}
                  {
                    authInfoByEnv && (
                      <TextInput
                        label="Example cURL"
                        value={[
                          `curl -X POST ${providerConfig.url || "http://<endpoint-url>"}`,
                          `  --header "${authInfoByEnv[selectedEnvName].name}: ${authInfoByEnv[selectedEnvName].value || "<api-key>"}"`,
                          `  -d '{"your": "data"}'`,
                        ].join(" \\\n")}
                        copyable
                        copyTooltipText="Copy cURL command"
                        multiline
                        minRows={3}
                        slotProps={{
                          input: {
                            readOnly: true,
                            sx: { fontFamily: "monospace", fontSize: "0.85rem" },
                          },
                        }}
                        size="small"
                      />
                    )
                  }
                </>
              </Form.Section>
            )}

            {providerConfig && (
              <Form.Section>
                <Form.Header>
                  LLM Service Provider
                </Form.Header>
                <Stack spacing={2.5}>
                  <Card>
                    <CardContent>
                      <ProviderDisplay
                        provider={
                          catalogProvider
                            ? {
                              name: catalogProvider.name ?? providerConfig.providerName ?? "",
                              template: catalogProvider.template,
                              version: catalogProvider.version,
                              deployments: catalogProvider.deployments,
                              security: catalogProvider.security,
                              rateLimiting: catalogProvider.rateLimiting,
                              policies: catalogProvider.policies,
                            }
                            : {
                              name: providerConfig.providerName ?? "",
                            }
                        }
                        isSelected={false}
                        hideCheckbox
                        templateInfo={
                          catalogProvider?.template
                            ? {
                              displayName: templateDisplayName ??
                                catalogProvider.template, logoUrl: templateLogo
                            }
                            : null
                        }
                      />
                    </CardContent>
                  </Card>
                </Stack>
              </Form.Section>
            )}


            <GuardrailsSection
              guardrails={guardrails}
              onAddGuardrail={handleAddGuardrail}
              onRemoveGuardrail={handleRemoveGuardrail}
            />



          </Stack>
        </Form.Section>
        {
          isDirty && (
            <Stack direction="row" spacing={2}>
              <Button variant="outlined" onClick={() => navigate(backHref)}>
                Cancel
              </Button>
              <Button
                variant="contained"
                onClick={handleSave}
                disabled={!name.trim() || updateConfig.isPending}
              >
                {updateConfig.isPending ? "Saving…" : "Save"}
              </Button>
            </Stack>
          )
        }
      </Stack>

    </PageLayout>
  );
};

export default ViewLLMProviderComponent;
