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
  ButtonBase,
  Card,
  CardContent,
  Divider,
  LinearProgress,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { CircleCheck, PlugZap } from "@wso2/oxygen-ui-icons-react";
import { useLocation, useNavigate } from "react-router-dom";
import { returnToState, StateNotice } from "@agent-management-platform/views";
import { useShadowSummary } from "@agent-management-platform/api-client";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import { ProviderLogo } from "../providerLogos";
import {
  CUSTOM_PLATFORM,
  formatCount,
  latestCloudScan,
  providerLabel,
  ScanStatusChip,
} from "../shadowUi";
import { isCancelledScan } from "./ScanProgress";
import { RunScanButton } from "./RunScanButton";
import { scanFindingsPath } from "./ScansTab";

interface OverviewPanelProps {
  orgName: string | undefined;
  canManage: boolean;
  onConnect: () => void;
  onOpenScans: () => void;
  onOpenPlatforms: () => void;
  /** Opens Unmanaged Agents, filtered to one platform when given. */
  onOpenAgents: (provider?: string) => void;
}

interface Step {
  title: string;
  description: string;
  done: boolean;
  action?: React.ReactNode;
}

/** The order to work in, one line per step, shown until the first scan. */
function GettingStarted({ steps }: { steps: Step[] }) {
  return (
    <Box>
      <Typography variant="subtitle1" sx={{ mb: 1 }}>
        Get started
      </Typography>
      <Card variant="outlined">
        <Box component="ol" sx={{ listStyle: "none", m: 0, p: 0 }}>
          {steps.map((s, i) => (
            <Box component="li" key={s.title}>
              {i > 0 && <Divider />}
              <Stack
                direction={{ xs: "column", sm: "row" }}
                spacing={{ xs: 1, sm: 2 }}
                alignItems={{ sm: "center" }}
                sx={{ px: 2, py: 1.5 }}
              >
                <Stack direction="row" spacing={1.5} alignItems="center" sx={{ flex: 1 }}>
                  <Box
                    sx={{ width: 20, display: "flex", justifyContent: "center", flexShrink: 0 }}
                  >
                    {s.done ? (
                      <Box sx={{ color: "success.main", display: "flex" }} aria-label="Done">
                        <CircleCheck size={18} />
                      </Box>
                    ) : (
                      <Typography variant="body2" color="text.secondary" fontWeight={500}>
                        {i + 1}
                      </Typography>
                    )}
                  </Box>
                  <Box sx={{ minWidth: 0 }}>
                    <Typography variant="body2" fontWeight={500}>
                      {s.title}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      {s.description}
                    </Typography>
                  </Box>
                </Stack>
                {s.action && <Box sx={{ pl: { xs: 4, sm: 0 } }}>{s.action}</Box>}
              </Stack>
            </Box>
          ))}
        </Box>
      </Card>
    </Box>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Box sx={{ flex: 1, px: { xs: 0, sm: 3 }, py: { xs: 1, sm: 0 }, "&:first-of-type": { pl: 0 } }}>
      <Typography variant="body2" color="text.secondary">
        {label}
      </Typography>
      <Typography variant="h4" sx={{ mt: 0.5 }}>
        {value}
      </Typography>
    </Box>
  );
}

/** One row per platform: logo, name, a bar relative to the largest, and the count. */
function PlatformBreakdown({
  rows,
  onOpen,
}: {
  rows: [string, number][];
  onOpen: (provider: string) => void;
}) {
  const max = Math.max(...rows.map(([, n]) => n));
  return (
    <Box>
      <Typography variant="subtitle1" sx={{ mb: 1 }}>
        Agents by source
      </Typography>
      <Stack spacing={0.5}>
        {rows.map(([p, n]) => (
          <ButtonBase
            key={p}
            onClick={() => onOpen(p)}
            aria-label={`${providerLabel(p)}, ${formatCount(n)} ${n === 1 ? "agent" : "agents"}`}
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1.5,
              width: "100%",
              px: 1,
              py: 1,
              borderRadius: 1,
              textAlign: "left",
              "&:hover": { bgcolor: "action.hover" },
            }}
          >
            <Box sx={{ display: "flex", flexShrink: 0 }}>
              <ProviderLogo provider={p} size={20} />
            </Box>
            <Typography variant="body2" noWrap sx={{ width: { xs: 120, sm: 180 }, flexShrink: 0 }}>
              {providerLabel(p)}
            </Typography>
            <LinearProgress
              variant="determinate"
              value={Math.max(2, (n / max) * 100)}
              aria-hidden
              sx={{ flex: 1, height: 6, borderRadius: 3 }}
            />
            <Typography
              variant="body2"
              sx={{ minWidth: 48, textAlign: "right", fontVariantNumeric: "tabular-nums" }}
            >
              {formatCount(n)}
            </Typography>
          </ButtonBase>
        ))}
      </Stack>
    </Box>
  );
}

