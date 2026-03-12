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
  RateLimitingLimitConfig,
  RateLimitingScopeConfig,
} from "@agent-management-platform/types";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Collapse,
  FormControl,
  FormLabel,
  Grid,
  ListingTable,
  MenuItem,
  SearchBar,
  Select,
  Skeleton,
  Stack,
  Switch,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown, FileCode, Search } from "@wso2/oxygen-ui-icons-react";
import { useParams, useSearchParams } from "react-router-dom";
import { useOpenApiSpec } from "../hooks/useOpenApiSpec";
import {
  extractResourcesFromSpec,
  getMethodChipColor,
  getResourceKey,
  parseOpenApiSpec,
  type ResourceItem,
} from "../utils/openapiResources";
import { z } from "zod";

const RESET_UNITS = [
  { value: "second", label: "second" },
  { value: "minute", label: "minute" },
  { value: "hour", label: "hour" },
  { value: "all", label: "all" },
] as const;

const criterionRowSchema = z
  .object({
    enabled: z.boolean(),
    quota: z.string(),
    duration: z.string(),
    unit: z.string(),
  })
  .refine(
    (data) => {
      if (!data.enabled) return true;
      const q = Number(data.quota);
      return !Number.isNaN(q) && q >= 1;
    },
    { message: "Quota must be >= 1", path: ["quota"] },
  )
  .refine(
    (data) => {
      if (!data.enabled) return true;
      const d = Number(data.duration);
      return !Number.isNaN(d) && d >= 1;
    },
    { message: "Reset duration must be >= 1", path: ["duration"] },
  );

const criteriaStateSchema = z.object({
  request: criterionRowSchema,
  token: criterionRowSchema,
  cost: criterionRowSchema,
});

/** Rate Limiting uses "-" separator for backendResourceWiseMap keys. */
const getRateLimitResourceKey = (r: ResourceItem) => getResourceKey(r, "-");

export interface CriteriaState {
  request: { enabled: boolean; quota: string; duration: string; unit: string };
  token: { enabled: boolean; quota: string; duration: string; unit: string };
  cost: { enabled: boolean; quota: string; duration: string; unit: string };
}

const DEFAULT_CRITERIA: CriteriaState = {
  request: { enabled: false, quota: "", duration: "1", unit: "minute" },
  token: { enabled: false, quota: "", duration: "1", unit: "minute" },
  cost: { enabled: false, quota: "", duration: "1", unit: "minute" },
};

function criteriaFromLimit(
  limit: RateLimitingLimitConfig | undefined,
): CriteriaState {
  const c = { ...DEFAULT_CRITERIA };
  if (!limit) return c;

  if (limit.request) {
    c.request = {
      enabled: limit.request.enabled,
      quota: String(limit.request.count ?? ""),
      duration: String(limit.request.reset?.duration ?? 1),
      unit: limit.request.reset?.unit ?? "minute",
    };
  }
  if (limit.token) {
    c.token = {
      enabled: limit.token.enabled,
      quota: String(limit.token.count ?? ""),
      duration: String(limit.token.reset?.duration ?? 1),
      unit: limit.token.reset?.unit ?? "minute",
    };
  }
  if (limit.cost) {
    c.cost = {
      enabled: limit.cost.enabled,
      quota: String(limit.cost.amount ?? ""),
      duration: String(limit.cost.reset?.duration ?? 1),
      unit: limit.cost.reset?.unit ?? "minute",
    };
  }
  return c;
}

function parseFinite(value: string | number | undefined): number | undefined {
  const v = Number(value);
  return Number.isFinite(v) ? v : undefined;
}

function limitFromCriteria(criteria: CriteriaState): RateLimitingLimitConfig {
  const limit: RateLimitingLimitConfig = {};
  if (criteria.request.enabled) {
    const duration = parseFinite(criteria.request.duration);
    const count = parseFinite(criteria.request.quota);
    const unit = criteria.request.unit || "minute";
    if (count !== undefined && duration !== undefined) {
      limit.request = {
        enabled: true,
        count,
        reset: { duration, unit },
      };
    }
  }
  if (criteria.token.enabled) {
    const duration = parseFinite(criteria.token.duration);
    const count = parseFinite(criteria.token.quota);
    const unit = criteria.token.unit || "minute";
    if (count !== undefined && duration !== undefined) {
      limit.token = {
        enabled: true,
        count,
        reset: { duration, unit },
      };
    }
  }
  if (criteria.cost.enabled) {
    const duration = parseFinite(criteria.cost.duration);
    const amount = parseFinite(criteria.cost.quota);
    const unit = criteria.cost.unit || "minute";
    if (amount !== undefined && duration !== undefined) {
      limit.cost = {
        enabled: true,
        amount,
        reset: { duration, unit },
      };
    }
  }
  return limit;
}

