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
import type {
  LLMProviderResponse,
  RateLimitingLimitConfig,
  RateLimitingScopeConfig,
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
  CircularProgress,
  Collapse,
  FormControl,
  FormLabel,
  Grid,
  ListingTable,
  MenuItem,
  Paper,
  SearchBar,
  Select,
  Skeleton,
  Stack,
  Switch,
  Tab,
  TablePagination,
  Tabs,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown, FileCode, Search } from "@wso2/oxygen-ui-icons-react";
import { useSearchParams } from "react-router-dom";
import { useOpenApiSpec } from "../hooks/useOpenApiSpec";
import {
  extractResourcesFromSpec,
  getMethodChipColor,
  getResourceKey,
  parseOpenApiSpec,
  type ResourceItem,
} from "../utils/openapiResources";
import { z } from "zod";
import { useUnsavedChangesGuard } from "@agent-management-platform/shared-component";

const RESET_UNITS = [
  { value: "minute", label: "Minute(s)" },
  { value: "hour",   label: "Hour(s)"   },
  { value: "day",    label: "Day(s)"    },
  { value: "week",   label: "Week(s)"   },
  { value: "month",  label: "Month(s)"  },
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
      return !Number.isNaN(d) && Number.isInteger(d) && d >= 1;
    },
    { message: "Duration must be a whole number >= 1", path: ["duration"] },
  );

const costCriterionRowSchema = z
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
      return !Number.isNaN(q) && q > 0;
    },
    { message: "Budget must be > 0", path: ["quota"] },
  )
  .refine(
    (data) => {
      if (!data.enabled) return true;
      const d = Number(data.duration);
      return !Number.isNaN(d) && Number.isInteger(d) && d >= 1;
    },
    { message: "Duration must be a whole number >= 1", path: ["duration"] },
  );

const criteriaStateSchema = z.object({
  request: criterionRowSchema,
  token: criterionRowSchema,
  cost: costCriterionRowSchema,
});

/** Rate Limiting uses "-" separator for backendResourceWiseMap keys. */
const getRateLimitResourceKey = (r: ResourceItem) => getResourceKey(r, "-");

export interface CriteriaState {
  request: { enabled: boolean; quota: string; duration: string; unit: string };
  token:   { enabled: boolean; quota: string; duration: string; unit: string };
  cost:    { enabled: boolean; quota: string; duration: string; unit: string };
}

const DEFAULT_CRITERIA: CriteriaState = {
  request: { enabled: false, quota: "", duration: "1", unit: "hour" },
  token:   { enabled: false, quota: "", duration: "1", unit: "hour" },
  cost:    { enabled: false, quota: "", duration: "1", unit: "day"  },
};

