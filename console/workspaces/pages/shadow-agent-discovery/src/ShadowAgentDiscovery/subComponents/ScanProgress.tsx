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

import { useEffect, useId, useState } from "react";
import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Divider,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { Ban, Clock } from "@wso2/oxygen-ui-icons-react";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import type {
  ShadowConnectionResponse,
  ShadowScanCallResult,
  ShadowScanConnectionResult,
  ShadowScanResponse,
} from "@agent-management-platform/types";
import { isShadowScanActive } from "@agent-management-platform/api-client";
import {
  formatCount,
  itemCount,
  OutcomeChip,
  outcomeLabel,
  providerLabel,
  ScanStatusChip,
  SOURCE_HOSTS,
} from "../shadowUi";
import { ProviderLogo } from "../providerLogos";

/** A relative time inside a sentence: "just now", not "Just now". */
const relTime = (t: string) => formatRelativeTime(t).replace(/^Just now$/, "just now");

/**
 * Whether a cancel stopped this connection: a cloud scan's unfinished
 * connections get "cancelled", a scan command's "cancelled: no results uploaded".
 */
export function isCancelledResult(r: Pick<ShadowScanConnectionResult, "outcome" | "message">) {
  return r.outcome === "failed" && /^cancelled(:|$)/.test(r.message ?? "");
}

/** Whether the user cancelled the scan (the API reports it as Failed). */
export function isCancelledScan(s: Pick<ShadowScanResponse, "status" | "results">): boolean {
  return s.status === "failed" && s.results.some(isCancelledResult);
}

export type ConnectorState = "waiting" | "scanning" | "done";

export interface ConnectorProgress {
  connectionId: string;
  name: string;
  provider: string;
  state: ConnectorState;
  result?: ShadowScanConnectionResult;
}

/**
 * Each connector of a scan and how far it got. A queued connector may have no
 * name or provider yet, so they come from the connections list.
 */
export function connectorProgress(
  scan: Pick<ShadowScanResponse, "connectionIds" | "results">,
  connections: ShadowConnectionResponse[],
): ConnectorProgress[] {
  return scan.connectionIds.map((id) => {
    const r = scan.results.find((x) => x.connectionId === id);
    const c = connections.find((x) => x.id === id);
    const state: ConnectorState =
      !r || r.outcome === "queued" ? "waiting" : r.outcome === "running" ? "scanning" : "done";
    return {
      connectionId: id,
      name: r?.connectionName || c?.name || id,
      provider: r?.provider || c?.provider || "",
      state,
      result: r,
    };
  });
}

/** How many of a scan's connectors finished; a connector a cancel stopped did not. */
export function finishedCount(scan: Pick<ShadowScanResponse, "connectionIds" | "results">) {
  return connectorProgress(scan, []).filter(
    (p) => p.state === "done" && !(p.result && isCancelledResult(p.result)),
  ).length;
}

/** "5 items" for a call that worked, else its outcome ("Permission denied"). */
export function callDetail(c: ShadowScanCallResult): string {
  return c.outcome === "ok" ? itemCount(c.items) : outcomeLabel(c.outcome);
}

/** "3 steps done · last: ListAgentRuntimes (5 items)". */
export function stepsSummary(calls: ShadowScanCallResult[]): string {
  const n = calls.length;
  if (n === 0) return "No steps done yet";
  const last = calls[n - 1];
  return `${n} ${n === 1 ? "step" : "steps"} done · last: ${last.call} (${callDetail(last)})`;
}

/**
 * One line of a queued or running scan's progress, for the Sources tab:
 * "1 of 3 connectors done · AWS prod: 3 steps done · last: ListAgentRuntimes (5 items)".
 */
export function progressLine(
  scan: ShadowScanResponse,
  connections: ShadowConnectionResponse[],
): string {
  const rows = connectorProgress(scan, connections);
  const done = rows.filter((r) => r.state === "done").length;
  const parts: string[] = [];
  if (rows.length > 1) parts.push(`${done} of ${rows.length} connectors done`);
  const now = rows.find((r) => r.state === "scanning");
  if (now) {
    const steps = stepsSummary(now.result?.calls ?? []);
    parts.push(rows.length > 1 ? `${now.name}: ${steps}` : steps);
  } else if (done === 0) {
    parts.push("Waiting to start");
  }
  return parts.join(" · ");
}

/** "45 s", "3 min 12 s", "1 h 5 min". */
export function formatElapsed(ms: number): string {
  if (!Number.isFinite(ms)) return "";
  const s = Math.max(0, Math.floor(ms / 1000));
  if (s < 60) return `${s} s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min ${s % 60} s`;
  return `${Math.floor(m / 60)} h ${m % 60} min`;
}

