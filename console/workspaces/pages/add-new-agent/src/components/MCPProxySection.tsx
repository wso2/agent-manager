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

import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
} from "@agent-management-platform/views";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Avatar,
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  Form,
  IconButton,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Check,
  ChevronDown,
  Circle,
  ExternalLink,
  Link,
  Plus,
  Search,
  ServerCog,
  Trash2,
} from "@wso2/oxygen-ui-icons-react";
import { useParams, useNavigate, generatePath } from "react-router-dom";
import {
  useListEnvironments,
  useListMCPProxies,
} from "@agent-management-platform/api-client";
import {
  EnvironmentVariablesReference,
  useMCPProxySecurity,
} from "@agent-management-platform/shared-component";
import { AGENT_ENV_KEY_MAX_LENGTH, type MCPProxyFormEntry } from "../form/schema";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  defaultMCPApiKeyVarName,
  defaultMCPUrlVarName,
  mcpEntryVarNames,
} from "../utils/mcpEnvVarNames";

interface ProxyInfo {
  id: string;
  name: string;
  version?: string;
  description?: string;
  status?: string;
}

const ProxyDisplay: React.FC<{
  proxy: ProxyInfo | null;
  isSelected: boolean;
  fallbackLabel?: string;
}> = ({ proxy, isSelected, fallbackLabel = "Select MCP Server" }) => {
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
        <Stack direction="row" spacing={0.5} alignItems="center">
          <Typography variant="h6">{proxy?.name ?? fallbackLabel} &nbsp;</Typography>
          {proxy?.version && (
            <Chip label={`v${proxy.version}`} size="small" variant="outlined" />
          )}
          {proxy?.status && (
            <Chip
              label={proxy.status}
              size="small"
              variant="outlined"
              color={proxy.status === "deployed" ? "success" : "default"}
            />
          )}
        </Stack>
        {proxy?.description && (
          <Typography variant="caption" color="text.secondary">
            {proxy.description}
          </Typography>
        )}
      </Stack>
    </Stack>
  );
};

// ─── Per-entry accordion card ────────────────────────────────────────────────

const ENV_VAR_REGEX = /^[A-Za-z_][A-Za-z0-9_]*$/;

interface EntryCardProps {
  entry: MCPProxyFormEntry;
  index: number;
  proxies: ProxyInfo[];
  environments: { name: string; displayName?: string; id?: string }[];
  agentNameUpper: string;
  orgId?: string;
  usedVarNames: Set<string>;
  onOpenDrawer: (index: number, envName: string) => void;
  onRemove: (index: number) => void;
  onUpdateEntry: (index: number, updated: MCPProxyFormEntry) => void;
}

