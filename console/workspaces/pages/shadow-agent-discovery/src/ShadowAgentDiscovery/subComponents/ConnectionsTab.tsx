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

import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  ListingTable,
  SearchBar,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Pencil,
  PlugZap,
  Plus,
  Radar,
  SquareTerminal,
  Trash2,
  TriangleAlert,
  X,
} from "@wso2/oxygen-ui-icons-react";
import {
  isShadowScanActive,
  useDeleteShadowConnection,
  useShadowConnections,
  useShadowProviders,
  useShadowSummary,
  useStartShadowScan,
  useTestShadowConnection,
} from "@agent-management-platform/api-client";
import {
  formatRelativeTime,
  isListEmpty,
  ResourceListShell,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";
import { returnToState } from "@agent-management-platform/views";
import type {
  ShadowConnectionResponse,
  ShadowConnectionTest,
  ShadowScanResponse,
} from "@agent-management-platform/types";
import {
  CUSTOM_PLATFORM,
  OutcomeChip,
  providerLabel,
  SOURCE_HOSTS,
  sourceScopeLabel,
  unlockLabels,
} from "../shadowUi";
import { scopedKeyCount } from "../capabilities";
import { ProviderLogo } from "../providerLogos";
import { GenerateCommandDialog } from "./GenerateCommandDialog";
import { RunScanButton } from "./RunScanButton";
import { scanFindingsPath } from "./ScansTab";
import { isCancelledScan, progressLine } from "./ScanProgress";
import { useConfirmCancelScan } from "./UploadResultsDialog";

/** The number of keys an OpenAI or Anthropic platform lists agents with. */
export function ScopedKeysChip({
  provider,
  secretHints,
}: {
  provider: string;
  secretHints: Record<string, string> | undefined;
}) {
  const n = scopedKeyCount(provider, secretHints);
  if (n === undefined) return null;
  const unit = provider === "anthropic" ? "workspace" : "project";
  return (
    <Tooltip title={`Agents are listed in each ${unit} that has a key.`}>
      <Chip size="small" variant="outlined" label={`${n} ${unit} ${n === 1 ? "key" : "keys"}`} />
    </Tooltip>
  );
}

/** A section of the Platforms tab, with what scans that kind of platform. */
function SectionHeading({
  title,
  description,
  action,
}: {
  title: string;
  description: string;
  action?: React.ReactNode;
}) {
  return (
    <Stack direction="row" spacing={2} alignItems="flex-end">
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography variant="subtitle1">{title}</Typography>
        <Typography variant="body2" color="text.secondary">
          {description}
        </Typography>
      </Box>
      {action && <Box sx={{ flexShrink: 0 }}>{action}</Box>}
    </Stack>
  );
}

/** The connectors a scan covers: their names when there are few, else how many. */
function scanTargets(ids: string[], connections: ShadowConnectionResponse[]): string {
  const names = ids
    .map((id) => connections.find((c) => c.id === id)?.name)
    .filter((n): n is string => !!n);
  const n = ids.length;
  if (names.length === n && n > 0 && n <= 2) return names.join(" and ");
  return `${n} ${n === 1 ? "connector" : "connectors"}`;
}

/**
 * Above the tables while a scan runs, so a disabled Scan button is explained: what is
 * being scanned, since when, how far it got, and a way to the scan or to cancel it.
 * Once a scan seen running here finishes, it says how it went until dismissed.
 */
function ScanProgressNotice({
  scan,
  running,
  connections,
  onView,
  onCancel,
  onDismiss,
}: {
  scan: ShadowScanResponse;
  running: boolean;
  connections: ShadowConnectionResponse[];
  onView: () => void;
  /** Set when the user may cancel the scan. */
  onCancel?: () => void;
  onDismiss: () => void;
}) {
  const view = (
    <Button color="inherit" size="small" onClick={onView}>
      View scan
    </Button>
  );
  const targets = scanTargets(scan.connectionIds, connections);
  if (running) {
    return (
      <Alert
        severity="info"
        icon={<CircularProgress size={18} color="inherit" />}
        action={
          <Stack direction="row" spacing={0.5} alignItems="center">
            {view}
            {onCancel && (
              <Button color="error" size="small" onClick={onCancel}>
                Cancel scan
              </Button>
            )}
          </Stack>
        }
      >
        Scanning {targets}, started{" "}
        {formatRelativeTime(scan.startedAt ?? scan.createdAt).replace(/^Just now$/, "just now")}.
        {/* Inline, so it shares the line when it fits and wraps only when it doesn't. */}{" "}
        {progressLine(scan, connections)}
      </Alert>
    );
  }
  const found = `${scan.findingsCount} ${scan.findingsCount === 1 ? "agent" : "agents"} found`;
  const outcome = isCancelledScan(scan)
    ? {
        severity: "info" as const,
        text:
          scan.findingsCount > 0
            ? `Scan of ${targets} cancelled: ${found} before it stopped.`
            : `Scan of ${targets} cancelled.`,
      }
    : scan.status === "succeeded"
      ? { severity: "success" as const, text: `Scan of ${targets} finished: ${found}.` }
      : scan.status === "partial"
        ? {
            severity: "warning" as const,
            text: `Scan of ${targets} finished, but some calls failed: ${found}.`,
          }
        : { severity: "error" as const, text: `Scan of ${targets} failed.` };
  return (
    <Alert
      severity={outcome.severity}
      action={
        <Stack direction="row" spacing={0.5} alignItems="center">
          {view}
          <IconButton size="small" color="inherit" aria-label="Dismiss" onClick={onDismiss}>
            <X size={16} />
          </IconButton>
        </Stack>
      }
    >
      {outcome.text}
    </Alert>
  );
}

interface ConnectionsTabProps {
  orgName: string | undefined;
  canManage: boolean;
  onAdd: () => void;
  onEdit: (c: ShadowConnectionResponse) => void;
}

/**
 * The platforms, in two sections: the AI platforms Agent Manager scans (how
 * their last test went, and a Scan action to scan one on its own), and the
 * source code repositories a scan command scans on the customer's machine.
 */
export function ConnectionsTab({ orgName, canManage, onAdd, onEdit }: ConnectionsTabProps) {
  const [search, setSearch] = useState("");
  const [commandFor, setCommandFor] = useState<ShadowConnectionResponse | null>(null);
  const [testResult, setTestResult] = useState<{
    name: string;
    result: ShadowConnectionTest;
  } | null>(null);
  const { data, isLoading, error } = useShadowConnections(orgName);
  const remove = useDeleteShadowConnection(orgName);
  const test = useTestShadowConnection(orgName);
  const scan = useStartShadowScan(orgName);
  const summary = useShadowSummary(orgName);
  const latestScan = summary.data?.latestScan;
  const scanActive = isShadowScanActive(latestScan?.status);
  // A scan started or seen running on this tab: the notice follows it until it ends,
  // then says how it went. Until the summary catches up, it is the scan just started.
  const [watchedScan, setWatchedScan] = useState<ShadowScanResponse | null>(null);
  useEffect(() => {
    if (scanActive && latestScan) setWatchedScan((w) => (w?.id === latestScan.id ? w : latestScan));
  }, [scanActive, latestScan]);
  const showScan = scanActive
    ? latestScan
    : watchedScan && latestScan?.id === watchedScan.id
      ? latestScan
      : (watchedScan ?? undefined);
  const scanRunning = !!showScan && isShadowScanActive(showScan.status);
  // The connectors of a scan this tab is starting, until the request returns.
  const [starting, setStarting] = useState<string[] | null>(null);
  // Only the connectors still waiting or being scanned: a finished one loses its chip.
  const unfinished = scanRunning
    ? showScan.connectionIds.filter((id) => {
        const o = showScan.results.find((r) => r.connectionId === id)?.outcome;
        return !o || o === "queued" || o === "running";
      })
    : [];
  const scanning = new Set([...unfinished, ...(starting ?? [])]);
  const navigate = useNavigate();
  const location = useLocation();
  const providers = useShadowProviders(orgName);
  const signalNames = useMemo(
    () => new Map((providers.data?.signals ?? []).map((d) => [d.code, d.name])),
    [providers.data],
  );
  const { addConfirmation } = useConfirmationDialog();
  const confirmCancel = useConfirmCancelScan(orgName);
  const connections = useMemo(() => data?.connections ?? [], [data]);
  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return connections;
    return connections.filter(
      (c) =>
        c.name.toLowerCase().includes(q) || providerLabel(c.provider).toLowerCase().includes(q),
    );
  }, [connections, search]);

  const addButton = canManage ? (
    <Button variant="outlined" startIcon={<Plus size={16} />} onClick={onAdd}>
      Add source
    </Button>
  ) : undefined;

  const aiPlatforms = filtered.filter(
    (c) => !SOURCE_HOSTS.includes(c.provider) && c.provider !== CUSTOM_PLATFORM,
  );
  const customPlatforms = filtered.filter((c) => c.provider === CUSTOM_PLATFORM);
  const sourceHosts = filtered.filter((c) => SOURCE_HOSTS.includes(c.provider));
  // Run scan sits with the connectors a scan calls: the AI platforms, or Bring your
  // own inventory when there is no AI platform. Never with the source code repositories.
  const runScan = canManage ? <RunScanButton orgName={orgName} /> : undefined;

  const handleDelete = (c: ShadowConnectionResponse) =>
    addConfirmation({
      title: `Delete ${c.name}?`,
      description: SOURCE_HOSTS.includes(c.provider)
        ? "Every agent found in these repositories is deleted from Agent Manager. The repositories are not changed."
        : "The stored credential and every agent this connector found are deleted from Agent Manager. The platform account is not changed.",
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      onConfirm: () => remove.mutate(c.id),
    });

  const handleTest = (c: ShadowConnectionResponse) =>
    test.mutate(c.id, { onSuccess: (result) => setTestResult({ name: c.name, result }) });

  // The table of platforms a scan calls (AI platforms and Bring your own inventory):
  // Scan, Test connector, Edit and Delete on each row.
  const cloudTable = (list: ShadowConnectionResponse[]) => (
      <ListingTable>
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell>Name</ListingTable.Cell>
            <ListingTable.Cell>Source</ListingTable.Cell>
            <ListingTable.Cell>Last test</ListingTable.Cell>
            <ListingTable.Cell>Created</ListingTable.Cell>
            <ListingTable.Cell align="right" />
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {list.map((c) => (
            <ListingTable.Row key={c.id}>
              <ListingTable.Cell>
                <Stack direction="row" spacing={1} alignItems="center">
                  <Typography variant="body2" fontWeight={500}>
                    {c.name}
                  </Typography>
                  {scanning.has(c.id) && (
                    <Chip
                      size="small"
                      variant="outlined"
                      color="info"
                      icon={<CircularProgress size={12} color="inherit" />}
                      label="Scanning"
                    />
                  )}
                  {c.endpointWarnings.length > 0 && (
                    <Tooltip title={c.endpointWarnings.join(". ")}>
                      <Box
                        component="span"
                        aria-label="Custom endpoint"
                        sx={{ display: "inline-flex", color: "warning.main" }}
                      >
                        <TriangleAlert size={16} />
                      </Box>
                    </Tooltip>
                  )}
                </Stack>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Stack
                  direction="row"
                  spacing={1}
                  alignItems="center"
                  flexWrap="wrap"
                  useFlexGap
                >
                  <ProviderLogo provider={c.provider} size={18} />
                  <Typography variant="body2">{providerLabel(c.provider)}</Typography>
                  <ScopedKeysChip provider={c.provider} secretHints={c.secretHints} />
                </Stack>
              </ListingTable.Cell>
              <ListingTable.Cell>
                {c.lastTest ? (
                  <Stack direction="row" spacing={1} alignItems="center">
                    <OutcomeChip outcome={c.lastTest.ok ? "ok" : "failed"} />
                    <Typography variant="caption" color="text.secondary">
                      {c.lastTestedAt ? formatRelativeTime(c.lastTestedAt) : ""}
                    </Typography>
                  </Stack>
                ) : (
                  <Typography variant="body2" color="text.secondary">
                    Not tested
                  </Typography>
                )}
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography variant="body2" color="text.secondary">
                  {formatRelativeTime(c.createdAt)}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell align="right">
                {canManage && (
                  <Stack direction="row" spacing={0.5} justifyContent="flex-end">
                    <Tooltip
                      title={
                        scanning.has(c.id)
                          ? "Scanning this connector"
                          : scanRunning
                            ? "Another scan is running. Wait for it to finish."
                            : "Scan this connector"
                      }
                    >
                      <span>
                        <IconButton
                          size="small"
                          aria-label={`Scan ${c.name}`}
                          disabled={scanRunning || scan.isPending}
                          onClick={() => {
                            setStarting([c.id]);
                            scan.mutate(
                              { connectionIds: [c.id] },
                              {
                                onSuccess: (started) => setWatchedScan(started),
                                onSettled: () => setStarting(null),
                              },
                            );
                          }}
                        >
                          <Radar size={18} />
                        </IconButton>
                      </span>
                    </Tooltip>
                    <Tooltip title="Test connector">
                      <span>
                        <IconButton
                          size="small"
                          aria-label={`Test ${c.name}`}
                          disabled={test.isPending}
                          onClick={() => handleTest(c)}
                        >
                          <PlugZap size={18} />
                        </IconButton>
                      </span>
                    </Tooltip>
                    <Tooltip title="Edit">
                      <IconButton
                        size="small"
                        aria-label={`Edit ${c.name}`}
                        onClick={() => onEdit(c)}
                      >
                        <Pencil size={18} />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="Delete">
                      <IconButton
                        size="small"
                        aria-label={`Delete ${c.name}`}
                        onClick={() => handleDelete(c)}
                      >
                        <Trash2 size={18} />
                      </IconButton>
                    </Tooltip>
                  </Stack>
                )}
              </ListingTable.Cell>
            </ListingTable.Row>
          ))}
        </ListingTable.Body>
      </ListingTable>
  );

  const toolbar = (
    <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5} alignItems={{ sm: "center" }}>
      <Box sx={{ width: { xs: "100%", sm: 400 } }}>
        <SearchBar
          placeholder="Search connectors"
          size="small"
          fullWidth
          value={search}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => setSearch(e.target.value)}
        />
      </Box>
      <Box sx={{ flex: 1 }} />
      {addButton}
    </Stack>
  );

  return (
    <>
      <ResourceListShell
        resourceName="shadow-agent-platforms"
        disablePaper
        searchValue={search}
        onSearchChange={setSearch}
        toolbar={toolbar}
        error={
          error
            ? new Error("Couldn't load the connectors. Reload the page to try again.")
            : undefined
        }
        isLoading={isLoading}
        isEmpty={isListEmpty({ isLoading, count: connections.length, isFiltered: !!search })}
        isSearchEmpty={!isLoading && connections.length > 0 && filtered.length === 0}
        emptyState={{
          illustration: <PlugZap size={64} />,
          title: "No sources added",
          description:
            "Connect an AI platform (OpenAI, Anthropic, Amazon Bedrock, Google Agent Platform, Microsoft Foundry) with a read-only credential, your own agent inventory with Bring your own inventory, or a GitHub organization to scan its source code.",
          action: addButton,
        }}
        searchEmptyState={{
          illustration: <PlugZap size={64} />,
          title: "No connectors match your search",
          description: "Try another name or source.",
        }}
      >
        <Stack spacing={4}>
          {starting && !scanRunning ? (
            <Alert severity="info" icon={<CircularProgress size={18} color="inherit" />}>
              Starting a scan of {scanTargets(starting, connections)}…
            </Alert>
          ) : (
            showScan && (
              <ScanProgressNotice
                scan={showScan}
                running={scanRunning}
                connections={connections}
                onView={() =>
                  navigate(scanFindingsPath(orgName ?? "", showScan.id), {
                    state: returnToState(location),
                  })
                }
                onCancel={canManage && scanRunning ? () => confirmCancel(showScan) : undefined}
                onDismiss={() => setWatchedScan(null)}
              />
            )
          )}
          {aiPlatforms.length > 0 && (
            <Stack spacing={1.5}>
              <SectionHeading
                title="AI platforms"
                description="Agent Manager reads their agent listings with a read-only credential when you run a scan."
                action={runScan}
              />
              {cloudTable(aiPlatforms)}
            </Stack>
          )}
          {sourceHosts.length > 0 && (
            <Stack spacing={1.5}>
              <SectionHeading
                title="Source code repositories"
                description="Scanned by a command you run on your machine. No token is stored in Agent Manager."
              />
              <ListingTable>
                <ListingTable.Head>
                  <ListingTable.Row>
                    <ListingTable.Cell>Name</ListingTable.Cell>
                    <ListingTable.Cell>Source</ListingTable.Cell>
                    <ListingTable.Cell>What it scans</ListingTable.Cell>
                    <ListingTable.Cell>Created</ListingTable.Cell>
                    <ListingTable.Cell align="right" />
                  </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                  {sourceHosts.map((c) => (
                    <ListingTable.Row key={c.id}>
                      <ListingTable.Cell>
                        <Typography variant="body2" fontWeight={500}>
                          {c.name}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Stack direction="row" spacing={1} alignItems="center">
                          <ProviderLogo provider={c.provider} size={18} />
                          <Typography variant="body2">{providerLabel(c.provider)}</Typography>
                        </Stack>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="body2">
                          {sourceScopeLabel(c.provider, c.settings)}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="body2" color="text.secondary">
                          {formatRelativeTime(c.createdAt)}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell align="right">
                        {canManage && (
                          <Stack direction="row" spacing={0.5} justifyContent="flex-end">
                            <Tooltip title="Generate command">
                              <IconButton
                                size="small"
                                aria-label={`Generate command for ${c.name}`}
                                onClick={() => setCommandFor(c)}
                              >
                                <SquareTerminal size={18} />
                              </IconButton>
                            </Tooltip>
                            <Tooltip title="Edit">
                              <IconButton
                                size="small"
                                aria-label={`Edit ${c.name}`}
                                onClick={() => onEdit(c)}
                              >
                                <Pencil size={18} />
                              </IconButton>
                            </Tooltip>
                            <Tooltip title="Delete">
                              <IconButton
                                size="small"
                                aria-label={`Delete ${c.name}`}
                                onClick={() => handleDelete(c)}
                              >
                                <Trash2 size={18} />
                              </IconButton>
                            </Tooltip>
                          </Stack>
                        )}
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  ))}
                </ListingTable.Body>
              </ListingTable>
            </Stack>
          )}
          {customPlatforms.length > 0 && (
            <Stack spacing={1.5}>
              <SectionHeading
                title="Bring your own inventory"
                description="Your own agent inventory APIs. Agent Manager calls each one's Agents URL with its API key when you run a scan."
                action={aiPlatforms.length === 0 ? runScan : undefined}
              />
              {cloudTable(customPlatforms)}
            </Stack>
          )}
        </Stack>
      </ResourceListShell>

      <GenerateCommandDialog
        orgName={orgName}
        target={commandFor}
        spec={providers.data?.providers.find((p) => p.provider === commandFor?.provider)}
        onClose={() => setCommandFor(null)}
      />

      <Dialog open={!!testResult} onClose={() => setTestResult(null)} maxWidth="sm" fullWidth>
        <DialogTitle>Test of {testResult?.name}</DialogTitle>
        <DialogContent>
          <Stack spacing={1.5}>
            <Typography variant="body2" color="text.secondary">
              One read per agent listing. A failed check names the agents a scan will miss.
            </Typography>
            {testResult?.result.checks.map((ch) => (
              <Box
                key={ch.name}
                sx={{ border: 1, borderColor: "divider", borderRadius: 1, p: 1.5 }}
              >
                <Stack
                  direction="row"
                  spacing={1}
                  alignItems="center"
                  justifyContent="space-between"
                >
                  <Typography variant="body2" fontWeight={500}>
                    {ch.name}
                  </Typography>
                  <OutcomeChip outcome={ch.outcome} />
                </Stack>
                {ch.message && (
                  <Typography variant="caption" color="text.secondary" component="p">
                    {ch.message}
                  </Typography>
                )}
                {ch.unlocks && ch.unlocks.length > 0 && (
                  <Typography variant="caption" color="text.secondary" component="p">
                    Lists: {unlockLabels(ch.unlocks, signalNames).join(", ")}
                  </Typography>
                )}
              </Box>
            ))}
          </Stack>
        </DialogContent>
        <DialogActions sx={{ justifyContent: "flex-start", px: 3, pb: 2 }}>
          <Button variant="outlined" onClick={() => setTestResult(null)}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}

export default ConnectionsTab;