/** The unmanaged agents found, by platform, and the latest scan. */
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
  const latest = latestCloudScan(s);
  const total = s?.totalFindings ?? 0;
  // The steps stay until a scan finishes with results, or agents arrive another way
  // (an uploaded inventory file is not a scan); a failed scan keeps them.
  const scanned = latest?.status === "succeeded" || latest?.status === "partial" || total > 0;
  const onboarded = s?.onboarded ?? 0;
  const unmanaged = Math.max(0, total - onboarded);
  const steps: Step[] = [
    {
      title: "Add a source",
      description:
        "Add a connector for an AI platform or a source code repository host, or upload a file " +
        "that lists your agents.",
      done: connected,
      // With no connector, the empty state above already has Add source.
      action: canManage && connected ? (
        <Button size="small" variant="outlined" onClick={onConnect}>
          Add source
        </Button>
      ) : undefined,
    },
    {
      title: "Test the connector",
      description: "Check that Agent Manager can read the agent listings.",
      done: false,
      action: connected ? (
        <Button size="small" variant="text" onClick={onOpenPlatforms}>
          Open sources
        </Button>
      ) : undefined,
    },
    {
      title: "Run a scan",
      description:
        "Scan your AI platform connectors. Source code repositories are scanned by a command " +
        "you run, and inventory files are read when you upload them.",
      done: scanned,
      action: canManage && connected ? <RunScanButton orgName={orgName} /> : undefined,
    },
    {
      title: "Onboard the unmanaged agents",
      description: "Review each agent found and onboard it as an external agent.",
      done: total > 0 && unmanaged === 0,
    },
  ];

  if (summary.isLoading) {
    return (
      <Stack spacing={3}>
        <Skeleton variant="rounded" height={160} />
        <Skeleton variant="rounded" height={96} />
      </Stack>
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
          title="Add a source to start"
          description="Shadow Agent Discovery finds agents that run outside Agent Manager."
          actions={
            canManage ? (
              <Button variant="contained" onClick={onConnect}>
                Add source
              </Button>
            ) : undefined
          }
        />
        <GettingStarted steps={steps} />
      </Stack>
    );
  }

  const byPlatform = Object.entries(s?.byProvider ?? {})
    .filter(([, n]) => n > 0)
    .sort(([, a], [, b]) => b - a);
  const platformCount = latest?.connectionIds.length ?? 0;
  return (
    <Stack spacing={3}>
      <Card variant="outlined">
        <CardContent>
          <Stack
            direction={{ xs: "column", sm: "row" }}
            divider={
              <Divider
                orientation="vertical"
                flexItem
                sx={{ display: { xs: "none", sm: "block" } }}
              />
            }
          >
            <Stat label="Unmanaged agents" value={formatCount(unmanaged)} />
            <Stat label="Managed" value={formatCount(onboarded)} />
            <Stat label="Connectors" value={formatCount(s?.connections)} />
          </Stack>
        </CardContent>
        <Divider />
        <CardContent>
          {latest ? (
            <Stack
              direction={{ xs: "column", sm: "row" }}
              spacing={2}
              alignItems={{ sm: "center" }}
            >
              <Stack direction="row" spacing={1} alignItems="center" sx={{ flex: 1 }}>
                <ScanStatusChip status={latest.status} cancelled={isCancelledScan(latest)} />
                <Typography variant="body2" color="text.secondary">
                  Latest scan {formatRelativeTime(latest.startedAt ?? latest.createdAt)} ·{" "}
                  {platformCount} {platformCount === 1 ? "connector" : "connectors"}
                </Typography>
              </Stack>
              <Stack direction="row" spacing={1}>
                <Button
                  variant="outlined"
                  onClick={() =>
                    navigate(scanFindingsPath(orgName ?? "", latest.id), {
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
            </Stack>
          ) : (
            <Stack direction="row" spacing={2} alignItems="center">
              <Typography variant="body2" color="text.secondary" sx={{ flex: 1 }}>
                {total === 0
                  ? "No scan yet."
                  : (s?.byProvider[CUSTOM_PLATFORM] ?? 0) > 0
                    ? "No platform scan yet. The agents below include those from uploaded " +
                      "inventory files."
                    : "No recent scan. The agents found earlier are below."}
              </Typography>
            </Stack>
          )}
        </CardContent>
      </Card>

      {byPlatform.length > 0 && <PlatformBreakdown rows={byPlatform} onOpen={onOpenAgents} />}

      {!scanned && <GettingStarted steps={steps} />}
    </Stack>
  );
}

export default OverviewPanel;
