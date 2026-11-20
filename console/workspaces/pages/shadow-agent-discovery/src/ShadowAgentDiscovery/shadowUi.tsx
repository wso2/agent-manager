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

import { useState } from "react";
import { Box, Chip, IconButton, Tooltip } from "@wso2/oxygen-ui";
import {
  Check,
  CircleCheck,
  CircleDashed,
  CircleX,
  Clock,
  Copy,
  TriangleAlert,
} from "@wso2/oxygen-ui-icons-react";
import type {
  ShadowCallOutcome,
  ShadowFindingKind,
  ShadowOwnerHint,
  ShadowProvider,
  ShadowReviewedBy,
  ShadowScanStatus,
} from "@agent-management-platform/types";

export const PROVIDER_LABELS: Record<ShadowProvider, string> = {
  openai: "OpenAI",
  anthropic: "Anthropic",
  bedrock: "Amazon Bedrock",
  google: "Google Agent Platform",
  foundry: "Microsoft Foundry",
  custom: "Your inventory",
  github: "GitHub",
};

/** Source repository hosts: scanned by the scan command on the customer's machine. */
export const SOURCE_HOSTS: string[] = ["github"];

/** The customer's own agent inventory API: neither an AI platform nor a source host. */
export const CUSTOM_PLATFORM = "custom";

/** The platforms whose agent listings a scan reads, in display order. */
export const AGENT_PLATFORMS: ShadowProvider[] = [
  "openai",
  "anthropic",
  "bedrock",
  "google",
  "foundry",
  "custom",
];

/** What a scan of each platform discovers, one line for the Add platform cards. */
export const DISCOVERS: Record<string, string> = {
  openai: "Assistants and agents in your OpenAI projects",
  anthropic: "Managed agents in your Anthropic workspaces",
  bedrock: "Bedrock agents and AgentCore runtimes",
  google: "Agent Runtime engines and Agent Registry agents",
  foundry: "Agents in your Foundry projects",
  custom:
    "Your own API that lists your agents. Implement the Agent Manager contract, then add its URL and an API key.",
  github: "Agents in your repositories' code",
};

export function providerLabel(p: string): string {
  return PROVIDER_LABELS[p as ShadowProvider] ?? p;
}

/**
 * The platform's name inside a sentence: "your inventory" for Bring your own
 * inventory (lowercase, so it reads naturally), the platform's label otherwise.
 */
export function providerInSentence(p: string): string {
  return p === CUSTOM_PLATFORM ? "your inventory" : providerLabel(p);
}

function countOf(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}

/** What a GitHub connection scans: "acme · All repositories · Public only". */
export function sourceScopeLabel(
  provider: string,
  settings: Record<string, unknown> | undefined,
): string {
  const s = settings ?? {};
  const listed = (v: unknown) => (Array.isArray(v) ? v.length : 0);
  const parts = [typeof s.organization === "string" ? s.organization : ""];
  if (provider === "github") {
    parts.push(
      s.allRepositories === true
        ? "All repositories"
        : countOf(listed(s.repositories), "repository", "repositories"),
      s.privateRepositories === true ? "Private and public" : "Public only",
    );
  }
  return parts.filter(Boolean).join(" · ");
}

export const KIND_LABELS: Record<ShadowFindingKind, string> = {
  platform_agent: "Agent",
  runtime: "Agent runtime",
  code_agent: "Agent in code",
};

/** What an agent is at its platform, in the platform's own words. */
const PLATFORM_KIND_LABELS: Record<string, Partial<Record<ShadowFindingKind, string>>> = {
  openai: { platform_agent: "Agent" },
  anthropic: { platform_agent: "Managed Agent" },
  bedrock: { runtime: "AgentCore runtime", platform_agent: "Bedrock agent" },
  google: { runtime: "Agent Runtime engine", platform_agent: "Agent Registry agent" },
  foundry: { platform_agent: "Foundry agent" },
  custom: { platform_agent: "Agent" },
};

/** The kind's label for the agent's platform. */
export function kindLabel(k: string, provider?: string): string {
  return (
    PLATFORM_KIND_LABELS[provider ?? ""]?.[k as ShadowFindingKind] ??
    KIND_LABELS[k as ShadowFindingKind] ??
    k
  );
}

/** Whether an agent is onboarded as an external agent. */
export function OnboardingChip({ onboarded }: { onboarded: boolean }) {
  return onboarded ? (
    <Chip
      size="small"
      color="success"
      variant="outlined"
      icon={<CircleCheck size={14} />}
      label="Onboarded"
    />
  ) : (
    <Chip size="small" variant="outlined" icon={<CircleDashed size={14} />} label="Not onboarded" />
  );
}

/**
 * Marks a code agent that AI-enhanced detection confirmed; nothing for agents
 * the rules proved.
 */
export function AiReviewChip({ reviewedBy }: { reviewedBy?: ShadowReviewedBy }) {
  if (reviewedBy !== "ai_review" && reviewedBy !== "ai_review_judge") return null;
  // describeChild keeps "AI" as the chip's accessible name.
  return (
    <Tooltip
      title={
        "Confirmed by AI-enhanced detection, not by the scan's rules. " +
        "Open the agent to check its evidence."
      }
      describeChild
    >
      <Chip size="small" variant="outlined" label="AI" />
    </Tooltip>
  );
}

/** The agent name rules of the console's Register an agent page. */
export const AGENT_NAME_PATTERN = /^[a-z][a-z0-9-]*[a-z0-9]$|^[a-z]$/;
export const AGENT_NAME_MAX_LENGTH = 25;

/** A suggested agent name from a display name: lowercase, digits and "-". */
export function suggestAgentName(displayName: string): string {
  let s = displayName
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^[^a-z]+/, "")
    .replace(/-+/g, "-");
  s = s.slice(0, AGENT_NAME_MAX_LENGTH).replace(/-+$/, "");
  return s || "agent";
}

/** A copyable code block: commands or environment variables. */
export function CodeHint({ code, label }: { code: string; label: string }) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    void navigator.clipboard
      ?.writeText(code)
      .then(() => setCopied(true))
      .catch(() => setCopied(false));
  };
  return (
    <Box
      sx={{
        position: "relative",
        border: 1,
        borderColor: "divider",
        borderRadius: 1,
        bgcolor: "action.hover",
        my: 1,
      }}
    >
      <Box
        component="pre"
        aria-label={label}
        sx={{ m: 0, p: 1.5, pr: 5, overflowX: "auto", fontSize: 12, fontFamily: "monospace" }}
      >
        {code}
      </Box>
      <Tooltip title={copied ? "Copied" : "Copy"}>
        <IconButton
          size="small"
          aria-label={`Copy ${label}`}
          sx={{ position: "absolute", top: 4, right: 4 }}
          onClick={copy}
        >
          {copied ? <Check size={16} /> : <Copy size={16} />}
        </IconButton>
      </Tooltip>
    </Box>
  );
}

const SCAN_STATUS_LABELS: Record<ShadowScanStatus, string> = {
  pending: "Pending",
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
          : status === "pending"
            ? "default"
            : "info";
  const Icon =
    status === "succeeded"
      ? CircleCheck
      : status === "failed"
        ? CircleX
        : status === "partial"
          ? TriangleAlert
          : status === "pending"
            ? Clock
            : CircleDashed;
  // Outlined, like the onboarding chip, so status chips read alike across the pages.
  return (
    <Chip
      size="small"
      variant="outlined"
      color={color}
      icon={<Icon size={14} />}
      label={SCAN_STATUS_LABELS[status] ?? status}
    />
  );
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
