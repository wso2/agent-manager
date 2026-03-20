/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
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

import React, { useCallback, useMemo, useRef, useState } from "react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
} from "@agent-management-platform/views";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Avatar,
  Box,
  Button,
  Chip,
  Divider,
  Form,
  IconButton,
  ListingTable,
  SearchBar,
  Stack,
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Check,
  ChevronDown,
  Circle,
  DoorClosedLocked,
  Plus,
  Trash2,
} from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import { useParams } from "react-router-dom";
import {
  useListCatalogLLMProviders,
  useListEnvironments,
  useListLLMProviderTemplates,
} from "@agent-management-platform/api-client";
import {
  GuardrailsSection,
  type GuardrailSelection,
} from "@agent-management-platform/llm-providers";
import type {
  CatalogRateLimitingSummary,
  CatalogSecuritySummary,
} from "@agent-management-platform/types";
import type { LLMProviderFormEntry } from "../form/schema";

type DeploymentSummary = { gatewayName?: string; deployedAt?: string };

interface ProviderInfo {
  uuid: string;
  id: string;
  name: string;
  version?: string;
  template?: string;
  deployments?: DeploymentSummary[];
  security?: CatalogSecuritySummary;
  rateLimiting?: CatalogRateLimitingSummary;
  policies?: string[];
}

function getLatestDeployment(
  deployments: DeploymentSummary[] | undefined,
): DeploymentSummary | null {
  if (!deployments?.length) return null;
  return [...deployments].sort(
    (a, b) =>
      new Date(b.deployedAt ?? 0).getTime() - new Date(a.deployedAt ?? 0).getTime(),
  )[0] ?? null;
}

const ProviderDisplay: React.FC<{
  provider: ProviderInfo | null;
  isSelected: boolean;
  templateInfo?: { displayName: string; logoUrl?: string } | null;
  fallbackLabel?: string;
}> = ({ provider, isSelected, templateInfo, fallbackLabel = "Select provider" }) => {
  const latest = getLatestDeployment(provider?.deployments);
  return (
    <Stack direction="row" spacing={2} flexGrow={1} alignItems="center">
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
      <Stack spacing={0.25} flexGrow={1}>
        <Stack direction="row" spacing={0.25} alignItems="center">
          <Typography variant="h6">{provider?.name ?? fallbackLabel} &nbsp;</Typography>
          {provider?.template && (
            <Tooltip title="Provider template" placement="top" arrow>
              <Chip
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
              />
            </Tooltip>
          )}
        </Stack>
        {latest?.deployedAt && (
          <Typography variant="caption" color="text.secondary">
            Deployed {formatDistanceToNow(new Date(latest.deployedAt), { addSuffix: true })}
          </Typography>
        )}
        <Divider orientation="vertical" />
        <Stack direction="column" spacing={0.25}>
          <Typography variant="caption" color="text.secondary">
            Rate Limiting:{" "}
            <Typography
              component="span"
              variant="body2"
              color={provider?.rateLimiting ? "text.primary" : "text.disabled"}
            >
              {provider?.rateLimiting
                ? (() => {
                  const limits: string[] = [];
                  const pl = provider.rateLimiting.providerLevel;
                  const cl = provider.rateLimiting.consumerLevel;
                  if (pl?.requestLimitCount) limits.push(`${pl.requestLimitCount} req/min`);
                  if (pl?.tokenLimitCount) limits.push(`${pl.tokenLimitCount} tokens/min`);
                  if (cl?.requestLimitCount) limits.push(`Consumer: ${cl.requestLimitCount} req/min`);
                  return limits.length > 0 ? limits.join(", ") : "Configured";
                })()
                : "Not configured"}
            </Typography>
          </Typography>
          <Typography variant="caption" color="text.secondary">
            Guardrails:{" "}
            <Typography
              component="span"
              variant="body2"
              color={provider?.policies?.length ? "text.primary" : "text.disabled"}
            >
              {provider?.policies?.length ? (
                <Stack direction="row" spacing={0.25} flexWrap="wrap" alignItems="center">
                  {provider.policies.slice(0, 3).map((p) => (
                    <Chip key={p} label={p} size="small" variant="outlined" />
                  ))}
                  {provider.policies.length > 3 && (
                    <Tooltip title={provider.policies.join(", ")} placement="top" arrow>
                      <Typography variant="caption" color="text.secondary">
                        {` +${provider.policies.length - 3} more..`}
                      </Typography>
                    </Tooltip>
                  )}
                </Stack>
              ) : (
                "None"
              )}
            </Typography>
          </Typography>
        </Stack>
      </Stack>
    </Stack>
  );
};

