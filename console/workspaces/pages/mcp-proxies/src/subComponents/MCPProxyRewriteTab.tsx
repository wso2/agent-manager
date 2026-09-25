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

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Button,
  Card,
  Chip,
  Collapse,
  List,
  ListItemButton,
  ListItemText,
  Skeleton,
  Stack,
  Switch,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown } from "@wso2/oxygen-ui-icons-react";
import { useMCPPoliciesCatalog } from "@agent-management-platform/api-client";
import {
  type MCPEndpointConfig,
  type MCPProxy,
  type MCPProxyPolicy,
  INPUT_LIMITS,
} from "@agent-management-platform/types";
import { TextInput } from "@agent-management-platform/views";
import { REWRITE_POLICY_NAME } from "../constants";
import type { CapabilityKind } from "./mcpEndpoints";

const KIND_LABEL: Record<CapabilityKind, string> = {
  tool: "Tool",
  resource: "Resource",
  prompt: "Prompt",
};

const KIND_CHIP_COLOR: Record<CapabilityKind, "info" | "success" | "error"> = {
  tool: "info",
  resource: "success",
  prompt: "error",
};

type ToolEntry = {
  name: string;
  description: string;
  inputSchema: string;
  outputSchema: string;
  target: string;
};

type ResourceEntry = {
  uri: string;
  description: string;
  target: string;
};

type PromptEntry = {
  name: string;
  description: string;
  target: string;
};

type RewriteState = {
  tool: Record<string, ToolEntry>;
  resource: Record<string, ResourceEntry>;
  prompt: Record<string, PromptEntry>;
};

type ItemMeta = {
  kind: CapabilityKind;
  /** Backend identifier — stable key for this capability. */
  backendId: string;
  /** Label shown in the list (defaults to backendId). */
  label: string;
};

function stringifyMaybe(value: unknown): string {
  if (value === undefined || value === null) return "";
  if (typeof value === "string") return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return "";
  }
}

function getBackendId(
  kind: CapabilityKind,
  raw: Record<string, unknown>,
): string | null {
  const value = kind === "resource" ? raw.uri ?? raw.name : raw.name ?? raw.uri;
  if (typeof value !== "string") return null;
  const trimmed = value.trim();
  return trimmed.length ? trimmed : null;
}

function buildDefaultsFromCapabilities(
  config: MCPEndpointConfig | undefined,
): { state: RewriteState; meta: ItemMeta[] } {
  const state: RewriteState = { tool: {}, resource: {}, prompt: {} };
  const meta: ItemMeta[] = [];
  const tools = config?.capabilities?.tools ?? [];
  const resources = config?.capabilities?.resources ?? [];
  const prompts = config?.capabilities?.prompts ?? [];

  tools.forEach((raw) => {
    const id = getBackendId("tool", raw);
    if (!id) return;
    state.tool[id] = {
      name: typeof raw.name === "string" ? raw.name : id,
      description:
        typeof raw.description === "string" ? raw.description : "",
      inputSchema: stringifyMaybe(raw.inputSchema),
      outputSchema: stringifyMaybe(raw.outputSchema),
      target: id,
    };
    meta.push({ kind: "tool", backendId: id, label: id });
  });
  resources.forEach((raw) => {
    const id = getBackendId("resource", raw);
    if (!id) return;
    state.resource[id] = {
      uri: typeof raw.uri === "string" ? raw.uri : id,
      description:
        typeof raw.description === "string" ? raw.description : "",
      target: id,
    };
    meta.push({ kind: "resource", backendId: id, label: id });
  });
  prompts.forEach((raw) => {
    const id = getBackendId("prompt", raw);
    if (!id) return;
    state.prompt[id] = {
      name: typeof raw.name === "string" ? raw.name : id,
      description:
        typeof raw.description === "string" ? raw.description : "",
      target: id,
    };
    meta.push({ kind: "prompt", backendId: id, label: id });
  });

  return { state, meta };
}

