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
  Box,
  Button,
  CircularProgress,
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
  ChevronDown,
  DoorClosedLocked,
  ExternalLink,
  Link,
  Plus,
  RefreshCw,
  Search,
  Trash2,
} from "@wso2/oxygen-ui-icons-react";
import { useParams, generatePath } from "react-router-dom";
import {
  useListCatalogLLMProviders,
  useListEnvironments,
  useListLLMProviderTemplates,
} from "@agent-management-platform/api-client";
import {
  LLMProviderCard,
  PolicyListSection,
  type PolicySelection as GuardrailSelection,
} from "@agent-management-platform/shared-component";
import { AGENT_ENV_KEY_MAX_LENGTH, type LLMProviderFormEntry } from "../form/schema";
import { SlotPortal } from "./OptionalSection";
import {
  absoluteRouteMap,
  withBasePath,
  type CatalogRateLimitingSummary,
} from "@agent-management-platform/types";

type DeploymentSummary = { gatewayName?: string; deployedAt?: string };

interface ProviderInfo {
  uuid: string;
  id: string;
  name: string;
  version?: string;
  template?: string;
  deployments?: DeploymentSummary[];
  rateLimiting?: CatalogRateLimitingSummary;
  policies?: string[];
}

// ─── Per-entry accordion card ────────────────────────────────────────────────

const ENV_VAR_REGEX = /^[A-Za-z_][A-Za-z0-9_]*$/;

