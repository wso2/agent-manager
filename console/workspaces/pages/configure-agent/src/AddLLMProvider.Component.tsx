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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { PageLayout, TextInput } from "@agent-management-platform/views";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Chip,
  Form,
  ListingTable,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  Check,
  Circle,
  Edit,
  Info,
  Link,
  Search,
  ServerCog,
} from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  type CatalogSecuritySummary,
  type CatalogRateLimitingSummary,
  type LLMPolicy,
} from "@agent-management-platform/types";
import {
  useCreateAgentModelConfig,
  useGetAgent,
  useGetAgentModelConfig,
  useListAgentModelConfigs,
  useListCatalogLLMProviders,
  useListLLMProviderTemplates,
  useUpdateAgentModelConfig,
} from "@agent-management-platform/api-client";
import {
  getErrorMessage,
  PolicyListSection,
  type PolicySelection as GuardrailSelection,
  usePipelineEnvironmentsState,
} from "@agent-management-platform/shared-component";
import { ProviderSelectDrawer } from "./ProviderSelectDrawer";
import { ConfigNameSection } from "./Configure/subComponents/ConfigNameSection";
import {
  ENV_VAR_KEYS,
  generateEnvVarNames,
  generateUniqueConfigName,
  type EnvVarKey,
} from "./utils/envConfig";

type DeploymentSummary = { gatewayName?: string; deployedAt?: string };

const ENV_VAR_DESCRIPTIONS: Record<EnvVarKey, string> = {
  url: "Base URL of the LLM provider",
  apikey: "API key for authenticating with the LLM provider",
};

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

function formatCost(amount: number): string {
  if (amount < 0.01) return `$${amount.toFixed(6)}`;
  return `$${amount.toFixed(2)}`;
}

function formatResetWindow(duration?: number, unit?: string): string {
  if (!unit) return "";
  const abbrev: Record<string, string> = {
    minute: "min",
    hour: "hr",
    day: "day",
  };
  const u = abbrev[unit.toLowerCase()] ?? unit;
  return duration && duration !== 1 ? `${duration} ${u}` : u;
}

const RateLimitDisplay: React.FC<{
  rateLimiting?: CatalogRateLimitingSummary;
}> = ({ rateLimiting }) => {
  if (!rateLimiting) {
    return (
      <Typography variant="caption" color="text.secondary">
        Rate Limiting:{" "}
        <Typography component="span" variant="caption" color="text.disabled">
          Not configured
        </Typography>
      </Typography>
    );
  }

  const cl = rateLimiting.consumerLevel;
  const pl = rateLimiting.providerLevel;

  // Whether the consumer level is actively enabled (rate limiting applies to this consumer)
  const consumerEnabled = cl?.globalEnabled ?? false;
  // Whether the consumer level has its own per-consumer numeric limits configured
  const consumerHasLimits =
    consumerEnabled &&
    (cl?.request != null || cl?.token != null || cl?.cost != null);

  if (!consumerEnabled && !pl?.globalEnabled) {
    return (
      <Typography variant="caption" color="text.secondary">
        Rate Limiting:{" "}
        <Typography component="span" variant="caption" color="text.disabled">
          Configured (disabled)
        </Typography>
      </Typography>
    );
  }

  // Use consumer-specific limits when set; otherwise fall back to provider limits for display
  const limitScope = consumerHasLimits ? cl : pl;
  // Org-wide whenever we fall back to provider-level limits (no per-consumer numeric overrides)
  const isOrgWide = !consumerHasLimits;

  const limits: { label: string; value: string }[] = [];
  if (limitScope?.request) {
    const w = formatResetWindow(
      limitScope.request.resetDuration,
      limitScope.request.resetUnit,
    );
    limits.push({
      label: "Requests",
      value: `${limitScope.request.limit.toLocaleString()}${w ? `/${w}` : ""}`,
    });
  }
  if (limitScope?.token) {
    const w = formatResetWindow(
      limitScope.token.resetDuration,
      limitScope.token.resetUnit,
    );
    limits.push({
      label: "Tokens",
      value: `${limitScope.token.limit.toLocaleString()}${w ? `/${w}` : ""}`,
    });
  }
  if (limitScope?.cost) {
    const w = formatResetWindow(
      limitScope.cost.resetDuration,
      limitScope.cost.resetUnit,
    );
    limits.push({
      label: "Budget",
      value: `${formatCost(limitScope.cost.limit)}${w ? `/${w}` : ""}`,
    });
  }

  return (
    <Stack
      direction="row"
      spacing={0.5}
      alignItems="center"
      flexWrap="wrap"
      useFlexGap
    >
      <Typography variant="caption" color="text.secondary">
        {isOrgWide ? "Your Quota (org-wide limit):" : "Your Quota:"}
      </Typography>
      {isOrgWide && (
        <Tooltip
          title="No per-consumer limit is configured. The org-wide provider limit applies to all consumers."
          placement="top"
          arrow
        >
          <Box
            component="span"
            sx={{
              display: "inline-flex",
              alignItems: "center",
              color: "text.secondary",
              cursor: "default",
            }}
          >
            <Info size={12} />
          </Box>
        </Tooltip>
      )}
      {limits.length > 0 ? (
        <Typography
          variant="caption"
          color={isOrgWide ? "text.secondary" : "text.primary"}
          sx={{ fontVariantNumeric: "tabular-nums" }}
        >
          {limits.map((l) => `${l.label}: ${l.value}`).join(" · ")}
        </Typography>
      ) : (
        <Typography variant="caption" color="text.secondary">
          Enabled (no numeric limits set)
        </Typography>
      )}
    </Stack>
  );
};

