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
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  PageLayout,
} from "@agent-management-platform/views";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Chip,
  Form,
  FormControl,
  FormLabel,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  Check,
  Circle,
  DoorClosedLocked,
} from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  useCreateAgentModelConfig,
  useGetAgentModelConfig,
  useListCatalogLLMProviders,
  useListEnvironments,
  useUpdateAgentModelConfig,
} from "@agent-management-platform/api-client";
import {
  GuardrailsSection,
  type GuardrailSelection,
} from "@agent-management-platform/llm-providers";

type DeploymentSummary = { deployedAt?: string };

function getLatestDeployment(
  deployments: DeploymentSummary[] | undefined,
): DeploymentSummary | null {
  if (!deployments?.length) return null;
  const sorted = [...deployments].sort(
    (a, b) =>
      new Date(b.deployedAt ?? 0).getTime() -
      new Date(a.deployedAt ?? 0).getTime(),
  );
  return sorted[0] ?? null;
}

const ProviderDisplay: React.FC<{
  provider: {
    name: string;
    template?: string;
    version?: string;
    deployments?: DeploymentSummary[];
  } | null;
  isSelected: boolean;
  fallbackLabel?: string;
}> = ({ provider, isSelected, fallbackLabel = "Select provider" }) => {
  const latest = getLatestDeployment(provider?.deployments);
  return (
    <Stack direction="row" spacing={2} alignItems="center">
      <Avatar
        sx={{
          height: 32,
          width: 32,
          backgroundColor: isSelected ? "primary.main" : "secondary.main",
          color: isSelected ? "common.white" : "text.secondary",
        }}
      >
        {isSelected ? <Check size={16} /> : <Circle size={16} />}
      </Avatar>
      <Stack spacing={0.5}>
        <Stack direction="row" spacing={0.5} alignItems="center">
          <Typography variant="body2" fontWeight={500}>
            {provider?.name ?? fallbackLabel}
          </Typography>
          {provider?.template && (
            <Chip label={provider.template} size="small" variant="outlined" />
          )}
          {provider?.version && (
            <Chip label={provider.version} size="small" variant="outlined" />
          )}
        </Stack>
        {latest?.deployedAt && (
          <Typography variant="caption" color="text.secondary">
            Deployed{" "}
            {formatDistanceToNow(new Date(latest.deployedAt), {
              addSuffix: true,
            })}
          </Typography>
        )}
      </Stack>
    </Stack>
  );
};

