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

import type { ReactNode } from "react";
import { Box, Button, Card, Form, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Info, ServerCog } from "@wso2/oxygen-ui-icons-react";
import { IconTile, RadioMark } from "@agent-management-platform/views";
import type { CatalogRateLimitingSummary } from "@agent-management-platform/types";
import { formatRelativeTime } from "../../utils/format";

export interface LLMProviderCardProvider {
  name: string;
  template?: string;
  deployments?: Array<{ gatewayName?: string; deployedAt?: string }>;
  rateLimiting?: CatalogRateLimitingSummary;
  policies?: string[];
}

export interface LLMProviderCardProps {
  provider: LLMProviderCardProvider | null;
  templateInfo?: { displayName: string; logoUrl?: string } | null;
  /** In a list: the whole card is a button that picks this provider. */
  onSelect?: () => void;
  selected?: boolean;
  /**
   * As the chosen provider on a page: shows a Change button instead of a
   * radio. Leave both this and `onSelect` unset for a read-only card.
   */
  onChange?: () => void;
}

// A bit larger than SelectionCard's 32px tile: the provider logo is the card's
// main identifier, and the card is taller with its detail line.
const TILE = 40;
const LOGO = 24;
const GAP = 1.5;

/**
 * An LLM service provider in the Agent-type card style (icon tile, name, one
 * subtitle line), with its rate limits and guardrails underneath.
 */
export function LLMProviderCard({
  provider,
  templateInfo,
  onSelect,
  selected = false,
  onChange,
}: LLMProviderCardProps) {
  const latest = latestDeployment(provider?.deployments);
  const subtitle = [
    templateInfo?.displayName ?? provider?.template,
    latest?.deployedAt && `Deployed ${formatRelativeTime(latest.deployedAt)}`,
  ]
    .filter(Boolean)
    .join(" · ");
  const highlighted = selected || !!onChange;

  // One text column (name, subtitle, details) beside the tile, with the radio
  // or Change button centred on the whole card, like SelectionCard.
  const body = (
    <Stack direction="row" alignItems="center" spacing={GAP} sx={{ width: "100%" }}>
      <IconTile selected={highlighted} size={TILE}>
        {templateInfo?.logoUrl ? (
          <Box
            component="img"
            src={templateInfo.logoUrl}
            alt=""
            sx={{ width: LOGO, height: LOGO, borderRadius: "50%" }}
          />
        ) : (
          <ServerCog size={LOGO - 2} />
        )}
      </IconTile>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography variant="subtitle2" sx={{ fontWeight: 500 }} noWrap>
          {provider?.name ?? "Select provider"}
        </Typography>
        {subtitle && (
          <Typography variant="caption" color="text.secondary" noWrap sx={{ display: "block" }}>
            {subtitle}
          </Typography>
        )}
        {provider && (
          <Stack
            direction="row"
            alignItems="center"
            flexWrap="wrap"
            useFlexGap
            columnGap={2}
            rowGap={0.25}
            sx={{ mt: 0.5 }}
          >
            <RateLimitDisplay rateLimiting={provider.rateLimiting} />
            {/* Inside the list's button card a focusable child isn't allowed. */}
            <GuardrailsLine policies={provider.policies} focusable={!onSelect || !!onChange} />
          </Stack>
        )}
      </Box>
      {onChange ? (
        <Button size="small" variant="outlined" onClick={onChange}>
          Change provider
        </Button>
      ) : (
        onSelect && <RadioMark selected={selected} />
      )}
    </Stack>
  );

  // CardContent adds extra bottom padding to its last child; keep it even.
  const content = (
    <Form.CardContent sx={{ width: "100%", py: 1.5, px: 2, "&:last-child": { pb: 1.5 } }}>
      {body}
    </Form.CardContent>
  );

  // A Change button can't sit inside a button, so the chosen-provider card is a
  // plain card. List options pick and close the drawer at once, so they are
  // plain buttons (not radios); the mark only shows which one is current.
  if (onChange || !onSelect) {
    return (
      <Card variant="outlined" sx={highlighted ? { borderColor: "primary.main" } : undefined}>
        {content}
      </Card>
    );
  }
  return (
    <Form.CardButton
      aria-label={`${provider?.name ?? "Select provider"}${selected ? ", current provider" : ""}`}
      selected={selected}
      onClick={onSelect}
    >
      {content}
    </Form.CardButton>
  );
}

function latestDeployment(deployments: LLMProviderCardProvider["deployments"]) {
  if (!deployments?.length) return null;
  return [...deployments].sort(
    (a, b) => new Date(b.deployedAt ?? 0).getTime() - new Date(a.deployedAt ?? 0).getTime(),
  )[0];
}

