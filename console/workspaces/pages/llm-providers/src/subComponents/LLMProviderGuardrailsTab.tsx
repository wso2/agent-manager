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
import {
  useGuardrailsCatalog,
  type GuardrailDefinition,
} from "@agent-management-platform/api-client";
import type {
  LLMPolicy,
  LLMPolicyPath,
  LLMProviderResponse,
  UpdateLLMProviderRequest,
} from "@agent-management-platform/types";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Button,
  Chip,
  Collapse,
  ListingTable,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown, Plus, ShieldAlert } from "@wso2/oxygen-ui-icons-react";
import type { ParameterValues } from "../PolicyParameterEditor/types";
import { GuardrailSelectorDrawer } from "../components/GuardrailSelectorDrawer";
import { useOpenApiSpec } from "../hooks/useOpenApiSpec";
import {
  extractResourcesFromSpec,
  getMethodChipColor,
  getResourceKey,
  parseOpenApiSpec,
  type ResourceItem,
} from "../utils/openapiResources";
import { z } from "zod";

const PolicyPathSchema = z.object({
  path: z.string(),
  methods: z.array(z.string()),
  params: z.record(z.string(), z.unknown()).optional(),
});

const PolicySchema = z.object({
  name: z.string().min(1),
  version: z.string().min(1),
  paths: z.array(PolicyPathSchema).optional(),
});

const PoliciesPayloadSchema = z.object({
  policies: z.array(PolicySchema),
});

function isGlobalPath(p: LLMPolicyPath): boolean {
  return p.path === "/*" && (p.methods?.includes("*") ?? false);
}

function pathMatchesResource(
  p: LLMPolicyPath,
  resourcePath: string,
  resourceMethod: string,
): boolean {
  if (p.path !== resourcePath) return false;
  const methods = p.methods ?? [];
  return methods.some(
    (m) => m.toUpperCase() === resourceMethod.toUpperCase(),
  );
}

function pathsIncludeEquivalent(
  paths: LLMPolicyPath[],
  newPath: LLMPolicyPath,
): boolean {
  const newMethods = [...(newPath.methods ?? [])].sort();
  return paths.some((p) => {
    if (p.path !== newPath.path) return false;
    const methods = [...(p.methods ?? [])].sort();
    return (
      methods.length === newMethods.length &&
      methods.every((m, i) => m === newMethods[i])
    );
  });
}

type DrawerContext =
  | { type: "global" }
  | { type: "resource"; method: string; path: string };

export type LLMProviderGuardrailsTabProps = {
  providerData: LLMProviderResponse | null | undefined;
  openapiSpecUrl?: string;
  isLoading?: boolean;
  error?: Error | null;
  onUpdate: (fields: UpdateLLMProviderRequest) => Promise<LLMProviderResponse>;
  isUpdating: boolean;
};