export const AddLLMProviderComponent: React.FC = () => {
  const { orgId, projectId, agentId, configId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    configId?: string;
  }>();
  const navigate = useNavigate();
  const isEditMode = !!configId;

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [selectedEnvIndex, setSelectedEnvIndex] = useState(0);
  const [providerByEnv, setProviderByEnv] = useState<
    Record<string, string | null>
  >({});
  const [guardrails, setGuardrails] = useState<GuardrailSelection[]>([]);
  const [providerDrawerOpen, setProviderDrawerOpen] = useState(false);

  const backHref =
    orgId && projectId && agentId
      ? generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents
          .children.configure.path,
        { orgId, projectId, agentId },
      )
      : "#";

  const { data: environments = [] } = useListEnvironments({
    orgName: orgId,
  });
  const { data: catalogData } = useListCatalogLLMProviders(
    { orgName: orgId },
    { limit: 50 },
  );
  const providers = useMemo(
    () =>
      (catalogData?.entries ?? []).map((e) => {
        const entry = e as { deployments?: DeploymentSummary[] };
        return {
          uuid: e.uuid,
          id: e.handle,
          name: e.name,
          version: e.version,
          template: e.template,
          deployments: entry.deployments ?? [],
        };
      }),
    [catalogData],
  );

  const {
    data: existingConfig,
    isLoading: isLoadingConfig,
    isError: isConfigError,
  } = useGetAgentModelConfig({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
    configId: configId ?? undefined,
  });

  useEffect(() => {
    if (!existingConfig || !isEditMode) return;
    setName(existingConfig.name);
    setDescription(existingConfig.description ?? "");
    const nextProviderByEnv: Record<string, string | null> = {};
    for (const [envName, mapping] of Object.entries(
      existingConfig.envMappings ?? {},
    )) {
      const config = mapping.configuration;
      const providerUuid =
        config?.providerUuid ?? config?.proxyUuid ?? undefined;
      if (providerUuid) {
        nextProviderByEnv[envName] = providerUuid;
      }
    }
    setProviderByEnv(nextProviderByEnv);
    const policies = Object.values(existingConfig.envMappings ?? {}).flatMap(
      (m) => m.configuration?.policies ?? [],
    );
    const seen = new Set<string>();
    const nextGuardrails: GuardrailSelection[] = [];
    for (const p of policies) {
      const key = `${p.name}@${p.version}`;
      if (seen.has(key)) continue;
      seen.add(key);
      const params = p.paths?.[0]?.params;
      nextGuardrails.push({
        name: p.name,
        version: p.version,
        settings: (params ?? {}) as Record<string, unknown>,
      });
    }
    setGuardrails(nextGuardrails);
  }, [existingConfig, isEditMode]);

  const createConfig = useCreateAgentModelConfig();
  const updateConfig = useUpdateAgentModelConfig();

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

  const handleAddGuardrail = useCallback((guardrail: GuardrailSelection) => {
    setGuardrails((prev) => {
      if (
        prev.some(
          (g) => g.name === guardrail.name && g.version === guardrail.version,
        )
      )
        return prev;
      return [...prev, guardrail];
    });
  }, []);

  const handleRemoveGuardrail = useCallback(
    (gName: string, gVersion: string) => {
      setGuardrails((prev) =>
        prev.filter((g) => !(g.name === gName && g.version === gVersion)),
      );
    },
    [],
  );

  const handleSave = useCallback(() => {
    const trimmedName = name.trim();
    if (!trimmedName) {
      return;
    }

    const envMappings: Record<
      string,
      {
        providerName?: string;
        configuration: { policies?: typeof policies };
      }
    > = {};
    let hasAtLeastOneProvider = false;

    for (const env of environments) {
      const providerUuid = providerByEnv[env.name] ?? null;
      if (providerUuid) {
        const provider = providers.find((p) => p.uuid === providerUuid);
        if (provider) {
          hasAtLeastOneProvider = true;
          envMappings[env.name] = {
            providerName: provider.id,
            configuration: {
              policies: policies.length > 0 ? policies : undefined,
            },
          };
        } else if (isEditMode && existingConfig) {
          // Provider not in current catalog page — preserve existing mapping
          // to avoid dropping providers beyond the catalog page limit.
          const existingMapping = existingConfig.envMappings?.[env.name];
          const existingProviderName =
            existingMapping?.configuration?.providerName;
          if (existingProviderName) {
            hasAtLeastOneProvider = true;
            envMappings[env.name] = {
              providerName: existingProviderName,
              configuration: {
                policies: policies.length > 0 ? policies : undefined,
              },
            };
          }
        }
      }
    }

    if (!hasAtLeastOneProvider) {
      return;
    }

    if (!orgId || !projectId || !agentId) {
      return;
    }

    const body = {
      name: trimmedName,
      description: description.trim() || undefined,
      envMappings,
    };

    if (isEditMode && configId) {
      updateConfig.mutate(
        {
          params: {
            orgName: orgId,
            projName: projectId,
            agentName: agentId,
            configId,
          },
          body,
        },
        {
          onSuccess: () => {
            navigate(backHref);
          },
        },
      );
    } else {
      createConfig.mutate(
        {
          params: {
            orgName: orgId,
            projName: projectId,
            agentName: agentId,
          },
          body: { ...body, type: "llm" as const },
        },
        {
          onSuccess: (data) => {
            // Collect authInfo from all env mappings to pass via router state
            const authInfoByEnv: Record<string,
              { type: string; in: string; name: string; value?: string }> = {};
            for (const [envName, mapping] of Object.entries(data.envMappings ?? {})) {
              if (mapping.configuration?.authInfo) {
                authInfoByEnv[envName] = mapping.configuration.authInfo;
              }
            }
            navigate(
              generatePath(
                absoluteRouteMap.children.org.children.projects.children.agents
                  .children.llmProviders.children.view.path,
                { orgId, projectId, agentId, configId: data.uuid },
              ),
              {
                state: { authInfoByEnv },
              },
            );
          },
        },
      );
    }
  }, [
    name,
    description,
    providerByEnv,
    environments,
    providers,
    policies,
    orgId,
    projectId,
    agentId,
    configId,
    isEditMode,
    existingConfig,
    createConfig,
    updateConfig,
    navigate,
    backHref,
  ]);

  const isFormValid =
    name.trim().length > 0 &&
    environments.some((env) => {
      const uuid = providerByEnv[env.name];
      if (!uuid) return false;
      if (providers.some((p) => p.uuid === uuid)) return true;
      // In edit mode, accept providers from the existing config even if not in catalog page
      if (isEditMode && existingConfig) {
        const existing = existingConfig.envMappings?.[env.name];
        return !!existing?.configuration?.providerName;
      }
      return false;
    });

  const mutationError = createConfig.isError
    ? createConfig.error
    : updateConfig.error;
  const isPending = createConfig.isPending || updateConfig.isPending;
  const resetMutation = useCallback(() => {
    createConfig.reset();
    updateConfig.reset();
  }, [createConfig, updateConfig]);

  const selectedEnvName = useMemo(
    () => environments[selectedEnvIndex]?.name ?? "",
    [environments, selectedEnvIndex],
  );

  if (isEditMode && isLoadingConfig) {
    return (
      <PageLayout
        title="Edit LLM Provider"
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

  if (isEditMode && !isLoadingConfig && (isConfigError || !existingConfig)) {
    return (
      <PageLayout
        title="Edit LLM Provider"
        backHref={backHref}
        disableIcon
        backLabel="Back to Configure"
      >
        <Alert severity="error" icon={<AlertTriangle size={18} />}>
          Config not found or failed to load.
        </Alert>
      </PageLayout>
    );
  }

  return (
    <PageLayout
      title={isEditMode ? "Edit LLM Provider" : "Add LLM Provider"}
      backHref={backHref}
      disableIcon
      backLabel="Back to Configure"
    >
      <Stack spacing={3}>
        {mutationError ? (
          <Alert
            severity="error"
            icon={<AlertTriangle size={18} />}
            onClose={resetMutation}
          >
            {String(
              mutationError instanceof Error
                ? mutationError.message
                : isEditMode
                  ? "Failed to update model config. Please try again."
                  : "Failed to create model config. Please try again.",
            )}
          </Alert>
        ) : null}
        <Form.Section>
          <Form.Header>Basic Details</Form.Header>
          <Form.Stack spacing={2}>
            <FormControl fullWidth>
              <FormLabel>Name</FormLabel>
              <TextField
                fullWidth
                size="small"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. OpenAI GPT5"
              />
            </FormControl>
            <FormControl fullWidth>
              <FormLabel>Description</FormLabel>
              <TextField
                fullWidth
                size="small"
                multiline
                minRows={3}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="Describe the LLM provider"
              />
            </FormControl>
          </Form.Stack>
        </Form.Section>

        <Form.Section>
          <Form.Header>LLM Model provider</Form.Header>
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
          <FormControl fullWidth size="small">
            <FormLabel>Select Provider</FormLabel>
            {catalogData && providers.length === 0 && (
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ display: "block", mt: 0.5, mb: 1 }}
              >
                No providers in catalog. Add LLM providers to the catalog from
                the LLM Providers page first.
              </Typography>
            )}
            <Form.CardButton
              onClick={() =>
                providers.length > 0 && setProviderDrawerOpen(true)
              }
              selected={!!providerByEnv[selectedEnvName]}
              disabled={providers.length === 0}
              aria-label={
                providerByEnv[selectedEnvName]
                  ? `Selected: ${providers.find((p) => p.uuid === providerByEnv[selectedEnvName])?.name ?? "Unknown"}. Click to change.`
                  : "Select provider. Click to open provider list."
              }
            >
              <Form.CardContent>
                <ProviderDisplay
                  provider={
                    providerByEnv[selectedEnvName]
                      ? providers.find(
                        (p) => p.uuid === providerByEnv[selectedEnvName],
                      ) ?? null
                      : null
                  }
                  isSelected={!!providerByEnv[selectedEnvName]}
                />
              </Form.CardContent>
            </Form.CardButton>
          </FormControl>

          <DrawerWrapper
            open={providerDrawerOpen}
            onClose={() => setProviderDrawerOpen(false)}
            minWidth={420}
            maxWidth={420}
          >
            <DrawerHeader
              icon={<DoorClosedLocked size={24} />}
              title="Select Provider"
              onClose={() => setProviderDrawerOpen(false)}
            />
            <DrawerContent>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                Choose a provider for this environment.
              </Typography>
              <Stack spacing={1} sx={{ flex: 1, overflowY: "auto" }}>
                {providers.map((p) => {
                  const isSelected = providerByEnv[selectedEnvName] === p.uuid;
                  return (
                    <Form.CardButton
                      key={p.uuid}
                      onClick={() => {
                        setProviderByEnv((prev) => ({
                          ...prev,
                          [selectedEnvName]: p.uuid,
                        }));
                        setProviderDrawerOpen(false);
                      }}
                      selected={isSelected}
                      aria-label={`${p.name}. ${isSelected ? "Selected" : "Click to select"}`}
                    >
                      <Form.CardContent>
                        <ProviderDisplay
                          provider={p}
                          isSelected={isSelected}
                        />
                      </Form.CardContent>
                    </Form.CardButton>
                  );
                })}
              </Stack>
            </DrawerContent>
          </DrawerWrapper>
          <GuardrailsSection
            guardrails={guardrails}
            onAddGuardrail={handleAddGuardrail}
            onRemoveGuardrail={handleRemoveGuardrail}
          />
        </Form.Section>

        {/* Actions */}
        <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
          <Button variant="outlined" onClick={() => navigate(backHref)}>
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleSave}
            disabled={!isFormValid || isPending}
          >
            {isPending ? "Saving…" : "Save"}
          </Button>
        </Box>
      </Stack>
    </PageLayout>
  );
};

export default AddLLMProviderComponent;