function mergeExistingPolicy(
  defaults: RewriteState,
  policy: MCPProxyPolicy | undefined,
): RewriteState {
  if (!policy?.params) return defaults;
  const params = policy.params as Record<string, unknown>;
  const next: RewriteState = {
    tool: { ...defaults.tool },
    resource: { ...defaults.resource },
    prompt: { ...defaults.prompt },
  };

  const toolEntries = Array.isArray(params.tools)
    ? (params.tools as Record<string, unknown>[])
    : [];
  toolEntries.forEach((entry) => {
    const backendId =
      typeof entry.target === "string" && entry.target.trim()
        ? entry.target.trim()
        : typeof entry.name === "string"
          ? entry.name.trim()
          : "";
    if (!backendId || !(backendId in next.tool)) return;
    next.tool[backendId] = {
      name:
        typeof entry.name === "string"
          ? entry.name
          : next.tool[backendId].name,
      description:
        typeof entry.description === "string"
          ? entry.description
          : next.tool[backendId].description,
      inputSchema:
        typeof entry.inputSchema === "string"
          ? entry.inputSchema
          : next.tool[backendId].inputSchema,
      outputSchema:
        typeof entry.outputSchema === "string"
          ? entry.outputSchema
          : next.tool[backendId].outputSchema,
      target:
        typeof entry.target === "string" && entry.target.trim()
          ? entry.target
          : next.tool[backendId].target,
    };
  });

  const resourceEntries = Array.isArray(params.resources)
    ? (params.resources as Record<string, unknown>[])
    : [];
  resourceEntries.forEach((entry) => {
    const backendId =
      typeof entry.target === "string" && entry.target.trim()
        ? entry.target.trim()
        : typeof entry.uri === "string"
          ? entry.uri.trim()
          : "";
    if (!backendId || !(backendId in next.resource)) return;
    next.resource[backendId] = {
      uri:
        typeof entry.uri === "string"
          ? entry.uri
          : next.resource[backendId].uri,
      description:
        typeof entry.description === "string"
          ? entry.description
          : next.resource[backendId].description,
      target:
        typeof entry.target === "string" && entry.target.trim()
          ? entry.target
          : next.resource[backendId].target,
    };
  });

  const promptEntries = Array.isArray(params.prompts)
    ? (params.prompts as Record<string, unknown>[])
    : [];
  promptEntries.forEach((entry) => {
    const backendId =
      typeof entry.target === "string" && entry.target.trim()
        ? entry.target.trim()
        : typeof entry.name === "string"
          ? entry.name.trim()
          : "";
    if (!backendId || !(backendId in next.prompt)) return;
    next.prompt[backendId] = {
      name:
        typeof entry.name === "string"
          ? entry.name
          : next.prompt[backendId].name,
      description:
        typeof entry.description === "string"
          ? entry.description
          : next.prompt[backendId].description,
      target:
        typeof entry.target === "string" && entry.target.trim()
          ? entry.target
          : next.prompt[backendId].target,
    };
  });

  return next;
}

