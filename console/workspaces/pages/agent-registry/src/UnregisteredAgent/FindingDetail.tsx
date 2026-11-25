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

import { useId, useState, type ReactNode } from "react";
import {
  Alert,
  Box,
  Breadcrumbs,
  Button,
  Chip,
  Collapse,
  Divider,
  Grid,
  IconButton,
  Link,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Check,
  ChevronDown,
  CircleCheck,
  Copy,
  ExternalLink,
  SearchX,
  TriangleAlert,
  UserPlus,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link as RouterLink, useParams } from "react-router-dom";
import { PageLayout, StateNotice, useReturnTo } from "@agent-management-platform/views";
import { useShadowFinding } from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type ShadowFindingDetail,
  type ShadowFindingSignal,
  type ShadowReviewedBy,
} from "@agent-management-platform/types";
import {
  CUSTOM_PLATFORM,
  kindLabel,
  OnboardingChip,
  ownerLabel,
  providerInSentence,
  providerLabel,
  shortDate,
} from "@agent-management-platform/shadow-agent-discovery";
import { CodeAgentOnboardActions } from "./OnboardActions";
import {
  ClampedText,
  Property,
  PropertiesSidebar,
  SectionCard,
  SidebarHeading,
  TimeAgo,
} from "../AgentPage/AgentPageParts";

/** A related agent's relation, as a short label. */
export function relationLabel(relation: string): string {
  return relation === "possible_duplicate" ? "Possible duplicate" : relation.replace(/_/g, " ");
}

/** Labels of the attributes an agent listing reports (the service's allow-list). */
const ATTRIBUTE_LABELS: Record<string, string> = {
  model: "Model",
  framework: "Framework",
  language: "Language",
  branch: "Branch",
  projectPath: "Folder",
  agentKind: "Agent kind",
  agentVersion: "Version",
  orchestrationType: "Orchestration",
  serverProtocol: "Server protocol",
  protocolType: "Protocols",
  recordType: "Registry record type",
  tools: "Tools",
  toolCount: "Tool count",
  mcpServers: "MCP servers",
  skills: "Skills",
  image: "Container image",
  repositoryUrl: "Source repository",
  endpoint: "Project endpoint",
  region: "Region",
  location: "Location",
  accountId: "Cloud account",
  accountKind: "Account kind",
  resourceId: "Resource ID",
  roleArn: "Execution role",
  serviceAccount: "Service account",
  project: "Project",
  workspace: "Workspace",
  deployment: "Deployment",
  archived: "Archived",
  state: "State",
  status: "Status",
  labels: "Labels",
  otelAgentId: "Telemetry agent ID",
  description: "Description",
  displayName: "Display name",
  runtimeReference: "Runtime reference",
  source: "Runs on",
  agentUrl: "Agent URL",
};