/** The current time, ticking every second while `active`. */
function useNow(active: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return undefined;
    setNow(Date.now());
    const t = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, [active]);
  return now;
}

/**
 * A connection's state in a scan: Waiting, Scanning, Cancelled, or its outcome.
 * OutcomeChip alone would show "queued" and "running" as raw failures.
 */
export function ConnectionResultChip({
  result,
}: {
  result: Pick<ShadowScanConnectionResult, "outcome" | "message">;
}) {
  if (result.outcome === "queued") {
    return <Chip size="small" variant="outlined" icon={<Clock size={14} />} label="Waiting" />;
  }
  if (result.outcome === "running") {
    return (
      <Chip
        size="small"
        variant="outlined"
        color="info"
        icon={<CircularProgress size={12} color="inherit" />}
        label="Scanning"
      />
    );
  }
  if (isCancelledResult(result)) {
    return <Chip size="small" variant="outlined" icon={<Ban size={14} />} label="Cancelled" />;
  }
  return <OutcomeChip outcome={result.outcome} />;
}

/** The provider calls a connector finished, each with its outcome. */
function CallList({ calls, id }: { calls: ShadowScanCallResult[]; id: string }) {
  return (
    <Box
      component="ul"
      id={id}
      aria-label="Steps done"
      sx={{ m: 0, mt: 1, p: 0, listStyle: "none", "& li + li": { mt: 0.75 } }}
    >
      {calls.map((c, i) => (
        <li key={`${c.call}-${i}`}>
          <Stack direction="row" spacing={1} alignItems="center">
            <Typography
              variant="caption"
              sx={{ flex: 1, minWidth: 0, fontFamily: "monospace", wordBreak: "break-all" }}
            >
              {c.call}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {itemCount(c.items)}
            </Typography>
            <OutcomeChip outcome={c.outcome} />
          </Stack>
          {c.message && (
            <Typography variant="caption" color="text.secondary" component="p">
              {c.message}
            </Typography>
          )}
        </li>
      ))}
    </Box>
  );
}

const monoCaptionSx = { fontFamily: "monospace", wordBreak: "break-all" } as const;

/**
 * A plain scrollable list: a scan can report thousands of repositories, so
 * rows are bare list items rather than components.
 */
const repoListSx = {
  typography: "caption",
  m: 0,
  p: 0,
  listStyle: "none",
  maxHeight: 320,
  overflowY: "auto",
  "& .repo": monoCaptionSx,
  "& .reason": { color: "text.secondary" },
  "& li + li": { mt: 0.5 },
} as const;

/** The repositories a source code scan scanned and could not scan. */
function ScanRepositories({ result }: { result: ShadowScanConnectionResult }) {
  const [showScanned, setShowScanned] = useState(false);
  const listId = useId();
  const scanned = result.scannedRepositories ?? [];
  const failed = result.failedRepositories ?? [];
  if (scanned.length === 0 && failed.length === 0) return null;
  const n = scanned.length;
  return (
    <Box sx={{ mt: 1 }}>
      <Typography variant="caption" color="text.secondary" component="p">
        Repositories: {formatCount(n)} scanned
        {failed.length > 0 && `, ${formatCount(failed.length)} failed`}
      </Typography>
      {failed.length > 0 && (
        <Box
          component="ul"
          aria-label="Failed repositories"
          tabIndex={0}
          sx={{ ...repoListSx, mt: 1 }}
        >
          {failed.map((f) => (
            <li key={f.name}>
              <div className="repo">{f.name}</div>
              {f.message && <div className="reason">{f.message}</div>}
            </li>
          ))}
        </Box>
      )}
      {n > 0 && (
        <>
          <Button
            variant="text"
            size="small"
            sx={{ px: 0 }}
            aria-expanded={showScanned}
            aria-controls={listId}
            onClick={() => setShowScanned((v) => !v)}
          >
            {showScanned
              ? "Hide scanned repositories"
              : `Show ${formatCount(n)} scanned ${n === 1 ? "repository" : "repositories"}`}
          </Button>
          {showScanned && (
            <Box
              component="ul"
              id={listId}
              aria-label="Scanned repositories"
              tabIndex={0}
              sx={repoListSx}
            >
              {scanned.map((name) => (
                <li key={name} className="repo">
                  {name}
                </li>
              ))}
            </Box>
          )}
        </>
      )}
    </Box>
  );
}

