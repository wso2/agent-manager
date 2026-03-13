/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
import { PageLayout } from "@agent-management-platform/views";
import {
  Alert,
  Box,
  Button,
  Form,
  FormControl,
  FormLabel,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, DoorClosedLocked } from "@wso2/oxygen-ui-icons-react";
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
    { limit: 45 },
  );
  const providers = useMemo(
    () =>
      (catalogData?.entries ?? []).map((e) => ({
        uuid: e.uuid,
        id: e.handle,
        name: e.name,
      })),
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
      const proxyUuid = config?.proxyUuid;
      if (proxyUuid) {
        nextProviderByEnv[envName] = proxyUuid;
      }
    }
    setProviderByEnv(nextProviderByEnv);
    const policies = Object.values(existingConfig.envMappings ?? {})
      .flatMap((m) => m.configuration?.policies ?? []);
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
        providerUuid?: string;
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
            providerUuid: provider.uuid,
            configuration: { policies: policies.length > 0 ? policies : undefined },
          };
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
          onSuccess: () => {
            navigate(backHref);
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
    createConfig,
    updateConfig,
    navigate,
    backHref,
  ]);

  const isFormValid =
    name.trim().length > 0 &&
    environments.some((env) => {
      const uuid = providerByEnv[env.name];
      return !!uuid && providers.some((p) => p.uuid === uuid);
    });

  const mutationError = createConfig.isError ? createConfig.error : updateConfig.error;
  const isPending = createConfig.isPending || updateConfig.isPending;
  const resetMutation = () => {
    createConfig.reset();
    updateConfig.reset();
  };

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
            <Select
              disabled={providers.length === 0}
              value={
                selectedEnvIndex < environments.length
                  ? providerByEnv[
                      environments[selectedEnvIndex]?.name ?? ""
                    ] ?? ""
                  : ""
              }
              onChange={(e) => {
                const envName = environments[selectedEnvIndex]?.name ?? "";
                setProviderByEnv((prev) => ({
                  ...prev,
                  [envName]: e.target.value || null,
                }));
              }}
              displayEmpty
              renderValue={(value) => {
                const provider = providers.find(
                  (p) => p.uuid === value || p.id === value,
                );
                return (
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      gap: 1,
                    }}
                  >
                    <DoorClosedLocked size={20} />
                    <Typography variant="body2">
                      {provider?.name ?? "Select provider"}
                    </Typography>
                  </Box>
                );
              }}
            >
              <MenuItem value="">
                <em>Select provider</em>
              </MenuItem>
              {providers.map((p) => (
                <MenuItem key={p.uuid} value={p.uuid}>
                  {p.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <GuardrailsSection
            guardrails={guardrails}
            onAddGuardrail={handleAddGuardrail}
            onRemoveGuardrail={handleRemoveGuardrail}
          />
        </Form.Section>

        {/* Actions */}
        <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
          <Button
            variant="outlined"
            onClick={() => navigate(backHref)}
          >
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