function getCriteriaFieldErrors(
  criteria: CriteriaState,
): Record<string, string> {
  const result = criteriaStateSchema.safeParse(criteria);
  if (result.success) return {};

  const errors: Record<string, string> = {};
  for (const issue of result.error.issues) {
    const path = issue.path.join(".");
    if (!errors[path]) {
      errors[path] = issue.message;
    }
  }
  return errors;
}

function hasConfiguredCriteria(criteria: CriteriaState): boolean {
  return (
    (criteria.request.enabled && Number(criteria.request.quota) >= 1) ||
    (criteria.token.enabled && Number(criteria.token.quota) >= 1) ||
    (criteria.cost.enabled && Number(criteria.cost.quota) >= 1)
  );
}

type CriteriaBlockProps = {
  criteria: CriteriaState;
  onChange: (c: CriteriaState) => void;
  disabled?: boolean;
  showCost?: boolean;
  fieldErrors?: Record<string, string>;
};

function CriteriaBlock({
  criteria,
  onChange,
  disabled = false,
  showCost = true,
  fieldErrors,
}: CriteriaBlockProps) {
  const [expanded, setExpanded] = useState({
    request: criteria.request.enabled,
    token: criteria.token.enabled,
    cost: criteria.cost.enabled,
  });

  useEffect(() => {
    setExpanded({
      request: criteria.request.enabled,
      token: criteria.token.enabled,
      cost: criteria.cost.enabled,
    });
  }, [
    criteria.request.enabled,
    criteria.token.enabled,
    criteria.cost.enabled,
  ]);

  const update = useCallback(
    (
      key: keyof CriteriaState,
      patch: Partial<CriteriaState[keyof CriteriaState]>,
    ) => {
      onChange({
        ...criteria,
        [key]: { ...criteria[key], ...patch },
      });
    },
    [criteria, onChange],
  );

  return (
    <Stack>
      {/* Request Count */}
      <Accordion
        expanded={expanded.request}
        onChange={(_, isExpanded) =>
          !disabled && setExpanded((e) => ({ ...e, request: isExpanded }))
        }
        disableGutters
      >
        <AccordionSummary expandIcon={<ChevronDown size={18} />}>
          <Stack
            direction="row"
            alignItems="center"
            justifyContent="space-between"
            width="100%"
          >
            <Typography variant="subtitle2">Request Count</Typography>
            <Switch
              size="small"
              checked={criteria.request.enabled}
              onChange={(_, v) => update("request", { enabled: v })}
              disabled={disabled}
              onClick={(e) => e.stopPropagation()}
            />
          </Stack>
        </AccordionSummary>
        <AccordionDetails>
          <Stack spacing={1.5}>
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 4 }}>
                <FormControl fullWidth size="small">
                  <FormLabel>Quota</FormLabel>
                  <TextField
                    size="small"
                    type="number"
                    value={criteria.request.quota}
                    onChange={(e) =>
                      update("request", { quota: e.target.value })
                    }
                    disabled={disabled}
                    error={!!fieldErrors?.["request.quota"]}
                    helperText={fieldErrors?.["request.quota"]}
                    slotProps={{
                      input: { inputProps: { min: 1 } },
                    }}
                  />
                </FormControl>
              </Grid>
              <Grid size={{ xs: 12, sm: 4 }}>
                <FormControl fullWidth size="small">
                  <FormLabel>Reset Duration</FormLabel>
                  <TextField
                    size="small"
                    type="number"
                    value={criteria.request.duration}
                    onChange={(e) =>
                      update("request", { duration: e.target.value })
                    }
                    disabled={disabled}
                    error={!!fieldErrors?.["request.duration"]}
                    helperText={fieldErrors?.["request.duration"]}
                    slotProps={{
                      input: { inputProps: { min: 1 } },
                    }}
                  />
                </FormControl>
              </Grid>
              <Grid size={{ xs: 12, sm: 4 }}>
                <FormControl fullWidth size="small">
                  <FormLabel>Reset unit</FormLabel>
                  <Select
                    size="small"
                    value={criteria.request.unit}
                    onChange={(e) =>
                      update("request", { unit: e.target.value })
                    }
                    disabled={disabled}
                  >
                    {RESET_UNITS.map((u) => (
                      <MenuItem key={u.value} value={u.value}>
                        {u.label}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              </Grid>
            </Grid>
          </Stack>
        </AccordionDetails>
      </Accordion>

      {/* Token Count */}
      <Accordion
        expanded={expanded.token}
        onChange={(_, isExpanded) =>
          !disabled && setExpanded((e) => ({ ...e, token: isExpanded }))
        }
        disableGutters
      >
        <AccordionSummary expandIcon={<ChevronDown size={18} />}>
          <Stack
            direction="row"
            alignItems="center"
            justifyContent="space-between"
            width="100%"
          >
            <Typography variant="subtitle2">Token Count</Typography>
            <Switch
              size="small"
              checked={criteria.token.enabled}
              onChange={(_, v) => update("token", { enabled: v })}
              disabled={disabled}
              onClick={(e) => e.stopPropagation()}
            />
          </Stack>
        </AccordionSummary>
        <AccordionDetails>
          <Stack spacing={1.5}>
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 4 }}>
                <FormControl fullWidth size="small">
                  <FormLabel>Quota</FormLabel>
                  <TextField
                    size="small"
                    type="number"
                    value={criteria.token.quota}
                    onChange={(e) => update("token", { quota: e.target.value })}
                    disabled={disabled}
                    error={!!fieldErrors?.["token.quota"]}
                    helperText={fieldErrors?.["token.quota"]}
                    slotProps={{
                      input: { inputProps: { min: 1 } },
                    }}
                  />
                </FormControl>
              </Grid>
              <Grid size={{ xs: 12, sm: 4 }}>
                <FormControl fullWidth size="small">
                  <FormLabel>Reset Duration</FormLabel>
                  <TextField
                    size="small"
                    type="number"
                    value={criteria.token.duration}
                    onChange={(e) =>
                      update("token", { duration: e.target.value })
                    }
                    disabled={disabled}
                    error={!!fieldErrors?.["token.duration"]}
                    helperText={fieldErrors?.["token.duration"]}
                    slotProps={{
                      input: { inputProps: { min: 1 } },
                    }}
                  />
                </FormControl>
              </Grid>
              <Grid size={{ xs: 12, sm: 4 }}>
                <FormControl fullWidth size="small">
                  <FormLabel>Reset unit</FormLabel>
                  <Select
                    size="small"
                    value={criteria.token.unit}
                    onChange={(e) => update("token", { unit: e.target.value })}
                    disabled={disabled}
                  >
                    {RESET_UNITS.map((u) => (
                      <MenuItem key={u.value} value={u.value}>
                        {u.label}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              </Grid>
            </Grid>
          </Stack>
        </AccordionDetails>
      </Accordion>

      {/* Cost */}
      {showCost && (
        <Accordion
          expanded={expanded.cost}
          onChange={(_, isExpanded) =>
            !disabled && setExpanded((e) => ({ ...e, cost: isExpanded }))
          }
          disableGutters
        >
          <AccordionSummary expandIcon={<ChevronDown size={18} />}>
            <Stack
              direction="row"
              alignItems="center"
              justifyContent="space-between"
              width="100%"
            >
              <Typography variant="subtitle2">Cost</Typography>
              <Switch
                size="small"
                checked={criteria.cost.enabled}
                onChange={(_, v) => update("cost", { enabled: v })}
                disabled={disabled}
                onClick={(e) => e.stopPropagation()}
              />
            </Stack>
          </AccordionSummary>
          <AccordionDetails>
            <Stack spacing={1.5}>
              <Grid container spacing={2}>
                <Grid size={{ xs: 12, sm: 4 }}>
                  <FormControl fullWidth size="small">
                    <FormLabel>Quota</FormLabel>
                    <TextField
                      size="small"
                      type="number"
                      value={criteria.cost.quota}
                      onChange={(e) =>
                        update("cost", { quota: e.target.value })
                      }
                      disabled={disabled}
                      error={!!fieldErrors?.["cost.quota"]}
                      helperText={fieldErrors?.["cost.quota"]}
                      slotProps={{
                        input: { inputProps: { min: 1 } },
                      }}
                    />
                  </FormControl>
                </Grid>
                <Grid size={{ xs: 12, sm: 4 }}>
                  <FormControl fullWidth size="small">
                    <FormLabel>Reset Duration</FormLabel>
                    <TextField
                      size="small"
                      type="number"
                      value={criteria.cost.duration}
                      onChange={(e) =>
                        update("cost", { duration: e.target.value })
                      }
                      disabled={disabled}
                      error={!!fieldErrors?.["cost.duration"]}
                      helperText={fieldErrors?.["cost.duration"]}
                      slotProps={{
                        input: { inputProps: { min: 1 } },
                      }}
                    />
                  </FormControl>
                </Grid>
                <Grid size={{ xs: 12, sm: 4 }}>
                  <FormControl fullWidth size="small">
                    <FormLabel>Reset unit</FormLabel>
                    <Select
                      size="small"
                      value={criteria.cost.unit}
                      onChange={(e) => update("cost", { unit: e.target.value })}
                      disabled={disabled}
                    >
                      {RESET_UNITS.map((u) => (
                        <MenuItem key={u.value} value={u.value}>
                          {u.label}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                </Grid>
              </Grid>
            </Stack>
          </AccordionDetails>
        </Accordion>
      )}
    </Stack>
  );
}

export type LLMProviderRateLimitingTabProps = {
  providerData: LLMProviderResponse | null | undefined;
  openapiSpecUrl?: string;
  isLoading?: boolean;
};

export function LLMProviderRateLimitingTab({
  providerData,
  openapiSpecUrl,
  isLoading = false,
}: LLMProviderRateLimitingTabProps) {
  const { orgId, providerId } = useParams<{
    orgId: string;
    providerId: string;
  }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const { mutateAsync: updateProvider, isPending } = useUpdateLLMProvider();

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

  const modeFromUrl = searchParams.get("mode");
  const initialMode =
    modeFromUrl === "global" || modeFromUrl === "resourceWise"
      ? modeFromUrl
      : "global";
  const [backendMode, setBackendMode] = useState<"global" | "resourceWise">(
    initialMode,
  );
  const [backendGlobalCriteria, setBackendGlobalCriteria] =
    useState<CriteriaState>(DEFAULT_CRITERIA);
  const [backendResourceWiseDefault, setBackendResourceWiseDefault] =
    useState<CriteriaState>(DEFAULT_CRITERIA);
  const [backendResourceWiseMap, setBackendResourceWiseMap] = useState<
    Record<string, CriteriaState>
  >({});
  const [backendResourceSearch, setBackendResourceSearch] = useState("");
  const [backendExpandedResources, setBackendExpandedResources] = useState<
    Set<string>
  >(new Set());
  const [criteriaFieldErrors, setCriteriaFieldErrors] = useState<
    Record<string, Record<string, string>>
  >({});

  const hasBackendGlobalConfig = useMemo(
    () => hasConfiguredCriteria(backendGlobalCriteria),
    [backendGlobalCriteria],
  );
  const hasBackendResourceWiseConfig = useMemo(() => {
    if (hasConfiguredCriteria(backendResourceWiseDefault)) return true;
    return Object.values(backendResourceWiseMap).some(hasConfiguredCriteria);
  }, [backendResourceWiseDefault, backendResourceWiseMap]);

  const lastSavedRef = useRef<string | null>(null);
  const [loadedAt, setLoadedAt] = useState(0);

  const getPayloadSnapshot = useCallback(() => {
    if (backendMode === "global") {
      return JSON.stringify(limitFromCriteria(backendGlobalCriteria));
    }
    const defaultLimit = limitFromCriteria(backendResourceWiseDefault);
    const resourcesPayload = Object.entries(backendResourceWiseMap)
      .filter(([, c]) => hasConfiguredCriteria(c))
      .map(([res, c]) => ({ resource: res, limit: limitFromCriteria(c) }))
      .sort((a, b) => a.resource.localeCompare(b.resource));
    return JSON.stringify({
      default: defaultLimit,
      resources: resourcesPayload,
    });
  }, [
    backendMode,
    backendGlobalCriteria,
    backendResourceWiseDefault,
    backendResourceWiseMap,
  ]);

  const loadFromProvider = useCallback(() => {
    if (!providerData) return;

    const urlMode = searchParams.get("mode");
    const pl = providerData.rateLimiting?.providerLevel;
    let newMode: "global" | "resourceWise" = "global";

    if (pl?.resourceWise) {
      newMode = "resourceWise";
      setBackendMode("resourceWise");
      setBackendGlobalCriteria(DEFAULT_CRITERIA);
      setBackendResourceWiseDefault(criteriaFromLimit(pl.resourceWise.default));
      const map: Record<string, CriteriaState> = {};
      for (const r of pl.resourceWise.resources ?? []) {
        map[r.resource] = criteriaFromLimit(r.limit);
      }
      setBackendResourceWiseMap(map);
    } else if (pl?.global) {
      newMode = "global";
      setBackendMode("global");
      setBackendGlobalCriteria(criteriaFromLimit(pl.global));
      setBackendResourceWiseDefault(DEFAULT_CRITERIA);
      setBackendResourceWiseMap({});
    } else {
      setBackendMode("global");
      setBackendGlobalCriteria(DEFAULT_CRITERIA);
      setBackendResourceWiseDefault(DEFAULT_CRITERIA);
      setBackendResourceWiseMap({});
    }
    if (!urlMode) {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.set("mode", newMode);
          return next;
        },
        { replace: true },
      );
    }
    setLoadedAt(Date.now());
  }, [providerData, searchParams, setSearchParams]);

  useEffect(() => {
    loadFromProvider();
  }, [loadFromProvider]);

  useEffect(() => {
    const m = searchParams.get("mode");
    if (m === "global" || m === "resourceWise") setBackendMode(m);
  }, [searchParams]);

  const currentSnapshotRef = useRef<string>("");
  currentSnapshotRef.current = getPayloadSnapshot();

  useEffect(() => {
    if (loadedAt > 0 && providerData) {
      lastSavedRef.current = currentSnapshotRef.current;
    }
  }, [loadedAt, providerData]);

  const isDirty = useMemo(() => {
    if (!providerData) return false;
    if (lastSavedRef.current === null) return false;
    const current = getPayloadSnapshot();
    return current !== lastSavedRef.current;
  }, [providerData, getPayloadSnapshot]);

  const handleSave = useCallback(async () => {
    if (!providerData || !orgId || !providerId) return;
    if (isLoading) return;

    if (backendMode === "global" && hasBackendResourceWiseConfig) {
      setStatus({
        message:
          "Backend cannot have both Provider-wide and Per Resource values. Remove one side and try again.",
        severity: "error",
      });
      return;
    }
    if (backendMode === "resourceWise" && hasBackendGlobalConfig) {
      setStatus({
        message:
          "Backend cannot have both Provider-wide and Per Resource values. Remove one side and try again.",
        severity: "error",
      });
      return;
    }

    const criteriaToValidate: {
      c: CriteriaState;
      blockKey: string;
    }[] = [];
    if (backendMode === "global") {
      criteriaToValidate.push({
        c: backendGlobalCriteria,
        blockKey: "global",
      });
    } else {
      criteriaToValidate.push({
        c: backendResourceWiseDefault,
        blockKey: "resourceWise-default",
      });
      Object.entries(backendResourceWiseMap).forEach(([res, c]) => {
        if (hasConfiguredCriteria(c)) {
          criteriaToValidate.push({ c, blockKey: `resourceWise-${res}` });
        }
      });
    }

    for (const { c, blockKey } of criteriaToValidate) {
      const errors = getCriteriaFieldErrors(c);
      if (Object.keys(errors).length > 0) {
        setCriteriaFieldErrors({ [blockKey]: errors });
        if (blockKey.startsWith("resourceWise-") && blockKey !== "resourceWise-default") {
          const resKey = blockKey.replace("resourceWise-", "");
          setBackendExpandedResources((prev) => new Set(prev).add(resKey));
        }
        return;
      }
    }
    setCriteriaFieldErrors({});

    const buildProviderLevel = (): RateLimitingScopeConfig => {
      if (backendMode === "global") {
        return {
          global: limitFromCriteria(backendGlobalCriteria),
          resourceWise: undefined,
        };
      }
      const resourcesPayload = Object.entries(backendResourceWiseMap)
        .filter(([, c]) => hasConfiguredCriteria(c))
        .map(([res, c]) => ({
          resource: res,
          limit: limitFromCriteria(c),
        }));
      return {
        global: undefined,
        resourceWise: {
          default: limitFromCriteria(backendResourceWiseDefault),
          resources: resourcesPayload,
        },
      };
    };

    try {
      await updateProvider({
        params: { orgName: orgId, providerId },
        body: {
          rateLimiting: {
            providerLevel: buildProviderLevel(),
          },
        },
      });
      setStatus({
        message: "Rate limits updated successfully.",
        severity: "success",
      });
      setCriteriaFieldErrors({});
      lastSavedRef.current = getPayloadSnapshot();
    } catch {
      setStatus({
        message: "Failed to update rate limits.",
        severity: "error",
      });
    }
  }, [
    providerData,
    orgId,
    providerId,
    isLoading,
    backendMode,
    backendGlobalCriteria,
    backendResourceWiseDefault,
    backendResourceWiseMap,
    hasBackendGlobalConfig,
    hasBackendResourceWiseConfig,
    updateProvider,
    getPayloadSnapshot,
  ]);

  const handleDiscard = useCallback(() => {
    loadFromProvider();
    setStatus(null);
    setCriteriaFieldErrors({});
  }, [loadFromProvider]);

  const filteredResources = useMemo(() => {
    if (!backendResourceSearch.trim()) return resources;
    const q = backendResourceSearch.toLowerCase();
    return resources.filter(
      (r) =>
        getRateLimitResourceKey(r).toLowerCase().includes(q) ||
        r.path.toLowerCase().includes(q) ||
        (r.summary ?? "").toLowerCase().includes(q),
    );
  }, [resources, backendResourceSearch]);

  if (isLoading) {
    return (
      <Stack spacing={3}>
        <Skeleton variant="rounded" height={120} />
        <Skeleton variant="rounded" height={200} />
      </Stack>
    );
  }

  if (!providerData) {
    return null;
  }

  return (
    <Stack spacing={3}>
      <Grid container spacing={3}>
        {/* Backend section */}
        <Grid size={{ xs: 12 }}>
          <Typography variant="h6" component="h2" sx={{ mb: 2 }}>
            Backend
          </Typography>
          <Stack spacing={2}>
            <FormControl>
              <FormLabel sx={{ mb: 1 }}>Mode</FormLabel>
              <ToggleButtonGroup
                sx={{ textTransform: "none" }}
                value={backendMode}
                exclusive
                color="primary"
                onChange={(_, v: "global" | "resourceWise" | null) => {
                  if (v) {
                    setBackendMode(v);
                    setSearchParams(
                      (prev) => {
                        const next = new URLSearchParams(prev);
                        next.set("mode", v);
                        return next;
                      },
                      { replace: true },
                    );
                  }
                }}
                size="small"
              >
                <Tooltip
                  title={
                    backendMode === "resourceWise" &&
                    hasBackendResourceWiseConfig
                      ? "If you need Provider-wide, remove Per Resource values first."
                      : ""
                  }
                >
                  <ToggleButton
                    value="global"
                    sx={{ textTransform: "none" }}
                    disabled={
                      backendMode === "resourceWise" &&
                      hasBackendResourceWiseConfig
                    }
                  >
                    Provider-wide
                  </ToggleButton>
                </Tooltip>
                <Tooltip
                  title={
                    backendMode === "global" && hasBackendGlobalConfig
                      ? "If you need Per Resource, remove Provider-wide values first."
                      : ""
                  }
                >
                  <span>
                    <ToggleButton
                      sx={{ textTransform: "none" }}
                      value="resourceWise"
                      disabled={
                        backendMode === "global" && hasBackendGlobalConfig
                      }
                    >
                      Per Resource
                    </ToggleButton>
                  </span>
                </Tooltip>
              </ToggleButtonGroup>
            </FormControl>

            {backendMode === "global" && (
              <CriteriaBlock
                criteria={backendGlobalCriteria}
                onChange={setBackendGlobalCriteria}
                fieldErrors={criteriaFieldErrors["global"]}
              />
            )}

            {backendMode === "resourceWise" && (
              <Stack spacing={2}>
                <Collapse in>
                  <Box>
                    <Typography variant="subtitle2" sx={{ mb: 1 }}>
                      Limit per Resource
                    </Typography>
                    <CriteriaBlock
                      criteria={backendResourceWiseDefault}
                      onChange={setBackendResourceWiseDefault}
                      fieldErrors={criteriaFieldErrors["resourceWise-default"]}
                    />
                  </Box>
                </Collapse>
                <SearchBar
                  placeholder="Search resources…"
                  value={backendResourceSearch}
                  onChange={(e) => setBackendResourceSearch(e.target.value)}
                  sx={{ maxWidth: 280 }}
                />
                {specLoading ? (
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      gap: 1,
                      py: 2,
                    }}
                  >
                    <CircularProgress size={16} />
                    <Typography variant="body2" color="text.secondary">
                      Loading OpenAPI spec…
                    </Typography>
                  </Box>
                ) : filteredResources.length === 0 ? (
                  <ListingTable.Container>
                    <ListingTable.EmptyState
                      illustration={
                        backendResourceSearch.trim() ? (
                          <Search size={64} />
                        ) : (
                          <FileCode size={64} />
                        )
                      }
                      title={
                        backendResourceSearch.trim()
                          ? "No resources match your search"
                          : "No resources found"
                      }
                      description={
                        backendResourceSearch.trim()
                          ? "Try a different keyword or clear the search filter."
                          : "No resources are available from the OpenAPI spec. Add an OpenAPI specification to define resources."
                      }
                    />
                  </ListingTable.Container>
                ) : (
                  <Stack>
                    {filteredResources.map((r) => {
                      const key = getRateLimitResourceKey(r);
                      const isExpanded = backendExpandedResources.has(key);
                      const criteria =
                        backendResourceWiseMap[key] ??
                        backendResourceWiseDefault;
                      return (
                        <Accordion
                          key={key}
                          expanded={isExpanded}
                          onChange={(_, expanded) =>
                            setBackendExpandedResources((prev) => {
                              const next = new Set(prev);
                              if (expanded) next.add(key);
                              else next.delete(key);
                              return next;
                            })
                          }
                          disableGutters
                        >
                          <AccordionSummary
                            expandIcon={<ChevronDown size={16} />}
                          >
                            <Stack
                              direction="row"
                              alignItems="center"
                              spacing={1}
                            >
                              <Chip
                                label={r.method}
                                size="small"
                                variant="outlined"
                                color={getMethodChipColor(r.method)}
                              />
                              <Typography variant="body2">{r.path}</Typography>
                            </Stack>
                          </AccordionSummary>
                          {isExpanded && (
                            <AccordionDetails>
                              <CriteriaBlock
                                criteria={criteria}
                                onChange={(c) =>
                                  setBackendResourceWiseMap((m) => ({
                                    ...m,
                                    [key]: c,
                                  }))
                                }
                                fieldErrors={
                                  criteriaFieldErrors[`resourceWise-${key}`]
                                }
                              />
                            </AccordionDetails>
                          )}
                        </Accordion>
                      );
                    })}
                  </Stack>
                )}
              </Stack>
            )}
          </Stack>
        </Grid>
      </Grid>

      <Stack spacing={1.5} width="100%">
        <Collapse
          in={
            !!status &&
            (status.severity === "error" || !isDirty)
          }
          timeout={300}
        >
          {status && (
            <Alert
              severity={status.severity}
              onClose={() => setStatus(null)}
              sx={{ width: "100%"}}
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
            onClick={() => void handleSave()}
            disabled={!isDirty || isPending}
          >
            {isPending ? "Saving..." : "Save"}
          </Button>
        </Stack>
      </Stack>
    </Stack>
  );
}
