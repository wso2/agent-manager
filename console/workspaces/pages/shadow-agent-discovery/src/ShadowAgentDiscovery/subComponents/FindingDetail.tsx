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

import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Divider,
  Grid,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import {
  CalendarPlus,
  CircleCheck,
  Clock,
  ExternalLink,
  Fingerprint,
  History,
  MapPin,
  SearchX,
  TriangleAlert,
  UserPlus,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link as RouterLink, useParams } from "react-router-dom";
import {
  PageLayout,
  PageMetaItem,
  StateNotice,
  useReturnTo,
} from "@agent-management-platform/views";
import { useShadowFinding } from "@agent-management-platform/api-client";
import { absoluteRouteMap, type ShadowFindingDetail } from "@agent-management-platform/types";
import { kindLabel, OnboardingChip, ownerLabel, providerLabel, shortDate } from "../shadowUi";

/** A related agent's relation, as a short label. */
export function relationLabel(relation: string): string {
  return relation === "possible_duplicate" ? "Possible duplicate" : relation.replace(/_/g, " ");
}

/** Labels of the attributes an agent listing reports (the service's allow-list). */
const ATTRIBUTE_LABELS: Record<string, string> = {
  model: "Model",
  framework: "Framework",
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
  reviewedBy: "Confirmed by",
};

/** Who confirmed a code agent before the scan published it. */
const REVIEWED_BY_LABELS: Record<string, string> = {
  rules: "Rules",
  ai_review: "AI review",
  ai_review_judge: "AI review and LLM judge",
};

/** Attributes that are lists, shown as chips. */
const CHIP_ATTRIBUTES = ["tools", "mcpServers", "skills", "protocolType", "labels"];
/** Long identifiers, shown on their own line in monospace. */
const ID_ATTRIBUTES = [
  "resourceId",
  "roleArn",
  "runtimeReference",
  "endpoint",
  "image",
  "repositoryUrl",
  "serviceAccount",
  "otelAgentId",
];

/** An attribute's value, as shown on the page. */
function attributeValue(key: string, v: unknown): string {
  if (key === "reviewedBy" && typeof v === "string") return REVIEWED_BY_LABELS[v] ?? v;
  return scalarText(v);
}

