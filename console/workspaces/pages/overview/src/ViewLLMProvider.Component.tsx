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
  Box,
  Button,
  Form,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle } from "@wso2/oxygen-ui-icons-react";
import { CodeBlock } from "@agent-management-platform/shared-component";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  useGetAgentModelConfig,
  useListEnvironments,
  useUpdateAgentModelConfig,
} from "@agent-management-platform/api-client";
import {
  GuardrailsSection,
  type GuardrailSelection,
} from "@agent-management-platform/llm-providers";

export const ViewLLMProviderComponent: React.FC = () => {
  const { orgId, projectId, agentId, configId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    configId: string;
  }>();
  const navigate = useNavigate();

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [selectedEnvIndex, setSelectedEnvIndex] = useState(0);
  const [guardrailsByEnv, setGuardrailsByEnv] = useState<
    Record<string, GuardrailSelection[]>
  >({});

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

  const updateConfig = useUpdateAgentModelConfig();

  useEffect(() => {
    if (!config) return;
    setName(config.name);
    setDescription(config.description ?? "");

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

  const guardrails = useMemo(
    () => guardrailsByEnv[selectedEnvName] ?? [],
    [guardrailsByEnv, selectedEnvName],
  );

  const policies = useMemo(
    () =>
      guardrails.map((g) => ({
        name: g.name,
        version: g.version,
        paths: [
          {
            path: "/*",
            methods: ["*"],
            params: g.settings ?? {},
          },
        ],
      })),
    [guardrails],
  );

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
        configuration: { policies?: typeof policies };
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
            configuration: { policies: pConfig.policies },
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
        backLabel="Back to Configure"
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
      backLabel="Back to Configure"
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

        <Form.Section>
          <Form.Header>LLM Providers</Form.Header>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Select an LLM Model provider per environment and specific guardrails
          </Typography>

          <Tabs
            value={selectedEnvIndex}
            onChange={(_, v: number) => setSelectedEnvIndex(v)}
            sx={{ mb: 2 }}
          >
            {environments.map((env, idx) => (
              <Tab
                key={env.name}
                label={env.displayName ?? env.name}
                value={idx}
              />
            ))}
          </Tabs>

          {providerConfig ? (
            <Stack spacing={2}>
              <Alert severity="info" sx={{ mb: 1 }}>
                <Typography variant="body2">
                  To route your agent&apos;s interactions through our governance
                  layer, use the credentials below in your client configuration.
                </Typography>
                <Typography
                  variant="body2"
                  sx={{ mt: 1, fontWeight: 600 }}
                >
                  Security Reminder: Treat your API Key like a password. Copy it
                  now and store it in a secure environment variable—it will not
                  be shown again.
                </Typography>
              </Alert>

              <TextInput
                label="Endpoint URL"
                value={providerConfig.url ?? ""}
                copyable
                copyTooltipText="Copy Endpoint URL"
                slotProps={{ input: { readOnly: true } }}
                size="small"
              />

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

              <Stack direction="row" spacing={1} alignItems="center">
                <Typography variant="body2" fontWeight={600}>
                  Provider
                </Typography>
                <Typography variant="body2">
                  {providerConfig.providerName}
                </Typography>
              </Stack>

              {config.environmentVariables?.length > 0 && (
                <Alert severity="warning" sx={{ mt: 2 }}>
                  <Typography variant="body2" fontWeight={600} sx={{ mb: 1 }}>
                    Environment Variables References
                  </Typography>
                  <Typography variant="body2" sx={{ mb: 2 }}>
                    The following environment variables will be applied during
                    deployment. If your code already uses different variables,
                    please update them below to ensure compatibility.
                  </Typography>
                  <Stack direction="row" spacing={3}>
                    <Stack spacing={1} sx={{ flex: 1 }}>
                      {config.environmentVariables.map((envVar) => (
                        <TextInput
                          key={envVar.key}
                          label={envVar.key}
                          value={envVar.name}
                          copyable
                          copyTooltipText={`Copy ${envVar.name}`}
                          slotProps={{ input: { readOnly: true } }}
                          size="small"
                        />
                      ))}
                    </Stack>
                    <Box sx={{ flex: 1 }}>
                      <CodeBlock
                        code={`import os\n\n${config.environmentVariables
                          .map(
                            (envVar) =>
                              `${envVar.key} = os.environ.get('${envVar.name}')`,
                          )
                          .join("\n")}`}
                        language="python"
                      />
                    </Box>
                  </Stack>
                </Alert>
              )}
            </Stack>
          ) : (
            <Typography variant="body2" color="text.secondary">
              No provider configured for this environment.
            </Typography>
          )}

          <GuardrailsSection
            guardrails={guardrails}
            onAddGuardrail={handleAddGuardrail}
            onRemoveGuardrail={handleRemoveGuardrail}
          />
        </Form.Section>

        <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-start" }}>
          <Button variant="outlined" onClick={() => navigate(backHref)}>
            Cancel
          </Button>
          <Button
            variant="contained"
            color="error"
            onClick={handleSave}
            disabled={!name.trim() || updateConfig.isPending}
          >
            {updateConfig.isPending ? "Saving…" : "Save"}
          </Button>
        </Box>
      </Stack>
    </PageLayout>
  );
};

export default ViewLLMProviderComponent;