const EntryCard: React.FC<EntryCardProps> = ({
  entry,
  index,
  proxies,
  environments,
  agentNameUpper,
  orgId,
  usedVarNames,
  onOpenDrawer,
  onRemove,
  onUpdateEntry,
}) => {
  const [selectedEnvIndex, setSelectedEnvIndex] = useState(0);

  const selectedEnvName = environments[selectedEnvIndex]?.name ?? "";
  const selectedEnvLabel = environments[selectedEnvIndex]?.displayName ?? selectedEnvName;
  const selectedEnvUuid = environments[selectedEnvIndex]?.id;
  const currentEnvProxyId = entry.selectedProxyByEnv[selectedEnvName]?.id ?? null;

  // Which runtime variables this binding actually needs. Scoped to the selected
  // environment's endpoint, matching how the server resolves MCP security; falls
  // back to the every-endpoint rule when the environment carries no uuid.
  const {
    authenticationType,
    spec,
    isLoading: isSecurityLoading,
    isResolved: isSecurityResolved,
  } = useMCPProxySecurity({
    orgName: orgId,
    proxyId: currentEnvProxyId,
    environmentUuid: selectedEnvUuid,
  });
  const showApiKeyField = spec.editableKeys.includes("apikey");
  // A failed fetch reports "" (unsecured) just like a genuinely open endpoint.
  // Acting on that would hide the API key field for a proxy that needs one, so
  // treat unresolved as unknown and say so.
  const isSecurityUnknown = !isSecurityLoading && !isSecurityResolved;

  // The resolved security owns apikeyVarName: only an API-key endpoint gets a
  // default name, and any other kind has it cleared. Keeping the name absent
  // until the proxy resolves means a submit that races the fetch omits the
  // variable rather than sending one the platform would inject empty.
  useEffect(() => {
    if (isSecurityLoading || !isSecurityResolved) return;
    const nextApikeyVarName = showApiKeyField
      ? (entry.apikeyVarName ?? defaultMCPApiKeyVarName(agentNameUpper, index))
      : undefined;
    if (
      entry.authenticationType === authenticationType &&
      entry.apikeyVarName === nextApikeyVarName
    ) {
      return;
    }
    onUpdateEntry(index, {
      ...entry,
      authenticationType,
      apikeyVarName: nextApikeyVarName,
    });
  }, [
    isSecurityLoading,
    isSecurityResolved,
    authenticationType,
    showApiKeyField,
    agentNameUpper,
    entry,
    index,
    onUpdateEntry,
  ]);

  // Flag when the URL and API key variable names within this same card are identical.
  // usedVarNames excludes the active entry, so this same-entry clash is checked separately.
  // Only meaningful while both fields exist.
  const sameEntryNameCollision =
    showApiKeyField &&
    entry.urlVarName !== undefined &&
    entry.urlVarName === entry.apikeyVarName;

  const firstProxyEntry = Object.values(entry.selectedProxyByEnv).find(
    (e): e is { id: string; name: string } => e !== null && e !== undefined,
  );
  const displayName = firstProxyEntry?.name ?? `MCP Server ${index + 1}`;

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

  return (
    <Accordion defaultExpanded>
      <AccordionSummary
        expandIcon={<ChevronDown size={18} />}
        aria-controls={`mcp-proxy-${index}-content`}
        id={`mcp-proxy-${index}-header`}
      >
        <Stack direction="row" alignItems="center" justifyContent="space-between" flexGrow={1} pr={1}>
          <Typography variant="subtitle2">{displayName}</Typography>
          <IconButton size="small" aria-label="Remove MCP Server" onClick={handleRemoveClick}>
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
            {currentEnvProxyId ? (
              <Form.CardButton onClick={handleOpenDrawer} selected>
                <Form.CardContent>
                  <ProxyDisplay
                    proxy={proxies.find((p) => p.id === currentEnvProxyId) ?? null}
                    isSelected
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
                Select MCP Server for {selectedEnvLabel}
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
                  value={entry.urlVarName ?? `${agentNameUpper}_MCP_${index + 1}_URL`}
                  onChange={handleUrlVarChange}
                  placeholder={`${agentNameUpper}_MCP_${index + 1}_URL`}
                  error={
                    (entry.urlVarName !== undefined && !ENV_VAR_REGEX.test(entry.urlVarName)) ||
                    (entry.urlVarName !== undefined && usedVarNames.has(entry.urlVarName)) ||
                    sameEntryNameCollision
                  }
                  helperText={
                    entry.urlVarName !== undefined && !ENV_VAR_REGEX.test(entry.urlVarName)
                      ? "Must match /^[A-Za-z_][A-Za-z0-9_]*$/"
                      : entry.urlVarName !== undefined && usedVarNames.has(entry.urlVarName)
                        ? "Name is already used by another config"
                        : sameEntryNameCollision
                          ? "URL and API key variable names must be different"
                          : undefined
                  }
                />
              </Form.ElementWrapper>
              {/*
                * Whether this slot is an API key input at all depends on the
                * proxy's security — only an API-key endpoint has one — so hold it
                * until that resolves rather than render a field we are about to
                * remove. The URL variable is needed either way and never waits.
                */}
              {isSecurityLoading ? (
                <Skeleton variant="rounded" height={40} sx={{ flex: 1 }} />
              ) : isSecurityUnknown || !showApiKeyField ? null : (
              <Form.ElementWrapper label="API key variable name" name="apikeyVarName">
                <TextField
                  slotProps={{ htmlInput: { maxLength: AGENT_ENV_KEY_MAX_LENGTH } }}
                  size="small"
                  fullWidth
                  value={entry.apikeyVarName ?? `${agentNameUpper}_MCP_${index + 1}_API_KEY`}
                  onChange={handleApikeyVarChange}
                  placeholder={`${agentNameUpper}_MCP_${index + 1}_API_KEY`}
                  error={
                    (entry.apikeyVarName !== undefined &&
                      !ENV_VAR_REGEX.test(entry.apikeyVarName)) ||
                    (entry.apikeyVarName !== undefined && usedVarNames.has(entry.apikeyVarName)) ||
                    sameEntryNameCollision
                  }
                  helperText={
                    entry.apikeyVarName !== undefined && !ENV_VAR_REGEX.test(entry.apikeyVarName)
                      ? "Must match /^[A-Za-z_][A-Za-z0-9_]*$/"
                      : entry.apikeyVarName !== undefined && usedVarNames.has(entry.apikeyVarName)
                        ? "Name is already used by another config"
                        : sameEntryNameCollision
                          ? "URL and API key variable names must be different"
                          : undefined
                  }
                />
              </Form.ElementWrapper>
              )}
            </Stack>
            {isSecurityUnknown && (
              <Alert severity="error" sx={{ mt: 2 }}>
                Couldn&apos;t load this MCP server&apos;s security settings, so the
                runtime variables it needs are unknown. Reselect the server or
                retry once it is reachable — saving now could leave the agent
                without a credential it needs.
              </Alert>
            )}
            {!isSecurityLoading && !isSecurityUnknown && spec.referenceRows.length > 0 && (
              <EnvironmentVariablesReference
                title="Injected at runtime"
                description="This MCP server uses OAuth (AgentID) security, so there is no API key to name. These values are injected into the agent's pod at runtime alongside the URL above; their names are fixed, only their values change per environment."
                rows={spec.referenceRows}
              />
            )}
          </Box>
        </Stack>
      </AccordionDetails>
    </Accordion>
  );
};

// ─── Main section ─────────────────────────────────────────────────────────────

interface MCPProxySectionProps {
  mcpProxies: MCPProxyFormEntry[];
  setMCPProxies: React.Dispatch<React.SetStateAction<MCPProxyFormEntry[]>>;
  agentDisplayName: string;
  initialEnvironmentName: string | undefined;
  isInitialEnvironmentLoading?: boolean;
  externalEnvKeys?: Set<string>;
}

export const MCPProxySection: React.FC<MCPProxySectionProps> = ({
  mcpProxies,
  setMCPProxies,
  agentDisplayName,
  initialEnvironmentName,
  isInitialEnvironmentLoading = false,
  externalEnvKeys = new Set(),
}) => {
  const { orgId } = useParams<{ orgId: string }>();

  // editingIndex: index of the entry whose proxy is being selected, or null when adding new
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [drawerEnvName, setDrawerEnvName] = useState<string>("");
  const [proxyDrawerOpen, setProxyDrawerOpen] = useState(false);
  const [proxySearchQuery, setProxySearchQuery] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const navigate = useNavigate();

  const { data: environments = [], isLoading: envsLoading } =
    useListEnvironments({ orgName: orgId });
  const targetEnvironments = useMemo(
    () => initialEnvironmentName
      ? environments.filter((env) => env.name === initialEnvironmentName)
      : [],
    [environments, initialEnvironmentName],
  );

  const { data: proxyData, isLoading: proxiesLoading } = useListMCPProxies(
    { orgName: orgId },
    { limit: 50 },
  );

  const proxies = useMemo<ProxyInfo[]>(
    () =>
      (proxyData?.list ?? []).flatMap((p) =>
        p.id
          ? [{
              id: p.id,
              name: p.name ?? p.id,
              version: p.version,
              description: p.description,
              status: p.status,
            }]
          : [],
      ),
    [proxyData],
  );

  const agentNameUpper = agentDisplayName
    ? agentDisplayName.toUpperCase().replace(/[^A-Z0-9]/g, "_")
    : "AGENT";

  const currentDrawerProxyId =
    editingIndex !== null
      ? (mcpProxies[editingIndex]?.selectedProxyByEnv[drawerEnvName]?.id ?? null)
      : null;

  const handleOpenDrawer = useCallback((index: number, envName: string) => {
    setEditingIndex(index);
    setDrawerEnvName(envName);
    setProxyDrawerOpen(true);
  }, []);

  const handleAddNew = useCallback(() => {
    const targetEnvironmentName = targetEnvironments[0]?.name;
    if (!targetEnvironmentName) return;
    setEditingIndex(null);
    setDrawerEnvName(targetEnvironmentName);
    setProxyDrawerOpen(true);
  }, [targetEnvironments]);

  const handleDrawerClose = useCallback(() => {
    if (searchTimerRef.current) {
      clearTimeout(searchTimerRef.current);
      searchTimerRef.current = null;
    }
    setProxyDrawerOpen(false);
    setProxySearchQuery("");
    setDebouncedSearch("");
  }, []);

  const handleProxySelect = useCallback(
    (proxyId: string, proxyName: string) => {
      setMCPProxies((prev) => {
        if (editingIndex === null) {
          if (targetEnvironments.length === 0) return prev;
          const selectedProxyByEnv: MCPProxyFormEntry["selectedProxyByEnv"] = {};
          for (const env of targetEnvironments) {
            selectedProxyByEnv[env.name] = { id: proxyId, name: proxyName };
          }
          const newIndex = prev.length;
          return [
            ...prev,
            {
              selectedProxyByEnv,
              urlVarName: defaultMCPUrlVarName(agentNameUpper, newIndex),
              // apikeyVarName is filled in by EntryCard once the proxy's security
              // resolves, and only when the endpoint actually uses an API key.
            },
          ];
        } else {
          // Changing proxy for an existing entry — only update the active env
          const updated = [...prev];
          const entry = updated[editingIndex];
          if (!entry) return prev;
          updated[editingIndex] = {
            ...entry,
            selectedProxyByEnv: {
              ...entry.selectedProxyByEnv,
              [drawerEnvName]: { id: proxyId, name: proxyName },
            },
            // The new proxy's security hasn't resolved yet. Carrying over the
            // previous proxy's authenticationType/apikeyVarName would let
            // hasUnresolvedMCPSecurity — which keys off authenticationType
            // being undefined — miss this resolve window, and a submit during
            // it could carry the old proxy's api key name onto a proxy that
            // may not use one (issue #1597). EntryCard's effect repopulates
            // both once the newly selected proxy's security resolves.
            authenticationType: undefined,
            apikeyVarName: undefined,
          };
          return updated;
        }
      });
      if (searchTimerRef.current) {
        clearTimeout(searchTimerRef.current);
        searchTimerRef.current = null;
      }
      setProxyDrawerOpen(false);
      setProxySearchQuery("");
      setDebouncedSearch("");
    },
    [editingIndex, drawerEnvName, targetEnvironments, agentNameUpper, setMCPProxies],
  );

  const handleRemoveEntry = useCallback(
    (index: number) => {
      setMCPProxies((prev) => prev.filter((_, i) => i !== index));
    },
    [setMCPProxies],
  );

  const handleUpdateEntry = useCallback(
    (index: number, updated: MCPProxyFormEntry) => {
      setMCPProxies((prev) => {
        const next = [...prev];
        next[index] = updated;
        return next;
      });
    },
    [setMCPProxies],
  );

  const handleSearchChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    setProxySearchQuery(val);
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
    searchTimerRef.current = setTimeout(() => setDebouncedSearch(val), 250);
  }, []);

  return (
    <Form.Section>
      <Form.Subheader>MCP Servers (Optional)</Form.Subheader>

      <Stack spacing={1}>
        {mcpProxies.map((entry, index) => {
          const usedVarNames = new Set([
            ...mcpProxies.flatMap((e, i) =>
              i === index ? [] : mcpEntryVarNames(e, i, agentNameUpper),
            ),
            ...Array.from(externalEnvKeys),
          ]);
          return (
            <EntryCard
              key={index}
              entry={entry}
              index={index}
              proxies={proxies}
              environments={targetEnvironments}
              agentNameUpper={agentNameUpper}
              orgId={orgId}
              usedVarNames={usedVarNames}
              onOpenDrawer={handleOpenDrawer}
              onRemove={handleRemoveEntry}
              onUpdateEntry={handleUpdateEntry}
            />
          );
        })}

        <Box sx={{ pt: mcpProxies.length > 0 ? 1 : 0 }}>
          <Button
            variant="outlined"
            size="small"
            startIcon={<Plus size={16} />}
            onClick={handleAddNew}
            disabled={
              envsLoading ||
              isInitialEnvironmentLoading ||
              proxiesLoading ||
              targetEnvironments.length === 0
            }
          >
            Add
          </Button>
        </Box>
      </Stack>

      <DrawerWrapper
        open={proxyDrawerOpen}
        onClose={handleDrawerClose}
        minWidth={740}
        maxWidth={740}
      >
        <DrawerHeader
          icon={<ServerCog size={24} />}
          title="Select MCP Server"
          onClose={handleDrawerClose}
        />
        <DrawerContent>
          <Stack>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              {editingIndex === null
                ? "Select an MCP server for this configuration."
                : "Change the MCP server for this configuration."}
            </Typography>
            <SearchBar
              placeholder="Search MCP servers"
              size="small"
              fullWidth
              value={proxySearchQuery}
              onChange={handleSearchChange}
              sx={{ mb: 1 }}
            />
            <Stack spacing={1} sx={{ flex: 1, overflowY: "auto" }}>
              {proxiesLoading ? (
                <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
                  <CircularProgress size={32} />
                </Box>
              ) : (() => {
                const filtered = proxies.filter((p) => {
                  if (!debouncedSearch.trim()) return true;
                  const q = debouncedSearch.toLowerCase();
                  return (
                    p.name.toLowerCase().includes(q) ||
                    (p.description ?? "").toLowerCase().includes(q)
                  );
                });

                if (filtered.length === 0) {
                  const isSearchMode = !!debouncedSearch.trim();
                  return (
                    <ListingTable.Container>
                      <ListingTable.EmptyState
                        illustration={<Search size={64} />}
                        title={isSearchMode ? "No MCP servers match your search" : "No MCP servers available"}
                        description={isSearchMode ? "Try a different keyword or clear the search filter." : "No MCP servers found. Add MCP servers from the organization MCP Servers page first."}
                        action={
                          (!isSearchMode && orgId) ? (
                            <Button
                              variant="contained"
                              size="small"
                              startIcon={<Link size={16} />}
                              onClick={() =>
                                navigate(
                                  generatePath(
                                    absoluteRouteMap.children.org.children.
                                      mcpProxies.children.add.path,
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
                  );
                }

                return filtered.map((p) => {
                  const isSelected = currentDrawerProxyId === p.id;
                  const handleClick = () => handleProxySelect(p.id, p.name);
                  return (
                    <Form.CardButton
                      key={p.id}
                      onClick={handleClick}
                      selected={isSelected}
                      aria-label={`${p.name}. ${isSelected ? "Selected" : "Click to select"}`}
                    >
                      <Form.CardContent>
                        <ProxyDisplay proxy={p} isSelected={isSelected} />
                      </Form.CardContent>
                    </Form.CardButton>
                  );
                });
              })()}
            </Stack>
            {orgId && (
              <>
                <Divider sx={{ mt: 2 }} />
                <Box
                  component="a"
                  href={generatePath(
                    absoluteRouteMap.children.org.children.mcpProxies.children.add.path,
                    { orgId },
                  )}
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
                    Register MCP Server
                  </Typography>
                  <ExternalLink size={14} />
                </Box>
              </>
            )}
          </Stack>
        </DrawerContent>
      </DrawerWrapper>
    </Form.Section>
  );
};