function DetailLine({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Typography variant="caption" color="text.secondary" component="div">
      {label}: {children}
    </Typography>
  );
}

function GuardrailsLine({ policies, focusable }: { policies?: string[]; focusable: boolean }) {
  return (
    <DetailLine label="Guardrails">
      {policies?.length ? (
        <Tooltip title={policies.join(", ")} placement="top" arrow>
          {/* Focusable so keyboard users can open the full list, as hover does. */}
          <Typography
            component="span"
            variant="caption"
            color="text.primary"
            tabIndex={focusable && policies.length > 3 ? 0 : undefined}
          >
            {policies.slice(0, 3).join(", ")}
            {policies.length > 3 && ` +${policies.length - 3} more`}
          </Typography>
        </Tooltip>
      ) : (
        <Typography component="span" variant="caption" color="text.disabled">
          None
        </Typography>
      )}
    </DetailLine>
  );
}

function formatCost(amount: number): string {
  return amount < 0.01 ? `$${amount.toFixed(6)}` : `$${amount.toFixed(2)}`;
}

function formatResetWindow(duration?: number, unit?: string): string {
  if (!unit) return "";
  const abbrev: Record<string, string> = { minute: "min", hour: "hr", day: "day" };
  const u = abbrev[unit.toLowerCase()] ?? unit;
  return duration && duration !== 1 ? `${duration} ${u}` : u;
}

type Limit = { limit: number; resetDuration?: number; resetUnit?: string } | undefined;

const METRICS = [
  ["Requests", "request", (n: number) => n.toLocaleString()],
  ["Tokens", "token", (n: number) => n.toLocaleString()],
  ["Budget", "cost", formatCost],
] as const;

/**
 * The consumer's quota. Both scopes are enforced, so each metric shows the
 * consumer's own limit when set, otherwise the enabled org-wide provider limit.
 */
function RateLimitDisplay({ rateLimiting }: { rateLimiting?: CatalogRateLimitingSummary }) {
  if (!rateLimiting) {
    return (
      <DetailLine label="Rate limiting">
        <Typography component="span" variant="caption" color="text.disabled">
          Not configured
        </Typography>
      </DetailLine>
    );
  }
  const cl = rateLimiting.consumerLevel?.globalEnabled ? rateLimiting.consumerLevel : undefined;
  const pl = rateLimiting.providerLevel?.globalEnabled ? rateLimiting.providerLevel : undefined;

  if (!cl && !pl) {
    return (
      <DetailLine label="Rate limiting">
        <Typography component="span" variant="caption" color="text.disabled">
          Configured (disabled)
        </Typography>
      </DetailLine>
    );
  }

  const fmt = (l: NonNullable<Limit>, value: (n: number) => string) => {
    const w = formatResetWindow(l.resetDuration, l.resetUnit);
    return `${value(l.limit)}${w ? `/${w}` : ""}`;
  };
  const limits = METRICS.flatMap(([label, key, value]) => {
    const own: Limit = cl?.[key];
    const orgWide: Limit = pl?.[key];
    if (own) return [{ label, text: fmt(own, value), orgWide: false }];
    if (orgWide) return [{ label, text: fmt(orgWide, value), orgWide: true }];
    return [];
  });
  // All shown limits org-wide (or none set with only the provider scope on): say so once.
  const isOrgWide = limits.length ? limits.every((l) => l.orgWide) : !cl;
  const mixed = !isOrgWide && limits.some((l) => l.orgWide);

  return (
    <Stack direction="row" spacing={0.5} alignItems="center" flexWrap="wrap" useFlexGap>
      <Typography variant="caption" color="text.secondary">
        {isOrgWide ? "Your quota (org-wide limit):" : "Your quota:"}
      </Typography>
      {(isOrgWide || mixed) && (
        <Tooltip
          title={
            isOrgWide
              ? "No per-consumer limit is configured. The org-wide provider limit applies to all consumers."
              : "Limits marked org-wide come from the provider and apply to all consumers."
          }
          placement="top"
          arrow
        >
          <Box component="span" sx={{ display: "inline-flex", color: "text.secondary" }}>
            <Info size={12} />
          </Box>
        </Tooltip>
      )}
      <Typography
        variant="caption"
        color={isOrgWide ? "text.secondary" : "text.primary"}
        sx={{ fontVariantNumeric: "tabular-nums" }}
      >
        {limits.length
          ? limits
            .map((l) => `${l.label}: ${l.text}${mixed && l.orgWide ? " (org-wide)" : ""}`)
            .join(" · ")
          : "Enabled (no numeric limits set)"}
      </Typography>
    </Stack>
  );
}