// ─── Per-entry accordion card ────────────────────────────────────────────────

interface EntryCardProps {
  entry: LLMProviderFormEntry;
  index: number;
  providers: ProviderInfo[];
  templateMap: Map<string, { displayName: string; logoUrl?: string }>;
  environments: { name: string; displayName?: string }[];
  agentNameUpper: string;
  onOpenDrawer: (index: number, envName: string) => void;
  onRemove: (index: number) => void;
  onUpdateEntry: (index: number, updated: LLMProviderFormEntry) => void;
}

const EntryCard: React.FC<EntryCardProps> = ({
  entry,
  index,
  providers,
  templateMap,
  environments,
  agentNameUpper,
  onOpenDrawer,
  onRemove,
  onUpdateEntry,
}) => {
  const [selectedEnvIndex, setSelectedEnvIndex] = useState(0);

  const selectedEnvName = environments[selectedEnvIndex]?.name ?? "";
  const currentEnvProviderUuid = entry.selectedProviderByEnv[selectedEnvName]?.uuid ?? null;

  const firstProviderEntry = Object.values(entry.selectedProviderByEnv).find(
    (e): e is { uuid: string; handle: string } => e !== null && e !== undefined,
  );
  const displayName =
    (firstProviderEntry ? providers.find((p) => p.uuid === firstProviderEntry.uuid)?.name : null)
    ?? `LLM Provider ${index + 1}`;

  const handleEnvTabChange = useCallback(
    (_: React.SyntheticEvent, v: number) => setSelectedEnvIndex(v),
    [],
  );

  const handleRemoveClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      onRemove(index);
    },
    [index, onRemove],
  );

  const handleOpenDrawer = useCallback(
    () => onOpenDrawer(index, selectedEnvName),
    [index, selectedEnvName, onOpenDrawer],
  );

  const handleUrlVarChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      onUpdateEntry(index, { ...entry, urlVarName: e.target.value });
    },
    [index, entry, onUpdateEntry],
  );

  const handleApikeyVarChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      onUpdateEntry(index, { ...entry, apikeyVarName: e.target.value });
    },
    [index, entry, onUpdateEntry],
  );

  const handleAddGuardrail = useCallback(
    (guardrail: GuardrailSelection) => {
      const guardrails = entry.guardrails.some(
        (g) => g.name === guardrail.name && g.version === guardrail.version)
      if (guardrails) return;

      onUpdateEntry(index, { ...entry, guardrails: [...entry.guardrails, guardrail] });
    },
    [index, entry, onUpdateEntry],
  );

  const handleRemoveGuardrail = useCallback(
    (gName: string, gVersion: string) => {
      onUpdateEntry(index, {
        ...entry,
        guardrails: entry.guardrails.filter((g) => !(g.name === gName && g.version === gVersion)),
      });
    },
    [index, entry, onUpdateEntry],
  );

  return (
    <Accordion defaultExpanded>
      <AccordionSummary
        expandIcon={<ChevronDown size={18} />}
        aria-controls={`llm-provider-${index}-content`}
        id={`llm-provider-${index}-header`}
      >
        <Stack direction="row" alignItems="center" justifyContent="space-between" flexGrow={1} pr={1}>
          <Typography variant="subtitle2">{displayName}</Typography>
          <IconButton size="small" aria-label="Remove LLM provider" onClick={handleRemoveClick}>
            <Trash2 size={16} />
          </IconButton>
        </Stack>
      </AccordionSummary>
      <AccordionDetails>
        <Stack spacing={3}>
          {environments.length > 1 && (
            <Tabs value={selectedEnvIndex} onChange={handleEnvTabChange} sx={{ mb: 1 }}>
              {environments.map((env, idx) => (
                <Tab key={env.name} label={env.displayName ?? env.name} value={idx} />
              ))}
            </Tabs>
          )}

          <Box>
            {currentEnvProviderUuid ? (
              <Form.CardButton onClick={handleOpenDrawer} selected>
                <Form.CardContent>
                  <ProviderDisplay
                    provider={providers.find((p) => p.uuid === currentEnvProviderUuid) ?? null}
                    isSelected
                    templateInfo={templateMap.get(
                      providers.find((p) => p.uuid === currentEnvProviderUuid)?.template ?? "",
                    )}
                  />
                </Form.CardContent>
              </Form.CardButton>
            ) : (
              <Button
                variant="outlined"
                size="small"
                startIcon={<Plus size={16} />}
                onClick={handleOpenDrawer}
              >
                Select Provider for {environments[selectedEnvIndex]?.displayName ?? selectedEnvName}
              </Button>
            )}
          </Box>

          <Box>
            <Typography variant="subtitle2" gutterBottom>
              Environment Variables
            </Typography>
            <Stack direction="row" spacing={2}>
              <Form.ElementWrapper label="URL variable name" name="urlVarName">
                <TextField
                  size="small"
                  fullWidth
                  value={entry.urlVarName ?? `${agentNameUpper}_URL`}
                  onChange={handleUrlVarChange}
                  placeholder={`${agentNameUpper}_URL`}
                />
              </Form.ElementWrapper>
              <Form.ElementWrapper label="API key variable name" name="apikeyVarName">
                <TextField
                  size="small"
                  fullWidth
                  value={entry.apikeyVarName ?? `${agentNameUpper}_API_KEY`}
                  onChange={handleApikeyVarChange}
                  placeholder={`${agentNameUpper}_API_KEY`}
                />
              </Form.ElementWrapper>
            </Stack>
          </Box>

          <GuardrailsSection
            guardrails={entry.guardrails}
            onAddGuardrail={handleAddGuardrail}
            onRemoveGuardrail={handleRemoveGuardrail}
          />
        </Stack>
      </AccordionDetails>
    </Accordion>
  );
};