/** The label of an attribute; an unknown key becomes words ("fooBar" is "Foo bar"). */
export function attributeLabel(key: string): string {
  if (ATTRIBUTE_LABELS[key]) return ATTRIBUTE_LABELS[key];
  const words = key.replace(/([a-z0-9])([A-Z])/g, "$1 $2").replace(/_/g, " ").toLowerCase();
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/** Shown first in What it uses, in this order; the other attributes follow. */
const LEADING_ATTRIBUTES = ["framework", "model", "language", "branch", "projectPath"];
/** Attributes that are lists, shown as chips. */
const CHIP_ATTRIBUTES = ["tools", "mcpServers", "skills", "protocolType", "labels"];
/** Long identifiers and URLs, shown under Technical IDs in monospace. */
const ID_ATTRIBUTES = [
  "resourceId",
  "roleArn",
  "runtimeReference",
  "endpoint",
  "image",
  "repositoryUrl",
  "agentUrl",
  "serviceAccount",
  "otelAgentId",
];
/** Shown elsewhere on the page, not in What it uses. */
const NOT_IN_DETAILS = ["description", "reviewedBy"];

export const AI_REVIEW_WARNING = "AI review can make mistakes, so check the evidence.";
const MERGED_HINT = "Results from these connections were merged into one agent.";

function isEmptyValue(v: unknown): boolean {
  if (v === null || v === undefined || v === "") return true;
  if (Array.isArray(v)) return v.length === 0;
  return typeof v === "object" && Object.keys(v as object).length === 0;
}

function scalarText(v: unknown): string {
  if (v === null || v === undefined) return "";
  if (typeof v === "boolean") return v ? "Yes" : "No";
  if (Array.isArray(v)) return v.map(scalarText).join(", ");
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

/** A list attribute as chip labels: a tool's type and name, an MCP server's label and origin. */
export function chipLabels(v: unknown): string[] {
  if (Array.isArray(v)) {
    return v
      .map((x) => {
        if (typeof x !== "object" || x === null) return String(x);
        const o = x as {
          type?: string;
          name?: string;
          label?: string;
          origin?: string;
        };
        if (o.label !== undefined) return o.origin ? `${o.label} · ${o.origin}` : o.label;
        return [o.type, o.name].filter(Boolean).join(" · ");
      })
      .filter(Boolean);
  }
  if (typeof v === "object" && v !== null) {
    return Object.entries(v as Record<string, unknown>).map(([k, x]) => `${k}: ${String(x)}`);
  }
  return v === undefined || v === null || v === "" ? [] : [String(v)];
}

/** Where a code agent is: `<repository>:<path>:<qualified name>`, maybe with `@N` for a repeat. */
export interface CodeLocation {
  repository: string;
  path: string;
  symbol: string;
  /** The N of an `@N` suffix: the same qualified name found again in the file. */
  occurrence?: number;
}

/** The runner caps a long location to a prefix, `#` and 12 hex characters of its hash. */
const CAPPED_ID = /#[0-9a-f]{12}$/;

export function parseCodeLocation(externalId: string): CodeLocation {
  const parts = externalId.split(":");
  const repository = parts[0] ?? "";
  // A capped location is cut mid-path, so only its repository can be shown.
  if (CAPPED_ID.test(externalId)) return { repository, path: "", symbol: "" };
  if (parts.length < 3) return { repository, path: parts.slice(1).join(":"), symbol: "" };
  const last = parts[parts.length - 1] ?? "";
  const repeat = /^(.+)@(\d+)$/.exec(last);
  return {
    repository,
    path: parts.slice(1, -1).join(":"),
    symbol: repeat ? (repeat[1] ?? last) : last,
    ...(repeat ? { occurrence: Number(repeat[2]) } : {}),
  };
}

/** Who confirmed a code agent; agents published before AI review existed are rule-proven. */
function reviewedByOf(f: ShadowFindingDetail): ShadowReviewedBy {
  const v = f.reviewedBy ?? f.attributes?.reviewedBy;
  return v === "ai_review" || v === "ai_review_judge" ? v : "rules";
}

export function confirmedByText(reviewedBy: ShadowReviewedBy): string {
  if (reviewedBy === "ai_review_judge") return "Confirmed by AI review, checked by a second model";
  if (reviewedBy === "ai_review") return "Confirmed by AI review";
  return "Confirmed by detection rules";
}

/** The platform's listing, inside a sentence: "the OpenAI API", "your inventory". */
function listingInSentence(provider: string): string {
  return provider === CUSTOM_PLATFORM
    ? providerInSentence(provider)
    : `the ${providerLabel(provider)} API`;
}

/** Whether no connection's latest scan found the agent any more. */
function isGone(f: ShadowFindingDetail): boolean {
  const sources = f.foundBy ?? [];
  return sources.length > 0 ? !sources.some((s) => s.present) : !f.present;
}

function CopyButton({ text, label }: { text: string; label: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <Tooltip title={copied ? "Copied" : "Copy"}>
      <IconButton
        size="small"
        aria-label={`Copy ${label}`}
        onClick={() => {
          void navigator.clipboard
            ?.writeText(text)
            .then(() => setCopied(true))
            .catch(() => setCopied(false));
        }}
      >
        {copied ? <Check size={14} /> : <Copy size={14} />}
      </IconButton>
    </Tooltip>
  );
}

/** One line of text cut to `maxWidth`, with the whole text in a tooltip. */
function Ellipsized({
  text,
  maxWidth,
  mono = false,
}: {
  text: string;
  maxWidth: Record<string, number>;
  mono?: boolean;
}) {
  return (
    <Tooltip title={text}>
      <Typography
        component="span"
        variant="body2"
        color="text.secondary"
        noWrap
        sx={{
          display: "block",
          maxWidth,
          ...(mono ? { fontFamily: "monospace", fontSize: 12 } : {}),
        }}
      >
        {text}
      </Typography>
    </Tooltip>
  );
}

function Dot() {
  return (
    <Typography component="span" variant="body2" color="text.disabled" aria-hidden>
      ·
    </Typography>
  );
}

/** The line under the title: what and where the agent is. */
function HeaderLine({ f }: { f: ShadowFindingDetail }) {
  const isCode = f.kind === "code_agent";
  const platform = providerLabel(f.provider);
  const loc = parseCodeLocation(f.externalId);
  const crumbs = [
    { text: loc.repository || f.scope, maxWidth: { xs: 120, sm: 220 }, mono: false },
    { text: loc.path, maxWidth: { xs: 140, sm: 320 }, mono: true },
    { text: loc.symbol, maxWidth: { xs: 120, sm: 220 }, mono: true },
  ].filter((c) => c.text);
  return (
    <Stack
      direction="row"
      alignItems="center"
      columnGap={1}
      rowGap={0.5}
      flexWrap="wrap"
      sx={{ minWidth: 0 }}
    >
      <Typography component="span" variant="body2" color="text.secondary">
        {isCode ? `Code agent on ${platform}` : `${kindLabel(f.kind, f.provider)} on ${platform}`}
      </Typography>
      {isCode && crumbs.length > 0 && (
        <>
          <Dot />
          <Stack direction="row" alignItems="center" spacing={0.5} sx={{ minWidth: 0 }}>
            <Breadcrumbs
              aria-label="Location in code"
              separator="›"
              sx={{ typography: "body2", color: "text.secondary", minWidth: 0 }}
            >
              {crumbs.map((c, i) => (
                <Ellipsized
                  key={`${i}|${c.text}`}
                  text={c.text}
                  maxWidth={c.maxWidth}
                  mono={c.mono}
                />
              ))}
            </Breadcrumbs>
            <CopyButton text={f.externalId} label="location" />
          </Stack>
        </>
      )}
      {!isCode && f.scope && (
        <>
          <Dot />
          <Ellipsized text={f.scope} maxWidth={{ xs: 200, sm: 360 }} />
        </>
      )}
    </Stack>
  );
}

/**
 * A "Show more" toggle: a text link with a chevron that opens its content below. A plain
 * link, not an Accordion, so the theme's tinted accordion header never shows, and the
 * text lines up with the card's own padding.
 */
function Disclosure({
  label,
  heading = false,
  children,
}: {
  label: string;
  /** A section heading (bold, full width, chevron on the right) instead of a link. */
  heading?: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const id = useId();
  const chevron = (
    <Box
      component="span"
      aria-hidden
      sx={{
        display: "inline-flex",
        transition: "transform 150ms",
        transform: open ? "rotate(180deg)" : "none",
      }}
    >
      <ChevronDown size={16} />
    </Box>
  );
  return (
    <Box>
      <Link
        component="button"
        type="button"
        underline={heading ? "none" : "hover"}
        color={heading ? "inherit" : "primary"}
        aria-expanded={open}
        aria-controls={id}
        onClick={() => setOpen((v) => !v)}
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 0.5,
          width: heading ? "100%" : "auto",
          justifyContent: heading ? "space-between" : "flex-start",
          typography: heading ? "subtitle2" : "body2",
          textAlign: "left",
        }}
      >
        {label}
        {chevron}
      </Link>
      <Collapse in={open} unmountOnExit>
        <Box id={id} sx={{ pt: 1.5 }}>
          {children}
        </Box>
      </Collapse>
    </Box>
  );
}

/**
 * Where one signal matched, in one grey line: the file for a code agent. The observed
 * values ("Yes", "4") mean little without their names, so they stay in Show matched code.
 */
function signalSummary(s: ShadowFindingSignal, isCode: boolean): string {
  if (!isCode) return s.source ? `Returned by ${s.source}` : "";
  return s.source ? `In ${s.source}` : "";
}

function ConfirmedBy({ f }: { f: ShadowFindingDetail }) {
  if (f.kind !== "code_agent") {
    return (
      <Typography variant="body2" color="text.secondary">
        Listed by {listingInSentence(f.provider)}.
      </Typography>
    );
  }
  const reviewedBy = reviewedByOf(f);
  return (
    <Box>
      <Typography variant="body2" color="text.secondary">
        {confirmedByText(reviewedBy)}
      </Typography>
      {reviewedBy !== "rules" && (
        <Stack direction="row" spacing={0.5} alignItems="center" sx={{ mt: 0.5 }}>
          <Box sx={{ display: "inline-flex", color: "warning.main" }} aria-hidden>
            <TriangleAlert size={14} />
          </Box>
          <Typography variant="caption" color="text.secondary">
            {AI_REVIEW_WARNING}
          </Typography>
        </Stack>
      )}
    </Box>
  );
}

/** The raw evidence: each signal's code, exact condition, source and observed values. */
function MatchedDetails({ signals }: { signals: ShadowFindingSignal[] }) {
  // One quiet panel, padded on every side, so the raw match reads as a single block.
  return (
    <Stack
      spacing={2}
      divider={<Divider flexItem />}
      sx={{ bgcolor: "action.hover", borderRadius: 1, p: 2 }}
    >
      {signals.map((s) => {
        const observed = Object.entries(s.observed ?? {}).filter(([, v]) => !isEmptyValue(v));
        return (
          <Box key={`${s.code}|${s.source}`} sx={{ minWidth: 0 }}>
            <Stack
              direction={{ xs: "column", sm: "row" }}
              spacing={1}
              justifyContent="space-between"
            >
              <Typography variant="caption" sx={{ fontFamily: "monospace" }}>
                {s.code}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                Seen {shortDate(s.detectedAt)}
              </Typography>
            </Stack>
            {s.condition && (
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                {s.condition}
              </Typography>
            )}
            {s.source && (
              <Typography
                variant="caption"
                component="p"
                sx={{ fontFamily: "monospace", wordBreak: "break-all", mt: 0.5 }}
              >
                {s.source}
              </Typography>
            )}
            {observed.length > 0 && (
              <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap" sx={{ mt: 1 }}>
                {observed.map(([k, v]) => (
                  <Chip key={k} size="small" label={`${k}: ${scalarText(v)}`} />
                ))}
              </Stack>
            )}
          </Box>
        );
      })}
    </Stack>
  );
}

/** Why this is an agent: one plain row per signal, who confirmed it, and the raw match. */
function EvidenceCard({ f }: { f: ShadowFindingDetail }) {
  const isCode = f.kind === "code_agent";
  return (
    <SectionCard title="Why this is an agent">
      {f.signals.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          No evidence recorded.
        </Typography>
      ) : (
        <Stack spacing={1.5}>
          {f.signals.map((s) => {
            const summary = signalSummary(s, isCode);
            return (
              <Stack key={`${s.code}|${s.source}`} direction="row" spacing={1.25}>
                <Box sx={{ color: "success.main", display: "flex", pt: 0.25 }} aria-hidden>
                  <CircleCheck size={16} />
                </Box>
                <Box sx={{ minWidth: 0 }}>
                  <Typography variant="body2">{s.name || s.code}</Typography>
                  {summary && (
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      component="p"
                      sx={{ wordBreak: "break-word" }}
                    >
                      {summary}
                    </Typography>
                  )}
                </Box>
              </Stack>
            );
          })}
        </Stack>
      )}
      <Divider sx={{ my: 2 }} />
      <ConfirmedBy f={f} />
      {f.signals.length > 0 && (
        <Box sx={{ mt: 1 }}>
          <Disclosure label={isCode ? "Show matched code" : "Show listing details"}>
            <MatchedDetails signals={f.signals} />
          </Disclosure>
        </Box>
      )}
    </SectionCard>
  );
}

