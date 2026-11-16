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

import { Chip } from "@wso2/oxygen-ui";
import type {
  ShadowCallOutcome,
  ShadowFindingKind,
  ShadowFindingStatus,
  ShadowOwnerHint,
  ShadowProvider,
  ShadowScanStatus,
} from "@agent-management-platform/types";

export const PROVIDER_LABELS: Record<ShadowProvider, string> = {
  openai: "OpenAI",
  anthropic: "Anthropic",
  bedrock: "Amazon Bedrock",
  google: "Google Agent Platform",
  foundry: "Microsoft Foundry",
  github: "GitHub",
  azure_devops: "Azure DevOps",
};

/** Source repository hosts: connection test only until source code analysis is added. */
export const SOURCE_HOSTS: string[] = ["github", "azure_devops"];

/** The platforms whose agent listings a scan reads, in display order. */
export const AGENT_PLATFORMS: ShadowProvider[] = [
  "openai",
  "anthropic",
  "bedrock",
  "google",
  "foundry",
];

export function providerLabel(p: string): string {
  return PROVIDER_LABELS[p as ShadowProvider] ?? p;
}

export const KIND_LABELS: Record<ShadowFindingKind, string> = {
  platform_agent: "Agent",
  runtime: "Agent runtime",
};

/** What an agent is at its platform, in the platform's own words. */
const PLATFORM_KIND_LABELS: Record<string, Partial<Record<ShadowFindingKind, string>>> = {
  openai: { platform_agent: "Agent" },
  anthropic: { platform_agent: "Managed Agent" },
  bedrock: { runtime: "AgentCore runtime", platform_agent: "Bedrock agent" },
  google: { runtime: "Agent Runtime engine", platform_agent: "Agent Registry agent" },
  foundry: { platform_agent: "Foundry agent" },
};

/** The kind's label for the agent's platform. */
export function kindLabel(k: string, provider?: string): string {
  return (
    PLATFORM_KIND_LABELS[provider ?? ""]?.[k as ShadowFindingKind] ??
    KIND_LABELS[k as ShadowFindingKind] ??
    k
  );
}

export const STATUS_LABELS: Record<ShadowFindingStatus, string> = {
  new: "New",
  reviewed: "Reviewed",
  dismissed: "Dismissed",
};

export function StatusChip({ status }: { status: ShadowFindingStatus }) {
  const color = status === "reviewed" ? "success" : status === "new" ? "info" : "default";
  return <Chip size="small" color={color} label={STATUS_LABELS[status] ?? status} />;
}

const SCAN_STATUS_LABELS: Record<ShadowScanStatus, string> = {
  queued: "Queued",
  running: "Running",
  succeeded: "Succeeded",
  partial: "Partly failed",
  failed: "Failed",
};

export function ScanStatusChip({ status }: { status: ShadowScanStatus }) {
  const color =
    status === "succeeded"
      ? "success"
      : status === "failed"
        ? "error"
        : status === "partial"
          ? "warning"
          : "info";
  return <Chip size="small" color={color} label={SCAN_STATUS_LABELS[status] ?? status} />;
}

const OUTCOME_LABELS: Record<ShadowCallOutcome, string> = {
  ok: "OK",
  skipped: "Skipped",
  auth_failed: "Sign-in failed",
  permission_denied: "Permission denied",
  not_found: "Not found",
  rate_limited: "Rate limited",
  unsupported_response: "Unexpected response",
  failed: "Failed",
};

export function OutcomeChip({ outcome }: { outcome: string }) {
  const color =
    outcome === "ok"
      ? "success"
      : outcome === "skipped"
        ? "default"
        : outcome === "partial" || outcome === "rate_limited" || outcome === "not_found"
          ? "warning"
          : "error";
  const label =
    outcome === "partial"
      ? "Partly failed"
      : (OUTCOME_LABELS[outcome as ShadowCallOutcome] ?? outcome);
  return <Chip size="small" variant="outlined" color={color} label={label} />;
}

/** The best single line to name who created an agent. */
export function ownerLabel(h: ShadowOwnerHint | undefined): string {
  if (!h) return "";
  return h.email || h.name || h.username || "";
}

/** A short date, or a dash. */
export function shortDate(iso: string | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

/**
 * Selection-card styles for a CardActionArea that fills its card: content is
 * top-aligned, so titles line up across a row of cards with different text.
 */
export const selectCardSx = {
  height: "100%",
  display: "flex",
  flexDirection: "column",
  alignItems: "stretch",
  justifyContent: "flex-start",
} as const;

/** A whole number with thousands separators. */
export function formatCount(n: number | undefined): string {
  return (n ?? 0).toLocaleString();
}

/** "1 item" / "3 items", with thousands separators. */
export function itemCount(n: number | undefined): string {
  return `${formatCount(n)} ${n === 1 ? "item" : "items"}`;
}

/**
 * The readable names of what a connection check unlocks: an evidence code
 * (e.g. "openai.agent_object") becomes its catalog name; anything else is
 * shown as given.
 */
export function unlockLabels(unlocks: string[], signalNames: Map<string, string>): string[] {
  return [...new Set(unlocks.map((u) => signalNames.get(u) ?? u))];
}
