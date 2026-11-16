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
  Grid,
  ListingTable,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { SearchX } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link as RouterLink, useParams } from "react-router-dom";
import { PageLayout, StateNotice, useReturnTo } from "@agent-management-platform/views";
import { useShadowFinding, useUpdateShadowFinding } from "@agent-management-platform/api-client";
import { absoluteRouteMap, type ShadowFindingDetail } from "@agent-management-platform/types";
import { kindLabel, ownerLabel, providerLabel, shortDate, StatusChip } from "../shadowUi";

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
};

function formatValue(v: unknown): string {
  if (v === null || v === undefined) return "—";
  if (typeof v === "boolean") return v ? "Yes" : "No";
  if (Array.isArray(v)) {
    return v
      .map((x) =>
        typeof x === "object" && x !== null
          ? [
              (x as { type?: string; label?: string }).type ?? (x as { label?: string }).label,
              (x as { name?: string; origin?: string }).name ?? (x as { origin?: string }).origin,
            ]
              .filter(Boolean)
              .join(": ")
          : String(x),
      )
      .join(", ");
  }
  if (typeof v === "object") {
    return Object.entries(v as Record<string, unknown>)
      .map(([k, x]) => `${k}=${String(x)}`)
      .join(", ");
  }
  return String(v);
}

function Field({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <Box>
      <Typography variant="caption" color="text.secondary">
        {label}
      </Typography>
      <Typography variant="body2" sx={{ wordBreak: "break-word" }}>
        {value || "—"}
      </Typography>
    </Box>
  );
}