/** One attribute: a small label above its value. */
function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Box sx={{ minWidth: 0 }}>
      <Typography variant="caption" color="text.secondary" component="p">
        {label}
      </Typography>
      <Typography variant="body2" component="div" sx={{ wordBreak: "break-word", mt: 0.25 }}>
        {children}
      </Typography>
    </Box>
  );
}

/** projectPath adds nothing when the agent's file is already inside it. */
function redundantProjectPath(f: ShadowFindingDetail): boolean {
  const v = f.attributes?.projectPath;
  if (typeof v !== "string" || f.kind !== "code_agent") return false;
  const folder = v.replace(/\\/g, "/").replace(/^\/+|\/+$/g, "");
  const path = parseCodeLocation(f.externalId).path.replace(/^\/+/, "");
  return folder === "" || path === folder || path.startsWith(`${folder}/`);
}

/** What the agent is built with and uses: the reported attributes, by their labels. */
function UsesCard({ f }: { f: ShadowFindingDetail }) {
  const isCode = f.kind === "code_agent";
  const attrs = f.attributes ?? {};
  const dropProjectPath = redundantProjectPath(f);
  const shown = Object.keys(attrs).filter(
    (k) =>
      !isEmptyValue(attrs[k]) &&
      !ID_ATTRIBUTES.includes(k) &&
      !NOT_IN_DETAILS.includes(k) &&
      !(k === "projectPath" && dropProjectPath),
  );
  const rank = (k: string) => {
    const i = LEADING_ATTRIBUTES.indexOf(k);
    return i < 0 ? LEADING_ATTRIBUTES.length : i;
  };
  const ordered = [...shown].sort((a, b) => rank(a) - rank(b));
  const lists = ordered.filter((k) => CHIP_ATTRIBUTES.includes(k) && chipLabels(attrs[k]).length);
  const facts = ordered.filter((k) => !CHIP_ATTRIBUTES.includes(k) && scalarText(attrs[k]));
  const description = typeof attrs.description === "string" ? attrs.description.trim() : "";
  return (
    <SectionCard
      title="What it uses"
      hint={
        isCode
          ? "Reported by the scan command. Source code, prompts and secrets are never stored."
          : `Reported by ${providerInSentence(f.provider)}. Instructions, prompts, tool ` +
            "definitions and secrets are never stored."
      }
    >
      <Stack spacing={2.5}>
        {description && <ClampedText text={description} />}
        {facts.length > 0 && (
          <Grid container spacing={2.5}>
            {facts.map((k) => (
              <Grid key={k} size={{ xs: 12, sm: 6, lg: 4 }}>
                <Fact label={attributeLabel(k)}>{scalarText(attrs[k])}</Fact>
              </Grid>
            ))}
          </Grid>
        )}
        {lists.map((k) => (
          <Fact key={k} label={attributeLabel(k)}>
            <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap" sx={{ mt: 0.5 }}>
              {chipLabels(attrs[k]).map((label) => (
                <Chip key={label} size="small" variant="outlined" label={label} />
              ))}
            </Stack>
          </Fact>
        ))}
        {!description && facts.length === 0 && lists.length === 0 && (
          <Typography variant="body2" color="text.secondary">
            Nothing more was reported about this agent.
          </Typography>
        )}
      </Stack>
    </SectionCard>
  );
}

