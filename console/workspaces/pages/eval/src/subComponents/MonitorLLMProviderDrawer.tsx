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

import { type CatalogLLMProviderEntry } from "@agent-management-platform/types";
import {
  useCreateLLMProvider,
  useListCatalogLLMProviders,
  useGetEnvironment,
  useListLLMProviderTemplates,
} from "@agent-management-platform/api-client";
import {
  AddLLMProviderForm,
  buildCreateLLMProviderRequest,
  mapLLMProviderTemplatesToCards,
  type AddLLMProviderFormValues,
  type GuardrailSelection,
} from "@agent-management-platform/llm-providers";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
} from "@agent-management-platform/views";
import {
  formatRelativeTime,
  useConfirmIfUnsaved,
  useUnsavedChangesGuard,
} from "@agent-management-platform/shared-component";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  Form,
  ListingTable,
  SearchBar,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  ArrowLeft,
  Check,
  Circle,
  Coins,
  DoorClosedLocked,
  Hash,
  Info,
  Plus,
  Zap,
} from "@wso2/oxygen-ui-icons-react";
import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import debounce from "lodash/debounce";

interface MonitorLLMProviderDrawerProps {
  open: boolean;
  onClose: () => void;
  selectedProviderName?: string;
  onProviderChange: (name: string | undefined) => void;
  // The monitor's environment. Only providers deployed to a managed gateway in it are
  // listed, as a monitor can't use any other; without one nothing is listed.
  environmentName?: string;
}

function getLatestDeployment(
  deployments: CatalogLLMProviderEntry["deployments"],
) {
  if (!deployments?.length) return null;
  return (
    [...deployments].sort(
      (a, b) =>
        new Date(b.deployedAt ?? 0).getTime() -
        new Date(a.deployedAt ?? 0).getTime(),
    )[0] ?? null
  );
}

function formatCost(amount: number): string {
  if (amount < 0.01) return `$${amount.toFixed(6)}`;
  return `$${amount.toFixed(2)}`;
}

function formatResetWindow(duration?: number, unit?: string): string {
  if (!unit) return "";
  const abbrev: Record<string, string> = { minute: "min", hour: "hr", day: "day" };
  const u = abbrev[unit.toLowerCase()] ?? unit;
  return duration && duration !== 1 ? `${duration} ${u}` : u;
}