function scalarText(v: unknown): string {
  if (v === null || v === undefined || v === "") return "—";
  if (typeof v === "boolean") return v ? "Yes" : "No";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

/** A list attribute as chip labels: a tool's type and name, an MCP server's label and origin. */
export function chipLabels(v: unknown): string[] {
  if (Array.isArray(v)) {
    return v
      .map((x) => {
        if (typeof x !== "object" || x === null) return String(x);
        const o = x as { type?: string; name?: string; label?: string; origin?: string };
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

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <Box sx={{ minWidth: 0 }}>
      <Typography variant="caption" color="text.secondary" component="p">
        {label}
      </Typography>
      <Typography variant="body2" component="div" sx={{ wordBreak: "break-word" }}>
        {children}
      </Typography>
    </Box>
  );
}

function SectionTitle({ title, hint }: { title: string; hint?: string }) {
  return (
    <Box sx={{ mb: 2 }}>
      <Typography variant="subtitle1">{title}</Typography>
      {hint && (
        <Typography variant="caption" color="text.secondary">
          {hint}
        </Typography>
      )}
    </Box>
  );
}

/** What the platform's agent listing reports, grouped: facts, lists and identifiers. */
function DetailsCard({ f }: { f: ShadowFindingDetail }) {
  const isCode = f.kind === "code_agent";
  const attrs = f.attributes ?? {};
  const entries = Object.entries(attrs).filter(
    ([, v]) => v !== null && v !== undefined && v !== "",
  );
  const lists = entries.filter(([k]) => CHIP_ATTRIBUTES.includes(k) && chipLabels(attrs[k]).length);
  const ids = entries.filter(([k]) => ID_ATTRIBUTES.includes(k));
  const description = typeof attrs.description === "string" ? attrs.description : "";
  const facts = entries.filter(
    ([k]) => !CHIP_ATTRIBUTES.includes(k) && !ID_ATTRIBUTES.includes(k) && k !== "description",
  );
  return (
    <Card variant="outlined" sx={{ height: "100%" }}>
      <CardContent>
        <SectionTitle
          title="Details"
          hint={
            isCode
              ? "What the scan command reported from the repository's code. Source code, prompts and secrets are never stored."
              : `What ${providerLabel(f.provider)} reports about this agent. Instructions, prompts, tool definitions and secrets are never stored.`
          }
        />
        <Stack spacing={2.5}>
          {description && <Typography variant="body2">{description}</Typography>}
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, sm: 6 }}>
              <Field label="Type">{kindLabel(f.kind, f.provider)}</Field>
            </Grid>
            {f.scope && (
              <Grid size={{ xs: 12, sm: 6 }}>
                <Field label={isCode ? "Repository" : "Scope"}>{f.scope}</Field>
              </Grid>
            )}
            {facts.map(([k, v]) => (
              <Grid key={k} size={{ xs: 12, sm: 6 }}>
                <Field label={ATTRIBUTE_LABELS[k] ?? k}>{attributeValue(k, v)}</Field>
              </Grid>
            ))}
          </Grid>
          {lists.length > 0 && (
            <>
              <Divider />
              <Stack spacing={2}>
                {lists.map(([k, v]) => (
                  <Field key={k} label={ATTRIBUTE_LABELS[k] ?? k}>
                    <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap" sx={{ mt: 0.5 }}>
                      {chipLabels(v).map((label) => (
                        <Chip key={label} size="small" variant="outlined" label={label} />
                      ))}
                    </Stack>
                  </Field>
                ))}
              </Stack>
            </>
          )}
          {ids.length > 0 && (
            <>
              <Divider />
              <Stack spacing={1.5}>
                {ids.map(([k, v]) => (
                  <Field key={k} label={ATTRIBUTE_LABELS[k] ?? k}>
                    <Box component="span" sx={{ fontFamily: "monospace", fontSize: 13 }}>
                      {scalarText(v)}
                    </Box>
                  </Field>
                ))}
              </Stack>
            </>
          )}
        </Stack>
      </CardContent>
    </Card>
  );
}

/** The platform connections that list the agent. */
function SourceCard({ f }: { f: ShadowFindingDetail }) {
  const isCode = f.kind === "code_agent";
  const sources = f.foundBy ?? [];
  return (
    <Card variant="outlined">
      <CardContent>
        <SectionTitle
          title="Source"
          hint={
            isCode
              ? "The GitHub or Azure DevOps platforms whose scans found this agent."
              : "The platform connections that list this agent."
          }
        />
        <Stack divider={<Divider flexItem />} spacing={1.25}>
          {sources.map((src) => (
            <Stack
              key={src.findingId}
              direction="row"
              spacing={1}
              alignItems="center"
              justifyContent="space-between"
            >
              <Typography variant="body2" fontWeight={500} sx={{ wordBreak: "break-word" }}>
                {src.connectionName}
              </Typography>
              {src.present ? (
                <Chip
                  size="small"
                  color="success"
                  variant="outlined"
                  icon={<CircleCheck size={14} />}
                  label={isCode ? "Found" : "Listed"}
                />
              ) : (
                <Chip
                  size="small"
                  color="warning"
                  variant="outlined"
                  icon={<TriangleAlert size={14} />}
                  label={isCode ? "No longer found" : "No longer listed"}
                />
              )}
            </Stack>
          ))}
        </Stack>
        {sources.length > 1 && (
          <Typography variant="caption" color="text.secondary" component="p" sx={{ mt: 1.5 }}>
            The same agent was found through {sources.length} connections. It is shown once.
          </Typography>
        )}
      </CardContent>
    </Card>
  );
}

function TimelineRow({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
}) {
  return (
    <Stack direction="row" spacing={1.5} alignItems="center">
      <Box sx={{ color: "text.secondary", display: "flex" }}>{icon}</Box>
      <Box sx={{ flex: 1 }}>
        <Typography variant="caption" color="text.secondary" component="p">
          {label}
        </Typography>
        <Typography variant="body2">{value}</Typography>
      </Box>
    </Stack>
  );
}

function ActivityCard({ f }: { f: ShadowFindingDetail }) {
  const owner = ownerLabel(f.ownerHint);
  return (
    <Card variant="outlined">
      <CardContent>
        <SectionTitle title="Activity" />
        <Stack spacing={1.5}>
          {f.providerCreatedAt && (
            <TimelineRow
              icon={<CalendarPlus size={16} />}
              label="Created at the platform"
              value={shortDate(f.providerCreatedAt)}
            />
          )}
          <TimelineRow
            icon={<History size={16} />}
            label="First seen by a scan"
            value={shortDate(f.firstSeenAt)}
          />
          <TimelineRow
            icon={<Clock size={16} />}
            label="Last seen by a scan"
            value={shortDate(f.lastSeenAt)}
          />
          {owner && (
            <TimelineRow
              icon={<UserPlus size={16} />}
              label={f.ownerHint?.source ? `Created by (${f.ownerHint.source})` : "Created by"}
              value={owner}
            />
          )}
        </Stack>
      </CardContent>
    </Card>
  );
}

/** The listings that returned the agent: what was called, the exact condition, what was seen. */
function EvidenceCard({ f }: { f: ShadowFindingDetail }) {
  const isCode = f.kind === "code_agent";
  return (
    <Card variant="outlined">
      <CardContent>
        <SectionTitle
          title="Evidence"
          hint={
            isCode
              ? "Why this is an agent: the code in the repository that matched each rule."
              : "Why this is an agent: the platform listing that returned it."
          }
        />
        {f.signals.length === 0 ? (
          <Typography variant="body2" color="text.secondary">
            No evidence recorded.
          </Typography>
        ) : (
          <Stack spacing={1.5}>
            {f.signals.map((s) => {
              const observed = Object.entries(s.observed ?? {});
              return (
                <Box
                  key={`${s.code}|${s.source}`}
                  sx={{ border: 1, borderColor: "divider", borderRadius: 1, p: 2 }}
                >
                  <Stack
                    direction={{ xs: "column", sm: "row" }}
                    spacing={1}
                    alignItems={{ sm: "center" }}
                    justifyContent="space-between"
                  >
                    <Stack
                      direction="row"
                      spacing={1}
                      alignItems="center"
                      flexWrap="wrap"
                      useFlexGap
                    >
                      <Typography variant="subtitle2">{s.name || s.code}</Typography>
                      <Chip
                        size="small"
                        variant="outlined"
                        label={s.source}
                        sx={{ fontFamily: "monospace" }}
                      />
                    </Stack>
                    <Typography variant="caption" color="text.secondary">
                      Seen {shortDate(s.detectedAt)}
                    </Typography>
                  </Stack>
                  <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                    {s.condition}
                  </Typography>
                  {observed.length > 0 && (
                    <Stack
                      direction="row"
                      spacing={1}
                      useFlexGap
                      flexWrap="wrap"
                      alignItems="center"
                      sx={{ mt: 1.5 }}
                    >
                      <Typography variant="caption" color="text.secondary">
                        Observed
                      </Typography>
                      {observed.map(([k, v]) => (
                        <Chip key={k} size="small" label={`${k}: ${scalarText(v)}`} />
                      ))}
                    </Stack>
                  )}
                </Box>
              );
            })}
          </Stack>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * One unregistered agent: what its platform reports, where it comes from, the
 * evidence, and onboarding it as an external agent.
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
  const returnTo = useReturnTo(`${listPath}?tab=agents`);
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
  const reviewedBy = f.attributes?.reviewedBy;
  const aiReviewed = reviewedBy === "ai_review" || reviewedBy === "ai_review_judge";
  const agentPath =
    f.onboarded && f.linkedProject && f.linkedAgent
      ? generatePath(absoluteRouteMap.children.org.children.projects.children.agents.path, {
          orgId: orgName ?? "",
          projectId: f.linkedProject,
          agentId: f.linkedAgent,
        })
      : "";
  const onboardPath = generatePath(
    absoluteRouteMap.children.org.children.shadowAgentDiscovery.children.finding.children.onboard
      .path,
    { orgId: orgName ?? "", findingId: f.id },
  );

  const actions = agentPath ? (
    <Button
      component={RouterLink}
      to={agentPath}
      variant="outlined"
      endIcon={<ExternalLink size={16} />}
    >
      Open registered agent
    </Button>
  ) : canManage ? (
    <Button
      component={RouterLink}
      to={onboardPath}
      variant="contained"
      startIcon={<UserPlus size={16} />}
    >
      Onboard as external agent
    </Button>
  ) : undefined;

  return (
    <PageLayout
      title={f.displayName}
      description={`${kindLabel(f.kind, f.provider)} on ${providerLabel(f.provider)}`}
      titleTail={
        <Stack direction="row" spacing={1} alignItems="center">
          <OnboardingChip onboarded={f.onboarded} />
          {f.state && <Chip size="small" variant="outlined" label={f.state} />}
        </Stack>
      }
      meta={
        <>
          <PageMetaItem
            icon={<Fingerprint size={12} />}
            label={isCode ? "Location in code" : "Platform ID"}
          >
            <Box component="span" sx={{ fontFamily: "monospace", fontSize: 12 }}>
              {f.externalId}
            </Box>
          </PageMetaItem>
          {f.scope && (
            <PageMetaItem icon={<MapPin size={12} />} label={isCode ? "Repository" : "Scope"}>
              {f.scope}
            </PageMetaItem>
          )}
        </>
      }
      actions={actions}
      disableIcon
      {...back}
    >
      <Stack spacing={3}>
        {f.onboarded && (
          <Alert severity="success">
            Onboarded as an external agent:{" "}
            {agentPath ? (
              <RouterLink to={agentPath}>
                {f.linkedProject} / {f.linkedAgent}
              </RouterLink>
            ) : (
              `${f.linkedProject} / ${f.linkedAgent}`
            )}
            {f.onboardedAt ? `, on ${shortDate(f.onboardedAt)}` : ""}.
          </Alert>
        )}
        {!f.present && (
          <Alert severity="warning">
            {isCode ? (
              <>
                The latest complete scan of {f.connectionName} no longer found this agent: it was
                removed from the repository&apos;s code, or the repository is no longer in the
                platform&apos;s scope. It was last seen {shortDate(f.lastSeenAt)}.
              </>
            ) : (
              <>
                The latest complete scan of {f.connectionName} no longer listed this agent: it was
                deleted at the platform, or it is no longer in the connection&apos;s scope. It was
                last seen {shortDate(f.lastSeenAt)}.
              </>
            )}
          </Alert>
        )}
        {aiReviewed && (
          <Alert severity="info">
            AI review confirmed this agent. AI review can make mistakes, so check the evidence.
          </Alert>
        )}
        <Grid container spacing={3}>
          <Grid size={{ xs: 12, md: 8 }}>
            <DetailsCard f={f} />
          </Grid>
          <Grid size={{ xs: 12, md: 4 }}>
            <Stack spacing={3}>
              <SourceCard f={f} />
              <ActivityCard f={f} />
            </Stack>
          </Grid>
        </Grid>
        <EvidenceCard f={f} />
        {f.related.length > 0 && (
          <Card variant="outlined">
            <CardContent>
              <SectionTitle
                title="Possible duplicates"
                hint="Agents on other platforms that may be the same agent."
              />
              <Stack spacing={1}>
                {f.related.map((r) => (
                  <Stack
                    key={`${r.findingId}|${r.relation}`}
                    direction="row"
                    spacing={1}
                    alignItems="center"
                  >
                    <Chip
                      size="small"
                      variant="outlined"
                      color="warning"
                      label={relationLabel(r.relation)}
                    />
                    <RouterLink
                      to={generatePath(
                        absoluteRouteMap.children.org.children.shadowAgentDiscovery.children.finding
                          .path,
                        { orgId: orgName ?? "", findingId: r.findingId },
                      )}
                    >
                      {r.displayName || r.findingId}
                    </RouterLink>
                    {r.provider && (
                      <Typography variant="caption" color="text.secondary">
                        {providerLabel(r.provider)}
                        {r.reason ? ` · ${r.reason}` : ""}
                      </Typography>
                    )}
                  </Stack>
                ))}
              </Stack>
            </CardContent>
          </Card>
        )}
      </Stack>
    </PageLayout>
  );
}

export default FindingDetail;