function StatusSection({ f, agentPath }: { f: ShadowFindingDetail; agentPath: string }) {
  const isCode = f.kind === "code_agent";
  const owner = ownerLabel(f.ownerHint);
  const linked =
    [f.linkedProject, f.linkedAgent].filter(Boolean).join(" / ") || "Unknown agent";
  return (
    <Box>
      <SidebarHeading title="Status" />
      <Stack spacing={1.25}>
        {f.onboarded ? (
          <>
            <Property label="Onboarded as">
              {agentPath ? (
                <Link component={RouterLink} to={agentPath}>
                  {linked}
                </Link>
              ) : (
                linked
              )}
            </Property>
            {f.onboardedAt && (
              <Property label="Onboarded">
                <TimeAgo iso={f.onboardedAt} />
              </Property>
            )}
          </>
        ) : (
          <Property label="Onboarding">Not onboarded</Property>
        )}
        {!isCode && f.state && <Property label="State">{f.state}</Property>}
        {owner && (
          <Property label="Created by">
            {owner}
            {f.ownerHint?.source && (
              <Typography variant="caption" color="text.secondary" component="p">
                From {f.ownerHint.source}
              </Typography>
            )}
          </Property>
        )}
        {!isCode && f.providerCreatedAt && (
          <Property label="Created at the platform">
            <TimeAgo iso={f.providerCreatedAt} />
          </Property>
        )}
        <Property label="Last found">
          <TimeAgo iso={f.lastSeenAt} />
        </Property>
        <Property label="First found">
          <TimeAgo iso={f.firstSeenAt} />
        </Property>
      </Stack>
    </Box>
  );
}

