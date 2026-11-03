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

import React, { useEffect, useMemo, useState } from "react";
import {
  Box,
  Checkbox,
  Chip,
  FormControl,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import type {
  Environment,
  GatewayDeploymentStatus,
  GatewayResponse,
  LLMDeploymentStatus,
} from "@agent-management-platform/types";
import {
  useListEnvironments,
  useListGateways,
} from "@agent-management-platform/api-client";

export interface EnvironmentGatewaySelectorProps {
  orgId: string;
  /** Gateway UUIDs currently selected (controlled). */
  value: string[];
  onChange: (gatewayIds: string[]) => void;
  /** Gateway UUIDs already deployed — rendered locked. Omit in create form. */
  lockedGatewayIds?: string[];
  /**
   * Each gateway's current deploy status, by UUID. A locked gateway whose
   * status isn't DEPLOYED is not locked, and a row whose gateway is deploying,
   * undeploying or failed shows that status instead of "Deployed".
   */
  gatewayStatuses?: Map<string, GatewayDeploymentStatus>;
  /** false while any checked 2+-candidate environment lacks a resolved gateway. */
  onValidityChange?: (isValid: boolean) => void;
  /**
   * Fired with true when there's exactly one environment and it's unambiguous
   * (a lock, or a single candidate gateway) — the component auto-selects it and
   * renders nothing, since there's no real choice to present. Callers should
   * hide their own wrapping title/caption for this selector when true.
   */
  onSingleChoiceChange?: (isSingleChoice: boolean) => void;
  disabled?: boolean;
}

export interface EnvironmentGatewaySelectorViewProps
  extends Omit<EnvironmentGatewaySelectorProps, "orgId"> {
  environments: Environment[];
  gateways: GatewayResponse[];
  isLoading?: boolean;
}

// A gateway belongs to exactly one environment (business rule; the wire shape
// is an array). A gateway with no mapping surfaces via the "Unmapped" row.
const environmentIdOf = (gateway: GatewayResponse): string | undefined =>
  gateway.environments?.[0]?.id;

const gatewayLabel = (gateway: GatewayResponse): string =>
  gateway.displayName || gateway.name;

const AMBIGUOUS_CAPTION = "Select an egress gateway for this environment.";
const UNAVAILABLE_CAPTION =
  "No egress-capable gateway is attached to this environment.";
// Shown only when another candidate gateway exists to switch to. A locked row
// with no other candidate offers no undeploy affordance at all (see
// isUndeployable below) — undeploying it would strand the provider with no
// visible way back in, since re-deploying to the same sole gateway would
// immediately auto-hide again.
const DEPLOYED_CAPTION_SWITCHABLE =
  "Placement is fixed once deployed. To use a different gateway, uncheck " +
  "this environment and save to undeploy, then select the new gateway and " +
  "save again.";

interface StatusChip {
  label: string;
  color: "info" | "warning";
}

// A DEPLOYED gateway is shown by its lock's "Deployed" chip instead.
const STATUS_CHIPS: Partial<Record<LLMDeploymentStatus, StatusChip>> = {
  DEPLOYING: { label: "Deploying", color: "info" },
  UNDEPLOYING: { label: "Undeploying", color: "info" },
  FAILED: { label: "Failed", color: "warning" },
};

function statusChipFor(status: GatewayDeploymentStatus): StatusChip | undefined {
  if (status.status === "FAILED" && status.statusDesired === "UNDEPLOYED") {
    return { label: "Undeploy failed", color: "warning" };
  }
  return STATUS_CHIPS[status.status];
}

interface EnvironmentRow {
  envId: string;
  label: string;
  candidates: GatewayResponse[];
  lockedGateway?: GatewayResponse;
  resolvedGateway?: GatewayResponse;
  checked: boolean;
  /** The row's gateway's deploy status, when it has a chip of its own. */
  statusChip?: StatusChip & { reason?: string };
}

export const EnvironmentGatewaySelectorView: React.FC<
  EnvironmentGatewaySelectorViewProps
> = ({
  environments,
  gateways,
  isLoading,
  value,
  onChange,
  lockedGatewayIds = [],
  gatewayStatuses,
  onValidityChange,
  onSingleChoiceChange,
  disabled,
}) => {
  const [pendingEnvIds, setPendingEnvIds] = useState<Set<string>>(new Set());

  const valueSet = useMemo(() => new Set(value), [value]);
  // A gateway whose deploy is still running or failed has no placement to
  // protect yet, so it stays editable even when the caller locks it.
  const lockedSet = useMemo(
    () =>
      new Set(
        lockedGatewayIds.filter((uuid) => {
          const status = gatewayStatuses?.get(uuid)?.status;
          return status === undefined || status === "DEPLOYED";
        }),
      ),
    [lockedGatewayIds, gatewayStatuses],
  );
  const gatewayByUuid = useMemo(
    () => new Map(gateways.map((gateway) => [gateway.uuid, gateway])),
    [gateways],
  );

  // Egress-capable only: ingress gateways are not legal LLM placement targets
  // and the server rejects them. No status filter — the server's candidate set
  // is not liveness-filtered either, so filtering here would offer a narrower
  // set than the server accepts and hide a valid choice whenever a gateway is
  // briefly disconnected.
  const gatewaysByEnv = useMemo(() => {
    const map: Record<string, GatewayResponse[]> = {};
    gateways.forEach((gateway) => {
      if (gateway.gatewayType !== "EGRESS" && gateway.gatewayType !== "BOTH") {
        return;
      }
      const envId = environmentIdOf(gateway);
      if (!envId) return;
      (map[envId] ??= []).push(gateway);
    });
    return map;
  }, [gateways]);

  const rows = useMemo<EnvironmentRow[]>(
    () =>
      environments.flatMap((env) => {
        if (!env.id) return [];
        const candidates = gatewaysByEnv[env.id] ?? [];
        const lockedGateway = candidates.find((candidate) =>
          lockedSet.has(candidate.uuid),
        );
        const resolvedGateway = candidates.find((candidate) =>
          valueSet.has(candidate.uuid),
        );
        const checked = lockedGateway
          ? valueSet.has(lockedGateway.uuid)
          : resolvedGateway != null || pendingEnvIds.has(env.id);
        // A sole candidate is named inline even while unchecked, so its
        // status still reads as belonging to it.
        const statusGateway =
          lockedGateway ??
          resolvedGateway ??
          (candidates.length === 1 ? candidates[0] : undefined);
        const status = statusGateway && gatewayStatuses?.get(statusGateway.uuid);
        const chip = status && statusChipFor(status);
        return [
          {
            envId: env.id,
            label: env.displayName || env.name,
            candidates,
            lockedGateway,
            resolvedGateway,
            checked,
            statusChip: chip && { ...chip, reason: status?.statusReason },
          },
        ];
      }),
    [
      environments,
      gatewaysByEnv,
      lockedSet,
      valueSet,
      pendingEnvIds,
      gatewayStatuses,
    ],
  );

  const unmappedSelectedUuids = useMemo(() => {
    const candidateUuids = new Set(
      rows.flatMap((row) => row.candidates.map((candidate) => candidate.uuid)),
    );
    return value.filter((uuid) => !candidateUuids.has(uuid));
  }, [rows, value]);

  const isValid =
    rows.every(
      (row) => !(row.checked && !row.lockedGateway && !row.resolvedGateway),
    ) && unmappedSelectedUuids.length === 0;

  useEffect(() => {
    onValidityChange?.(isValid);
  }, [isValid, onValidityChange]);

  // The one case where there's truly nothing to *decide*: a single
  // environment with exactly one unlocked candidate gateway, and no orphaned
  // selection left over to clean up. Any other shape (2+ environments, one
  // with an ambiguous gateway, or a stale reference to surface) is a real
  // choice and still needs to render. A locked (already deployed) single row
  // is not "nothing to choose" — it's a fact worth showing, so it still
  // renders below via the normal locked-row path.
  const singleRow = rows.length === 1 ? rows[0] : undefined;
  const isSingleChoice = Boolean(
    !isLoading &&
      singleRow &&
      unmappedSelectedUuids.length === 0 &&
      (singleRow.lockedGateway || singleRow.candidates.length === 1),
  );
  // A deploy status is a fact worth showing too, and hiding the row would
  // also re-select a gateway the user just unchecked.
  const hidesSingleChoice =
    isSingleChoice && !singleRow?.lockedGateway && !singleRow?.statusChip;

  useEffect(() => {
    onSingleChoiceChange?.(isSingleChoice);
  }, [isSingleChoice, onSingleChoiceChange]);

  const setPending = (envId: string, pending: boolean) => {
    setPendingEnvIds((prev) => {
      const next = new Set(prev);
      if (pending) next.add(envId);
      else next.delete(envId);
      return next;
    });
  };

  const emitRemove = (uuid: string) => {
    onChange(value.filter((selected) => selected !== uuid));
  };

  // Evicting any same-environment gateway before adding keeps the emitted
  // array inside the server's one-gateway-per-environment placement rule.
  const emitAdd = (gateway: GatewayResponse) => {
    const envId = environmentIdOf(gateway);
    const next = value.filter((uuid) => {
      if (uuid === gateway.uuid) return false;
      const other = gatewayByUuid.get(uuid);
      if (!other || envId === undefined) return true;
      return environmentIdOf(other) !== envId;
    });
    onChange([...next, gateway.uuid]);
  };

  // Nothing for the user to decide, so commit the sole answer on their behalf
  // instead of requiring a checkbox click for a choice that isn't really one.
  // Gated on hidesSingleChoice (unlocked only): once the row is locked, an
  // unchecked state is the user intentionally undeploying, not an unmade
  // choice, and must not be force-recommitted.
  useEffect(() => {
    if (!hidesSingleChoice || !singleRow || singleRow.checked) return;
    emitAdd(singleRow.lockedGateway ?? singleRow.candidates[0]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hidesSingleChoice, singleRow]);

  const handleToggle = (row: EnvironmentRow) => {
    if (row.lockedGateway) {
      // A sole candidate offers no undeploy affordance (see isUndeployable) —
      // its checkbox is disabled, so this only guards against a stray event.
      if (row.candidates.length <= 1) return;
      if (row.checked) emitRemove(row.lockedGateway.uuid);
      else emitAdd(row.lockedGateway);
      return;
    }
    if (row.checked) {
      setPending(row.envId, false);
      if (row.resolvedGateway) emitRemove(row.resolvedGateway.uuid);
      return;
    }
    if (row.candidates.length === 1) emitAdd(row.candidates[0]);
    else setPending(row.envId, true);
  };

  const handleSelect = (row: EnvironmentRow, uuid: string) => {
    const gateway = gatewayByUuid.get(uuid);
    if (!gateway) return;
    setPending(row.envId, false);
    emitAdd(gateway);
  };

  if (isLoading) {
    return (
      <Stack spacing={1}>
        {[0, 1, 2].map((index) => (
          <Skeleton key={index} variant="rounded" height={40} />
        ))}
      </Stack>
    );
  }

  // Auto-selected above with nothing to decide; a locked single row still
  // renders below to confirm what's deployed.
  if (hidesSingleChoice) return null;

  const selectedCount = rows.filter((row) =>
    row.candidates.some((candidate) => valueSet.has(candidate.uuid)),
  ).length;

  // The gateway name shown inline next to the environment label — only when
  // there's exactly one possible answer (a lock, or a single candidate), so
  // it reads as a fact rather than a choice still to be made.
  const inlineGatewayLabel = (row: EnvironmentRow): string | undefined => {
    if (row.lockedGateway) return gatewayLabel(row.lockedGateway);
    if (row.candidates.length === 1) return gatewayLabel(row.candidates[0]);
    return undefined;
  };

  const renderRowContent = (row: EnvironmentRow) => {
    if (row.candidates.length === 0) {
      return (
        <Typography variant="caption" color="text.secondary">
          {UNAVAILABLE_CAPTION}
        </Typography>
      );
    }
    if (row.lockedGateway) {
      // The gateway is already deployed and can't change here, so it's shown
      // as plain text inline above rather than a disabled (never-editable) select.
      // With no other candidate, there's nothing to switch to and no undeploy
      // affordance either (see isUndeployable) — this row is purely a fact.
      if (row.candidates.length <= 1) return null;
      return (
        <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
          {DEPLOYED_CAPTION_SWITCHABLE}
        </Typography>
      );
    }
    if (row.candidates.length === 1) {
      // Unambiguous — shown inline next to the environment name instead.
      return null;
    }
    // 2+ candidates: which gateway to use is a real choice, so only surface
    // the picker once the user has actually opted into this environment.
    if (!row.checked) return null;
    return (
      <>
        <FormControl fullWidth>
          <Select
            size="small"
            value={row.resolvedGateway?.uuid ?? ""}
            disabled={disabled}
            onChange={(event) => handleSelect(row, event.target.value)}
            displayEmpty
            renderValue={(selected) =>
              selected
                ? gatewayLabel(gatewayByUuid.get(selected as string)!)
                : "Select a gateway"
            }
            SelectDisplayProps={{
              "aria-label": `Egress gateway for ${row.label}`,
            }}
          >
            {row.candidates.map((candidate) => (
              <MenuItem key={candidate.uuid} value={candidate.uuid}>
                {gatewayLabel(candidate)}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        {!row.resolvedGateway && (
          <Typography variant="caption" color="error" sx={{ mt: 0.5 }}>
            {AMBIGUOUS_CAPTION}
          </Typography>
        )}
      </>
    );
  };

  // Width of the checkbox + its gap to the label, so content indented below
  // the checkbox+label line lines up under the label rather than the box.
  const CHECKBOX_INDENT = 4.5;

  return (
    <Stack spacing={1.5}>
      {rows.map((row) => {
        const belowContent = renderRowContent(row);
        return (
          <Stack
            key={row.envId}
            spacing={0.5}
            sx={row.candidates.length === 0 ? { opacity: 0.5 } : undefined}
          >
            <Stack direction="row" spacing={1} alignItems="center">
              <Checkbox
                size="small"
                checked={row.checked}
                disabled={
                  disabled ||
                  row.candidates.length === 0 ||
                  (Boolean(row.lockedGateway) && row.candidates.length === 1)
                }
                onChange={() => handleToggle(row)}
                inputProps={{ "aria-label": row.label }}
                sx={{ p: 0.5 }}
              />
              <Typography variant="body2">{row.label}</Typography>
              {inlineGatewayLabel(row) && (
                <Typography variant="caption" color="text.disabled">
                  {inlineGatewayLabel(row)}
                </Typography>
              )}
              {row.lockedGateway && (
                <Chip
                  label="Deployed"
                  size="small"
                  variant="outlined"
                  color="success"
                />
              )}
              {row.statusChip && (
                <Tooltip title={row.statusChip.reason ?? ""}>
                  <Chip
                    label={row.statusChip.label}
                    size="small"
                    variant="outlined"
                    color={row.statusChip.color}
                  />
                </Tooltip>
              )}
            </Stack>
            {belowContent && (
              <Box sx={{ pl: CHECKBOX_INDENT }}>{belowContent}</Box>
            )}
          </Stack>
        );
      })}
      {unmappedSelectedUuids.map((uuid) => {
        const gateway = gatewayByUuid.get(uuid);
        const label = gateway ? gatewayLabel(gateway) : uuid;
        return (
          <Stack
            key={uuid}
            direction="row"
            spacing={1}
            alignItems="flex-start"
          >
            <Checkbox
              size="small"
              checked
              disabled={disabled}
              onChange={() => emitRemove(uuid)}
              inputProps={{ "aria-label": label }}
              sx={{ p: 0.5 }}
            />
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Stack direction="row" spacing={1} alignItems="center">
                <Typography variant="body2">{label}</Typography>
                <Chip label="Unmapped" size="small" variant="outlined" />
              </Stack>
            </Box>
          </Stack>
        );
      })}
      {rows.length > 1 && (
        <Typography variant="caption" color="text.secondary">
          {selectedCount} of {rows.length} environments selected.
        </Typography>
      )}
    </Stack>
  );
};

export const EnvironmentGatewaySelector: React.FC<
  EnvironmentGatewaySelectorProps
> = ({ orgId, ...viewProps }) => {
  const { data: environments, isLoading: isLoadingEnvironments } =
    useListEnvironments({ orgName: orgId });
  // limit: 500 — without it the server's default page size silently truncates
  // the gateway list and environments would wrongly render as unavailable.
  const { data: gatewaysData, isLoading: isLoadingGateways } = useListGateways(
    { orgName: orgId },
    { limit: 500 },
  );
  return (
    <EnvironmentGatewaySelectorView
      environments={environments ?? []}
      gateways={gatewaysData?.gateways ?? []}
      isLoading={isLoadingEnvironments || isLoadingGateways}
      {...viewProps}
    />
  );
};
