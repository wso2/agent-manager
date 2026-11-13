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
import { useFeatureFlag } from "@agent-management-platform/feature-management";
import {
  inferredEgressGateway,
  showGatewayHosting,
  type GatewayDeploymentStatus,
  type GatewayResponse,
  type LLMDeploymentStatus,
} from "@agent-management-platform/types";
import { useListGateways } from "@agent-management-platform/api-client";
import { GatewayHostingChip } from "../GatewayHostingChip";

export interface EgressGatewaySelectorProps {
  orgId: string;
  /** Gateway UUIDs currently selected (controlled); at most one. */
  value: string[];
  onChange: (gatewayIds: string[]) => void;
  /** Gateway UUIDs already deployed — rendered locked. Omit in create form. */
  lockedGatewayIds?: string[];
  /**
   * Each gateway's current deploy status, by UUID. A locked gateway whose
   * status isn't DEPLOYED is not locked, and a gateway that is deploying,
   * undeploying or failed shows that status instead of "Deployed".
   */
  gatewayStatuses?: Map<string, GatewayDeploymentStatus>;
  /** false while the selector is checked with 2+ candidates and none picked. */
  onValidityChange?: (isValid: boolean) => void;
  /**
   * Fired with true when the choice is unambiguous (a lock, or a single
   * candidate gateway); an unlocked one is auto-selected and renders nothing.
   * Callers should hide their own wrapping title/caption when true.
   */
  onSingleChoiceChange?: (isSingleChoice: boolean) => void;
  disabled?: boolean;
}

export interface EgressGatewaySelectorViewProps
  extends Omit<EgressGatewaySelectorProps, "orgId"> {
  gateways: GatewayResponse[];
  isLoading?: boolean;
}

const gatewayLabel = (gateway: GatewayResponse): string =>
  gateway.displayName || gateway.name;

const AMBIGUOUS_CAPTION = "Select an egress gateway.";
const UNAVAILABLE_CAPTION = "No egress-capable gateway is available.";
// Shown only when another candidate exists to switch to. A locked sole
// candidate offers no undeploy affordance: re-deploying to it would
// immediately auto-hide again, stranding the provider.
const DEPLOYED_CAPTION_SWITCHABLE =
  "Placement is fixed once deployed. To use a different gateway, uncheck " +
  "and save to undeploy, then select the new gateway and save again.";

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