function SourcesSection({ f }: { f: ShadowFindingDetail }) {
  const isCode = f.kind === "code_agent";
  const sources =
    f.foundBy && f.foundBy.length > 0
      ? f.foundBy
      : [
          {
            findingId: f.id,
            connectionId: f.connectionId,
            connectionName: f.connectionName,
            present: f.present,
          },
        ];
  const n = sources.length;
  return (
    <Box>
      <SidebarHeading
        title={`Found by ${n} ${n === 1 ? "connection" : "connections"}`}
        hint={n > 1 ? MERGED_HINT : undefined}
      />
      <Stack spacing={1}>
        {sources.map((src) => (
          <Stack key={src.findingId} direction="row" spacing={1} alignItems="center">
            <Box
              aria-hidden
              sx={{
                width: 8,
                height: 8,
                borderRadius: "50%",
                flexShrink: 0,
                bgcolor: src.present ? "success.main" : "warning.main",
              }}
            />
            <Typography variant="body2" noWrap sx={{ minWidth: 0, flex: 1 }}>
              {src.connectionName}
            </Typography>
            <Typography variant="caption" color="text.secondary" sx={{ flexShrink: 0 }}>
              {src.present ? (isCode ? "Found" : "Listed") : "Not in latest scan"}
            </Typography>
          </Stack>
        ))}
      </Stack>
    </Box>
  );
}

