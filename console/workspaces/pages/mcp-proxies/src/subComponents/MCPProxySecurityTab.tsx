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

import { type SetStateAction, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  useDeleteMCPProxyScope,
  useListMCPProxyScopes,
  useUpdateMCPProxyScope,
} from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import type {
  APIKeyLocation,
  Environment,
  MCPEndpointConfig,
  MCPProxy,
  MCPProxyScopeResponse,
} from "@agent-management-platform/types";
import {
  Alert,
  Autocomplete,
  Button,
  Chip,
  Collapse,
  FormControl,
  FormLabel,
  Grid,
  IconButton,
  ListingTable,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, ShieldX, Trash } from "@wso2/oxygen-ui-icons-react";
import {
  type AuthenticationType,
  getAuthenticationTypeLabel,
  getCapabilityId,
  isToolBlockedByAcl,
  resolveAuthenticationType,
} from "./mcpEndpoints";
import { CreateScopeDrawer } from "./CreateScopeDrawer";

const KEY_LOCATION_OPTIONS: { value: APIKeyLocation; label: string }[] = [
  { value: "header", label: "header" },
  { value: "query", label: "query" },
];

const AUTHENTICATION_TYPE_OPTIONS: AuthenticationType[] = [
  "",
  "apiKey",
  "identity",
];

// A local editable row for the identity-security tool-scope-binding table.
// One row per tool discovered on this endpoint — every tool always has a
// row, whether or not it currently has any scopes assigned — so the tool
// itself doubles as the row's key.
type ToolScopeRow = {
  tool: string;
  scopes: MCPProxyScopeResponse[];
};

// One scope's replacement tool list, as sent by Save.
type ScopeToolUpdate = { action: string; tools: string[] };

// Builds one row per discovered tool, seeded with whichever scopes reference it.
// `appliedUpdates` overrides a scope's tool list, so a Save that committed only
// some of its updates can derive the rows the server now holds rather than the
// ones it last returned.
function toolScopeRowsFor(
  toolEntries: string[],
  scopes: MCPProxyScopeResponse[],
  appliedUpdates: ScopeToolUpdate[] = [],
): ToolScopeRow[] {
  const toolsByAction = new Map(appliedUpdates.map((u) => [u.action, u.tools]));
  const toolToScopes = new Map<string, MCPProxyScopeResponse[]>();
  for (const scope of scopes) {
    for (const tool of toolsByAction.get(scope.action) ?? scope.tools) {
      const scopesForTool = toolToScopes.get(tool) ?? [];
      scopesForTool.push(scope);
      toolToScopes.set(tool, scopesForTool);
    }
  }
  return toolEntries.map((tool) => ({
    tool,
    scopes: toolToScopes.get(tool) ?? [],
  }));
}

// Compares rows by tool and scope-action set only — the identity that Save
// actually writes, ignoring incidental churn in the scope objects themselves.
const serializeToolScopeRows = (rows: ToolScopeRow[]) =>
  JSON.stringify(
    rows.map((row) => ({
      tool: row.tool,
      scopes: row.scopes.map((s) => s.action).sort(),
    })),
  );

// Diffs the desired (action -> tools) mapping built from the current rows
// against the last-fetched scope list, producing only the tool-list updates
// needed to bring the server in sync. Scopes themselves are created via the
// Create Scope panel and deleted explicitly from the scopes list below — a
// scope ending up with zero tools here is still a valid, saved state, not a
// signal to delete it, and the server accepts the empty list (see
// validateScopeTools in mcp_proxy_scope_service.go).
function computeScopeToolUpdates(
  rows: ToolScopeRow[],
  catalogScopes: MCPProxyScopeResponse[],
): ScopeToolUpdate[] {
  const desired = new Map<string, Set<string>>(
    catalogScopes.map((s) => [s.action, new Set<string>()]),
  );
  for (const row of rows) {
    for (const scope of row.scopes) {
      desired.get(scope.action)?.add(row.tool);
    }
  }
  const setsEqual = (a: Set<string>, b: Set<string>) =>
    a.size === b.size && [...a].every((v) => b.has(v));

  const updates: ScopeToolUpdate[] = [];
  for (const scope of catalogScopes) {
    const desiredTools = desired.get(scope.action) ?? new Set<string>();
    if (!setsEqual(desiredTools, new Set(scope.tools))) {
      updates.push({ action: scope.action, tools: [...desiredTools] });
    }
  }
  return updates;
}

