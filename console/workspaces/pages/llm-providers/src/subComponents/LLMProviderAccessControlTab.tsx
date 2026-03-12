/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useUpdateLLMProvider } from "@agent-management-platform/api-client";
import type {
  LLMProviderResponse,
  RouteException,
} from "@agent-management-platform/types";
import {
  Alert,
  Box,
  Button,
  Collapse,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Form,
  IconButton,
  ListingTable,
  Skeleton,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  FileCode,
  HelpCircle,
  Inbox,

  Upload,
} from "@wso2/oxygen-ui-icons-react";
import { useParams } from "react-router-dom";
import { useOpenApiSpec } from "../hooks/useOpenApiSpec";
import {
  extractResourcesFromSpec,
  getResourceKey,
  parseOpenApiSpec,
  type ResourceItem,
} from "../utils/openapiResources";
import { ExpandableResourceRow } from "../components/ResourceView";

function buildOperationSpec(
  rootSpec: Record<string, unknown>,
  method: string,
  path: string,
): Record<string, unknown> | null {
  const methodKey = method.toLowerCase();
  const paths = rootSpec.paths as Record<string, unknown> | undefined;
  const pathEntry = paths?.[path] as Record<string, unknown> | undefined;
  const operation = pathEntry?.[methodKey] as
    | Record<string, unknown>
    | undefined;
  if (!operation) return null;

  const operationPathItem: Record<string, unknown> = {
    [methodKey]: operation,
  };

  const commonPathKeys = ["parameters", "servers", "summary", "description"];
  commonPathKeys.forEach((key) => {
    if (pathEntry?.[key] !== undefined) {
      operationPathItem[key] = pathEntry[key];
    }
  });

  return {
    ...(rootSpec.openapi ? { openapi: rootSpec.openapi } : {}),
    ...(rootSpec.swagger ? { swagger: rootSpec.swagger } : {}),
    ...(rootSpec.info ? { info: rootSpec.info } : {}),
    ...(rootSpec.servers ? { servers: rootSpec.servers } : {}),
    ...(rootSpec.components ? { components: rootSpec.components } : {}),
    ...(rootSpec.security ? { security: rootSpec.security } : {}),
    ...(rootSpec.tags ? { tags: rootSpec.tags } : {}),
    ...(rootSpec.basePath ? { basePath: rootSpec.basePath } : {}),
    ...(rootSpec.host ? { host: rootSpec.host } : {}),
    ...(rootSpec.schemes ? { schemes: rootSpec.schemes } : {}),
    ...(rootSpec.consumes ? { consumes: rootSpec.consumes } : {}),
    ...(rootSpec.produces ? { produces: rootSpec.produces } : {}),
    ...(rootSpec.definitions ? { definitions: rootSpec.definitions } : {}),
    ...(rootSpec.securityDefinitions
      ? { securityDefinitions: rootSpec.securityDefinitions }
      : {}),
    paths: {
      [path]: operationPathItem,
    },
  };
}

export type LLMProviderAccessControlTabProps = {
  providerData: LLMProviderResponse | null | undefined;
  openapiSpecUrl: string | undefined;
  isLoading?: boolean;
};