export const ProviderDisplay: React.FC<{
  provider: {
    name: string;
    template?: string;
    version?: string;
    deployments?: DeploymentSummary[];
    security?: CatalogSecuritySummary;
    rateLimiting?: CatalogRateLimitingSummary;
    policies?: string[];
  } | null;
  isSelected: boolean;
  hideCheckbox?: boolean;
  templateInfo?: { displayName: string; logoUrl?: string } | null;
  fallbackLabel?: string;
}> = ({
  provider,
  isSelected,
  templateInfo,
  fallbackLabel = "Select provider",
  hideCheckbox,
}) => {
    const latest = getLatestDeployment(provider?.deployments);
    const avatarSize = hideCheckbox ? 36 : 32;
    return (
      <Stack direction="row" spacing={2} flexGrow={1} alignItems="flex-start">
        {!hideCheckbox && (
          <Avatar
            sx={{
              height: avatarSize,
              width: avatarSize,
              backgroundColor: isSelected ? "primary.main" : "secondary.main",
              color: isSelected ? "common.white" : "text.secondary",
            }}
          >
            {isSelected ? <Check size={16} /> : <Circle size={16} />}
          </Avatar>
        )}
        {hideCheckbox && (
          <Avatar
            src={templateInfo?.logoUrl}
            sx={{ height: avatarSize, width: avatarSize, backgroundColor: "action.selected" }}
          >
            <ServerCog size={16} />
          </Avatar>
        )}

        <Stack spacing={0.5} flexGrow={1} sx={{ minWidth: 0 }}>
          {/* Title + provider template */}
          <Stack
            direction="row"
            spacing={0.75}
            alignItems="center"
            flexWrap="wrap"
            useFlexGap
            sx={{ minHeight: avatarSize }}
          >
            <Typography variant="h4">
              {provider?.name ?? fallbackLabel}
            </Typography>
            {provider?.template && (
              <Tooltip title="Service Provider template" placement="top" arrow>
                {hideCheckbox ?
                  (<Typography variant="caption" color="text.disabled">
                    | {templateInfo?.displayName ?? provider.template}
                  </Typography>) :
                  (<Chip
                    label={templateInfo?.displayName ?? provider.template}
                    size="small"
                    variant="outlined"
                    icon={
                      templateInfo?.logoUrl ? (
                        <Box
                          component="img"
                          src={templateInfo.logoUrl}
                          alt={templateInfo.displayName}
                          sx={{ width: 14, height: 14, borderRadius: "100%" }}
                        />
                      ) : undefined
                    }
                  />)}


              </Tooltip>
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

          <RateLimitDisplay rateLimiting={provider?.rateLimiting} />

          {/* Guardrails — plain inline text (full list shown on hover) */}
          <Typography variant="caption" color="text.secondary">
            Guardrails:{" "}
            {provider?.policies?.length ? (
              <Tooltip
                title={provider.policies.join(", ")}
                placement="top"
                arrow
              >
                <Typography component="span" variant="caption" color="text.primary">
                  {provider.policies.slice(0, 3).join(", ")}
                  {provider.policies.length > 3 &&
                    ` +${provider.policies.length - 3} more`}
                </Typography>
              </Tooltip>
            ) : (
              <Typography component="span" variant="caption" color="text.disabled">
                None
              </Typography>
            )}
          </Typography>
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

  const [selectedEnvIndex, setSelectedEnvIndex] = useState(0);
  type SelectedProvider = {
    uuid: string;
    id: string;
    template?: string;
    /** Catalog provider display name, used to seed the config name. */
    name?: string;
  };
  const [providerByEnv, setProviderByEnv] = useState<
    Record<string, SelectedProvider | null>
  >({});
  const [guardrailsByEnv, setGuardrailsByEnv] = useState<
    Record<string, GuardrailSelection[]>
  >({});
  const [envVarNames, setEnvVarNames] = useState<Record<string, string>>(() =>
    generateEnvVarNames(""),
  );
  // Track whether the user has manually edited env var names
  const envVarNamesEditedRef = useRef(false);
  const [providerDrawerOpen, setProviderDrawerOpen] = useState(false);
  // Config name: auto-populated from the selected provider, but editable — when
  // environments use different providers there is no single obvious default.
  const [configName, setConfigName] = useState("");
  const configNameEditedRef = useRef(false);

  const backHref =
    orgId && projectId && agentId
      ? generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents
          .children.configure.path,
        { orgId, projectId, agentId },
      )
      : "#";

  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const isExternal = agent?.provisioning?.type === "external";

  const { environments, isLoading: isLoadingEnvironments } =
    usePipelineEnvironmentsState(orgId, projectId);
  const { data: existingConfigsList } = useListAgentModelConfigs({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const selectedEnvironmentId = environments[selectedEnvIndex]?.id;
  const { data: catalogData } = useListCatalogLLMProviders(
    { orgName: orgId },
    { limit: 50, environmentId: selectedEnvironmentId },
  );
  const { data: templatesData } = useListLLMProviderTemplates({
    orgName: orgId,
  });
  const templateMap = useMemo(() => {
    const map = new Map<string, { displayName: string; logoUrl?: string }>();
    for (const t of templatesData?.templates ?? []) {
      map.set(t.name, { displayName: t.name, logoUrl: t.metadata?.logoUrl });
      map.set(t.id, { displayName: t.name, logoUrl: t.metadata?.logoUrl });
    }
    return map;
  }, [templatesData]);
  const providers = useMemo(
    () =>
      (catalogData?.entries ?? []).map((e) => ({
        uuid: e.uuid,
        id: e.handle,
        name: e.name,
        version: e.version,
        template: e.template,
        deployments: e.deployments ?? [],
        security: e.security,
        rateLimiting: e.rateLimiting,
        policies: e.policies ?? [],
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
    const nextProviderByEnv: Record<string, SelectedProvider | null> = {};
    for (const [envName, mapping] of Object.entries(
      existingConfig.envMappings ?? {},
    )) {
      const config = mapping.configuration;
      const providerUuid =
        config?.providerUuid ?? config?.proxyUuid ?? undefined;
      if (providerUuid && config?.providerName) {
        nextProviderByEnv[envName] = {
          uuid: providerUuid,
          id: config.providerName,
        };
      }
    }
    setProviderByEnv(nextProviderByEnv);
    const nextGuardrailsByEnv: Record<string, GuardrailSelection[]> = {};
    for (const [envName, mapping] of Object.entries(
      existingConfig.envMappings ?? {},
    )) {
      const envPolicies = mapping.configuration?.policies ?? [];
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
      nextGuardrailsByEnv[envName] = envGuardrails;
    }
    setGuardrailsByEnv(nextGuardrailsByEnv);

    // Populate env var names from the existing config
    if (existingConfig.environmentVariables?.length) {
      const names: Record<string, string> = {};
      for (const ev of existingConfig.environmentVariables) {
        names[ev.key] = ev.name;
      }
      setEnvVarNames(names);
      envVarNamesEditedRef.current = true;
    }

    // Populate the config name from the existing config (and treat it as
    // user-set so it isn't overwritten by the auto-name effect).
    if (existingConfig.name) {
      setConfigName(existingConfig.name);
      configNameEditedRef.current = true;
    }
  }, [existingConfig, isEditMode]);

  // Auto-generate env var names from the selected provider's template in create mode
  const primaryTemplate = useMemo(() => {
    const firstEnvName = environments[0]?.name;
    const entry = firstEnvName ? providerByEnv[firstEnvName] : undefined;
    return entry ? (entry.template ?? entry.id ?? "") : "";
  }, [providerByEnv, environments]);

  useEffect(() => {
    if (isEditMode || envVarNamesEditedRef.current) return;
    setEnvVarNames(generateEnvVarNames(primaryTemplate));
  }, [primaryTemplate, isEditMode]);

  // Suggested config name: a unique name derived from the display name of the
  // first environment that has a provider (what the user sees on the card),
  // falling back to the template/handle. Used both to auto-populate the field
  // and as the save-time fallback, so the rule lives in one place.
  const suggestedConfigName = useMemo(() => {
    const firstWithProvider = environments
      .map((env) => providerByEnv[env.name])
      .find(Boolean);
    const basis =
      firstWithProvider?.name ??
      firstWithProvider?.template ??
      firstWithProvider?.id ??
      "";
    if (!basis) return "";
    const existingNames = (existingConfigsList?.configs ?? []).map((c) => c.name);
    return generateUniqueConfigName(basis, "llm", existingNames);
  }, [environments, providerByEnv, existingConfigsList]);

  // Auto-populate the config name until the user renames it.
  useEffect(() => {
    if (isEditMode || configNameEditedRef.current) return;
    setConfigName(suggestedConfigName);
  }, [suggestedConfigName, isEditMode]);

  const selectedEnvName = useMemo(
    () => environments[selectedEnvIndex]?.name ?? "",
    [environments, selectedEnvIndex],
  );

  const createConfig = useCreateAgentModelConfig();
  const updateConfig = useUpdateAgentModelConfig();

  const guardrails = useMemo(
    () => guardrailsByEnv[selectedEnvName] ?? [],
    [guardrailsByEnv, selectedEnvName],
  );

  const handleAddGuardrail = useCallback(
    (guardrail: GuardrailSelection) => {
      setGuardrailsByEnv((prev) => {
        const list = prev[selectedEnvName] ?? [];
        const exists = list.some(
          (g) => g.name === guardrail.name && g.version === guardrail.version,
        );
        if (exists) return prev;
        return { ...prev, [selectedEnvName]: [...list, guardrail] };
      });
    },
    [selectedEnvName],
  );

  const handleEditGuardrail = useCallback(
    (guardrail: GuardrailSelection) => {
      setGuardrailsByEnv((prev) => {
        const list = prev[selectedEnvName] ?? [];
        const updated = list.map((g) =>
          g.name === guardrail.name && g.version === guardrail.version
            ? guardrail
            : g,
        );
        return { ...prev, [selectedEnvName]: updated };
      });
    },
    [selectedEnvName],
  );

  const handleRemoveGuardrail = useCallback(
    (gName: string, gVersion: string) => {
      setGuardrailsByEnv((prev) => {
        const list = prev[selectedEnvName] ?? [];
        const filtered = list.filter(
          (g) => !(g.name === gName && g.version === gVersion),
        );
        return { ...prev, [selectedEnvName]: filtered };
      });
    },
    [selectedEnvName],
  );

  const handleSave = useCallback(() => {
    const envMappings: Record<
      string,
      {
        providerName?: string;
        configuration: { policies?: LLMPolicy[] };
      }
    > = {};
    let hasAtLeastOneProvider = false;

    for (const env of environments) {
      const entry = providerByEnv[env.name];
      if (!entry) continue;

      hasAtLeastOneProvider = true;

      const envGuardrails = guardrailsByEnv[env.name] ?? [];
      const envPolicies = envGuardrails.map((g) => ({
        name: g.name,
        version: g.version,
        paths: [{ path: "/*", methods: ["*"], params: g.settings ?? {} }],
      }));
      envMappings[env.name] = {
        providerName: entry.id,
        configuration: {
          policies: envPolicies.length > 0 ? envPolicies : undefined,
        },
      };
    }

    if (!hasAtLeastOneProvider) {
      return;
    }

    if (!orgId || !projectId || !agentId) {
      return;
    }

    const environmentVariables = !isExternal
      ? ENV_VAR_KEYS.map((key) => ({
        key,
        name: (envVarNames[key] ?? "").trim(),
      })).filter((ev) => ev.name.length > 0)
      : [];

    // Prefer the (auto-populated, user-editable) config name; fall back to the
    // suggested name if the field was cleared.
    const fallbackName = isEditMode
      ? (existingConfig?.name ?? suggestedConfigName)
      : suggestedConfigName;
    const finalName = configName.trim() || fallbackName;

    const body = {
      name: finalName,
      envMappings,
      environmentVariables:
        environmentVariables.length > 0 ? environmentVariables : undefined,
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
            navigate(
              generatePath(
                absoluteRouteMap.children.org.children.projects.children.agents
                  .children.configure.children.llmProviders.children.view.path,
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
    providerByEnv,
    environments,
    guardrailsByEnv,
    envVarNames,
    configName,
    isExternal,
    orgId,
    projectId,
    agentId,
    configId,
    isEditMode,
    existingConfig,
    suggestedConfigName,
    createConfig,
    updateConfig,
    navigate,
    backHref,
  ]);

  const hasProviderForEnv = (envName: string) =>
    !!providerByEnv[envName] ||
    (isEditMode &&
      !!existingConfig?.envMappings?.[envName]?.configuration?.providerName);

  const hasAnyProvider = environments.some((env) =>
    hasProviderForEnv(env.name),
  );

  const allEnvsHaveProvider =
    environments.length > 0 &&
    environments.every((env) => hasProviderForEnv(env.name));
  const isFormValid = hasAnyProvider;

  const mutationError = createConfig.isError
    ? createConfig.error
    : updateConfig.error;
  const isPending = createConfig.isPending || updateConfig.isPending;
  const resetMutation = useCallback(() => {
    createConfig.reset();
    updateConfig.reset();
  }, [createConfig, updateConfig]);

  // Hold off rendering until the pipeline environments resolve (and, in edit
  // mode, the existing config). Otherwise the env tabs briefly show all org
  // environments before collapsing to the pipeline subset.
  if (isLoadingEnvironments || (isEditMode && isLoadingConfig)) {
    return (
      <PageLayout docs={["configureAgentLlm", "llmServiceProvider", "gateway"]}
        title={isEditMode ? "Edit LLM Configuration" : "Add LLM Configuration"}
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
      <PageLayout docs={["configureAgentLlm", "llmServiceProvider", "gateway"]}
        title="Edit LLM Configuration"
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
    <PageLayout docs={["configureAgentLlm", "llmServiceProvider", "gateway"]}
      title={isEditMode ? "Edit LLM Configuration" : "Add LLM Configuration"}
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
            {getErrorMessage(mutationError) ||
              (isEditMode
                ? "Failed to update model config. Please try again."
                : "Failed to create model config. Please try again.")}
          </Alert>
        ) : null}
        <Form.Section>
          <Form.Subheader>LLM Service Provider</Form.Subheader>
          {environments.length > 1 && (
            <>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                Select which LLM Service provider to use in each environment.
              </Typography>
              <Tabs
                value={selectedEnvIndex}
                onChange={(_, v: number) => setSelectedEnvIndex(v)}
                sx={{ borderBottom: 1, borderColor: "divider", mb: 2 }}
              >
                {environments.map((env, idx) => {
                  const hasProvider = hasProviderForEnv(env.name);
                  return (
                    <Tab
                      key={env.name}
                      label={
                        <Stack
                          direction="row"
                          spacing={0.5}
                          alignItems="center"
                        >
                          <span>{env.displayName ?? env.name}</span>
                          {!hasProvider && (
                            <Tooltip
                              title="No provider selected"
                              placement="top"
                              arrow
                            >
                              <Box
                                component="span"
                                sx={{
                                  width: 8,
                                  height: 8,
                                  borderRadius: "50%",
                                  bgcolor: "warning.main",
                                  display: "inline-block",
                                }}
                              />
                            </Tooltip>
                          )}
                        </Stack>
                      }
                      value={idx}
                    />
                  );
                })}
              </Tabs>
            </>
          )}

          {providerByEnv[selectedEnvName] ? (
            (() => {
              const selectedUuid = providerByEnv[selectedEnvName]?.uuid;
              const fullProvider =
                providers.find((p) => p.uuid === selectedUuid) ?? null;
              return (
                <Form.CardButton
                  onClick={() => setProviderDrawerOpen(true)}
                  selected
                  aria-label={`Selected: ${fullProvider?.name ?? "Unknown"}. Click to change.`}
                  sx={{ position: "relative" }}
                >
                  <Tooltip title="Change provider" placement="top" arrow>
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
                    <ProviderDisplay
                      provider={fullProvider}
                      isSelected={false}
                      hideCheckbox
                      templateInfo={templateMap.get(
                        fullProvider?.template ?? "",
                      )}
                    />
                  </Form.CardContent>
                </Form.CardButton>
              );
            })()
          ) : (
            <Box>
              {catalogData && providers.length === 0 ? (
                <ListingTable.Container>
                  <ListingTable.EmptyState
                    illustration={<Search size={64} />}
                    title="No service providers available"
                    description="No LLM service providers found in the catalog. Add LLM service providers from the organization LLM Service Providers page first."
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
                                  .llmProviders.children.add.path,
                                { orgId },
                              ),
                            )
                          }
                        >
                          Add LLM Service Provider
                        </Button>
                      ) : undefined
                    }
                  />
                </ListingTable.Container>
              ) : (
                <Box sx={{ pt: 1 }}>
                  <Button
                    variant="outlined"
                    onClick={() => setProviderDrawerOpen(true)}
                    disabled={providers.length === 0 || !selectedEnvName}
                    startIcon={<Link size={16} />}
                  >
                    Select a Service Provider
                  </Button>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ display: "block", mt: 1 }}
                  >
                    Selecting a provider will auto-generate environment variable
                    names below.
                  </Typography>
                </Box>
              )}
            </Box>
          )}

          <ProviderSelectDrawer
            open={providerDrawerOpen}
            onClose={() => setProviderDrawerOpen(false)}
            providers={providers}
            templateMap={templateMap}
            selectedUuid={providerByEnv[selectedEnvName]?.uuid ?? undefined}
            subtitle={
              environments.length > 1
                ? `Choose the catalog provider for the ${environments[selectedEnvIndex]?.displayName ?? environments[selectedEnvIndex]?.name ?? ""} environment.`
                : "Choose the catalog provider for this agent."
            }
            onSelect={(uuid) => {
              if (!selectedEnvName) return;
              const picked = providers.find((p) => p.uuid === uuid);
              if (!picked) return;
              setProviderByEnv((prev) => ({
                ...prev,
                [selectedEnvName]: {
                  uuid,
                  id: picked.id,
                  template: picked.template,
                  name: picked.name,
                },
              }));
              // A different provider's gateways may not support the guardrails
              // already selected for this env — clear them so only guardrails
              // valid for the newly-picked provider can be re-added and saved.
              setGuardrailsByEnv((prev) => ({
                ...prev,
                [selectedEnvName]: [],
              }));
            }}
          />
          {providerByEnv[selectedEnvName] && (
            <PolicyListSection
              providerId={providerByEnv[selectedEnvName]?.uuid}
              title="Guardrails"
              description="Add safety policies to enforce consistent protections."
              addButtonLabel="Add Guardrail"
              drawerAddTitle="Add Guardrail"
              drawerEditTitle="Edit Guardrail"
              drawerAddSubtitle="Choose a guardrail to configure advanced options."
              drawerEditSubtitle="Update the guardrail configuration."
              policyNoun="guardrail"
              loadingLabel="Loading guardrails..."
              searchPlaceholder="Search guardrails..."
              catalogErrorLabel="Failed to load guardrails."
              emptySearchTitle="No guardrails match your search"
              emptyCatalogTitle="No guardrails available"
              emptyCatalogDescription="No guardrail policies are available in the catalog."
              policies={guardrails}
              onAdd={handleAddGuardrail}
              onEdit={handleEditGuardrail}
              onRemove={handleRemoveGuardrail}
            />
          )}
        </Form.Section>

        {hasAnyProvider && (
          <ConfigNameSection
            value={configName}
            onChange={(value) => {
              configNameEditedRef.current = true;
              setConfigName(value);
            }}
            description="A name for this LLM configuration."
            placeholder="my-llm-configuration"
          />
        )}

        {hasAnyProvider && !isExternal && (
          <Form.Section>
            <Form.Subheader>Environment Variable Names</Form.Subheader>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              These names are shared across all environments. The platform
              injects the actual URL and API key values at runtime per
              environment. Edit only if your code uses different names.
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
                  {ENV_VAR_KEYS.map((key) => (
                    <ListingTable.Row key={key}>
                      <ListingTable.Cell>
                        <TextInput
                          value={envVarNames[key] ?? ""}
                          onChange={(e) => {
                            envVarNamesEditedRef.current = true;
                            setEnvVarNames((prev) => ({
                              ...prev,
                              [key]: e.target.value,
                            }));
                          }}
                          copyable
                          copyTooltipText={`Copy ${envVarNames[key] ?? key}`}
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
        )}

        {hasAnyProvider && !allEnvsHaveProvider && (
          <Alert severity="info" icon={<AlertTriangle size={18} />}>
            Some environments don&apos;t have a provider selected. They will be
            skipped on save.
          </Alert>
        )}

        {/* Actions */}
        <Box sx={{ display: "flex", gap: 1 }}>
          <Button variant="outlined" onClick={() => navigate(backHref)}>
            Cancel
          </Button>
          <Tooltip
            title={
              !isFormValid && !isPending
                ? "Select a service provider for at least one environment to enable save"
                : ""
            }
            placement="top"
          >
            <span>
              <Button
                variant="contained"
                onClick={handleSave}
                disabled={!isFormValid || isPending}
              >
                {isPending ? "Saving…" : "Save"}
              </Button>
            </span>
          </Tooltip>
        </Box>
      </Stack>
    </PageLayout>
  );
};

export default AddLLMProviderComponent;
