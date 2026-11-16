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
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Grid,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronRight, CircleCheck, PlugZap } from "@wso2/oxygen-ui-icons-react";
import { useLocation, useNavigate } from "react-router-dom";
import { returnToState, StateNotice } from "@agent-management-platform/views";
import { useShadowSummary } from "@agent-management-platform/api-client";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import { formatCount, providerLabel, ScanStatusChip } from "../shadowUi";
import { RunScanButton } from "./RunScanButton";
import { scanFindingsPath } from "./ScansTab";

interface OverviewPanelProps {
  orgName: string | undefined;
  canManage: boolean;
  onConnect: () => void;
  onOpenScans: () => void;
  onOpenPlatforms: () => void;
  /** Opens Unregistered Agents, filtered to one platform when given. */
  onOpenAgents: (provider?: string) => void;
}

interface Step {
  title: string;
  description: string;
  done: boolean;
  action?: React.ReactNode;
}

/** The order to work in: add a platform, test it, scan, review the agents. */
function GettingStarted({ steps }: { steps: Step[] }) {
  return (
    <Box>
      <Typography variant="subtitle1" sx={{ mb: 1.5 }}>
        How to find unregistered agents
      </Typography>
      <Stack
        direction={{ xs: "column", lg: "row" }}
        spacing={1}
        alignItems={{ lg: "stretch" }}
        component="ol"
        sx={{ listStyle: "none", m: 0, p: 0 }}
      >
        {steps.map((s, i) => (
          <Stack
            key={s.title}
            component="li"
            direction={{ xs: "column", lg: "row" }}
            spacing={1}
            alignItems={{ lg: "center" }}
            sx={{ flex: 1, minWidth: 0 }}
          >
            <Card variant="outlined" sx={{ flex: 1, height: "100%" }}>
              <CardContent>
                <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 0.5 }}>
                  {s.done ? (
                    <Box sx={{ color: "success.main", display: "flex" }} aria-label="Done">
                      <CircleCheck size={18} />
                    </Box>
                  ) : (
                    <Typography variant="body2" color="text.secondary" fontWeight={500}>
                      {i + 1}.
                    </Typography>
                  )}
                  <Typography variant="body2" fontWeight={500}>
                    {s.title}
                  </Typography>
                </Stack>
                <Typography variant="caption" color="text.secondary" component="p">
                  {s.description}
                </Typography>
                {s.action && <Box sx={{ mt: 1.5 }}>{s.action}</Box>}
              </CardContent>
            </Card>
            {i < steps.length - 1 && (
              <Box sx={{ display: { xs: "none", lg: "flex" }, color: "text.disabled" }} aria-hidden>
                <ChevronRight size={18} />
              </Box>
            )}
          </Stack>
        ))}
      </Stack>
    </Box>
  );
}

function StatTile({ label, value, hint }: { label: string; value: string; hint: string }) {
  return (
    <Card variant="outlined" sx={{ height: "100%" }}>
      <CardContent>
        <Typography variant="body2" color="text.secondary">
          {label}
        </Typography>
        <Typography variant="h4" sx={{ my: 0.5 }}>
          {value}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {hint}
        </Typography>
      </CardContent>
    </Card>
  );
}