function criteriaFromLimit(
  limit: RateLimitingLimitConfig | undefined,
): CriteriaState {
  const c: CriteriaState = { ...DEFAULT_CRITERIA };
  if (!limit) return c;

  if (limit.request) {
    c.request = {
      enabled:  limit.request.enabled,
      quota:    String(limit.request.count ?? ""),
      duration: String(limit.request.reset?.duration ?? 1),
      unit:     limit.request.reset?.unit ?? "hour",
    };
  }
  if (limit.token) {
    c.token = {
      enabled:  limit.token.enabled,
      quota:    String(limit.token.count ?? ""),
      duration: String(limit.token.reset?.duration ?? 1),
      unit:     limit.token.reset?.unit ?? "hour",
    };
  }
  if (limit.cost) {
    c.cost = {
      enabled:  limit.cost.enabled,
      quota:    String(limit.cost.amount ?? ""),
      duration: String(limit.cost.reset?.duration ?? 1),
      unit:     limit.cost.reset?.unit ?? "day",
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
    const count = parseFinite(criteria.request.quota);
    const duration = parseFinite(criteria.request.duration);
    if (count !== undefined && duration !== undefined) {
      limit.request = {
        enabled: true,
        count,
        reset: { duration, unit: criteria.request.unit },
      };
    }
  }
  if (criteria.token.enabled) {
    const count = parseFinite(criteria.token.quota);
    const duration = parseFinite(criteria.token.duration);
    if (count !== undefined && duration !== undefined) {
      limit.token = {
        enabled: true,
        count,
        reset: { duration, unit: criteria.token.unit },
      };
    }
  }
  if (criteria.cost.enabled) {
    const amount = parseFinite(criteria.cost.quota);
    const duration = parseFinite(criteria.cost.duration);
    if (amount !== undefined && duration !== undefined) {
      limit.cost = {
        enabled: true,
        amount,
        reset: { duration, unit: criteria.cost.unit },
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
    (criteria.cost.enabled && Number(criteria.cost.quota) > 0)
  );
}

function hasEnabledCriteria(criteria: CriteriaState): boolean {
  return criteria.request.enabled || criteria.token.enabled || criteria.cost.enabled;
}

// ─── CriterionCard ────────────────────────────────────────────────────────────

type CriterionKey = "request" | "token" | "cost";

type CriterionCardProps = {
  label: string;
  quotaLabel: string;
  quotaPlaceholder: string;
  quotaMin: number;
  quotaStep: number;
  criterionKey: CriterionKey;
  value: CriteriaState[CriterionKey];
  onChange: (patch: Partial<CriteriaState[CriterionKey]>) => void;
  disabled?: boolean;
  fieldErrors?: Record<string, string>;
};

function CriterionCard({
  label,
  quotaLabel,
  quotaPlaceholder,
  quotaMin,
  quotaStep,
  criterionKey,
  value,
  onChange,
  disabled = false,
  fieldErrors,
}: CriterionCardProps) {
  const [expanded, setExpanded] = useState(value.enabled);

  useEffect(() => {
    setExpanded(value.enabled);
  }, [value.enabled]);

  const errorKey = `${criterionKey}.quota`;
  const durationErrorKey = `${criterionKey}.duration`;

  return (
    <Paper
      variant="outlined"
      sx={{
        borderRadius: 2,
        overflow: "hidden",
        opacity: disabled ? 0.6 : 1,
      }}
    >
      <Accordion
        expanded={expanded}
        onChange={(_, isExpanded) => setExpanded(isExpanded)}
        disableGutters
        elevation={0}
        sx={{ background: "transparent", "&:before": { display: "none" } }}
      >
        <AccordionSummary
          expandIcon={<ChevronDown size={18} />}
          sx={{ px: 2, py: 1 }}
        >
          <Stack
            direction="row"
            alignItems="center"
            justifyContent="space-between"
            width="100%"
            pr={1}
          >
            <Stack direction="row" alignItems="center" spacing={1}>
              <Typography variant="subtitle2">{label}</Typography>
              {value.enabled && (
                <Chip
                  label="Enabled"
                  size="small"
                  color="primary"
                  variant="outlined"
                  sx={{ height: 20, fontSize: "0.7rem" }}
                />
              )}
            </Stack>
            <Switch
              size="small"
              checked={value.enabled}
              onChange={(_, v) => onChange({ enabled: v })}
              disabled={disabled}
              onClick={(e) => e.stopPropagation()}
            />
          </Stack>
        </AccordionSummary>
        <AccordionDetails sx={{ px: 2, pb: 2, pt: 0 }}>
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, sm: 4 }}>
              <FormControl fullWidth size="small">
                <FormLabel>{quotaLabel}</FormLabel>
                <TextField
                  size="small"
                  type="number"
                  placeholder={quotaPlaceholder}
                  value={value.quota}
                  onChange={(e) =>
                    onChange({
                      quota:
                        criterionKey === "cost"
                          ? e.target.value
                          : e.target.value === ""
                          ? ""
                          : String(Math.trunc(Number(e.target.value))),
                    })
                  }
                  disabled={disabled || !value.enabled}
                  error={!!fieldErrors?.[errorKey]}
                  helperText={fieldErrors?.[errorKey]}
                  slotProps={{ input: { inputProps: { min: quotaMin, step: quotaStep } } }}
                />
              </FormControl>
            </Grid>
            <Grid size={{ xs: 12, sm: 4 }}>
              <FormControl fullWidth size="small">
                <FormLabel>Duration</FormLabel>
                <TextField
                  size="small"
                  type="number"
                  placeholder="e.g. 1"
                  value={value.duration}
                  onChange={(e) => onChange({ duration: e.target.value })}
                  disabled={disabled || !value.enabled}
                  error={!!fieldErrors?.[durationErrorKey]}
                  helperText={fieldErrors?.[durationErrorKey]}
                  slotProps={{ input: { inputProps: { min: 1, step: 1 } } }}
                />
              </FormControl>
            </Grid>
            <Grid size={{ xs: 12, sm: 4 }}>
              <FormControl fullWidth size="small">
                <FormLabel>Unit</FormLabel>
                <Select
                  size="small"
                  value={value.unit}
                  onChange={(e) => onChange({ unit: e.target.value })}
                  disabled={disabled || !value.enabled}
                >
                  {RESET_UNITS.map((u) => (
                    <MenuItem key={u.value} value={u.value}>{u.label}</MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Grid>
          </Grid>
        </AccordionDetails>
      </Accordion>
    </Paper>
  );
}

// ─── CriteriaBlock ────────────────────────────────────────────────────────────

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
  const update = useCallback(
    (key: CriterionKey, patch: Partial<CriteriaState[CriterionKey]>) => {
      onChange({ ...criteria, [key]: { ...criteria[key], ...patch } });
    },
    [criteria, onChange],
  );

  return (
    <Stack spacing={1.5}>
      <CriterionCard
        label="Request Counts"
        quotaLabel="Quota (requests)"
        quotaPlaceholder="e.g. 1000"
        quotaMin={1}
        quotaStep={1}
        criterionKey="request"
        value={criteria.request}
        onChange={(p) => update("request", p)}
        disabled={disabled}
        fieldErrors={fieldErrors}
      />
      <CriterionCard
        label="Token Count"
        quotaLabel="Quota (tokens)"
        quotaPlaceholder="e.g. 500000"
        quotaMin={1}
        quotaStep={1}
        criterionKey="token"
        value={criteria.token}
        onChange={(p) => update("token", p)}
        disabled={disabled}
        fieldErrors={fieldErrors}
      />
      {showCost && (
        <CriterionCard
          label="Cost"
          quotaLabel="Budget (USD)"
          quotaPlaceholder="e.g. 10.00"
          quotaMin={0.000001}
          quotaStep={0.000001}
          criterionKey="cost"
          value={criteria.cost}
          onChange={(p) => update("cost", p)}
          disabled={disabled}
          fieldErrors={fieldErrors}
        />
      )}
    </Stack>
  );
}

// ─── Main component ───────────────────────────────────────────────────────────

export type LLMProviderRateLimitingTabProps = {
  providerData: LLMProviderResponse | null | undefined;
  openapiSpecUrl?: string;
  isLoading?: boolean;
  onUpdate: (fields: UpdateLLMProviderRequest) => Promise<LLMProviderResponse>;
  isUpdating: boolean;
};

export function LLMProviderRateLimitingTab({
  providerData,
  openapiSpecUrl,
  isLoading = false,
  onUpdate,
  isUpdating,
}: LLMProviderRateLimitingTabProps) {
  const [searchParams, setSearchParams] = useSearchParams();

  // Inner tab: 0 = Provider Level, 1 = Consumer Level
  const [innerTab, setInnerTab] = useState(0);

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
      setStatus({ message: "Failed to load OpenAPI spec.", severity: "error" });
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
  const [providerMode, setProviderMode] = useState<"global" | "resourceWise">(initialMode);
  const [providerGlobalCriteria, setProviderGlobalCriteria] =
    useState<CriteriaState>(DEFAULT_CRITERIA);
  const [providerResourceWiseDefault, setProviderResourceWiseDefault] =
    useState<CriteriaState>(DEFAULT_CRITERIA);
  const [providerResourceWiseMap, setProviderResourceWiseMap] =
    useState<Record<string, CriteriaState>>({});
  const [resourceSearch, setResourceSearch] = useState("");
  const [expandedResources, setExpandedResources] = useState<Set<string>>(new Set());
  const [criteriaFieldErrors, setCriteriaFieldErrors] =
    useState<Record<string, Record<string, string>>>({});

  const [consumerGlobalCriteria, setConsumerGlobalCriteria] =
    useState<CriteriaState>(DEFAULT_CRITERIA);

  const hasProviderGlobalConfig = useMemo(
    () => hasConfiguredCriteria(providerGlobalCriteria),
    [providerGlobalCriteria],
  );
  const hasProviderResourceWiseConfig = useMemo(() => {
    if (hasConfiguredCriteria(providerResourceWiseDefault)) return true;
    return Object.values(providerResourceWiseMap).some(hasConfiguredCriteria);
  }, [providerResourceWiseDefault, providerResourceWiseMap]);

  const lastSavedRef = useRef<string | null>(null);
  const [loadedAt, setLoadedAt] = useState(0);

  const getPayloadSnapshot = useCallback(() => {
    const globalLimit = limitFromCriteria(providerGlobalCriteria);
    const defaultLimit = limitFromCriteria(providerResourceWiseDefault);
    const resourcesPayload = Object.entries(providerResourceWiseMap)
      .filter(([, c]) => hasConfiguredCriteria(c))
      .map(([res, c]) => ({ resource: res, limit: limitFromCriteria(c) }))
      .sort((a, b) => a.resource.localeCompare(b.resource));
    return JSON.stringify({
      global: globalLimit,
      resourceWise: { default: defaultLimit, resources: resourcesPayload },
      consumerGlobal: limitFromCriteria(consumerGlobalCriteria),
    });
  }, [
    providerGlobalCriteria,
    providerResourceWiseDefault,
    providerResourceWiseMap,
    consumerGlobalCriteria,
  ]);

  const loadFromProvider = useCallback(() => {
    if (!providerData) return;

    const urlMode = searchParams.get("mode");
    const pl = providerData.rateLimiting?.providerLevel;
    let newMode: "global" | "resourceWise" = "global";

    if (pl?.resourceWise) {
      newMode = "resourceWise";
      setProviderMode("resourceWise");
      setProviderGlobalCriteria(DEFAULT_CRITERIA);
      setProviderResourceWiseDefault(criteriaFromLimit(pl.resourceWise.default));
      const map: Record<string, CriteriaState> = {};
      for (const r of pl.resourceWise.resources ?? []) {
        map[r.resource] = criteriaFromLimit(r.limit);
      }
      setProviderResourceWiseMap(map);
    } else if (pl?.global) {
      newMode = "global";
      setProviderMode("global");
      setProviderGlobalCriteria(criteriaFromLimit(pl.global));
      setProviderResourceWiseDefault(DEFAULT_CRITERIA);
      setProviderResourceWiseMap({});
    } else {
      setProviderMode("global");
      setProviderGlobalCriteria(DEFAULT_CRITERIA);
      setProviderResourceWiseDefault(DEFAULT_CRITERIA);
      setProviderResourceWiseMap({});
    }
    if (!urlMode) {
      setSearchParams(
        (prev) => { const next = new URLSearchParams(prev); next.set("mode", newMode); return next; },
        { replace: true },
      );
    }

    const cl = providerData.rateLimiting?.consumerLevel;
    if (cl?.global) {
      setConsumerGlobalCriteria(criteriaFromLimit(cl.global));
    } else {
      setConsumerGlobalCriteria(DEFAULT_CRITERIA);
    }

    setLoadedAt(Date.now());
  }, [providerData, searchParams, setSearchParams]);

  useEffect(() => { loadFromProvider(); }, [loadFromProvider]);

  useEffect(() => {
    const m = searchParams.get("mode");
    if (m === "global" || m === "resourceWise") setProviderMode(m);
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
    return getPayloadSnapshot() !== lastSavedRef.current;
  }, [providerData, getPayloadSnapshot]);

  const filteredResources = useMemo(() => {
    if (!resourceSearch.trim()) return resources;
    const q = resourceSearch.toLowerCase();
    return resources.filter(
      (r) =>
        getRateLimitResourceKey(r).toLowerCase().includes(q) ||
        r.path.toLowerCase().includes(q) ||
        (r.summary ?? "").toLowerCase().includes(q),
    );
  }, [resources, resourceSearch]);

  const RESOURCES_PER_PAGE = 10;
  const [resourcePage, setResourcePage] = useState(0);
  useEffect(() => { setResourcePage(0); }, [filteredResources]);

  const pagedResources = useMemo(
    () => filteredResources.slice(
      resourcePage * RESOURCES_PER_PAGE,
      (resourcePage + 1) * RESOURCES_PER_PAGE,
    ),
    [filteredResources, resourcePage],
  );

  const { allowNavigation } = useUnsavedChangesGuard(isDirty);

  const handleSave = useCallback(async () => {
    if (!providerData || isLoading) return;

    if (providerMode === "global" && hasProviderResourceWiseConfig) {
      setStatus({ message: "Provider Level cannot have both Provider-wide and Per Resource values. Remove one side and try again.", severity: "error" });
      return;
    }
    if (providerMode === "resourceWise" && hasProviderGlobalConfig) {
      setStatus({ message: "Provider Level cannot have both Provider-wide and Per Resource values. Remove one side and try again.", severity: "error" });
      return;
    }

    const criteriaToValidate: { c: CriteriaState; blockKey: string }[] = [];
    if (providerMode === "global") {
      criteriaToValidate.push({ c: providerGlobalCriteria, blockKey: "global" });
    } else {
      criteriaToValidate.push({ c: providerResourceWiseDefault, blockKey: "resourceWise-default" });
      Object.entries(providerResourceWiseMap).forEach(([res, c]) => {
        if (hasEnabledCriteria(c)) {
          criteriaToValidate.push({ c, blockKey: `resourceWise-${res}` });
        }
      });
    }
    criteriaToValidate.push({ c: consumerGlobalCriteria, blockKey: "consumer-global" });

    for (const { c, blockKey } of criteriaToValidate) {
      const errors = getCriteriaFieldErrors(c);
      if (Object.keys(errors).length > 0) {
        setCriteriaFieldErrors({ [blockKey]: errors });
        if (blockKey === "consumer-global") {
          setInnerTab(1);
        } else {
          setInnerTab(0);
          if (blockKey.startsWith("resourceWise-") && blockKey !== "resourceWise-default") {
            const resKey = blockKey.replace("resourceWise-", "");
            setExpandedResources((prev) => new Set(prev).add(resKey));
            const resIndex = filteredResources.findIndex(
              (r) => getRateLimitResourceKey(r) === resKey,
            );
            if (resIndex >= 0) setResourcePage(Math.floor(resIndex / RESOURCES_PER_PAGE));
          }
        }
        return;
      }
    }
    setCriteriaFieldErrors({});

    const buildProviderLevel = (): RateLimitingScopeConfig => {
      if (providerMode === "global") {
        return { global: limitFromCriteria(providerGlobalCriteria), resourceWise: undefined };
      }
      const resourcesPayload = Object.entries(providerResourceWiseMap)
        .filter(([, c]) => hasConfiguredCriteria(c))
        .map(([res, c]) => ({ resource: res, limit: limitFromCriteria(c) }));
      return {
        global: undefined,
        resourceWise: {
          default: limitFromCriteria(providerResourceWiseDefault),
          resources: resourcesPayload,
        },
      };
    };

    try {
      await onUpdate({
        rateLimiting: {
          providerLevel: buildProviderLevel(),
          consumerLevel: {
            global: limitFromCriteria(consumerGlobalCriteria),
            resourceWise: undefined,
          },
        },
      });
      setStatus({ message: "Rate limits updated successfully.", severity: "success" });
      setCriteriaFieldErrors({});
      lastSavedRef.current = getPayloadSnapshot();
    } catch {
      setStatus({ message: "Failed to update rate limits.", severity: "error" });
    }
  }, [
    providerData, isLoading, providerMode,
    providerGlobalCriteria, providerResourceWiseDefault, providerResourceWiseMap,
    hasProviderGlobalConfig, hasProviderResourceWiseConfig,
    consumerGlobalCriteria, onUpdate, getPayloadSnapshot, filteredResources,
  ]);

  const handleDiscard = useCallback(() => {
    loadFromProvider();
    setStatus(null);
    setCriteriaFieldErrors({});
  }, [loadFromProvider]);

  if (isLoading) {
    return (
      <Stack spacing={3}>
        <Skeleton variant="rounded" height={48} />
        <Skeleton variant="rounded" height={120} />
        <Skeleton variant="rounded" height={120} />
      </Stack>
    );
  }

  if (!providerData) return null;

  return (
    <Stack spacing={3}>
      {/* Inner tabs */}
      <Box sx={{ borderBottom: 1, borderColor: "divider" }}>
        <Tabs
          value={innerTab}
          onChange={(_, v: number) => setInnerTab(v)}
        >
          <Tab label="Provider Level" />
          <Tab label="Consumer Level" />
        </Tabs>
      </Box>

      {/* ── Provider Level tab ── */}
      {innerTab === 0 && (
        <Stack spacing={2}>
          <Typography variant="body2" color="text.secondary">
            Aggregate limits enforced on the upstream LLM provider across all consumers.
          </Typography>

          <FormControl>
            <FormLabel sx={{ mb: 1 }}>Mode</FormLabel>
            <ToggleButtonGroup
              value={providerMode}
              exclusive
              color="primary"
              onChange={(_, v: "global" | "resourceWise" | null) => {
                if (v) {
                  setProviderMode(v);
                  // ?mode= mirrors this in-form toggle, not a page change.
                  allowNavigation(() =>
                    setSearchParams(
                      (prev) => { const next = new URLSearchParams(prev); next.set("mode", v); return next; },
                      { replace: true },
                    ),
                  );
                }
              }}
              size="small"
            >
              <Tooltip
                title={
                  providerMode === "resourceWise" && (hasProviderResourceWiseConfig || isDirty)
                    ? "Remove Per Resource values first to switch to Provider-wide."
                    : ""
                }
              >
                <Box component="span">
                  <ToggleButton
                    value="global"
                    sx={{ textTransform: "none" }}
                    disabled={isDirty || (providerMode === "resourceWise" && hasProviderResourceWiseConfig)}
                  >
                    Provider-wide
                  </ToggleButton>
                </Box>
              </Tooltip>
              <Tooltip
                title={
                  providerMode === "global" && (hasProviderGlobalConfig || isDirty)
                    ? "Remove Provider-wide values first to switch to Per Resource."
                    : ""
                }
              >
                <Box component="span">
                  <ToggleButton
                    value="resourceWise"
                    sx={{ textTransform: "none" }}
                    disabled={isDirty || (providerMode === "global" && hasProviderGlobalConfig)}
                  >
                    Per Resource
                  </ToggleButton>
                </Box>
              </Tooltip>
            </ToggleButtonGroup>
          </FormControl>

          {providerMode === "global" && (
            <CriteriaBlock
              criteria={providerGlobalCriteria}
              onChange={setProviderGlobalCriteria}
              fieldErrors={criteriaFieldErrors["global"]}
            />
          )}

          {providerMode === "resourceWise" && (
            <Stack spacing={2}>
              <Paper variant="outlined" sx={{ borderRadius: 2 }}>
                <Accordion defaultExpanded disableGutters elevation={0} sx={{ background: "transparent", "&:before": { display: "none" } }}>
                  <AccordionSummary expandIcon={<ChevronDown size={18} />} sx={{ px: 2 }}>
                    <Stack spacing={0.25}>
                      <Typography variant="subtitle2">Default Resource Limit</Typography>
                      <Typography variant="caption" color="text.secondary">
                        Applied to any resource that has no specific override below.
                      </Typography>
                    </Stack>
                  </AccordionSummary>
                  <AccordionDetails sx={{ px: 2, pb: 2 }}>
                    <CriteriaBlock
                      criteria={providerResourceWiseDefault}
                      onChange={setProviderResourceWiseDefault}
                      fieldErrors={criteriaFieldErrors["resourceWise-default"]}
                    />
                  </AccordionDetails>
                </Accordion>
              </Paper>

              {/* No resources in the spec means nothing to search. */}
              {!(!specLoading && resources.length === 0) && (
                <SearchBar
                  placeholder="Search resources…"
                  value={resourceSearch}
                  onChange={(e) => setResourceSearch(e.target.value)}
                />
              )}

              {specLoading ? (
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, py: 2 }}>
                  <CircularProgress size={16} />
                  <Typography variant="body2" color="text.secondary">Loading OpenAPI spec…</Typography>
                </Box>
              ) : filteredResources.length === 0 ? (
                <ListingTable.Container>
                  <ListingTable.EmptyState
                    illustration={
                      resourceSearch.trim() ? <Search size={64} /> : <FileCode size={64} />
                    }
                    title={resourceSearch.trim() ? "No resources match your search" : "No resources found"}
                    description={
                      resourceSearch.trim()
                        ? "Try a different keyword or clear the search filter."
                        : "No resources are available from the OpenAPI spec. Add an OpenAPI specification to define resources."
                    }
                  />
                </ListingTable.Container>
              ) : (
                <Stack spacing={1}>
                  {pagedResources.map((r) => {
                    const key = getRateLimitResourceKey(r);
                    const isExpanded = expandedResources.has(key);
                    const criteria = providerResourceWiseMap[key] ?? DEFAULT_CRITERIA;
                    const hasOverride = hasConfiguredCriteria(criteria);
                    return (
                      <Paper key={key} variant="outlined" sx={{ borderRadius: 2, overflow: "hidden" }}>
                        <Accordion
                          expanded={isExpanded}
                          onChange={(_, expanded) =>
                            setExpandedResources((prev) => {
                              const next = new Set(prev);
                              if (expanded) next.add(key); else next.delete(key);
                              return next;
                            })
                          }
                          disableGutters
                          elevation={0}
                          sx={{ background: "transparent", "&:before": { display: "none" } }}
                        >
                          <AccordionSummary expandIcon={<ChevronDown size={16} />} sx={{ px: 2 }}>
                            <Stack direction="row" alignItems="center" spacing={1}>
                              <Chip
                                label={r.method}
                                size="small"
                                variant="outlined"
                                color={getMethodChipColor(r.method)}
                                sx={{ minWidth: 72, justifyContent: "center" }}
                              />
                              <Typography variant="body2">{r.path}</Typography>
                              {hasOverride && (
                                <Chip
                                  label="Custom"
                                  size="small"
                                  color="primary"
                                  sx={{ height: 18, fontSize: "0.65rem" }}
                                />
                              )}
                            </Stack>
                          </AccordionSummary>
                          {isExpanded && (
                            <AccordionDetails sx={{ px: 2, pb: 2 }}>
                              <CriteriaBlock
                                criteria={criteria}
                                onChange={(c) =>
                                  setProviderResourceWiseMap((m) => ({ ...m, [key]: c }))
                                }
                                fieldErrors={criteriaFieldErrors[`resourceWise-${key}`]}
                              />
                            </AccordionDetails>
                          )}
                        </Accordion>
                      </Paper>
                    );
                  })}
                  {filteredResources.length > RESOURCES_PER_PAGE && (
                    <TablePagination
                      component="div"
                      count={filteredResources.length}
                      page={resourcePage}
                      rowsPerPage={RESOURCES_PER_PAGE}
                      rowsPerPageOptions={[RESOURCES_PER_PAGE]}
                      onPageChange={(_e, newPage) => setResourcePage(newPage)}
                    />
                  )}
                </Stack>
              )}
            </Stack>
          )}
        </Stack>
      )}

      {/* ── Consumer Level tab ── */}
      {innerTab === 1 && (
        <Stack spacing={2}>
          <Typography variant="body2" color="text.secondary">
            Limits applied per agent (consumer). Each connected agent gets its own independent
            quota — enabling a criterion here prevents any single agent from exhausting the
            shared provider limit.
          </Typography>
          <CriteriaBlock
            criteria={consumerGlobalCriteria}
            onChange={setConsumerGlobalCriteria}
            fieldErrors={criteriaFieldErrors["consumer-global"]}
          />
        </Stack>
      )}

      {/* ── Action bar — persists across both tabs ── */}
      <Stack spacing={1.5} width="100%">
        <Collapse in={!!status && (status.severity === "error" || !isDirty)} timeout={300}>
          {status && (
            <Alert severity={status.severity} onClose={() => setStatus(null)} sx={{ width: "100%" }}>
              {status.message}
            </Alert>
          )}
        </Collapse>
        <Stack direction="row" spacing={1.5} justifyContent="flex-end">
          <Button variant="outlined" onClick={handleDiscard} disabled={!isDirty || isUpdating}>
            Discard
          </Button>
          <Button variant="contained" onClick={() => void handleSave()} disabled={!isDirty || isUpdating}>
            {isUpdating ? "Saving..." : "Save"}
          </Button>
        </Stack>
      </Stack>
    </Stack>
  );
}