function buildPolicyParams(
  state: RewriteState,
  meta: ItemMeta[],
): Record<string, unknown> {
  const hasTools = meta.some((m) => m.kind === "tool");
  const hasResources = meta.some((m) => m.kind === "resource");
  const hasPrompts = meta.some((m) => m.kind === "prompt");
  const params: Record<string, unknown> = {};
  if (hasTools) {
    params.tools = meta
      .filter((m) => m.kind === "tool")
      .map((m) => {
        const entry = state.tool[m.backendId];
        const out: Record<string, unknown> = {
          name: entry.name,
          // description and inputSchema are required for tools; fall back to
          // the backend id so the gateway's minLength: 1 validation passes
          // when the upstream MCP server reports an empty value.
          description: entry.description.trim() || m.backendId,
          inputSchema:
            entry.inputSchema.trim() || '{"type":"object"}',
          target: entry.target.trim() ? entry.target : m.backendId,
        };
        if (entry.outputSchema.trim()) {
          out.outputSchema = entry.outputSchema;
        }
        return out;
      });
  }
  if (hasResources) {
    params.resources = meta
      .filter((m) => m.kind === "resource")
      .map((m) => {
        const entry = state.resource[m.backendId];
        const out: Record<string, unknown> = {
          name: entry.uri,
          uri: entry.uri,
          target: entry.target.trim() ? entry.target : m.backendId,
        };
        // description is optional for resources; the gateway enforces
        // minLength: 1 when present, so omit it entirely if blank.
        if (entry.description.trim()) {
          out.description = entry.description;
        }
        return out;
      });
  }
  if (hasPrompts) {
    params.prompts = meta
      .filter((m) => m.kind === "prompt")
      .map((m) => {
        const entry = state.prompt[m.backendId];
        const out: Record<string, unknown> = {
          name: entry.name,
          target: entry.target.trim() ? entry.target : m.backendId,
        };
        if (entry.description.trim()) {
          out.description = entry.description;
        }
        return out;
      });
  }
  return params;
}

export type MCPProxyRewriteTabProps = {
  config: MCPEndpointConfig | undefined;
  selectedEndpointId: string;
  orgName: string | undefined;
  isLoading?: boolean;
  onUpdate: (fields: Partial<MCPEndpointConfig>) => Promise<MCPProxy>;
  isUpdating: boolean;
};