export type MCPProxySecurityTabProps = {
  config: MCPEndpointConfig | undefined;
  selectedEndpointId: string;
  orgName: string | undefined;
  proxyId: string | undefined;
  /**
   * Environments the selected endpoint is deployed to, for the Create Scope
   * panel's role picker.
   */
  environments: Environment[];
  isLoading?: boolean;
  onUpdate: (fields: Partial<MCPEndpointConfig>) => Promise<MCPProxy>;
  isUpdating: boolean;
};

export function MCPProxySecurityTab({
  config,
  selectedEndpointId,
  orgName,
  proxyId,
  environments,
  isLoading = false,
  onUpdate,
  isUpdating,
}: MCPProxySecurityTabProps) {
  const [authenticationType, setAuthenticationType] =
    useState<AuthenticationType>("apiKey");
  const [keyValue, setKeyValue] = useState("");
  const [keyIn, setKeyIn] = useState<APIKeyLocation>("header");
  const [status, setStatus] = useState<{
    message: string;
    severity: "success" | "error";
  } | null>(null);
  const [fieldErrors, setFieldErrors] = useState<{ keyValue?: string }>({});
  // Covers the whole of handleSave, unlike the isUpdating prop, which only
  // tracks the endpoint-config mutation and goes idle again while the
  // scope-update loop below it is still running.
  const [isSaving, setIsSaving] = useState(false);
  const [createScopeDrawerOpen, setCreateScopeDrawerOpen] = useState(false);
  const [authorizationTab, setAuthorizationTab] = useState<"tools" | "scopes">("tools");
  const { addConfirmation } = useConfirmationDialog();

  // Tracks what was last confirmed persisted (seeded from config, refreshed on
  // save) rather than re-deriving "saved" straight from the config prop on
  // every render — config only reflects a save once its background refetch
  // lands, which would otherwise leave authIsDirty true for a beat after a
  // successful save.
  const lastSavedAuthRef = useRef<{
    type: AuthenticationType;
    key: string;
    in: APIKeyLocation;
  }>({ type: "apiKey", key: "", in: "header" });

  const authIsDirty = useMemo(() => {
    if (!config) return false;
    const saved = lastSavedAuthRef.current;
    if (authenticationType !== saved.type) return true;
    if (keyValue.trim() !== saved.key) return true;
    if (keyIn !== saved.in) return true;
    return false;
  }, [config, authenticationType, keyValue, keyIn]);

  useEffect(() => {
    if (!config || !selectedEndpointId) return;
    const nextType = resolveAuthenticationType(config);
    const nextKey =
      config.security?.apiKey?.key ?? (nextType === "apiKey" ? "X-API-Key" : "");
    const nextIn = (config.security?.apiKey?.in as APIKeyLocation) ?? "header";
    setAuthenticationType(nextType);
    setKeyValue(nextKey);
    setKeyIn(nextIn);
    setFieldErrors({});
    lastSavedAuthRef.current = { type: nextType, key: nextKey, in: nextIn };
  }, [config, selectedEndpointId]);

  // --- Agent Identity: per-tool scope-binding (RBAC) state ---

  const toolEntries = useMemo(() => {
    const identifiers: string[] = [];
    for (const raw of config?.capabilities?.tools ?? []) {
      const identifier = getCapabilityId("tool", raw);
      if (identifier) identifiers.push(identifier);
    }
    return identifiers;
  }, [config?.capabilities?.tools]);

  // Computed once per tool list / ACL policy change rather than per row and per
  // dropdown option on every render — isToolBlockedByAcl re-parses the ACL
  // policy's params each call, and every row's Select renders one option per tool.
  const blockedToolIds = useMemo(
    () =>
      new Set(
        toolEntries.filter((identifier) =>
          isToolBlockedByAcl(config, identifier),
        ),
      ),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only reads config.policies
    [toolEntries, config?.policies],
  );

  const { data: scopesData, isPending: scopesPending } = useListMCPProxyScopes(
    { orgName: orgName ?? "", proxyId: proxyId ?? "" },
    { enabled: authenticationType === "identity" && !!proxyId },
  );
  // The persisted source of truth: what the tool-scope table diffs against on
  // Save, and the only source of options the Autocomplete offers (scopes are
  // created via the Create Scope panel, not from this table).
  const catalogScopes: MCPProxyScopeResponse[] = useMemo(
    () => scopesData?.scopes ?? [],
    [scopesData],
  );
  const updateMCPProxyScope = useUpdateMCPProxyScope();
  const deleteMCPProxyScope = useDeleteMCPProxyScope();

  // One row per known tool, pre-populated from the endpoint's discovered
  // tool list so scopes can be assigned directly — no separate "add tool"
  // step. Starts empty and is seeded by the effects below once the tool
  // list and scope catalog are both available.
  const [toolScopeRows, setToolScopeRows] = useState<ToolScopeRow[]>([]);
  // Must be state, not a ref: toolScopesDirty is memoized on it and the reseed
  // effect below is gated on toolScopesDirty, so a ref write would leave the tab
  // pinned as dirty and never adopt the refetched catalog. (lastSavedAuthRef
  // above can stay a ref: authIsDirty already re-derives from the config prop,
  // which the same refetch replaces.)
  const [lastSavedToolScopeRows, setLastSavedToolScopeRows] = useState<ToolScopeRow[]>([]);

  // Rows and their saved snapshot move together whenever the server's state
  // changes, as opposed to a user edit, which moves the rows alone. Missing the
  // paired write is what pinned the tab as dirty before.
  const resetToolScopeRows = useCallback((rows: SetStateAction<ToolScopeRow[]>) => {
    setToolScopeRows(rows);
    setLastSavedToolScopeRows(rows);
  }, []);

  const toolScopesDirty = useMemo(
    () =>
      serializeToolScopeRows(toolScopeRows) !==
      serializeToolScopeRows(lastSavedToolScopeRows),
    [toolScopeRows, lastSavedToolScopeRows],
  );

  // Memoized since both reseed effects below would otherwise rebuild this from
  // scratch on every render they fire in, even when neither input changed.
  const derivedToolScopeRows = useMemo<ToolScopeRow[]>(
    () => toolScopeRowsFor(toolEntries, catalogScopes),
    [toolEntries, catalogScopes],
  );

  // Switching endpoint tabs discards unsaved row edits, consistent with the
  // auth-fields effect above — even though scopes are shared across every
  // endpoint of the proxy, this tab's Save/Discard state is still per endpoint.
  useEffect(() => {
    if (!selectedEndpointId) return;
    resetToolScopeRows(derivedToolScopeRows);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- handled by the effect below
  }, [selectedEndpointId]);

  // Reseed when the scope list refetches (e.g. right after Save invalidates
  // the query), but only while there are no unsaved edits — otherwise this
  // would clobber in-progress changes on a stray background refetch. Doesn't
  // depend on selectedEndpointId — the effect above already handles tab
  // switches, and scopes are proxy-level so switching tabs alone never
  // changes catalogScopes.
  useEffect(() => {
    if (!selectedEndpointId || toolScopesDirty) return;
    resetToolScopeRows(derivedToolScopeRows);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- guard, not a trigger dep
  }, [catalogScopes]);

  const setRowScopes = useCallback((tool: string, scopes: MCPProxyScopeResponse[]) => {
    setToolScopeRows((prev) =>
      prev.map((row) => (row.tool === tool ? { ...row, scopes } : row)),
    );
  }, []);

  const handleToolScopeRowScopesChange = useCallback(
    (tool: string, options: MCPProxyScopeResponse[]) => {
      setRowScopes(tool, options);
    },
    [setRowScopes],
  );

  // The saved method, used both to warn about a pending switch and to confirm
  // it at Save time. Read from `config`, not lastSavedAuthRef, which defaults
  // to "apiKey" until the sync effect above runs and would otherwise warn
  // about a method nobody actually configured.
  const savedAuthenticationType = useMemo(
    () => resolveAuthenticationType(config),
    [config],
  );

  // Switching methods breaks agents already configured against this proxy,
  // but selecting a method in the dropdown changes nothing until Save — so
  // this warns inline (see the Alert below) instead of interrupting with a
  // dialog that reads as if the change had already been applied.
  const pendingAuthTypeSwitch =
    !!savedAuthenticationType && authenticationType !== savedAuthenticationType;

  const handleAuthTypeChange = useCallback((nextType: AuthenticationType) => {
    setAuthenticationType(nextType);
  }, []);

  const isDirty = authIsDirty || toolScopesDirty;
  // Gates every control a save touches, so no edit or second Save can land
  // while updates are still in flight.
  const saveInProgress = isUpdating || isSaving;

  const handleDiscard = useCallback(() => {
    if (!config) return;
    const nextType = resolveAuthenticationType(config);
    setAuthenticationType(nextType);
    setKeyValue(
      config.security?.apiKey?.key ??
      (nextType === "apiKey" ? "X-API-Key" : ""),
    );
    setKeyIn((config.security?.apiKey?.in as APIKeyLocation) ?? "header");
    setFieldErrors({});
    setStatus(null);

    setToolScopeRows(lastSavedToolScopeRows);
  }, [config, lastSavedToolScopeRows]);

  const performSave = useCallback(async () => {
    if (!config) return;

    if (authenticationType === "apiKey" && keyValue.trim().length === 0) {
      const message = "API Key is required when using API Key authentication";
      setFieldErrors({ keyValue: message });
      setStatus({ message, severity: "error" });
      return;
    }
    setFieldErrors({});

    setIsSaving(true);
    try {
      await onUpdate({
        security: {
          enabled: config.security?.enabled ?? true,
          apiKey: {
            enabled: authenticationType === "apiKey",
            key: authenticationType === "apiKey" ? keyValue.trim() : "",
            in: keyIn,
          },
          identity: {
            enabled: authenticationType === "identity",
          },
        },
      });

      // Record the auth snapshot as soon as the auth mutation itself
      // succeeds — independent of whether the scope-update step below
      // (a separate set of REST calls) succeeds, so a failure there doesn't
      // leave the already-saved auth settings looking dirty.
      lastSavedAuthRef.current = {
        type: authenticationType,
        key: authenticationType === "apiKey" ? keyValue.trim() : "",
        in: keyIn,
      };

      // Scopes belong to the proxy, not this endpoint's auth mode, and are
      // saved via their own REST calls rather than bundled into the security
      // payload above.
      if (authenticationType === "identity" && toolScopesDirty && orgName && proxyId) {
        const updates = computeScopeToolUpdates(toolScopeRows, catalogScopes);

        // Sequential rather than Promise.all: a rejection must not leave the
        // remaining updates in flight, and the committed prefix has to be
        // knowable so the snapshot below can record what the server holds.
        let committedCount = 0;
        try {
          for (const u of updates) {
            await updateMCPProxyScope.mutateAsync({
              params: { orgName, proxyId, scopeAction: u.action },
              body: { tools: u.tools },
            });
            committedCount += 1;
          }
        } finally {
          // Keeping the pre-save snapshot instead would leave Discard restoring
          // a state the server no longer has.
          setLastSavedToolScopeRows(
            committedCount === updates.length
              ? toolScopeRows
              : toolScopeRowsFor(toolEntries, catalogScopes, updates.slice(0, committedCount)),
          );
        }
      }

      setStatus({
        message: "Updated security settings.",
        severity: "success",
      });
    } catch {
      setStatus({
        message: "Failed to update security.",
        severity: "error",
      });
    } finally {
      setIsSaving(false);
    }
  }, [
    config,
    authenticationType,
    keyValue,
    keyIn,
    toolScopeRows,
    toolScopesDirty,
    catalogScopes,
    toolEntries,
    orgName,
    proxyId,
    onUpdate,
    updateMCPProxyScope,
  ]);

  // The only confirmation in this flow, and it sits on the action that
  // actually changes the proxy. Cancelling here leaves the pending selection
  // intact so the user can adjust it rather than losing their edits.
  const handleSave = useCallback(() => {
    if (pendingAuthTypeSwitch) {
      addConfirmation({
        title: "Apply new authentication method?",
        description: `This proxy is currently secured with ${getAuthenticationTypeLabel(savedAuthenticationType)}. Saving will switch it to ${getAuthenticationTypeLabel(authenticationType)} and break any agent already configured to use it, until their tool configuration is updated to match.`,
        confirmButtonText: "Save & Apply",
        confirmButtonColor: "warning",
        onConfirm: () => void performSave(),
      });
      return;
    }
    void performSave();
  }, [
    pendingAuthTypeSwitch,
    addConfirmation,
    savedAuthenticationType,
    authenticationType,
    performSave,
  ]);

  const handleDeleteScope = useCallback(
    (scope: MCPProxyScopeResponse) => {
      if (!orgName || !proxyId) return;
      addConfirmation({
        title: "Delete Scope",
        description: `Are you sure you want to delete "${scope.action}"? Any tool it's currently assigned to will lose that requirement, and any role granted it will lose access through it. This action cannot be undone.`,
        confirmButtonText: "Delete",
        confirmButtonColor: "error",
        onConfirm: async () => {
          await deleteMCPProxyScope.mutateAsync({
            orgName,
            proxyId,
            scopeAction: scope.action,
          });
          // Drop it from any row referencing it immediately — the reseed
          // effect above only reseeds while there are no unsaved edits, and a
          // deleted scope shouldn't linger as a selectable/selected option.
          // Also drop it from the last-saved snapshot: it's been deleted on
          // the server, so keeping it there would make toolScopesDirty
          // compare against a stale scope and let Discard resurrect it.
          const dropDeletedScope = (rows: ToolScopeRow[]) =>
            rows.map((row) => ({
              ...row,
              scopes: row.scopes.filter((s) => s.action !== scope.action),
            }));
          resetToolScopeRows(dropDeletedScope);
        },
      });
    },
    [orgName, proxyId, addConfirmation, deleteMCPProxyScope, resetToolScopeRows],
  );

  const isDisabled = isLoading || !config;
  const hasNoDiscoveredTools = !isLoading && !!config && toolEntries.length === 0;

  if (isLoading) {
    return (
      <Stack spacing={2}>
        <Typography variant="h6">Authentication</Typography>
        <Stack spacing={2}>
          {[1, 2, 3].map((i) => (
            <Stack key={i} spacing={0.5}>
              <Skeleton variant="text" width={120} height={16} />
              <Skeleton variant="rounded" height={40} />
            </Stack>
          ))}
        </Stack>
      </Stack>
    );
  }

  if (!config) {
    return null;
  }

  return (
    <Stack spacing={2}>
      <Typography variant="h6">Authentication</Typography>

      <Grid container spacing={3}>
        <Grid size={{ xs: 12, md: 5 }}>
          <FormControl fullWidth disabled={isDisabled}>
            <FormLabel>Method</FormLabel>
            <Select
              size="small"
              displayEmpty
              value={authenticationType || ""}
              onChange={(e) =>
                handleAuthTypeChange((e.target.value as AuthenticationType) || "")
              }
            >
              {AUTHENTICATION_TYPE_OPTIONS.map((type) => (
                <MenuItem key={type || "none"} value={type}>
                  {getAuthenticationTypeLabel(type)}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Grid>
      </Grid>

      {/* Replaces the old switch-time modal: the impact stays visible for as
          long as the change is pending, and says plainly that nothing has
          been applied yet. */}
      <Collapse in={pendingAuthTypeSwitch} timeout={300}>
        <Alert severity="warning" sx={{ width: "100%", maxWidth: 640 }}>
          <Typography variant="body2" fontWeight={600} gutterBottom>
            Not applied yet — Save to apply
          </Typography>
          This proxy is still secured with{" "}
          {getAuthenticationTypeLabel(savedAuthenticationType)}. Switching to{" "}
          {getAuthenticationTypeLabel(authenticationType)} will break any agent
          already configured to use it, until their tool configuration is
          updated to match. Use Discard to keep the current method.
        </Alert>
      </Collapse>

      {authenticationType === "identity" && (
        <Stack spacing={2}>
          <Stack spacing={0.5}>
            <Typography variant="h6">Authorization</Typography>
            <Typography variant="body2" color="text.secondary">
              Restrict access to individual tools by assigning catalog
              scopes. Callers need a token that includes every scope
              required by a tool to invoke it.
            </Typography>
          </Stack>

          <Stack direction="row" alignItems="center" justifyContent="flex-end">
            <Button
              variant="outlined"
              size="small"
              startIcon={<Plus size={16} />}
              onClick={() => setCreateScopeDrawerOpen(true)}
            >
              Create Scope
            </Button>
          </Stack>

          <Tabs
            value={authorizationTab}
            onChange={(_e, value) => setAuthorizationTab(value)}
            sx={{ borderBottom: 1, borderColor: "divider" }}
          >
            <Tab label="Tools" value="tools" />
            <Tab label="Scopes" value="scopes" />
          </Tabs>

          {authorizationTab === "tools" &&
            (hasNoDiscoveredTools ? (
              <ListingTable.Container>
                <ListingTable.EmptyState
                  illustration={<ShieldX size={64} />}
                  title="No Tools Available"
                  description="This MCP Server has no tools to bind scopes to. Scopes can still be created and granted to roles."
                />
              </ListingTable.Container>
            ) : (
              <ListingTable.Container>
                <ListingTable density="compact">
                  <ListingTable.Head>
                    <ListingTable.Row>
                      <ListingTable.Cell width="30%">Tool</ListingTable.Cell>
                      <ListingTable.Cell>Scopes</ListingTable.Cell>
                    </ListingTable.Row>
                  </ListingTable.Head>
                  <ListingTable.Body>
                    {toolScopeRows.map((row) => (
                      <ListingTable.Row key={row.tool}>
                        <ListingTable.Cell>
                          <Stack direction="row" alignItems="center" spacing={1}>
                            <Typography
                              component="span"
                              variant="body2"
                              sx={{ fontFamily: "monospace" }}
                              noWrap
                            >
                              {row.tool}
                            </Typography>
                            {blockedToolIds.has(row.tool) && (
                              <Tooltip title="Blocked by Manage Tools">
                                <Stack color="warning.main" direction="row" alignItems="center">
                                  <ShieldX size={14} />
                                </Stack>
                              </Tooltip>
                            )}
                          </Stack>
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          <Autocomplete
                            multiple
                            size="small"
                            disableCloseOnSelect
                            disabled={isDisabled || saveInProgress}
                            options={catalogScopes}
                            value={row.scopes}
                            onChange={(_e, value) =>
                              handleToolScopeRowScopesChange(row.tool, value)
                            }
                            getOptionLabel={(option) => option.action}
                            isOptionEqualToValue={(option, value) => option.action === value.action}
                            renderTags={(value, getTagProps) =>
                              value.map((option, index) => (
                                <Chip
                                  {...getTagProps({ index })}
                                  key={option.action}
                                  label={option.action}
                                  size="small"
                                />
                              ))
                            }
                            renderInput={(params) => (
                              <TextField {...params} placeholder="Assign scopes..." />
                            )}
                            noOptionsText="No scopes in the catalog"
                            sx={{ minWidth: 280 }}
                          />
                        </ListingTable.Cell>
                      </ListingTable.Row>
                    ))}
                  </ListingTable.Body>
                </ListingTable>
              </ListingTable.Container>
            ))}

          {authorizationTab === "scopes" &&
            (!scopesPending && catalogScopes.length === 0 ? (
              <ListingTable.Container>
                <ListingTable.EmptyState
                  illustration={<ShieldX size={64} />}
                  title="No scopes yet"
                  description='Click "Create Scope" to add one.'
                />
              </ListingTable.Container>
            ) : (
              <ListingTable.Container>
                <ListingTable density="compact">
                  <ListingTable.Head>
                    <ListingTable.Row>
                      <ListingTable.Cell width="30%">Name</ListingTable.Cell>
                      <ListingTable.Cell>Description</ListingTable.Cell>
                      <ListingTable.Cell align="center" width="60px" />
                    </ListingTable.Row>
                  </ListingTable.Head>
                  <ListingTable.Body>
                    {catalogScopes.map((scope) => (
                      <ListingTable.Row key={scope.action}>
                        <ListingTable.Cell>
                          <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
                            {scope.action}
                          </Typography>
                        </ListingTable.Cell>
                        <ListingTable.Cell>{scope.description || "-"}</ListingTable.Cell>
                        <ListingTable.Cell align="center">
                          <Tooltip title="Delete scope">
                            <IconButton
                              size="small"
                              color="error"
                              onClick={() => handleDeleteScope(scope)}
                            >
                              <Trash size={16} />
                            </IconButton>
                          </Tooltip>
                        </ListingTable.Cell>
                      </ListingTable.Row>
                    ))}
                  </ListingTable.Body>
                </ListingTable>
              </ListingTable.Container>
            ))}
        </Stack>
      )}

      {authenticationType === "apiKey" && (
        <Grid container spacing={3}>
          <Grid size={{ xs: 12, md: 5 }}>
            <FormControl fullWidth disabled={isDisabled}>
              <FormLabel>Key Location</FormLabel>
              <Select
                size="small"
                value={keyIn}
                onChange={(e) => setKeyIn(e.target.value as APIKeyLocation)}
              >
                {KEY_LOCATION_OPTIONS.map((opt) => (
                  <MenuItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Grid>
          <Grid size={{ xs: 12, md: 5 }}>
            <FormControl
              fullWidth
              disabled={isDisabled}
              error={!!fieldErrors.keyValue}
            >
              <FormLabel>
                {keyIn === "query" ? "Query Param Key" : "Header Key"}
              </FormLabel>
              <TextField
                size="small"
                value={keyValue}
                onChange={(e) => {
                  setKeyValue(e.target.value);
                  if (fieldErrors.keyValue) setFieldErrors({});
                }}
                error={!!fieldErrors.keyValue}
                helperText={fieldErrors.keyValue}
                sx={{
                  "& .MuiInputBase-input": {
                    fontFamily: "monospace",
                  },
                }}
              />
            </FormControl>
          </Grid>
        </Grid>
      )}

      <Stack spacing={1.5} width="100%">
        {/* Success messages hide as soon as the user edits again, but errors
            must not: a failed save leaves the rows it couldn't commit dirty,
            which would otherwise swallow the only report of the failure. */}
        <Collapse in={!!status && (status.severity === "error" || !isDirty)} timeout={300}>
          {status && (
            <Alert
              severity={status.severity}
              onClose={() => setStatus(null)}
              sx={{ width: "100%", maxWidth: 480 }}
            >
              {status.message}
            </Alert>
          )}
        </Collapse>
        <Stack direction="row" spacing={1.5} justifyContent="flex-end">
          <Button
            variant="outlined"
            onClick={handleDiscard}
            disabled={!isDirty || saveInProgress}
          >
            Discard
          </Button>
          <Button
            variant="contained"
            onClick={handleSave}
            disabled={saveInProgress || !isDirty}
          >
            {saveInProgress ? "Saving..." : "Save"}
          </Button>
        </Stack>
      </Stack>

      {orgName && proxyId && (
        <CreateScopeDrawer
          open={createScopeDrawerOpen}
          onClose={() => setCreateScopeDrawerOpen(false)}
          orgName={orgName}
          proxyId={proxyId}
          environments={environments}
          tools={toolEntries}
          blockedTools={blockedToolIds}
        />
      )}
    </Stack>
  );
}