const RateLimitDisplay: React.FC<{ rateLimiting?: CatalogLLMProviderEntry["rateLimiting"] }> = ({ rateLimiting }) => {
  if (!rateLimiting) {
    return (
      <Typography variant="caption" color="text.secondary">
        Rate Limiting: <Typography component="span" variant="body2" color="text.disabled">Not configured</Typography>
      </Typography>
    );
  }

  const cl = rateLimiting.consumerLevel;
  const pl = rateLimiting.providerLevel;

  const consumerEnabled = cl?.globalEnabled ?? false;
  const consumerHasLimits =
    consumerEnabled && (cl?.request != null || cl?.token != null || cl?.cost != null);

  if (!consumerEnabled && !pl?.globalEnabled) {
    return (
      <Typography variant="caption" color="text.secondary">
        Rate Limiting: <Typography component="span" variant="body2" color="text.disabled">Configured (disabled)</Typography>
      </Typography>
    );
  }

  const limitScope = consumerHasLimits ? cl : pl;
  const isOrgWide = !consumerHasLimits;

  const limits: { icon: React.ReactNode; label: string; value: string }[] = [];
  if (limitScope?.request) {
    const w = formatResetWindow(limitScope.request.resetDuration, limitScope.request.resetUnit);
    limits.push({ icon: <Zap size={12} />, label: "Requests", value: `${limitScope.request.limit.toLocaleString()}${w ? `/${w}` : ""}` });
  }
  if (limitScope?.token) {
    const w = formatResetWindow(limitScope.token.resetDuration, limitScope.token.resetUnit);
    limits.push({ icon: <Hash size={12} />, label: "Tokens", value: `${limitScope.token.limit.toLocaleString()}${w ? `/${w}` : ""}` });
  }
  if (limitScope?.cost) {
    const w = formatResetWindow(limitScope.cost.resetDuration, limitScope.cost.resetUnit);
    limits.push({ icon: <Coins size={12} />, label: "Budget", value: `${formatCost(limitScope.cost.limit)}${w ? `/${w}` : ""}` });
  }

  return (
    <Stack spacing={0.5}>
      <Stack direction="row" spacing={0.5} alignItems="center">
        <Typography variant="caption" color="text.secondary">
          {isOrgWide ? "Your Quota (org-wide limit):" : "Your Quota:"}
        </Typography>
        {isOrgWide && (
          <Tooltip
            title="No per-consumer limit is configured. The org-wide provider limit applies to all consumers."
            placement="top"
            arrow
          >
            <Box component="span" sx={{ display: "inline-flex", alignItems: "center", color: "text.secondary", cursor: "default" }}>
              <Info size={12} />
            </Box>
          </Tooltip>
        )}
      </Stack>
      {limits.length > 0 ? (
        <Stack direction="row" spacing={0.75} flexWrap="wrap">
          {limits.map(({ icon, label, value }) => (
            <Chip
              key={label}
              icon={<Box component="span" sx={{ display: "inline-flex", alignItems: "center", pl: 0.5 }}>{icon}</Box>}
              label={`${label}: ${value}`}
              size="small"
              variant="outlined"
              color={isOrgWide ? "default" : "primary"}
              sx={{ fontVariantNumeric: "tabular-nums" }}
            />
          ))}
        </Stack>
      ) : (
        <Typography variant="body2" color="text.secondary">Enabled (no numeric limits set)</Typography>
      )}
    </Stack>
  );
};

interface ProviderCardContentProps {
  entry: CatalogLLMProviderEntry;
  isSelected: boolean;
  templateInfo?: { displayName: string; logoUrl?: string } | null;
}