/** The unregistered agents found, by platform, and the latest scan. */
export function OverviewPanel({
  orgName,
  canManage,
  onConnect,
  onOpenScans,
  onOpenPlatforms,
  onOpenAgents,
}: OverviewPanelProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const summary = useShadowSummary(orgName);
  const s = summary.data;
  const connected = (s?.connections ?? 0) > 0;
  const scanned = !!s?.latestScan;
  const total = s?.totalFindings ?? 0;
  const toReview = s?.byStatus?.new ?? 0;
  const steps: Step[] = [
    {
      title: "Add a platform",
      description:
        "Connect an AI platform (OpenAI, Anthropic, Amazon Bedrock, Google Agent Platform, Microsoft Foundry) or a source repository host with a read-only credential.",
      done: connected,
      action: canManage ? (
        <Button size="small" variant="outlined" onClick={onConnect}>
          Add platform
        </Button>
      ) : undefined,
    },
    {
      title: "Test the connection",
      description: "Test connection lists one agent per listing and shows what works.",
      done: false,
      action: connected ? (
        <Button size="small" variant="outlined" onClick={onOpenPlatforms}>
          Open platforms
        </Button>
      ) : undefined,
    },
    {
      title: "Run a scan",
      description:
        "Run scan at the top of the page scans all platforms or the ones you choose; Scan on a row in Platforms scans one.",
      done: scanned,
      action: canManage && connected ? <RunScanButton orgName={orgName} /> : undefined,
    },
    {
      title: "Review the unregistered agents",
      description:
        "Unregistered Agents lists every agent the platforms list, once, with the evidence. Mark each one reviewed or dismissed.",
      done: total > 0 && toReview === 0,
      action: scanned ? (
        <Button size="small" variant="outlined" onClick={() => onOpenAgents()}>
          View agents
        </Button>
      ) : undefined,
    },
  ];

  if (summary.isLoading) {
    return (
      <Grid container spacing={2}>
        {[0, 1, 2].map((i) => (
          <Grid key={i} size={{ xs: 12, sm: 6, lg: 4 }}>
            <Skeleton variant="rounded" height={112} />
          </Grid>
        ))}
      </Grid>
    );
  }
  if (summary.error) {
    return (
      <Typography variant="body2" color="error">
        Couldn&apos;t load the summary. Reload the page to try again.
      </Typography>
    );
  }
  if (s && s.connections === 0) {
    return (
      <Stack spacing={3}>
        <StateNotice
          icon={PlugZap}
          title="Add a platform to start"
          description="Shadow Agent Discovery reads the agent listings of your AI platforms with a read-only credential and shows the agents that run outside Agent Manager."
          actions={
            canManage ? (
              <Button variant="contained" onClick={onConnect}>
                Add platform
              </Button>
            ) : undefined
          }
        />
        <GettingStarted steps={steps} />
      </Stack>
    );
  }

  const byPlatform = Object.entries(s?.byProvider ?? {}).filter(([, n]) => n > 0);
  return (
    <Stack spacing={3}>
      <Grid container spacing={2}>
        <Grid size={{ xs: 12, sm: 6, lg: 4 }}>
          <StatTile
            label="Unregistered agents"
            value={formatCount(total)}
            hint="Agents the platforms list, each counted once"
          />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, lg: 4 }}>
          <StatTile
            label="To review"
            value={formatCount(toReview)}
            hint="Agents not reviewed yet"
          />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, lg: 4 }}>
          <StatTile
            label="Platforms connected"
            value={formatCount(s?.connections)}
            hint="Connections to AI platforms and source repository hosts"
          />
        </Grid>
      </Grid>

      <Card variant="outlined">
        <CardContent>
          <Stack direction={{ xs: "column", sm: "row" }} spacing={2} alignItems={{ sm: "center" }}>
            <Box sx={{ flex: 1 }}>
              <Typography variant="subtitle1">Latest scan</Typography>
              {s?.latestScan ? (
                <Stack direction="row" spacing={1} alignItems="center" sx={{ mt: 0.5 }}>
                  <ScanStatusChip status={s.latestScan.status} />
                  <Typography variant="body2" color="text.secondary">
                    {formatRelativeTime(s.latestScan.startedAt ?? s.latestScan.createdAt)} ·{" "}
                    {s.latestScan.connectionIds.length}{" "}
                    {s.latestScan.connectionIds.length === 1 ? "platform" : "platforms"} scanned
                  </Typography>
                </Stack>
              ) : (
                <Typography variant="body2" color="text.secondary">
                  No scan yet. Use Run scan at the top of the page.
                </Typography>
              )}
            </Box>
            {s?.latestScan && (
              <Stack direction="row" spacing={1}>
                <Button
                  variant="outlined"
                  onClick={() =>
                    navigate(scanFindingsPath(orgName ?? "", s.latestScan?.id ?? ""), {
                      state: returnToState(location),
                    })
                  }
                >
                  View agents
                </Button>
                <Button variant="text" onClick={onOpenScans}>
                  All scans
                </Button>
              </Stack>
            )}
          </Stack>
        </CardContent>
      </Card>

      {byPlatform.length > 0 && (
        <Card variant="outlined">
          <CardContent>
            <Typography variant="subtitle1" sx={{ mb: 1 }}>
              Unregistered agents by platform
            </Typography>
            <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap">
              {byPlatform.map(([p, n]) => (
                <Chip
                  key={p}
                  variant="outlined"
                  label={`${providerLabel(p)} · ${formatCount(n)}`}
                  onClick={() => onOpenAgents(p)}
                />
              ))}
            </Stack>
          </CardContent>
        </Card>
      )}

      <GettingStarted steps={steps} />
    </Stack>
  );
}

export default OverviewPanel;