interface EntryCardProps {
  entry: LLMProviderFormEntry;
  index: number;
  providers: ProviderInfo[];
  templateMap: Map<string, { displayName: string; logoUrl?: string }>;
  environments: { name: string; displayName?: string }[];
  agentNameUpper: string;
  usedVarNames: Set<string>;
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
  usedVarNames,
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
      const val = e.target.value;
      if (val !== "" && !ENV_VAR_REGEX.test(val)) return;
      onUpdateEntry(index, { ...entry, urlVarName: val });
    },
    [index, entry, onUpdateEntry],
  );

  const handleApikeyVarChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const val = e.target.value;
      if (val !== "" && !ENV_VAR_REGEX.test(val)) return;
      onUpdateEntry(index, { ...entry, apikeyVarName: val });
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

  const handleEditGuardrail = useCallback(
    (guardrail: GuardrailSelection) => {
      onUpdateEntry(index, {
        ...entry,
        guardrails: entry.guardrails.map((g) =>
          g.name === guardrail.name && g.version === guardrail.version
            ? guardrail
            : g,
        ),
      });
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
              <LLMProviderCard
                provider={providers.find((p) => p.uuid === currentEnvProviderUuid) ?? null}
                templateInfo={templateMap.get(
                  providers.find((p) => p.uuid === currentEnvProviderUuid)?.template ?? "",
                )}
                onChange={handleOpenDrawer}
              />
            ) : (
              <Button
                variant="outlined"
                size="small"
                startIcon={<Plus size={16} />}
                onClick={handleOpenDrawer}
              >
                Select provider for {environments[selectedEnvIndex]?.displayName ?? selectedEnvName}
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
                  slotProps={{ htmlInput: { maxLength: AGENT_ENV_KEY_MAX_LENGTH } }}
                  size="small"
                  fullWidth
                  value={entry.urlVarName ?? `${agentNameUpper}_${index + 1}_URL`}
                  onChange={handleUrlVarChange}
                  placeholder={`${agentNameUpper}_${index + 1}_URL`}
                  error={
                    (entry.urlVarName !== undefined && !ENV_VAR_REGEX.test(entry.urlVarName)) ||
                    (entry.urlVarName !== undefined && usedVarNames.has(entry.urlVarName))
                  }
                  helperText={
                    entry.urlVarName !== undefined && !ENV_VAR_REGEX.test(entry.urlVarName)
                      ? "Must match /^[A-Za-z_][A-Za-z0-9_]*$/"
                      : entry.urlVarName !== undefined && usedVarNames.has(entry.urlVarName)
                        ? "Name is already used by another provider"
                        : undefined
                  }
                />
              </Form.ElementWrapper>
              <Form.ElementWrapper label="API key variable name" name="apikeyVarName">
                <TextField
                  slotProps={{ htmlInput: { maxLength: AGENT_ENV_KEY_MAX_LENGTH } }}
                  size="small"
                  fullWidth
                  value={entry.apikeyVarName ?? `${agentNameUpper}_${index + 1}_API_KEY`}
                  onChange={handleApikeyVarChange}
                  placeholder={`${agentNameUpper}_${index + 1}_API_KEY`}
                  error={
                    (entry.apikeyVarName !== undefined && 
                      !ENV_VAR_REGEX.test(entry.apikeyVarName)) ||
                    (entry.apikeyVarName !== undefined && usedVarNames.has(entry.apikeyVarName))
                  }
                  helperText={
                    entry.apikeyVarName !== undefined && !ENV_VAR_REGEX.test(entry.apikeyVarName)
                      ? "Must match /^[A-Za-z_][A-Za-z0-9_]*$/"
                      : entry.apikeyVarName !== undefined && usedVarNames.has(entry.apikeyVarName)
                        ? "Name is already used by another provider"
                        : undefined
                  }
                />
              </Form.ElementWrapper>
            </Stack>
          </Box>

          <PolicyListSection
            title="Guardrails"
            description="Add safety policies to enforce consistent protections."
            addButtonLabel="Add guardrail"
            drawerAddTitle="Add Guardrail"
            drawerEditTitle="Edit Guardrail"
            drawerAddSubtitle="Choose a guardrail to configure advanced options."
            drawerEditSubtitle="Update the guardrail configuration."
            policyNoun="guardrail"
            loadingLabel="Loading guardrails..."
            searchPlaceholder="Search guardrails"
            catalogErrorLabel="Failed to load guardrails."
            emptySearchTitle="No guardrails match your search"
            emptyCatalogTitle="No guardrails available"
            emptyCatalogDescription="No guardrail policies are available in the catalog."
            policies={entry.guardrails}
            onAdd={handleAddGuardrail}
            onEdit={handleEditGuardrail}
            onRemove={handleRemoveGuardrail}
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
  initialEnvironmentName: string | undefined;
  isInitialEnvironmentLoading?: boolean;
  externalEnvKeys?: Set<string>;
  /** The parent already titles this section (e.g. an accordion). */
  hideHeading?: boolean;
  /** Render the add button here (e.g. a parent accordion header) instead of below the entries. */
  actionSlot?: HTMLElement | null;
}

export const LLMProviderSection: React.FC<LLMProviderSectionProps> = ({
  llmProviders,
  setLLMProviders,
  agentDisplayName,
  initialEnvironmentName,
  isInitialEnvironmentLoading = false,
  externalEnvKeys = new Set(),
  hideHeading = false,
  actionSlot,
}) => {
  const { orgId } = useParams<{ orgId: string }>();

  // editingIndex: index of the entry whose provider is being selected, or null when adding new
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [drawerEnvName, setDrawerEnvName] = useState<string>("");
  const [providerDrawerOpen, setProviderDrawerOpen] = useState(false);
  const [providerSearchQuery, setProviderSearchQuery] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const { data: environments = [], isLoading: envsLoading } =
    useListEnvironments({ orgName: orgId });
  const targetEnvironments = useMemo(
    () => initialEnvironmentName
      ? environments.filter((env) => env.name === initialEnvironmentName)
      : [],
    [environments, initialEnvironmentName],
  );
  const drawerEnvironmentId = drawerEnvName
    ? environments.find((e) => e.name === drawerEnvName)?.id
    : undefined;
  const {
    data: catalogData,
    isLoading: catalogLoading,
    refetch: refetchCatalog,
  } = useListCatalogLLMProviders(
    { orgName: orgId },
    { limit: 50, environmentId: drawerEnvironmentId },
  );

  // The "Add LLM provider" action opens in a new tab (see below) so the
  // Create Agent form's state is never unmounted. Refetch the catalog when
  // the user tabs back so a freshly created provider shows up without them
  // having to manually refresh.
  React.useEffect(() => {
    if (!providerDrawerOpen) return;
    const onFocus = () => {
      if (document.visibilityState === "visible") refetchCatalog();
    };
    window.addEventListener("focus", onFocus);
    document.addEventListener("visibilitychange", onFocus);
    return () => {
      window.removeEventListener("focus", onFocus);
      document.removeEventListener("visibilitychange", onFocus);
    };
  }, [providerDrawerOpen, refetchCatalog]);
  const { data: templatesData, isLoading: templatesLoading } =
    useListLLMProviderTemplates({ orgName: orgId });

  const drawerLoading = catalogLoading || templatesLoading;

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
    const targetEnvironmentName = targetEnvironments[0]?.name;
    if (!targetEnvironmentName) return;
    setEditingIndex(null);
    setDrawerEnvName(targetEnvironmentName);
    setProviderDrawerOpen(true);
  }, [targetEnvironments]);

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
          if (targetEnvironments.length === 0) return prev;
          const selectedProviderByEnv: LLMProviderFormEntry["selectedProviderByEnv"] = {};
          for (const env of targetEnvironments) {
            selectedProviderByEnv[env.name] = { uuid: providerUuid, handle: providerHandle };
          }
          const newIndex = prev.length;
          return [
            ...prev,
            {
              selectedProviderByEnv,
              urlVarName: `${agentNameUpper}_${newIndex + 1}_URL`,
              apikeyVarName: `${agentNameUpper}_${newIndex + 1}_API_KEY`,
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
    [editingIndex, drawerEnvName, targetEnvironments, agentNameUpper, setLLMProviders],
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

  const addButton = (
        <Button
          variant="outlined"
          size="small"
          startIcon={<Plus size={16} />}
          onClick={handleAddNew}
          disabled={
            envsLoading ||
            isInitialEnvironmentLoading ||
            catalogLoading ||
            targetEnvironments.length === 0
          }
        >
          Add LLM provider
        </Button>
  );

  return (
    <Stack spacing={2}>
      {!hideHeading && <Typography variant="subtitle1">LLM Providers (Optional)</Typography>}

      <Stack spacing={1}>
        {llmProviders.map((entry, index) => {
          const usedVarNames = new Set([
            ...llmProviders.flatMap((e, i) =>
              i === index ? [] : [
                e.urlVarName ?? `${agentNameUpper}_${i + 1}_URL`,
                e.apikeyVarName ?? `${agentNameUpper}_${i + 1}_API_KEY`,
              ],
            ),
            ...Array.from(externalEnvKeys),
          ]);
          return (
            <EntryCard
              key={index}
              entry={entry}
              index={index}
              providers={providers}
              templateMap={templateMap}
              environments={targetEnvironments}
              agentNameUpper={agentNameUpper}
              usedVarNames={usedVarNames}
              onOpenDrawer={handleOpenDrawer}
              onRemove={handleRemoveEntry}
              onUpdateEntry={handleUpdateEntry}
            />
          );
        })}

        <Box sx={{ pt: llmProviders.length > 0 ? 1 : 0 }}>
          <SlotPortal slot={actionSlot}>{addButton}</SlotPortal>
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
            <Stack direction="row" spacing={1} sx={{ mb: 1 }}>
              <SearchBar
                placeholder="Search providers"
                size="small"
                fullWidth
                value={providerSearchQuery}
                onChange={handleSearchChange}
              />
              <Tooltip title="Refresh provider list" placement="top" arrow>
                <IconButton
                  size="small"
                  aria-label="Refresh provider list"
                  onClick={() => refetchCatalog()}
                  disabled={catalogLoading}
                >
                  <RefreshCw size={16} />
                </IconButton>
              </Tooltip>
            </Stack>
            <Stack spacing={1} sx={{ flex: 1, overflowY: "auto" }}>
              {drawerLoading ? (
                <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
                  <CircularProgress size={32} />
                </Box>
              ) : (() => {
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
                  const isSearchMode = !!debouncedSearch.trim();
                  return (
                    <ListingTable.Container>

                    <ListingTable.EmptyState
                      illustration={<Search size={64} />}
                      title={isSearchMode ? "No LLM Service Providers match your search" : "No LLM Service Providers available"}
                      description={isSearchMode ? "Try a different keyword or clear the search filter." : "No LLM Service Providers found in the catalog. Add LLM service providers from the organization LLM Service Providers page first."}
                      action={
                        (!isSearchMode && orgId) ? (
                          <Button
                            variant="contained"
                            size="small"
                            startIcon={<Link size={16} />}
                            component="a"
                            href={withBasePath(generatePath(
                              absoluteRouteMap.children.org.children.
                                llmProviders.children.add.path,
                              { orgId },
                            ))}
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            Add LLM service provider
                          </Button>
                        ) : undefined
                      }
                    />
                  </ListingTable.Container>
                  );
                }

                return filtered.map((p) => {
                  const isSelected = currentDrawerProviderUuid === p.uuid;
                  const handleClick = () => handleProviderSelect(p.uuid, p.id);
                  return (
                    <LLMProviderCard
                      key={p.uuid}
                      provider={p}
                      templateInfo={templateMap.get(p.template ?? "")}
                      selected={isSelected}
                      onSelect={handleClick}
                    />
                  );
                });
              })()}
            </Stack>
            {orgId && (
              <>
                <Divider sx={{ mt: 2 }} />
                <Box
                  component="a"
                  href={withBasePath(generatePath(
                    absoluteRouteMap.children.org.children.llmProviders.children.add.path,
                    { orgId },
                  ))}
                  target="_blank"
                  rel="noopener noreferrer"
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    pt: 1.5,
                    color: "primary.main",
                    textDecoration: "none",
                    cursor: "pointer",
                    "&:hover": { textDecoration: "underline" },
                  }}
                >
                  <Plus size={16} />
                  <Typography variant="body2" color="primary">
                    Add LLM provider
                  </Typography>
                  <ExternalLink size={14} />
                </Box>
              </>
            )}
          </Stack>
        </DrawerContent>
      </DrawerWrapper>
    </Stack>
  );
};