// ─── Main section ─────────────────────────────────────────────────────────────

interface LLMProviderSectionProps {
  llmProviders: LLMProviderFormEntry[];
  setLLMProviders: React.Dispatch<React.SetStateAction<LLMProviderFormEntry[]>>;
  agentDisplayName: string;
}

export const LLMProviderSection: React.FC<LLMProviderSectionProps> = ({
  llmProviders,
  setLLMProviders,
  agentDisplayName,
}) => {
  const { orgId } = useParams<{ orgId: string }>();

  // editingIndex: index of the entry whose provider is being selected, or null when adding new
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [drawerEnvName, setDrawerEnvName] = useState<string>("");
  const [providerDrawerOpen, setProviderDrawerOpen] = useState(false);
  const [providerSearchQuery, setProviderSearchQuery] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const { data: environments = [] } = useListEnvironments({ orgName: orgId });
  const { data: catalogData } = useListCatalogLLMProviders({ orgName: orgId }, { limit: 50 });
  const { data: templatesData } = useListLLMProviderTemplates({ orgName: orgId });

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

  const agentNameUpper = agentDisplayName
    ? agentDisplayName.toUpperCase().replace(/[^A-Z0-9]/g, "_")
    : "AGENT";

  const currentDrawerProviderUuid =
    editingIndex !== null
      ? (llmProviders[editingIndex]?.selectedProviderByEnv[drawerEnvName]?.uuid ?? null)
      : null;

  const handleOpenDrawer = useCallback((index: number, envName: string) => {
    setEditingIndex(index);
    setDrawerEnvName(envName);
    setProviderDrawerOpen(true);
  }, []);

  const handleAddNew = useCallback(() => {
    setEditingIndex(null);
    setProviderDrawerOpen(true);
  }, []);

  const handleDrawerClose = useCallback(() => {
    if (searchTimerRef.current) {
      clearTimeout(searchTimerRef.current);
      searchTimerRef.current = null;
    }
    setProviderDrawerOpen(false);
    setProviderSearchQuery("");
    setDebouncedSearch("");
  }, []);

  const handleProviderSelect = useCallback(
    (providerUuid: string, providerHandle: string) => {
      setLLMProviders((prev) => {
        if (editingIndex === null) {
          // Adding a new entry — assign this provider to all environments
          const selectedProviderByEnv: LLMProviderFormEntry["selectedProviderByEnv"] = {};
          for (const env of environments) {
            selectedProviderByEnv[env.name] = { uuid: providerUuid, handle: providerHandle };
          }
          return [
            ...prev,
            {
              selectedProviderByEnv,
              urlVarName: `${agentNameUpper}_URL`,
              apikeyVarName: `${agentNameUpper}_API_KEY`,
              guardrails: [],
            },
          ];
        } else {
          // Changing provider for an existing entry — only update the active env
          const updated = [...prev];
          const entry = updated[editingIndex];
          if (!entry) return prev;
          updated[editingIndex] = {
            ...entry,
            selectedProviderByEnv: {
              ...entry.selectedProviderByEnv,
              [drawerEnvName]: { uuid: providerUuid, handle: providerHandle },
            },
          };
          return updated;
        }
      });
      if (searchTimerRef.current) {
        clearTimeout(searchTimerRef.current);
        searchTimerRef.current = null;
      }
      setProviderDrawerOpen(false);
      setProviderSearchQuery("");
      setDebouncedSearch("");
    },
    [editingIndex, drawerEnvName, environments, agentNameUpper, setLLMProviders],
  );

  const handleRemoveEntry = useCallback(
    (index: number) => {
      setLLMProviders((prev) => prev.filter((_, i) => i !== index));
    },
    [setLLMProviders],
  );

  const handleUpdateEntry = useCallback(
    (index: number, updated: LLMProviderFormEntry) => {
      setLLMProviders((prev) => {
        const next = [...prev];
        next[index] = updated;
        return next;
      });
    },
    [setLLMProviders],
  );

  const handleSearchChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    setProviderSearchQuery(val);
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
    searchTimerRef.current = setTimeout(() => setDebouncedSearch(val), 250);
  }, []);

  return (
    <Form.Section>
      <Form.Subheader>LLM Providers (Optional)</Form.Subheader>

      <Stack spacing={1}>
        {llmProviders.map((entry, index) => (
          <EntryCard
            key={index}
            entry={entry}
            index={index}
            providers={providers}
            templateMap={templateMap}
            environments={environments}
            agentNameUpper={agentNameUpper}
            onOpenDrawer={handleOpenDrawer}
            onRemove={handleRemoveEntry}
            onUpdateEntry={handleUpdateEntry}
          />
        ))}

        <Box sx={{ pt: llmProviders.length > 0 ? 1 : 0 }}>
          <Button
            variant="outlined"
            size="small"
            startIcon={<Plus size={16} />}
            onClick={handleAddNew}
            disabled={providers.length === 0 && !!catalogData}
          >
            Add
          </Button>
          {catalogData && providers.length === 0 && (
            <Typography variant="caption" color="text.secondary" sx={{ ml: 1 }}>
              No providers available. Add LLM providers from the organization page first.
            </Typography>
          )}
        </Box>
      </Stack>

      <DrawerWrapper
        open={providerDrawerOpen}
        onClose={handleDrawerClose}
        minWidth={740}
        maxWidth={740}
      >
        <DrawerHeader
          icon={<DoorClosedLocked size={24} />}
          title="Select Provider"
          onClose={handleDrawerClose}
        />
        <DrawerContent>
          <Stack>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              {editingIndex === null
                ? "Select a provider for this LLM configuration."
                : "Change the provider for this LLM configuration."}
            </Typography>
            <SearchBar
              placeholder="Search providers"
              size="small"
              fullWidth
              value={providerSearchQuery}
              onChange={handleSearchChange}
              sx={{ mb: 1 }}
            />
            <Stack spacing={1} sx={{ flex: 1, overflowY: "auto" }}>
              {(() => {
                const filtered = providers.filter((p) => {
                  if (!debouncedSearch.trim()) return true;
                  const q = debouncedSearch.toLowerCase();
                  return (
                    p.name.toLowerCase().includes(q) ||
                    (p.template ?? "").toLowerCase().includes(q) ||
                    (templateMap.get(p.template ?? "")?.displayName ?? "").toLowerCase().includes(q)
                  );
                });

                if (filtered.length === 0) {
                  return (
                    <ListingTable.Container>
                      <ListingTable.EmptyState
                        title={debouncedSearch.trim() ? "No providers match your search" : "No providers available"}
                        description={
                          debouncedSearch.trim()
                            ? "Try a different keyword or clear the search filter."
                            : "No providers are available in the catalog."
                        }
                      />
                    </ListingTable.Container>
                  );
                }

                return filtered.map((p) => {
                  const isSelected = currentDrawerProviderUuid === p.uuid;
                  const handleClick = () => handleProviderSelect(p.uuid, p.id);
                  return (
                    <Form.CardButton
                      key={p.uuid}
                      onClick={handleClick}
                      selected={isSelected}
                      aria-label={`${p.name}. ${isSelected ? "Selected" : "Click to select"}`}
                    >
                      <Form.CardContent>
                        <ProviderDisplay
                          provider={p}
                          isSelected={isSelected}
                          templateInfo={templateMap.get(p.template ?? "")}
                        />
                      </Form.CardContent>
                    </Form.CardButton>
                  );
                });
              })()}
            </Stack>
          </Stack>
        </DrawerContent>
      </DrawerWrapper>
    </Form.Section>
  );
};