function EvidenceCard({ finding }: { finding: ShadowFindingDetail }) {
  return (
    <Card variant="outlined">
      <CardContent>
        <Typography variant="subtitle1" gutterBottom>
          Evidence
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
          The platform listing that returned this agent.
        </Typography>
        {finding.signals.length === 0 ? (
          <Typography variant="body2" color="text.secondary">
            No evidence recorded.
          </Typography>
        ) : (
          <Box sx={{ overflowX: "auto" }}>
            <ListingTable>
              <ListingTable.Head>
                <ListingTable.Row>
                  <ListingTable.Cell>Evidence</ListingTable.Cell>
                  <ListingTable.Cell>Exact condition</ListingTable.Cell>
                  <ListingTable.Cell>Observed</ListingTable.Cell>
                  <ListingTable.Cell>Seen</ListingTable.Cell>
                </ListingTable.Row>
              </ListingTable.Head>
              <ListingTable.Body>
                {finding.signals.map((s) => (
                  <ListingTable.Row key={`${s.code}|${s.source}`}>
                    <ListingTable.Cell>
                      <Typography variant="body2" fontWeight={500}>
                        {s.name || s.code}
                      </Typography>
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ fontFamily: "monospace" }}
                      >
                        {s.source}
                      </Typography>
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Typography variant="caption" color="text.secondary">
                        {s.condition}
                      </Typography>
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Typography variant="caption">{formatValue(s.observed)}</Typography>
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Typography variant="caption" color="text.secondary">
                        {shortDate(s.detectedAt)}
                      </Typography>
                    </ListingTable.Cell>
                  </ListingTable.Row>
                ))}
              </ListingTable.Body>
            </ListingTable>
          </Box>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * One unregistered agent: what its platform lists, the evidence, who found it
 * and its review status.
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
  const update = useUpdateShadowFinding(orgName, findingId);
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

  const attributes = Object.entries(f.attributes ?? {});
  const owner = ownerLabel(f.ownerHint);
  const foundBy = f.foundBy ?? [];

  return (
    <PageLayout
      title={f.displayName}
      description={`${providerLabel(f.provider)} · ${kindLabel(f.kind, f.provider)}`}
      disableIcon
      {...back}
    >
      <Stack spacing={3}>
        {!f.present && (
          <Alert severity="warning">
            The latest complete scan of {f.connectionName} no longer listed this agent: it was
            deleted at the platform, or it is no longer in the connection&apos;s scope. It was last
            seen {shortDate(f.lastSeenAt)}.
          </Alert>
        )}
        <Card variant="outlined">
          <CardContent>
            <Grid container spacing={2} alignItems="center">
              <Grid size={{ xs: 6, md: 3 }}>
                <Typography variant="caption" color="text.secondary" component="p">
                  Status
                </Typography>
                <StatusChip status={f.status} />
              </Grid>
              <Grid size={{ xs: 6, md: 3 }}>
                <Field label="State at the platform" value={f.state} />
              </Grid>
              <Grid size={{ xs: 12, md: 6 }}>
                {canManage && (
                  <Stack direction="row" spacing={1} justifyContent={{ md: "flex-end" }}>
                    {f.status !== "reviewed" && (
                      <Button
                        size="small"
                        variant="outlined"
                        disabled={update.isPending}
                        onClick={() => update.mutate({ status: "reviewed" })}
                      >
                        Mark reviewed
                      </Button>
                    )}
                    {f.status === "new" ? (
                      <Button
                        size="small"
                        variant="outlined"
                        disabled={update.isPending}
                        onClick={() => update.mutate({ status: "dismissed" })}
                      >
                        Dismiss
                      </Button>
                    ) : (
                      <Button
                        size="small"
                        variant="outlined"
                        disabled={update.isPending}
                        onClick={() => update.mutate({ status: "new" })}
                      >
                        Reopen
                      </Button>
                    )}
                  </Stack>
                )}
              </Grid>
            </Grid>
          </CardContent>
        </Card>

        <EvidenceCard finding={f} />

        <Grid container spacing={3}>
          <Grid size={{ xs: 12, md: 6 }}>
            <Card variant="outlined" sx={{ height: "100%" }}>
              <CardContent>
                <Typography variant="subtitle1" gutterBottom>
                  Details
                </Typography>
                <Stack spacing={1.5}>
                  <Field
                    label="Platform ID"
                    value={<span style={{ fontFamily: "monospace" }}>{f.externalId}</span>}
                  />
                  {f.scope && <Field label="Scope" value={f.scope} />}
                  {attributes.map(([k, v]) => (
                    <Field key={k} label={ATTRIBUTE_LABELS[k] ?? k} value={formatValue(v)} />
                  ))}
                  <Typography variant="caption" color="text.secondary">
                    Only what the platform&apos;s agent listing reports. Instructions, prompts, tool
                    definitions and secrets are never stored.
                  </Typography>
                </Stack>
              </CardContent>
            </Card>
          </Grid>
          <Grid size={{ xs: 12, md: 6 }}>
            <Stack spacing={3} sx={{ height: "100%" }}>
              <Card variant="outlined">
                <CardContent>
                  <Typography variant="subtitle1" gutterBottom>
                    Found by
                  </Typography>
                  <Stack spacing={1}>
                    {foundBy.map((src) => (
                      <Stack key={src.findingId} direction="row" spacing={1} alignItems="center">
                        <Typography variant="body2">{src.connectionName}</Typography>
                        {!src.present && (
                          <Chip
                            size="small"
                            variant="outlined"
                            color="warning"
                            label="No longer listed"
                          />
                        )}
                      </Stack>
                    ))}
                    {foundBy.length > 1 && (
                      <Typography variant="caption" color="text.secondary">
                        The same agent (same platform and platform ID) was found through{" "}
                        {foundBy.length} connections; it is shown once.
                      </Typography>
                    )}
                  </Stack>
                  <Grid container spacing={1.5} sx={{ mt: 1 }}>
                    <Grid size={6}>
                      <Field
                        label="Created at the platform"
                        value={shortDate(f.providerCreatedAt)}
                      />
                    </Grid>
                    <Grid size={6}>
                      <Field label="First seen by a scan" value={shortDate(f.firstSeenAt)} />
                    </Grid>
                    <Grid size={6}>
                      <Field label="Last seen by a scan" value={shortDate(f.lastSeenAt)} />
                    </Grid>
                  </Grid>
                </CardContent>
              </Card>
              {owner && (
                <Card variant="outlined">
                  <CardContent>
                    <Typography variant="subtitle1" gutterBottom>
                      Owner
                    </Typography>
                    <Stack spacing={1.5}>
                      <Field label="Created by" value={owner} />
                      {f.ownerHint?.name && f.ownerHint.name !== owner && (
                        <Field label="Name" value={f.ownerHint.name} />
                      )}
                      {f.ownerHint?.username && f.ownerHint.username !== owner && (
                        <Field label="Username / principal" value={f.ownerHint.username} />
                      )}
                      {f.ownerHint?.source && <Field label="Source" value={f.ownerHint.source} />}
                    </Stack>
                  </CardContent>
                </Card>
              )}
            </Stack>
          </Grid>
        </Grid>

        {f.related.length > 0 && (
          <Card variant="outlined">
            <CardContent>
              <Typography variant="subtitle1" gutterBottom>
                Possible duplicates
              </Typography>
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