function ProviderCardContent({
  entry,
  isSelected,
  templateInfo,
}: ProviderCardContentProps) {
  const latest = getLatestDeployment(entry.deployments);

  return (
    <Stack direction="row" spacing={2} flexGrow={1} alignItems="center">
      <Avatar
        sx={{
          height: 32,
          width: 32,
          backgroundColor: isSelected ? "primary.main" : "secondary.main",
          color: isSelected ? "common.white" : "text.secondary",
          flexShrink: 0,
        }}
      >
        {isSelected ? <Check size={16} /> : <Circle size={16} />}
      </Avatar>
      <Stack spacing={0.25} flexGrow={1}>
        <Stack direction="row" spacing={0.25} alignItems="center">
          <Typography variant="h6">{entry.name}&nbsp;</Typography>
          {templateInfo && (
            <Tooltip title="Provider template" placement="top" arrow>
              <Chip
                label={templateInfo.displayName}
                size="small"
                variant="outlined"
                icon={
                  templateInfo.logoUrl ? (
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
            Deployed{" "}
            {formatRelativeTime(latest.deployedAt)}
          </Typography>
        )}
        <Divider orientation="vertical" />
        <Stack direction="column" spacing={0.5}>
          <RateLimitDisplay rateLimiting={entry.rateLimiting} />
          <Typography variant="caption" color="text.secondary">
            Guardrails:{" "}
            <Typography
              component="span"
              variant="body2"
              color={entry.policies?.length ? "text.primary" : "text.disabled"}
            >
              {entry.policies?.length ? (
                <Stack direction="row" spacing={0.25} flexWrap="wrap" alignItems="center">
                  {entry.policies.slice(0, 3).map((p) => (
                    <Chip key={p} label={p} size="small" variant="outlined" />
                  ))}
                  {entry.policies.length > 3 && (
                    <Tooltip title={entry.policies.join(", ")} placement="top" arrow>
                      <Typography variant="caption" color="text.secondary">
                        {` +${entry.policies.length - 3} more..`}
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
}

export function MonitorLLMProviderDrawer({
  open,
  onClose,
  selectedProviderName,
  onProviderChange,
  environmentName,
}: MonitorLLMProviderDrawerProps) {
  const { orgId } = useParams<{ orgId: string }>();
  const { canWrite: canCreateProvider } = useResourceAccess("llm_provider");
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");

  const PAGE_SIZE = 100;
  const [fetchOffset, setFetchOffset] = useState(0);
  // refreshKey increments each time the drawer opens so the accumulate effect
  // re-runs even when fetchOffset and catalogData haven't changed (cached data).
  const [refreshKey, setRefreshKey] = useState(0);
  // Each page by its response offset, so a page that comes back changed (a retry
  // or a refetch) replaces the old one instead of being dropped or appended twice.
  const [pages, setPages] = useState<Map<number, CatalogLLMProviderEntry[]>>(
    () => new Map(),
  );
  const allProviders = useMemo(
    () =>
      [...pages.entries()]
        .sort(([a], [b]) => a - b)
        .flatMap(([, entries]) => entries),
    [pages],
  );

  const {
    data: environment,
    isLoading: isLoadingEnvironment,
    isError: isEnvironmentError,
    refetch: refetchEnvironment,
  } = useGetEnvironment({ orgName: orgId, envName: environmentName });
  const environmentLabel = environment?.displayName || environmentName;
  // Without the environment's id the list can't be filtered, so it isn't fetched at all.
  const environmentUnresolved = !isLoadingEnvironment && !environment?.id;
  const {
    data: catalogData,
    isFetching,
    isError: isCatalogError,
    refetch: refetchCatalog,
  } = useListCatalogLLMProviders(
    { orgName: orgId },
    {
      limit: PAGE_SIZE,
      offset: fetchOffset,
      environmentId: environment?.id,
      gatewayHosting: "MANAGED",
    },
    { enabled: !!environment?.id },
  );
  const isLoadingProviders = isFetching || isLoadingEnvironment;
  const { data: templatesData, isLoading: isLoadingTemplates } =
    useListLLMProviderTemplates(
      { orgName: orgId },
      { limit: 50, offset: 0 },
    );

  const templateMap = useMemo(() => {
    const map = new Map<string, { displayName: string; logoUrl?: string }>();
    for (const t of templatesData?.templates ?? []) {
      map.set(t.name, { displayName: t.name, logoUrl: t.metadata?.logoUrl });
      map.set(t.id, { displayName: t.name, logoUrl: t.metadata?.logoUrl });
    }
    return map;
  }, [templatesData]);

  const templateCards = useMemo(
    () => mapLLMProviderTemplatesToCards(templatesData?.templates),
    [templatesData],
  );

  const [mode, setMode] = useState<"list" | "create">("list");
  const [isCreateFormDirty, setIsCreateFormDirty] = useState(false);
  const isCreateDraftDirty = open && mode === "create" && isCreateFormDirty;
  useUnsavedChangesGuard(isCreateDraftDirty);
  const confirmIfUnsaved = useConfirmIfUnsaved();
  // Only this drawer's own draft decides the prompt — a dirty monitor wizard
  // behind it stays on screen, so it mustn't trigger a warning here.
  const backToList = useCallback(
    () =>
      confirmIfUnsaved(() => {
        setIsCreateFormDirty(false);
        setMode("list");
      }, isCreateDraftDirty),
    [confirmIfUnsaved, isCreateDraftDirty],
  );
  // Header X, Escape and backdrop discard the create draft, so confirm first.
  const handleDrawerClose = useCallback(
    () => confirmIfUnsaved(onClose, isCreateDraftDirty),
    [confirmIfUnsaved, onClose, isCreateDraftDirty],
  );
  // Cancel is an explicit discard, so it skips the unsaved-changes prompt.
  const cancelCreate = useCallback(() => {
    setIsCreateFormDirty(false);
    setMode("list");
  }, []);
  const {
    mutate: createLLMProvider,
    isPending: isCreatingProvider,
  } = useCreateLLMProvider();

  const debouncedSetSearch = useMemo(
    () => debounce((value: string) => setDebouncedSearch(value), 250),
    [],
  );

  useEffect(() => () => debouncedSetSearch.cancel(), [debouncedSetSearch]);

  // Reset accumulated state when the drawer opens. Bumping refreshKey forces
  // the accumulate effect to re-run even when fetchOffset is already 0 and
  // catalogData is still the cached result from a previous open.
  useEffect(() => {
    if (open) {
      setPages(new Map());
      setFetchOffset(0);
      setRefreshKey((k) => k + 1);
      setMode("list");
      setIsCreateFormDirty(false);
    }
  }, [open]);

  // Accumulate each page as it arrives and trigger the next fetch if needed.
  useEffect(() => {
    if (!catalogData?.entries) return;
    // Key on the response offset, not the requested fetchOffset, so a stale
    // cache hit for the new offset only rewrites the page it belongs to.
    const responseOffset = catalogData.offset ?? 0;
    const entries = catalogData.entries;
    setPages((prev) => {
      if (prev.get(responseOffset) === entries) return prev;
      const next = new Map(prev);
      next.set(responseOffset, entries);
      return next;
    });
    if (responseOffset + entries.length < catalogData.total) {
      setFetchOffset(responseOffset + entries.length);
    }
    // refreshKey is intentionally included so this effect re-runs on open
    // even when fetchOffset and catalogData are unchanged (cached page 0).
  }, [catalogData, fetchOffset, refreshKey]);

  const filteredProviders = useMemo(() => {
    if (!debouncedSearch.trim()) return allProviders;
    const q = debouncedSearch.toLowerCase();
    return allProviders.filter(
      (p) =>
        p.name.toLowerCase().includes(q) ||
        (p.template ?? "").toLowerCase().includes(q) ||
        (templateMap.get(p.template ?? "")?.displayName ?? "")
          .toLowerCase()
          .includes(q),
    );
  }, [allProviders, debouncedSearch, templateMap]);

  const handleSearchChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      setSearch(event.target.value);
      debouncedSetSearch(event.target.value);
    },
    [debouncedSetSearch],
  );

  const handleSelect = useCallback(
    (providerHandle: string) => {
      onProviderChange(providerHandle);
      onClose();
    },
    [onProviderChange, onClose],
  );

  const handleCreateProvider = useCallback(
    (values: AddLLMProviderFormValues, guardrails: GuardrailSelection[]) => {
      if (!orgId) return;
      const body = buildCreateLLMProviderRequest(
        values,
        guardrails,
        templateCards,
      );
      createLLMProvider(
        { params: { orgName: orgId }, body },
        {
          onSuccess: (data) => {
            onProviderChange(data.id);
            onClose();
          },
        },
      );
    },
    [orgId, templateCards, createLLMProvider, onProviderChange, onClose],
  );

  return (
    <DrawerWrapper open={open} onClose={handleDrawerClose} maxWidth={520}>
      <DrawerHeader
        icon={<DoorClosedLocked size={24} />}
        title={mode === "create" ? "Create LLM Provider" : "Select LLM Provider"}
        onClose={handleDrawerClose}
      />
      <DrawerContent>
        {mode === "list" || !canCreateProvider ? (
          <Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">
              Select the LLM provider for all LLM-judge evaluators in this
              monitor.
              {environmentLabel &&
                ` Only providers deployed to a managed gateway in ${environmentLabel} are listed.`}
            </Typography>
            <SearchBar
              placeholder="Search providers"
              size="small"
              fullWidth
              value={search}
              onChange={handleSearchChange}
            />
            {environmentUnresolved && (
              <Alert
                severity={environmentName ? "error" : "info"}
                action={
                  isEnvironmentError ? (
                    <Button size="small" color="inherit" onClick={() => refetchEnvironment()}>
                      Retry
                    </Button>
                  ) : undefined
                }
              >
                {!environmentName
                  ? "Choose the monitor's environment first; its LLM providers are listed here."
                  : isEnvironmentError
                    ? `Couldn't load environment ${environmentName}, so its LLM providers can't be listed.`
                    : `Environment ${environmentName} wasn't found, so its LLM providers can't be listed.`}
              </Alert>
            )}
            {isCatalogError && !isFetching && (
              <Alert
                severity="error"
                action={
                  <Button size="small" color="inherit" onClick={() => refetchCatalog()}>
                    Retry
                  </Button>
                }
              >
                Couldn&apos;t load the LLM providers.
              </Alert>
            )}
            {isLoadingProviders && filteredProviders.length === 0 && (
              <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
                <CircularProgress size={32} />
              </Box>
            )}
            {filteredProviders.length === 0 &&
              !isLoadingProviders &&
              !environmentUnresolved &&
              !isCatalogError && (
              <ListingTable.EmptyState
                title={
                  search.trim()
                    ? "No providers match your search"
                    : `No LLM providers deployed to ${environmentLabel}`
                }
                description={
                  search.trim()
                    ? "Try a different keyword."
                    : `A monitor can only use an LLM provider deployed to a managed gateway in ${environmentLabel}. Deploy one from its Deployment tab on the LLM Providers page.`
                }
              />
            )}
            {filteredProviders.length > 0 && (
              <Stack spacing={1}>
                {filteredProviders.map((entry) => {
                  const isSelected = entry.handle === selectedProviderName;
                  const templateInfo = templateMap.get(entry.template ?? "");
                  return (
                    <Form.CardButton
                      key={entry.uuid}
                      onClick={() => handleSelect(entry.handle)}
                      selected={isSelected}
                    >
                      <Form.CardContent>
                        <ProviderCardContent
                          entry={entry}
                          isSelected={isSelected}
                          templateInfo={templateInfo}
                        />
                      </Form.CardContent>
                    </Form.CardButton>
                  );
                })}
              </Stack>
            )}
            {canCreateProvider && (
              <>
                <Divider />
                <Button
                  variant="text"
                  size="small"
                  startIcon={<Plus size={16} />}
                  onClick={() => setMode("create")}
                  sx={{ alignSelf: "flex-start" }}
                >
                  Add LLM provider
                </Button>
              </>
            )}
          </Stack>
        ) : (
          <Stack spacing={2}>
            <Button
              variant="text"
              size="small"
              startIcon={<ArrowLeft size={16} />}
              onClick={backToList}
              sx={{ alignSelf: "flex-start" }}
            >
              Back to providers
            </Button>
            <Typography variant="body2" color="text.secondary">
              Create an LLM provider for this monitor&apos;s LLM-judge
              evaluators. It will be added to your organization&apos;s
              provider catalog.
              {environmentLabel &&
                ` This monitor can only use it if it's deployed to a managed gateway in ${environmentLabel}.`}
            </Typography>
            <AddLLMProviderForm
              orgId={orgId ?? ""}
              templates={templateCards}
              isLoadingTemplates={isLoadingTemplates}
              isSubmitting={isCreatingProvider}
              submitLabel="Create & use provider"
              showAdvancedBasicDetails={false}
              showGuardrails={false}
              onCancel={cancelCreate}
              onDirtyChange={setIsCreateFormDirty}
              onSubmit={handleCreateProvider}
            />
          </Stack>
        )}
      </DrawerContent>
    </DrawerWrapper>
  );
}

export default MonitorLLMProviderDrawer;