export function LLMProviderGuardrailsTab({
  providerData,
  openapiSpecUrl,
  isLoading = false,
  error: providerError = null,
  onUpdate,
  isUpdating,
}: LLMProviderGuardrailsTabProps) {
  const { data: catalogData } = useGuardrailsCatalog();

  const availableGuardrails = useMemo(
    () => catalogData?.data ?? [],
    [catalogData],
  );

  const [status, setStatus] = useState<{
    message: string;
    severity: "success" | "error";
  } | null>(null);
  const fallbackOpenapi = providerData?.openapi?.trim() ?? "";
  const {
    text: openapiText,
    isLoading: specLoading,
    error: specError,
  } = useOpenApiSpec(openapiSpecUrl, fallbackOpenapi);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [drawerContext, setDrawerContext] = useState<DrawerContext | null>(null);
  const [expandedResources, setExpandedResources] = useState<Set<string>>(
    new Set(),
  );

  const serverPolicies = useMemo(
    () => providerData?.policies ?? [],
    [providerData?.policies],
  );

  const [localPolicies, setLocalPolicies] = useState<LLMPolicy[]>([]);
  const lastSavedRef = useRef<string | null>(
    JSON.stringify(serverPolicies),
  );

  useEffect(() => {
    setLocalPolicies(serverPolicies);
    lastSavedRef.current = JSON.stringify(serverPolicies);
  }, [serverPolicies]);

  const policies = localPolicies;

  useEffect(() => {
    if (specError) {
      setStatus({
        message: "Failed to load OpenAPI spec.",
        severity: "error",
      });
    }
  }, [specError]);

  const resources = useMemo(() => {
    if (!openapiText.trim()) return [];
    const spec = parseOpenApiSpec(openapiText);
    return spec ? extractResourcesFromSpec(spec) : [];
  }, [openapiText]);

  type PolicyEntry = {
    policyIndex: number;
    pathIndex: number;
    policy: LLMPolicy;
    path: LLMPolicyPath;
  };

  const globalEntries = useMemo(() => {
    const entries: PolicyEntry[] = [];
    policies.forEach((policy, pi) => {
      (policy.paths ?? []).forEach((path, pathIdx) => {
        if (isGlobalPath(path)) {
          entries.push({ policyIndex: pi, pathIndex: pathIdx, policy, path });
        }
      });
    });
    return entries;
  }, [policies]);

  const getResourceGuardrails = useCallback(
    (resource: ResourceItem) => {
      const entries: PolicyEntry[] = [];
      policies.forEach((policy, pi) => {
        (policy.paths ?? []).forEach((path, pathIdx) => {
          if (pathMatchesResource(path, resource.path, resource.method)) {
            entries.push({ policyIndex: pi, pathIndex: pathIdx, policy, path });
          }
        });
      });
      return entries;
    },
    [policies],
  );

  const isDirty = useMemo(() => {
    if (lastSavedRef.current === null) return false;
    return JSON.stringify(localPolicies) !== lastSavedRef.current;
  }, [localPolicies]);

  const handleSave = useCallback(async () => {
    const result = PoliciesPayloadSchema.safeParse({
      policies: localPolicies,
    });
    if (!result.success) {
      const first = result.error.issues[0];
      setStatus({
        message: first?.message ?? "Validation failed",
        severity: "error",
      });
      return;
    }

    try {
      const payload = result.data.policies.map((p) => ({
        ...p,
        paths: p.paths ?? [],
      })) as LLMPolicy[];
      await onUpdate({
        policies: payload,
      });
      lastSavedRef.current = JSON.stringify(localPolicies);
      setStatus({
        message: "Guardrails saved successfully.",
        severity: "success",
      });
    } catch {
      setStatus({
        message: "Failed to save guardrails.",
        severity: "error",
      });
    }
  }, [localPolicies, onUpdate]);

  const handleDiscard = useCallback(() => {
    setLocalPolicies(serverPolicies);
    lastSavedRef.current = JSON.stringify(serverPolicies);
    setStatus(null);
  }, [serverPolicies]);

  const handleAddGuardrail = useCallback(
    (guardrail: GuardrailDefinition, values: ParameterValues) => {
      if (!drawerContext) return;

      const params = (values ?? {}) as Record<string, unknown>;
      const newPath: LLMPolicyPath =
        drawerContext.type === "global"
          ? { path: "/*", methods: ["*"], params }
          : {
              path: drawerContext.path,
              methods: [drawerContext.method],
              params,
            };

      const existing = policies.find(
        (p) => p.name === guardrail.name && p.version === guardrail.version,
      );

      let nextPolicies: LLMPolicy[];
      if (existing) {
        const currentPaths = existing.paths ?? [];
        const dedupedPaths = pathsIncludeEquivalent(currentPaths, newPath)
          ? currentPaths
          : [...currentPaths, newPath];
        nextPolicies = policies.map((p) =>
          p.name === guardrail.name && p.version === guardrail.version
            ? { ...p, paths: dedupedPaths }
            : p,
        );
      } else {
        nextPolicies = [
          ...policies,
          {
            name: guardrail.name,
            version: guardrail.version,
            paths: [newPath],
          },
        ];
      }

      setLocalPolicies(nextPolicies);
    },
    [drawerContext, policies],
  );

  const handleRemoveGuardrail = useCallback(
    (policyIndex: number, pathIndex: number) => {
      const policy = policies[policyIndex];
      if (!policy) return;

      const nextPaths = (policy.paths ?? []).filter((_, i) => i !== pathIndex);
      const nextPolicies =
        nextPaths.length === 0
          ? policies.filter((_, i) => i !== policyIndex)
          : policies.map((p, i) =>
              i === policyIndex ? { ...p, paths: nextPaths } : p,
            );

      setLocalPolicies(nextPolicies);
    },
    [policies],
  );

  const handleOpenDrawer = useCallback((context: DrawerContext) => {
    setDrawerContext(context);
    setDrawerOpen(true);
  }, []);

  const handleCloseDrawer = useCallback(() => {
    setDrawerOpen(false);
    setDrawerContext(null);
  }, []);

  const handleDrawerSubmit = useCallback(
    (guardrail: GuardrailDefinition, settings: ParameterValues) => {
      if (!drawerContext) return;
      handleAddGuardrail(guardrail, settings);
      handleCloseDrawer();
    },
    [drawerContext, handleAddGuardrail, handleCloseDrawer],
  );

  const getDisplayName = useCallback(
    (p: LLMPolicy): string => {
      const def = availableGuardrails.find(
        (g: GuardrailDefinition) =>
          g.name === p.name && g.version === p.version,
      );
      return def?.displayName ?? p.name;
    },
    [availableGuardrails],
  );

  if (isLoading) {
    return (
      <Stack spacing={3}>
        <Skeleton variant="rounded" height={120} />
        <Skeleton variant="rounded" height={200} />
      </Stack>
    );
  }

  if (!providerData && !providerError) {
    return null;
  }

  return (
    <Stack spacing={3}>
      {providerError && (
        <Alert severity="error" sx={{ width: "100%" }}>
          {providerError instanceof Error
            ? providerError.message
            : "Failed to load provider."}
        </Alert>
      )}

      <Collapse
        in={!!status && (status.severity === "error" || !isDirty)}
        timeout={300}
      >
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

      <Stack spacing={3}>
        <Box>
          <Typography variant="h6" component="h2" sx={{ mb: 0.5 }}>
            Global Guardrails
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Applies for all resources
          </Typography>
          <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
            {globalEntries.map(({ policyIndex, pathIndex, policy }) => (
              <Chip
                key={`${policyIndex}-${pathIndex}`}
                label={`${getDisplayName(policy)} (v${policy.version})`}
                color="warning"
                variant="outlined"
                onDelete={() =>
                  handleRemoveGuardrail(policyIndex, pathIndex)
                }
                disabled={isUpdating}
              />
            ))}
            <Button
              variant="contained"
              size="small"
              startIcon={<Plus size={16} />}
              onClick={() => handleOpenDrawer({ type: "global" })}
              disabled={isUpdating}
            >
              Add Guardrail
            </Button>
          </Stack>
        </Box>

        <Box>
          <Typography variant="h6" component="h2" sx={{ mb: 2 }}>
            Resource-wise Guardrails
          </Typography>
          {specLoading ? (
            <Stack direction="row" spacing={1} alignItems="center" sx={{ py: 2 }}>
              <Skeleton variant="circular" width={16} height={16} />
              <Typography variant="body2" color="text.secondary">
                Loading OpenAPI spec…
              </Typography>
            </Stack>
          ) : resources.length === 0 ? (
            <ListingTable.Container>
              <ListingTable.EmptyState
                illustration={<ShieldAlert size={64} />}
                title="No resources found"
                description="Add an OpenAPI specification to define resources for resource-wise guardrails."
              />
            </ListingTable.Container>
          ) : (
            <Stack spacing={0}>
              {resources.map((resource) => {
                const key = getResourceKey(resource);
                const isExpanded = expandedResources.has(key);
                const resourceGuardrails = getResourceGuardrails(resource);
                return (
                  <Accordion
                    key={key}
                    expanded={isExpanded}
                    onChange={(_, exp) =>
                      setExpandedResources((prev) => {
                        const next = new Set(prev);
                        if (exp) next.add(key);
                        else next.delete(key);
                        return next;
                      })
                    }
                    disableGutters
                  >
                    <AccordionSummary expandIcon={<ChevronDown size={18} />}>
                      <Stack
                        direction="row"
                        alignItems="center"
                        spacing={1}
                      >
                        <Chip
                          label={resource.method}
                          size="small"
                          variant="outlined"
                          color={getMethodChipColor(resource.method)}
                        />
                        <Typography variant="body2">{resource.path}</Typography>
                      </Stack>
                    </AccordionSummary>
                    <AccordionDetails>
                      <Typography variant="subtitle2" sx={{ mb: 1 }}>
                        Guardrails
                      </Typography>
                      <Stack
                        direction="row"
                        spacing={1}
                        flexWrap="wrap"
                        useFlexGap
                        sx={{ mb: 1 }}
                      >
                        {resourceGuardrails.length === 0 ? (
                          <Typography variant="body2" color="text.secondary">
                            No guardrails added yet.
                          </Typography>
                        ) : (
                          resourceGuardrails.map(
                            ({ policyIndex, pathIndex, policy }) => (
                              <Chip
                                key={`${resource.path}-${policyIndex}-${pathIndex}`}
                                label={`${getDisplayName(policy)} (v${policy.version})`}
                                color="warning"
                                variant="outlined"
                                onDelete={() =>
                                  handleRemoveGuardrail(
                                    policyIndex,
                                    pathIndex,
                                  )
                                }
                                disabled={isUpdating}
                              />
                            ),
                          )
                        )}
                      </Stack>
                      <Button
                        variant="outlined"
                        size="small"
                        startIcon={<Plus size={16} />}
                        onClick={() =>
                          handleOpenDrawer({
                            type: "resource",
                            method: resource.method,
                            path: resource.path,
                          })
                        }
                        disabled={isUpdating}
                      >
                        Add Guardrail
                      </Button>
                    </AccordionDetails>
                  </Accordion>
                );
              })}
            </Stack>
          )}
        </Box>
      </Stack>

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
          disabled={!isDirty || isUpdating}
        >
          {isUpdating ? "Saving..." : "Save"}
        </Button>
      </Stack>

      <GuardrailSelectorDrawer
        open={drawerOpen}
        onClose={handleCloseDrawer}
        onSubmit={handleDrawerSubmit}
        title="Guardrails"
        subtitle="Choose a guardrail to configure advanced options."
        minWidth={600}
        maxWidth={600}
      />
    </Stack>
  );
}