function DuplicatesSection({ f, orgName }: { f: ShadowFindingDetail; orgName: string }) {
  return (
    <Box>
      <SidebarHeading title="Might be the same agent" />
      <Stack spacing={1.25}>
        {f.related.map((r) => (
          <Box key={`${r.findingId}|${r.relation}`} sx={{ minWidth: 0 }}>
            <Typography variant="body2" sx={{ wordBreak: "break-word" }}>
              <Link
                component={RouterLink}
                to={generatePath(
                  absoluteRouteMap.children.org.children.agentRegistry.children.unregisteredAgent
                    .path,
                  { orgId: orgName, findingId: r.findingId },
                )}
              >
                {r.displayName || r.findingId}
              </Link>
            </Typography>
            <Typography variant="caption" color="text.secondary" component="p">
              {[relationLabel(r.relation), r.provider && providerLabel(r.provider), r.reason]
                .filter(Boolean)
                .join(" · ")}
            </Typography>
          </Box>
        ))}
      </Stack>
    </Box>
  );
}

/** The platform ID or code location and the long identifiers, each with a copy button. */
function TechnicalIds({ f }: { f: ShadowFindingDetail }) {
  const isCode = f.kind === "code_agent";
  const ids: [string, string][] = [
    [isCode ? "Location in code" : "Platform ID", f.externalId],
    ...Object.entries(f.attributes ?? {})
      .filter(([k, v]) => ID_ATTRIBUTES.includes(k) && !isEmptyValue(v))
      .map(([k, v]): [string, string] => [attributeLabel(k), scalarText(v)]),
  ];
  return (
    <Disclosure label="Technical IDs" heading>
      <Stack spacing={1.5} divider={<Divider flexItem />}>
        {ids.map(([label, value]) => (
          <Stack key={label} direction="row" spacing={1} alignItems="flex-start">
            <Box sx={{ minWidth: 0, flex: 1 }}>
              <Typography variant="caption" color="text.secondary" component="p">
                {label}
              </Typography>
              <Typography
                variant="caption"
                component="p"
                sx={{ fontFamily: "monospace", wordBreak: "break-all", mt: 0.25 }}
              >
                {value}
              </Typography>
            </Box>
            <CopyButton text={value} label={label} />
          </Stack>
        ))}
      </Stack>
    </Disclosure>
  );
}