export const EgressGatewaySelectorView: React.FC<EgressGatewaySelectorViewProps> = ({
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
  const selfHostedEnabled = useFeatureFlag("enableSelfHostedGateways");
  const showHosting = showGatewayHosting(selfHostedEnabled, gateways);
  const [pending, setPending] = useState(false);

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

  // Egress-capable only: ingress gateways are not legal LLM placement targets.
  // No status filter — the server's candidate set is not liveness-filtered.
  const candidates = useMemo(
    () =>
      gateways.filter(
        (gateway) => gateway.gatewayType === "EGRESS" || gateway.gatewayType === "BOTH",
      ),
    [gateways],
  );
  const defaultGateway = inferredEgressGateway(candidates);
  const soleCandidate = candidates.length === 1 ? candidates[0] : undefined;
  const lockedGateway = candidates.find((candidate) => lockedSet.has(candidate.uuid));
  const resolvedGateway = candidates.find((candidate) => valueSet.has(candidate.uuid));
  const checked = lockedGateway
    ? valueSet.has(lockedGateway.uuid)
    : resolvedGateway != null || pending;
  const statusGateway = lockedGateway ?? resolvedGateway ?? soleCandidate ?? defaultGateway;
  const status = statusGateway && gatewayStatuses?.get(statusGateway.uuid);
  const chip = status && statusChipFor(status);
  const statusChip = chip && { ...chip, reason: status?.statusReason };

  const unknownSelectedUuids = useMemo(() => {
    const candidateUuids = new Set(candidates.map((candidate) => candidate.uuid));
    return value.filter((uuid) => !candidateUuids.has(uuid));
  }, [candidates, value]);

  const isValid =
    !(checked && !lockedGateway && !resolvedGateway) && unknownSelectedUuids.length === 0;

  useEffect(() => {
    onValidityChange?.(isValid);
  }, [isValid, onValidityChange]);

  // Nothing to decide: one managed candidate (a lone self-hosted gateway is
  // still the user's pick), or a lock, with no stale selection to clean up.
  const isSingleChoice = Boolean(
    !isLoading &&
      unknownSelectedUuids.length === 0 &&
      (lockedGateway || (candidates.length === 1 && defaultGateway)),
  );
  // A lock or a deploy status is a fact worth showing, and hiding the row
  // would also re-select a gateway the user just unchecked.
  const hidesSingleChoice = isSingleChoice && !lockedGateway && !statusChip;

  useEffect(() => {
    onSingleChoiceChange?.(isSingleChoice);
  }, [isSingleChoice, onSingleChoiceChange]);

  const select = (gateway: GatewayResponse) => onChange([gateway.uuid]);

  // Commit the sole answer instead of requiring a click for a non-choice.
  useEffect(() => {
    if (!hidesSingleChoice || !defaultGateway || checked) return;
    select(defaultGateway);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hidesSingleChoice, defaultGateway, checked]);

  const handleToggle = () => {
    if (lockedGateway) {
      if (candidates.length <= 1) return;
      onChange(checked ? [] : [lockedGateway.uuid]);
      return;
    }
    if (checked) {
      setPending(false);
      onChange([]);
      return;
    }
    const picked = defaultGateway ?? soleCandidate;
    if (picked) select(picked);
    else setPending(true);
  };

  const handleSelect = (uuid: string) => {
    const gateway = gatewayByUuid.get(uuid);
    if (!gateway) return;
    setPending(false);
    select(gateway);
  };

  if (isLoading) {
    return <Skeleton variant="rounded" height={40} />;
  }

  if (hidesSingleChoice) return null;

  const caption = lockedGateway
    ? gatewayLabel(lockedGateway)
    : candidates.length === 1
      ? gatewayLabel(candidates[0])
      : candidates.length > 1
        ? `${candidates.length} gateways available`
        : undefined;

  const renderHostingChip = (gateway: GatewayResponse) =>
    showHosting && <GatewayHostingChip gateway={gateway} />;

  const renderBelow = () => {
    if (candidates.length === 0) {
      return (
        <Typography variant="caption" color="text.secondary">
          {UNAVAILABLE_CAPTION}
        </Typography>
      );
    }
    if (lockedGateway) {
      if (candidates.length <= 1) return null;
      return (
        <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
          {DEPLOYED_CAPTION_SWITCHABLE}
        </Typography>
      );
    }
    if (candidates.length === 1 || !checked) return null;
    return (
      <>
        <FormControl fullWidth>
          <Select
            size="small"
            value={resolvedGateway?.uuid ?? ""}
            disabled={disabled}
            onChange={(event) => handleSelect(event.target.value)}
            displayEmpty
            renderValue={(selected) =>
              selected
                ? gatewayLabel(gatewayByUuid.get(selected as string)!)
                : "Select a gateway"
            }
            SelectDisplayProps={{ "aria-label": "Egress gateway" }}
          >
            {candidates.map((candidate) => (
              <MenuItem key={candidate.uuid} value={candidate.uuid}>
                <Stack direction="row" spacing={1} alignItems="center">
                  <span>{gatewayLabel(candidate)}</span>
                  {renderHostingChip(candidate)}
                </Stack>
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        {!resolvedGateway && (
          <Typography variant="caption" color="error" sx={{ mt: 0.5 }}>
            {AMBIGUOUS_CAPTION}
          </Typography>
        )}
      </>
    );
  };

  // Width of the checkbox + its gap to the label, so content below lines up
  // under the label rather than the box.
  const CHECKBOX_INDENT = 4.5;
  const below = renderBelow();

  return (
    <Stack spacing={1.5}>
      <Stack spacing={0.5} sx={candidates.length === 0 ? { opacity: 0.5 } : undefined}>
        <Stack direction="row" spacing={1} alignItems="center">
          <Checkbox
            size="small"
            checked={checked}
            disabled={
              disabled ||
              candidates.length === 0 ||
              (Boolean(lockedGateway) && candidates.length === 1)
            }
            onChange={handleToggle}
            inputProps={{ "aria-label": "Deploy to an egress gateway" }}
            sx={{ p: 0.5 }}
          />
          <Typography variant="body2">Deploy to an egress gateway</Typography>
          {caption && (
            <Typography variant="caption" color="text.disabled">
              {caption}
            </Typography>
          )}
          {candidates.length === 1 && renderHostingChip(candidates[0])}
          {lockedGateway && candidates.length > 1 && renderHostingChip(lockedGateway)}
          {lockedGateway && (
            <Chip label="Deployed" size="small" variant="outlined" color="success" />
          )}
          {statusChip && (
            <Tooltip title={statusChip.reason ?? ""}>
              <Chip
                label={statusChip.label}
                size="small"
                variant="outlined"
                color={statusChip.color}
              />
            </Tooltip>
          )}
        </Stack>
        {below && <Box sx={{ pl: CHECKBOX_INDENT }}>{below}</Box>}
      </Stack>
      {unknownSelectedUuids.map((uuid) => {
        const gateway = gatewayByUuid.get(uuid);
        const label = gateway ? gatewayLabel(gateway) : uuid;
        return (
          <Stack key={uuid} direction="row" spacing={1} alignItems="center">
            <Checkbox
              size="small"
              checked
              disabled={disabled}
              onChange={() => onChange(value.filter((selected) => selected !== uuid))}
              inputProps={{ "aria-label": label }}
              sx={{ p: 0.5 }}
            />
            <Typography variant="body2">{label}</Typography>
            <Chip label="Unavailable" size="small" variant="outlined" />
          </Stack>
        );
      })}
    </Stack>
  );
};

export const EgressGatewaySelector: React.FC<EgressGatewaySelectorProps> = ({
  orgId,
  ...viewProps
}) => {
  // limit: 500 — without it the server's default page size silently truncates
  // the gateway list and the selector would wrongly render as unavailable.
  const { data: gatewaysData, isLoading } = useListGateways({ orgName: orgId }, { limit: 500 });
  return (
    <EgressGatewaySelectorView
      gateways={gatewaysData?.gateways ?? []}
      isLoading={isLoading}
      {...viewProps}
    />
  );
};