export function MCPProxyRewriteTab({
  config,
  selectedEndpointId,
  orgName,
  isLoading = false,
  onUpdate,
  isUpdating,
}: MCPProxyRewriteTabProps) {
  const lastSavedRef = useRef<{
    enabled: boolean;
    state: RewriteState;
  } | null>(null);
  const [enabled, setEnabled] = useState(false);
  const [state, setState] = useState<RewriteState>({
    tool: {},
    resource: {},
    prompt: {},
  });
  const [meta, setMeta] = useState<ItemMeta[]>([]);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [openSections, setOpenSections] = useState<
    Record<CapabilityKind, boolean>
  >({ tool: true, resource: true, prompt: true });
  const [status, setStatus] = useState<{
    message: string;
    severity: "success" | "error";
  } | null>(null);

  const { data: catalog, isLoading: isCatalogLoading } = useMCPPoliciesCatalog(
    orgName,
  );
  const availableRewritePolicy = useMemo(
    () => catalog?.data?.find((p) => p.name === REWRITE_POLICY_NAME),
    [catalog],
  );

  useEffect(() => {
    if (!config || !selectedEndpointId) return;
    const { state: defaults, meta: defaultsMeta } =
      buildDefaultsFromCapabilities(config);
    const existing = config.policies?.find(
      (p) => p.name === REWRITE_POLICY_NAME,
    );
    const merged = mergeExistingPolicy(defaults, existing);
    const isEnabled = !!existing;
    setEnabled(isEnabled);
    setState(merged);
    setMeta(defaultsMeta);
    lastSavedRef.current = { enabled: isEnabled, state: merged };
    setSelectedKey((current) => {
      if (current) {
        const stillExists = defaultsMeta.some(
          (m) => `${m.kind}::${m.backendId}` === current,
        );
        if (stillExists) return current;
      }
      const first = defaultsMeta[0];
      return first ? `${first.kind}::${first.backendId}` : null;
    });
  }, [config, selectedEndpointId]);

  const selected = useMemo(() => {
    if (!selectedKey) return null;
    const idx = selectedKey.indexOf("::");
    if (idx < 0) return null;
    const kind = selectedKey.slice(0, idx) as CapabilityKind;
    const backendId = selectedKey.slice(idx + 2);
    const m = meta.find(
      (entry) => entry.kind === kind && entry.backendId === backendId,
    );
    if (!m) return null;
    return m;
  }, [selectedKey, meta]);

  const hasAnyCapability = meta.length > 0;

  const isDirty = useMemo(() => {
    const saved = lastSavedRef.current;
    if (!saved) return false;
    if (saved.enabled !== enabled) return true;
    // When disabled, only the toggle matters; field edits are ignored.
    if (!enabled) return false;
    return JSON.stringify(saved.state) !== JSON.stringify(state);
  }, [enabled, state]);

  const updateTool = useCallback(
    (backendId: string, patch: Partial<ToolEntry>) => {
      setState((prev) => ({
        ...prev,
        tool: {
          ...prev.tool,
          [backendId]: { ...prev.tool[backendId], ...patch },
        },
      }));
    },
    [],
  );

  const updateResource = useCallback(
    (backendId: string, patch: Partial<ResourceEntry>) => {
      setState((prev) => ({
        ...prev,
        resource: {
          ...prev.resource,
          [backendId]: { ...prev.resource[backendId], ...patch },
        },
      }));
    },
    [],
  );

  const updatePrompt = useCallback(
    (backendId: string, patch: Partial<PromptEntry>) => {
      setState((prev) => ({
        ...prev,
        prompt: {
          ...prev.prompt,
          [backendId]: { ...prev.prompt[backendId], ...patch },
        },
      }));
    },
    [],
  );

  const handleSave = useCallback(async () => {
    if (!config) return;
    const existingPolicies = config.policies ?? [];

    let nextPolicies: MCPProxyPolicy[];
    if (enabled) {
      if (!availableRewritePolicy) {
        setStatus({
          message:
            "Rewrite policy is not available on the active gateway.",
          severity: "error",
        });
        return;
      }
      const params = buildPolicyParams(state, meta);
      const newPolicy: MCPProxyPolicy = {
        name: REWRITE_POLICY_NAME,
        version: availableRewritePolicy.version,
        displayName: availableRewritePolicy.displayName,
        params,
      };
      const existingIndex = existingPolicies.findIndex(
        (p) => p.name === REWRITE_POLICY_NAME,
      );
      nextPolicies =
        existingIndex >= 0
          ? existingPolicies.map((p, i) =>
              i === existingIndex ? newPolicy : p,
            )
          : [...existingPolicies, newPolicy];
    } else {
      nextPolicies = existingPolicies.filter(
        (p) => p.name !== REWRITE_POLICY_NAME,
      );
    }

    try {
      await onUpdate({ policies: nextPolicies });
      lastSavedRef.current = { enabled, state };
      setStatus({
        message: enabled
          ? "Rewrite policy updated successfully."
          : "Rewrite policy disabled.",
        severity: "success",
      });
    } catch {
      setStatus({
        message: "Failed to update rewrite policy.",
        severity: "error",
      });
    }
  }, [config, enabled, availableRewritePolicy, state, meta, onUpdate]);

  const handleDiscard = useCallback(() => {
    const saved = lastSavedRef.current;
    if (saved) {
      setEnabled(saved.enabled);
      setState(saved.state);
    }
    setStatus(null);
  }, []);

  if (isLoading || isCatalogLoading) {
    return (
      <Stack spacing={2}>
        <Skeleton variant="rounded" height={48} />
        <Box
          sx={{ display: "grid", gridTemplateColumns: "320px 1fr", gap: 3 }}
        >
          <Skeleton variant="rounded" height={400} />
          <Skeleton variant="rounded" height={400} />
        </Box>
      </Stack>
    );
  }

  if (!hasAnyCapability) {
    return (
      <Stack
        alignItems="center"
        justifyContent="center"
        spacing={1}
        sx={{ minHeight: 200, textAlign: "center" }}
      >
        <Typography variant="subtitle1" fontWeight={600}>
          No Capabilities Available
        </Typography>
        <Typography variant="body2" color="text.secondary">
          This MCP Server has no tools, resources, or prompts to rewrite.
        </Typography>
      </Stack>
    );
  }

  const isError = !!status && (status.severity === "error" || !isDirty);

  return (
    <Stack spacing={2}>
      <Stack
        direction="row"
        alignItems="center"
        justifyContent="space-between"
        spacing={2}
      >
        <Stack spacing={0.5} sx={{ flex: 1, minWidth: 0 }}>
          <Typography variant="subtitle1" fontWeight={600}>
            Rewrite
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Rewrite the user-facing names of tools, resources, and prompts
            exposed by this MCP Server.
          </Typography>
        </Stack>
        <Stack direction="row" alignItems="center" spacing={1}>
          <Typography variant="body2" color="text.secondary">
            {enabled ? "Enabled" : "Disabled"}
          </Typography>
          <Switch
            checked={enabled}
            onChange={(_, checked) => setEnabled(checked)}
            disabled={isUpdating}
          />
        </Stack>
      </Stack>

      {enabled && !availableRewritePolicy && (
        <Alert severity="warning">
          The rewrite policy ({REWRITE_POLICY_NAME}) is not reported as available
          by the active gateway. Saving is disabled.
        </Alert>
      )}

      {enabled && (
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", md: "320px 1fr" },
          gap: 3,
          alignItems: "flex-start",
        }}
      >
        <Stack
          spacing={1}
          sx={{ maxHeight: "calc(100vh - 520px)", overflowY: "auto", pr: 0.5 }}
        >
          {(["tool", "resource", "prompt"] as CapabilityKind[]).map((kind) => {
            const items = meta.filter((m) => m.kind === kind);
            if (!items.length) return null;
            return (
              <Accordion
                key={kind}
                variant="outlined"
                disableGutters
                expanded={openSections[kind]}
                onChange={(_, isExpanded) =>
                  setOpenSections((prev) => ({ ...prev, [kind]: isExpanded }))
                }
              >
                <AccordionSummary expandIcon={<ChevronDown size={18} />}>
                  <Stack direction="row" alignItems="center" spacing={1}>
                    <Typography variant="subtitle2" fontWeight={600}>
                      {KIND_LABEL[kind]}s
                    </Typography>
                    <Chip label={items.length} size="small" variant="outlined" />
                  </Stack>
                </AccordionSummary>
                <AccordionDetails sx={{ p: 0 }}>
                  <List disablePadding>
                    {items.map((m) => {
                      const key = `${m.kind}::${m.backendId}`;
                      const isSelected = selectedKey === key;
                      return (
                        <ListItemButton
                          key={key}
                          selected={isSelected}
                          onClick={() => setSelectedKey(key)}
                          sx={{ gap: 1 }}
                        >
                          <Chip
                            label={KIND_LABEL[m.kind]}
                            size="small"
                            color={KIND_CHIP_COLOR[m.kind]}
                            variant={isSelected ? "filled" : "outlined"}
                            sx={{ minWidth: 72, justifyContent: "center" }}
                          />
                          <ListItemText
                            primary={
                              <Tooltip title={m.label} arrow placement="right">
                                <Typography
                                  variant="body2"
                                  noWrap
                                  sx={{ fontFamily: "monospace" }}
                                >
                                  {m.label}
                                </Typography>
                              </Tooltip>
                            }
                          />
                        </ListItemButton>
                      );
                    })}
                  </List>
                </AccordionDetails>
              </Accordion>
            );
          })}
        </Stack>

        <Card variant="outlined" sx={{ p: 2, minHeight: 240 }}>
          {selected ? (
            <RewriteFieldsForm
              meta={selected}
              state={state}
              onChangeTool={updateTool}
              onChangeResource={updateResource}
              onChangePrompt={updatePrompt}
            />
          ) : (
            <Stack
              alignItems="center"
              justifyContent="center"
              spacing={1}
              sx={{ py: 6, textAlign: "center" }}
            >
              <Typography variant="subtitle2" fontWeight={600}>
                Select a capability
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Pick a tool, resource, or prompt from the list to configure its
                rewrite.
              </Typography>
            </Stack>
          )}
        </Card>
      </Box>
      )}

      <Stack spacing={1.5}>
        <Collapse in={isError} timeout={300}>
          {status && (
            <Alert
              severity={status.severity}
              onClose={() => setStatus(null)}
              sx={{ width: "100%" }}
            >
              {status.message}
            </Alert>
          )}
        </Collapse>
        <Stack direction="row" spacing={1.5} justifyContent="flex-end">
          <Button
            variant="outlined"
            onClick={handleDiscard}
            disabled={!isDirty || isUpdating}
          >
            Discard
          </Button>
          <Button
            variant="contained"
            onClick={() => void handleSave()}
            disabled={
              !isDirty ||
              isUpdating ||
              (enabled && !availableRewritePolicy)
            }
          >
            {isUpdating ? "Saving..." : "Save"}
          </Button>
        </Stack>
      </Stack>
    </Stack>
  );
}
function RewriteFieldsForm({
  meta,
  state,
  onChangeTool,
  onChangeResource,
  onChangePrompt,
}: {
  meta: ItemMeta;
  state: RewriteState;
  onChangeTool: (backendId: string, patch: Partial<ToolEntry>) => void;
  onChangeResource: (
    backendId: string,
    patch: Partial<ResourceEntry>,
  ) => void;
  onChangePrompt: (backendId: string, patch: Partial<PromptEntry>) => void;
}) {
  const [advancedOpen, setAdvancedOpen] = useState(false);

  return (
    <Stack spacing={2}>
      <Stack direction="row" alignItems="center" spacing={1}>
        <Chip
          label={KIND_LABEL[meta.kind]}
          size="small"
          color={KIND_CHIP_COLOR[meta.kind]}
          variant="outlined"
        />
        <Typography
          variant="subtitle1"
          fontWeight={600}
          sx={{ fontFamily: "monospace" }}
        >
          {meta.label}
        </Typography>
      </Stack>
      <Typography variant="caption" color="text.secondary">
        Backend identifier — sent to the upstream MCP server. Rewrite the
        values below to control what clients see.
      </Typography>

      {meta.kind === "tool" && (
        <>
          <TextInput
            maxLength={INPUT_LIMITS.NAME}
            label="Name"
            size="small"
            fullWidth
            value={state.tool[meta.backendId]?.name ?? ""}
            onChange={(e) =>
              onChangeTool(meta.backendId, { name: e.target.value })
            }
          />
          <TextInput
            maxLength={INPUT_LIMITS.DESCRIPTION}
            label="Description"
            size="small"
            fullWidth
            multiline
            minRows={2}
            value={state.tool[meta.backendId]?.description ?? ""}
            onChange={(e) =>
              onChangeTool(meta.backendId, { description: e.target.value })
            }
          />
          <TextInput
            maxLength={INPUT_LIMITS.SOURCE}
            label="Input Schema"
            size="small"
            fullWidth
            multiline
            minRows={6}
            value={state.tool[meta.backendId]?.inputSchema ?? ""}
            onChange={(e) =>
              onChangeTool(meta.backendId, { inputSchema: e.target.value })
            }
            sx={{ "& .MuiInputBase-input": { fontFamily: "monospace" } }}
          />
          <Stack>
          <Accordion
            variant="outlined"
            disableGutters
            expanded={advancedOpen}
            onChange={(_, isExpanded) => setAdvancedOpen(isExpanded)}
          >
            <AccordionSummary expandIcon={<ChevronDown size={16} />}>
              <Typography variant="body2" fontWeight={600} color="primary.main">
                Advanced
              </Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Stack spacing={2}>
                <TextInput
                  maxLength={INPUT_LIMITS.SOURCE}
                  label="Output Schema"
                  size="small"
                  fullWidth
                  multiline
                  minRows={4}
                  value={state.tool[meta.backendId]?.outputSchema ?? ""}
                  onChange={(e) =>
                    onChangeTool(meta.backendId, {
                      outputSchema: e.target.value,
                    })
                  }
                  sx={{
                    "& .MuiInputBase-input": { fontFamily: "monospace" },
                  }}
                />
                <TextInput
                  maxLength={INPUT_LIMITS.SHORT_TEXT}
                  label="Target"
                  size="small"
                  fullWidth
                  helperText="Backend tool name to forward calls to."
                  value={state.tool[meta.backendId]?.target ?? ""}
                  onChange={(e) =>
                    onChangeTool(meta.backendId, { target: e.target.value })
                  }
                  sx={{
                    "& .MuiInputBase-input": { fontFamily: "monospace" },
                  }}
                />
              </Stack>
            </AccordionDetails>
          </Accordion>
        </Stack>
        </>
      )}

      {meta.kind === "resource" && (
        <>
          <TextInput
            maxLength={INPUT_LIMITS.URL}
            label="URI"
            size="small"
            fullWidth
            value={state.resource[meta.backendId]?.uri ?? ""}
            onChange={(e) =>
              onChangeResource(meta.backendId, { uri: e.target.value })
            }
            sx={{ "& .MuiInputBase-input": { fontFamily: "monospace" } }}
          />
          <TextInput
            maxLength={INPUT_LIMITS.DESCRIPTION}
            label="Description"
            size="small"
            fullWidth
            multiline
            minRows={2}
            value={state.resource[meta.backendId]?.description ?? ""}
            onChange={(e) =>
              onChangeResource(meta.backendId, { description: e.target.value })
            }
          />
          <Accordion
            variant="outlined"
            disableGutters
            expanded={advancedOpen}
            onChange={(_, isExpanded) => setAdvancedOpen(isExpanded)}
          >
            <AccordionSummary expandIcon={<ChevronDown size={16} />}>
              <Typography variant="body2" fontWeight={600}>
                Advanced
              </Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Stack spacing={2}>
                <TextInput
                  maxLength={INPUT_LIMITS.SHORT_TEXT}
                  label="Target"
                  size="small"
                  fullWidth
                  helperText="Backend resource URI to forward reads to."
                  value={state.resource[meta.backendId]?.target ?? ""}
                  onChange={(e) =>
                    onChangeResource(meta.backendId, {
                      target: e.target.value,
                    })
                  }
                  sx={{
                    "& .MuiInputBase-input": { fontFamily: "monospace" },
                  }}
                />
              </Stack>
            </AccordionDetails>
          </Accordion>
        </>
      )}

      {meta.kind === "prompt" && (
        <>
          <TextInput
            maxLength={INPUT_LIMITS.NAME}
            label="Name"
            size="small"
            fullWidth
            value={state.prompt[meta.backendId]?.name ?? ""}
            onChange={(e) =>
              onChangePrompt(meta.backendId, { name: e.target.value })
            }
          />
          <TextInput
            maxLength={INPUT_LIMITS.DESCRIPTION}
            label="Description"
            size="small"
            fullWidth
            multiline
            minRows={2}
            value={state.prompt[meta.backendId]?.description ?? ""}
            onChange={(e) =>
              onChangePrompt(meta.backendId, { description: e.target.value })
            }
          />
          <Accordion
            variant="outlined"
            disableGutters
            expanded={advancedOpen}
            onChange={(_, isExpanded) => setAdvancedOpen(isExpanded)}
          >
            <AccordionSummary expandIcon={<ChevronDown size={16} />}>
              <Typography variant="body2" fontWeight={600}>
                Advanced
              </Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Stack spacing={2}>
                <TextInput
                  maxLength={INPUT_LIMITS.SHORT_TEXT}
                  label="Target"
                  size="small"
                  fullWidth
                  helperText="Backend prompt name to forward requests to."
                  value={state.prompt[meta.backendId]?.target ?? ""}
                  onChange={(e) =>
                    onChangePrompt(meta.backendId, { target: e.target.value })
                  }
                  sx={{
                    "& .MuiInputBase-input": { fontFamily: "monospace" },
                  }}
                />
              </Stack>
            </AccordionDetails>
          </Accordion>
        </>
      )}
    </Stack>
  );
}