function Sidebar({
  f,
  orgName,
  agentPath,
}: {
  f: ShadowFindingDetail;
  orgName: string;
  agentPath: string;
}) {
  return (
    <PropertiesSidebar>
      <StatusSection f={f} agentPath={agentPath} />
      <SourcesSection f={f} />
      {f.related.length > 0 && <DuplicatesSection f={f} orgName={orgName} />}
      <TechnicalIds f={f} />
    </PropertiesSidebar>
  );
}

/**
 * One unregistered agent: why it is an agent and what it uses, with its status,
 * connections and IDs beside them, and onboarding it (a code agent can also be
 * deployed as a platform-hosted agent).
 */
export function FindingDetail({
  orgName,
  canManage,
  listPath,
}: {
  orgName: string | undefined;
  canManage: boolean;
  listPath: string;
}) {
  const { findingId } = useParams<{ findingId: string }>();
  const returnTo = useReturnTo(listPath);
  const { data: f, isLoading, error } = useShadowFinding(orgName, findingId);
  const back = { backHref: returnTo, backLabel: "Back" };

  if (isLoading) {
    return (
      <PageLayout title="Agent" disableIcon {...back}>
        <Skeleton variant="rounded" height={360} />
      </PageLayout>
    );
  }
  if (error || !f) {
    return (
      <PageLayout title="Agent" disableIcon {...back}>
        <StateNotice
          icon={SearchX}
          tone="error"
          title="Couldn't load this agent"
          description="It may have been deleted with its platform. Go back to Unregistered Agents."
        />
      </PageLayout>
    );
  }

  const isCode = f.kind === "code_agent";
  const gone = isGone(f);
  const agentPath =
    f.onboarded && f.linkedProject && f.linkedAgent
      ? generatePath(absoluteRouteMap.children.org.children.projects.children.agents.path, {
          orgId: orgName ?? "",
          projectId: f.linkedProject,
          agentId: f.linkedAgent,
        })
      : "";
  const onboardPath = generatePath(
    absoluteRouteMap.children.org.children.agentRegistry.children.unregisteredAgent.children.onboard
      .path,
    { orgId: orgName ?? "", findingId: f.id },
  );

  const actions =
    agentPath || canManage ? (
      <Stack direction="row" spacing={1.5}>
        {agentPath ? (
          <Button
            component={RouterLink}
            to={agentPath}
            variant="contained"
            endIcon={<ExternalLink size={16} />}
          >
            Open registered agent
          </Button>
        ) : canManage && isCode ? (
          <CodeAgentOnboardActions f={f} orgName={orgName ?? ""} onboardPath={onboardPath} />
        ) : (
          canManage && (
            <Button
              component={RouterLink}
              to={onboardPath}
              variant="contained"
              startIcon={<UserPlus size={16} />}
            >
              Onboard as external agent
            </Button>
          )
        )}
      </Stack>
    ) : undefined;

  return (
    <PageLayout
      title={f.displayName}
      description={<HeaderLine f={f} />}
      titleTail={
        <Stack direction="row" spacing={1} alignItems="center">
          <OnboardingChip onboarded={f.onboarded} />
          {gone && (
            <Chip
              size="small"
              variant="outlined"
              color="warning"
              icon={<TriangleAlert size={14} />}
              label="No longer found"
            />
          )}
        </Stack>
      }
      actions={actions}
      disableIcon
      {...back}
    >
      <Stack spacing={3}>
        {gone && (
          <Alert severity="warning" sx={{ py: 0 }}>
            {isCode
              ? "The latest scans no longer find this agent. It was removed from the code, " +
                "or its repository is out of scope."
              : "The latest scans no longer list this agent. It was deleted at the platform, " +
                "or it is out of scope."}
          </Alert>
        )}
        <Grid container spacing={3}>
          <Grid size={{ xs: 12, lg: 8 }}>
            <Stack spacing={3}>
              <EvidenceCard f={f} />
              <UsesCard f={f} />
            </Stack>
          </Grid>
          <Grid size={{ xs: 12, lg: 4 }}>
            <Sidebar f={f} orgName={orgName ?? ""} agentPath={agentPath} />
          </Grid>
        </Grid>
      </Stack>
    </PageLayout>
  );
}

export default FindingDetail;