export function LLMProviderAccessControlTab({
  providerData,
  openapiSpecUrl,
  isLoading = false,
}: LLMProviderAccessControlTabProps) {
  const { orgId, providerId } = useParams<{
    orgId: string;
    providerId: string;
  }>();
  const { mutateAsync: updateProvider, isPending } = useUpdateLLMProvider();
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const lastSavedRef = useRef<{
    mode: "allow" | "deny";
    exceptionKeys: string;
    openapi: string;
    exceptionResources: ResourceItem[];
  } | null>(null);

  const [resourceMode, setResourceMode] = useState<"allow" | "deny">("allow");
  const [pendingResourceMode, setPendingResourceMode] = useState<
    "allow" | "deny" | null
  >(null);
  const [resourceModeConfirmOpen, setResourceModeConfirmOpen] = useState(false);
  const [exceptionResources, setExceptionResources] = useState<ResourceItem[]>(
    [],
  );
  const fallbackOpenapi = providerData?.openapi?.trim() ?? "";
  const {
    text: openapiText,
    setText: setOpenapiText,
    isLoading: specLoading,
    error: specError,
  } = useOpenApiSpec(openapiSpecUrl, fallbackOpenapi);
  const [availableSearch, setAvailableSearch] = useState("");
  const [exceptionSearch, setExceptionSearch] = useState("");
  const [selectedAvailableKeys, setSelectedAvailableKeys] = useState<string[]>(
    [],
  );
  const [selectedExceptionKeys, setSelectedExceptionKeys] = useState<string[]>(
    [],
  );
  const [openKey, setOpenKey] = useState<string | null>(null);
  const [status, setStatus] = useState<{
    message: string;
    severity: "success" | "error";
  } | null>(null);

  useEffect(() => {
    if (specError) {
      setStatus({
        message: "Failed to load OpenAPI spec.",
        severity: "error",
      });
    }
  }, [specError]);

  useEffect(() => {
    if (!providerData) return;
    const mode = String(providerData.accessControl?.mode || "")
      .toLowerCase()
      .replace(/-/g, "_");
    const resolvedMode = mode === "deny_all" ? "deny" : "allow";
    setResourceMode(resolvedMode);
    const openapi = providerData.openapi?.trim() ?? "";
    const exceptions = providerData.accessControl?.exceptions || [];
    const exceptionItems = exceptions.flatMap((ex) =>
      (ex.methods ?? ["GET"]).map((method) => ({
        method,
        path: ex.path ?? "",
      })),
    );
    setExceptionResources(exceptionItems);
    lastSavedRef.current = {
      mode: resolvedMode,
      exceptionKeys: exceptionItems
        .map((e) => getResourceKey(e))
        .sort()
        .join(","),
      openapi,
      exceptionResources: exceptionItems,
    };
  }, [providerData]);

  const parseOpenApiText = useCallback((text: string): ResourceItem[] => {
    if (!text.trim()) return [];
    const spec = parseOpenApiSpec(text);
    return spec ? extractResourcesFromSpec(spec) : [];
  }, []);

  const updateAccessControl = useCallback(
    async (
      mode: "allow" | "deny",
      exceptions: ResourceItem[],
      nextOpenapi: string,
    ) => {
      if (!providerData || !orgId || !providerId) return;
      const accessControlMode = mode === "allow" ? "allow_all" : "deny_all";
      const byPath = new Map<string, string[]>();
      for (const r of exceptions) {
        const path = r.path ?? "";
        const methods = byPath.get(path) ?? [];
        if (!methods.includes(r.method)) methods.push(r.method);
        byPath.set(path, methods);
      }
      const exceptionPayload: RouteException[] = Array.from(byPath.entries()).map(
        ([path, methods]) => ({ path, methods }),
      );

      try {
        await updateProvider({
          params: { orgName: orgId, providerId },
          body: {
            openapi: nextOpenapi,
            accessControl: {
              mode: accessControlMode,
              exceptions: exceptionPayload,
            },
          },
        });
        lastSavedRef.current = {
          mode,
          exceptionKeys: exceptions
            .map((e) => getResourceKey(e))
            .sort()
            .join(","),
          openapi: nextOpenapi,
          exceptionResources: exceptions,
        };
        setStatus({
          message: "Access control updated successfully.",
          severity: "success",
        });
      } catch {
        setStatus({
          message: "Failed to update access control.",
          severity: "error",
        });
      }
    },
    [providerData, orgId, providerId, updateProvider],
  );

  const handleResourceModeChange = (
    _event: React.MouseEvent<HTMLElement>,
    newMode: "allow" | "deny" | null,
  ) => {
    if (!newMode || newMode === resourceMode) return;
    setPendingResourceMode(newMode);
    setResourceModeConfirmOpen(true);
  };

  const handleCancelResourceModeChange = () => {
    setResourceModeConfirmOpen(false);
    setPendingResourceMode(null);
  };

  const handleApplyResourceModeChange = () => {
    if (!pendingResourceMode || pendingResourceMode === resourceMode) {
      handleCancelResourceModeChange();
      return;
    }
    setResourceMode(pendingResourceMode);
    setResourceModeConfirmOpen(false);
    setPendingResourceMode(null);
  };

  const handleUploadClick = () => fileInputRef.current?.click();

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    try {
      const text = await file.text();
      const imported = parseOpenApiText(text);
      if (!imported.length) {
        setStatus({
          message: "No resources found in specification.",
          severity: "error",
        });
        return;
      }
      setOpenapiText(text);
      setStatus({
        message: "Specification imported. Click Save to apply.",
        severity: "success",
      });
    } catch {
      setStatus({
        message: "Failed to import specification.",
        severity: "error",
      });
    } finally {
      e.target.value = "";
    }
  };

  const normalized = useMemo(() => {
    const base = parseOpenApiText(openapiText);
    return base.map((r) => ({ ...r, method: r.method.toUpperCase() }));
  }, [openapiText, parseOpenApiText]);

  const parsedOpenApiSpec = useMemo(
    () => parseOpenApiSpec(openapiText),
    [openapiText],
  );

  const operationSpecByKey = useMemo(() => {
    const map = new Map<string, Record<string, unknown>>();
    if (!parsedOpenApiSpec) return map;
    normalized.forEach((resource) => {
      const operationSpec = buildOperationSpec(
        parsedOpenApiSpec,
        resource.method,
        resource.path,
      );
      if (operationSpec) {
        map.set(getResourceKey(resource), operationSpec);
      }
    });
    return map;
  }, [normalized, parsedOpenApiSpec]);

  const resourceSummaryByKey = useMemo(() => {
    const map = new Map<string, string>();
    for (const resource of normalized) {
      if (resource.summary) {
        map.set(getResourceKey(resource), resource.summary);
      }
    }
    return map;
  }, [normalized]);

  const exceptionKeySet = useMemo(() => {
    const set = new Set<string>();
    exceptionResources.forEach((r) => set.add(getResourceKey(r)));
    return set;
  }, [exceptionResources]);

  const availableResources = useMemo(
    () => normalized.filter((r) => !exceptionKeySet.has(getResourceKey(r))),
    [exceptionKeySet, normalized],
  );

  const filteredAvailableResources = useMemo(() => {
    const query = availableSearch.trim().toLowerCase();
    if (!query) return availableResources;
    return availableResources.filter((r) => {
      const haystack = `${r.method} ${r.path} ${r.summary ?? ""}`.toLowerCase();
      return haystack.includes(query);
    });
  }, [availableResources, availableSearch]);

  const filteredExceptionResources = useMemo(() => {
    const query = exceptionSearch.trim().toLowerCase();
    if (!query) return exceptionResources;
    return exceptionResources.filter((r) => {
      const summary =
        r.summary || resourceSummaryByKey.get(getResourceKey(r)) || "";
      const haystack = `${r.method} ${r.path} ${summary}`.toLowerCase();
      return haystack.includes(query);
    });
  }, [exceptionResources, exceptionSearch, resourceSummaryByKey]);

  const selectedAvailableKeySet = useMemo(
    () => new Set(selectedAvailableKeys),
    [selectedAvailableKeys],
  );
  const selectedExceptionKeySet = useMemo(
    () => new Set(selectedExceptionKeys),
    [selectedExceptionKeys],
  );

  const toggleAvailableSelection = (resource: ResourceItem) => {
    const key = getResourceKey(resource);
    setSelectedAvailableKeys((prev) =>
      prev.includes(key) ? prev.filter((k) => k !== key) : [...prev, key],
    );
  };

  const toggleExceptionSelection = (resource: ResourceItem) => {
    const key = getResourceKey(resource);
    setSelectedExceptionKeys((prev) =>
      prev.includes(key) ? prev.filter((k) => k !== key) : [...prev, key],
    );
  };

  const moveSelectedToExceptions = () => {
    if (!selectedAvailableKeys.length) return;
    const selected = availableResources.filter((r) =>
      selectedAvailableKeySet.has(getResourceKey(r)),
    );
    if (!selected.length) return;
    setExceptionResources((prev) => [...prev, ...selected]);
    setSelectedAvailableKeys([]);
  };

  const moveSelectedToAvailable = () => {
    if (!selectedExceptionKeys.length) return;
    setExceptionResources((prev) =>
      prev.filter((r) => !selectedExceptionKeySet.has(getResourceKey(r))),
    );
    setSelectedExceptionKeys([]);
  };

  const moveAllToExceptions = () => {
    setExceptionResources(normalized);
    setSelectedAvailableKeys([]);
  };

  const moveAllToAvailable = () => {
    setExceptionResources([]);
    setSelectedExceptionKeys([]);
  };

  const isDirty = useMemo(() => {
    const saved = lastSavedRef.current;
    if (!saved) return false;
    const currentExceptionKeys = exceptionResources
      .map((e) => getResourceKey(e))
      .sort()
      .join(",");
    return (
      resourceMode !== saved.mode ||
      currentExceptionKeys !== saved.exceptionKeys ||
      openapiText !== saved.openapi
    );
  }, [resourceMode, exceptionResources, openapiText]);

  const handleSave = () => {
    void updateAccessControl(resourceMode, exceptionResources, openapiText);
  };

  const handleDiscard = useCallback(() => {
    const saved = lastSavedRef.current;
    if (saved) {
      setResourceMode(saved.mode);
      setExceptionResources(saved.exceptionResources);
      setOpenapiText(saved.openapi);
    } else if (providerData) {
      const mode = String(providerData.accessControl?.mode || "")
        .toLowerCase()
        .replace(/-/g, "_");
      const resolvedMode = mode === "deny_all" ? "deny" : "allow";
      const exceptions = providerData.accessControl?.exceptions || [];
      const exceptionItems = exceptions.flatMap((ex) =>
        (ex.methods ?? ["GET"]).map((method) => ({
          method,
          path: ex.path ?? "",
        })),
      );
      setResourceMode(resolvedMode);
      setExceptionResources(exceptionItems);
      setOpenapiText(providerData.openapi?.trim() ?? "");
    }
    setStatus(null);
  }, [providerData, setOpenapiText]);

  type EmptyStateConfig = {
    illustration: React.ReactNode;
    title: string;
    description: string;
  };

  const renderResourceList = (
    resources: ResourceItem[],
    selectedKeySet: Set<string>,
    onToggle: (r: ResourceItem) => void,
    emptyState: EmptyStateConfig,
    getResourceWithSummary?: (r: ResourceItem) => ResourceItem,
  ) => (
    <Box sx={{ flex: 1, minHeight: 0, overflowY: "auto", pr: 0.5 }}>
      <Stack spacing={1.25} minHeight="100%">
        {resources.map((resource) => {
          const key = getResourceKey(resource);
          const isOpen = openKey === key;
          const resourceWithSummary = getResourceWithSummary
            ? getResourceWithSummary(resource)
            : resource;
          return (
            <ExpandableResourceRow
              key={key}
              resource={resourceWithSummary}
              isOpen={isOpen}
              operationSpec={operationSpecByKey.get(key)}
              selected={selectedKeySet.has(key)}
              onRowClick={() => onToggle(resource)}
              onToggleOpen={() => setOpenKey(isOpen ? null : key)}
            />
          );
        })}
        {!resources.length && (
          <ListingTable.Container sx={{ flex: 1, minHeight: 0 }}>
            <ListingTable.EmptyState
              illustration={emptyState.illustration}
              title={emptyState.title}
              description={emptyState.description}
            />
          </ListingTable.Container>
        )}
      </Stack>
    </Box>
  );

  if (isLoading || specLoading) {
    return (
      <Stack spacing={2}>
        <Skeleton variant="rounded" height={48} />
        <Box
          sx={{ display: "grid", gridTemplateColumns: "1fr auto 1fr", gap: 2 }}
        >
          <Skeleton variant="rounded" height={400} />
          <Skeleton variant="rounded" height={200} />
          <Skeleton variant="rounded" height={400} />
        </Box>
      </Stack>
    );
  }

  if (!providerData) {
    return null;
  }

  const isError = !!status && (status.severity === "error" || !isDirty);
  return (
    <Box height={isError ? "calc(100vh - 370px)" : "calc(100vh - 420px)"}>
      <Stack spacing={2}>
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            flexWrap: "wrap",
            gap: 2,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
            <Typography variant="body1">Mode</Typography>
            <ToggleButtonGroup
              size="small"
              value={resourceMode}
              exclusive
              onChange={handleResourceModeChange}
            >
              <ToggleButton
                color="primary"
                value="allow"
                sx={{ textTransform: "none" }}
              >
                Allow all
                <Tooltip
                  arrow
                  title="Allow all exposes every resource. Exceptions are hidden."
                >
                  <IconButton size="small">
                    <HelpCircle size={16} />
                  </IconButton>
                </Tooltip>
              </ToggleButton>
              <ToggleButton
                color="primary"
                value="deny"
                sx={{ textTransform: "none" }}
              >
                Deny all
                <Tooltip
                  arrow
                  title="Deny all hides every resource. Exceptions are exposed."
                >
                  <IconButton size="small">
                    <HelpCircle size={16} />
                  </IconButton>
                </Tooltip>
              </ToggleButton>
            </ToggleButtonGroup>
          </Box>

          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Button
              variant="outlined"
              size="small"
              startIcon={<Upload size={16} />}
              onClick={handleUploadClick}
              disabled={isPending}
            >
              Import from specification
            </Button>
            <input
              ref={fileInputRef}
              type="file"
              hidden
              accept=".json,.yaml,.yml"
              onChange={handleFileChange}
            />
          </Box>
        </Box>

        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", md: "1fr auto 1fr" },
            gap: 2,
          }}
        >
          <Box
            sx={{
              display: "flex",
              flexDirection: "column",
              overflow: "hidden",
              overflowY: "scroll",
              flex: 1,
              minHeight: 0,
            }}
          >
            <Form.Section>
              <Form.Header>
                {resourceMode === "allow"
                  ? "Allowed Resources"
                  : "Denied Resources"}
              </Form.Header>
              <Form.Stack spacing={1.5} height="calc(100vh - 620px)">
                <TextField
                  size="small"
                  placeholder="Search resources"
                  value={availableSearch}
                  fullWidth
                  onChange={(e) => setAvailableSearch(e.target.value)}
                />
                {renderResourceList(
                  filteredAvailableResources,
                  selectedAvailableKeySet,
                  toggleAvailableSelection,
                  {
                    illustration: <FileCode size={64} />,
                    title: "No available resources",
                    description:
                      "Import a specification or use a template with OpenAPI to see resources here.",
                  },
                )}
              </Form.Stack>
            </Form.Section>
          </Box>

          <Box
            sx={{
              display: "flex",
              alignItems: "flex-start",
              justifyContent: "center",
              marginTop: 15,
            }}
          >
            <Stack spacing={1}>
              <Tooltip title="Move all to exceptions" arrow>
                <span>
                  <IconButton
                    size="small"
                    onClick={moveAllToExceptions}
                    disabled={!availableResources.length || isPending}
                    sx={{ border: "1px solid", borderColor: "divider" }}
                  >
                    <ChevronsRight size={18} />
                  </IconButton>
                </span>
              </Tooltip>
              <Tooltip title="Move selected to exceptions" arrow>
                <span>
                  <IconButton
                    size="small"
                    onClick={moveSelectedToExceptions}
                    disabled={!selectedAvailableKeys.length || isPending}
                    sx={{ border: "1px solid", borderColor: "divider" }}
                  >
                    <ChevronRight size={18} />
                  </IconButton>
                </span>
              </Tooltip>
              <Tooltip title="Move selected to available" arrow>
                <span>
                  <IconButton
                    size="small"
                    onClick={moveSelectedToAvailable}
                    disabled={!selectedExceptionKeys.length || isPending}
                    sx={{ border: "1px solid", borderColor: "divider" }}
                  >
                    <ChevronLeft size={18} />
                  </IconButton>
                </span>
              </Tooltip>
              <Tooltip title="Move all to available" arrow>
                <span>
                  <IconButton
                    size="small"
                    onClick={moveAllToAvailable}
                    disabled={!exceptionResources.length || isPending}
                    sx={{ border: "1px solid", borderColor: "divider" }}
                  >
                    <ChevronsLeft size={18} />
                  </IconButton>
                </span>
              </Tooltip>
            </Stack>
          </Box>

          <Form.Section>
            <Form.Header>
              {resourceMode === "allow"
                ? "Denied Resources"
                : "Allowed Resources"}
            </Form.Header>
            <Form.Stack spacing={1.5} height="calc(100vh - 620px)">
              <TextField
                size="small"
                placeholder="Search resources"
                value={exceptionSearch}
                fullWidth
                onChange={(e) => setExceptionSearch(e.target.value)}
              />
              {renderResourceList(
                filteredExceptionResources,
                selectedExceptionKeySet,
                toggleExceptionSelection,
                {
                  illustration: <Inbox size={64} />,
                  title: "No selected resources",
                  description:
                    "Use the arrow buttons to move resources between the lists.",
                },
                (r) => ({
                  ...r,
                  summary:
                    r.summary ||
                    resourceSummaryByKey.get(getResourceKey(r)) ||
                    "",
                }),
              )}
            </Form.Stack>
          </Form.Section>
        </Box>

        <Stack spacing={1.5} width="100%">
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
              disabled={!isDirty || isPending}
            >
              Discard
            </Button>
            <Button
              variant="contained"
              onClick={handleSave}
              disabled={!isDirty || isPending}
            >
              {isPending ? "Saving..." : "Save"}
            </Button>
          </Stack>
        </Stack>
      </Stack>

      <Dialog
        open={resourceModeConfirmOpen}
        onClose={handleCancelResourceModeChange}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Confirm Resource Mode Change</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Change resource mode from{" "}
            {resourceMode === "allow" ? "Allow all" : "Deny all"} to{" "}
            {pendingResourceMode === "allow" ? "Allow all" : "Deny all"}?
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCancelResourceModeChange}>Cancel</Button>
          <Button
            variant="contained"
            onClick={handleApplyResourceModeChange}
            disabled={!pendingResourceMode || isPending}
          >
            Apply
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