function ConnectorRow({ row, pending }: { row: ConnectorProgress; pending: boolean }) {
  const [showSteps, setShowSteps] = useState(false);
  const listId = useId();
  const r = row.result;
  const calls = r?.calls ?? [];
  const last = calls[calls.length - 1];
  const found = r?.findingsCount ?? 0;
  const cancelled = !!r && isCancelledResult(r);
  let detail = providerLabel(row.provider);
  if (row.state === "waiting") {
    detail += pending ? " · Waiting for the scan command's results" : " · Not started";
  }
  if (row.state === "scanning" && r?.startedAt) {
    detail += ` · Started ${relTime(r.startedAt)}`;
  }
  if (row.state === "done") {
    detail += cancelled
      ? " · Cancelled before it finished"
      : ` · Done · ${found} ${found === 1 ? "agent" : "agents"} found`;
  }
  return (
    <Box component="li" sx={{ px: 2, py: 1.5 }} aria-label={row.name}>
      <Stack direction="row" spacing={1.5} alignItems="center">
        <ProviderLogo provider={row.provider} size={20} />
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Typography variant="body2" fontWeight={500} noWrap>
            {row.name}
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {detail}
          </Typography>
        </Box>
        <ConnectionResultChip result={r ?? { outcome: "queued" }} />
      </Stack>
      <Box sx={{ pl: 4.5, mt: 0.5 }}>
        {row.state === "scanning" && (
          <Typography variant="caption" color="text.secondary" component="p">
            {last ? `Last step: ${last.call} (${callDetail(last)})` : "Starting…"}
          </Typography>
        )}
        {row.state === "done" && r?.message && !cancelled && (
          <Typography variant="caption" color="text.secondary" component="p">
            {r.message}
          </Typography>
        )}
        {calls.length > 0 && (
          <>
            <Button
              variant="text"
              size="small"
              sx={{ px: 0 }}
              aria-expanded={showSteps}
              aria-controls={listId}
              onClick={() => setShowSteps((v) => !v)}
            >
              {showSteps
                ? "Hide steps"
                : `Show ${calls.length} ${calls.length === 1 ? "step" : "steps"}${
                    row.state === "scanning" ? " done" : ""
                  }`}
            </Button>
            {showSteps && <CallList calls={calls} id={listId} />}
          </>
        )}
        {r && SOURCE_HOSTS.includes(row.provider) && <ScanRepositories result={r} />}
      </Box>
    </Box>
  );
}

/** The card's summary line: how far the scan got, or how it ended. */
function summaryLine(scan: ShadowScanResponse, rows: ConnectorProgress[]): string {
  const n = rows.length;
  const connectors = `${n} ${n === 1 ? "connector" : "connectors"}`;
  const done = rows.filter((r) => r.state === "done").length;
  if (isShadowScanActive(scan.status)) {
    return `${done} of ${connectors} done. The agents found show up below as each connector finishes.`;
  }
  if (scan.status === "pending") return "Waiting for the results of its scan command.";
  const agents = `${scan.findingsCount} ${scan.findingsCount === 1 ? "agent" : "agents"} found`;
  if (scan.inventoryUpload) return `Inventory file uploaded · ${agents}.`;
  if (isCancelledScan(scan)) {
    return `${finishedCount(scan)} of ${connectors} finished before the scan was cancelled · ${agents}.`;
  }
  return `${connectors} scanned · ${agents}.`;
}

/**
 * A scan's progress, kept after it ends: when it started, how long it ran, and
 * each connector, waiting, being scanned or done, with the steps it took.
 */
export function ScanProgressCard({
  scan,
  connections,
}: {
  scan: ShadowScanResponse;
  connections: ShadowConnectionResponse[];
}) {
  const active = isShadowScanActive(scan.status);
  const now = useNow(active);
  const rows = connectorProgress(scan, connections);
  const started = scan.startedAt;
  const end = active ? now : scan.finishedAt ? new Date(scan.finishedAt).getTime() : NaN;
  const ran = started ? formatElapsed(end - new Date(started).getTime()) : "";
  const timing = started
    ? `Started ${relTime(started)}${ran ? ` · ${ran} ${active ? "elapsed" : "taken"}` : ""}`
    : `${scan.status === "pending" ? "Created" : "Queued"} ${relTime(scan.createdAt)}`;
  return (
    <Card variant="outlined">
      <CardContent>
        <Stack direction="row" spacing={1.5} alignItems="center" flexWrap="wrap" useFlexGap>
          <Typography variant="subtitle1">Progress</Typography>
          <ScanStatusChip status={scan.status} cancelled={isCancelledScan(scan)} />
          <Typography variant="body2" color="text.secondary">
            {timing}
          </Typography>
        </Stack>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          {summaryLine(scan, rows)}
        </Typography>
      </CardContent>
      <Divider />
      <Stack
        component="ul"
        aria-label="Connectors"
        divider={<Divider component="li" aria-hidden />}
        sx={{ m: 0, p: 0, listStyle: "none" }}
      >
        {rows.map((row) => (
          <ConnectorRow key={row.connectionId} row={row} pending={scan.status === "pending"} />
        ))}
      </Stack>
    </Card>
  );
}
