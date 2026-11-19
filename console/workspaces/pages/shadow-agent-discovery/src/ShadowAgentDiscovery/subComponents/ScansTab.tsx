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

import { useId, useState } from "react";
import {
  Box,
  Button,
  Divider,
  Drawer,
  IconButton,
  ListingTable,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { History, Trash2, X } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useLocation, useNavigate } from "react-router-dom";
import { returnToState } from "@agent-management-platform/views";
import {
  isShadowScanActive,
  useClearShadowScans,
  useShadowScan,
  useShadowScans,
} from "@agent-management-platform/api-client";
import {
  formatRelativeTime,
  isListEmpty,
  ResourceListShell,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  type ShadowScanConnectionResult,
  type ShadowScanResponse,
} from "@agent-management-platform/types";
import {
  formatCount,
  itemCount,
  OutcomeChip,
  providerLabel,
  ScanStatusChip,
  SOURCE_HOSTS,
} from "../shadowUi";

/** The path of a scan's agents page. */
export function scanFindingsPath(orgName: string, scanId: string): string {
  return generatePath(
    absoluteRouteMap.children.org.children.shadowAgentDiscovery.children.scan.path,
    {
      orgId: orgName,
      scanId,
    },
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

/** How loading the scan detail, which has the repositories, is going. */
export interface RepositoriesLoad {
  isLoading: boolean;
  failed: boolean;
}

/**
 * The repositories a source code scan scanned and could not scan. Only the
 * scan detail has them; a list item or a scan that never published has none.
 */
function ScanRepositories({
  result,
  load,
}: {
  result: ShadowScanConnectionResult;
  load?: RepositoriesLoad;
}) {
  const [showScanned, setShowScanned] = useState(false);
  const listId = useId();
  const scanned = result.scannedRepositories ?? [];
  const failed = result.failedRepositories ?? [];
  if (scanned.length === 0 && failed.length === 0) {
    if (!load || !SOURCE_HOSTS.includes(result.provider)) return null;
    const note = load.isLoading
      ? "Loading repositories…"
      : load.failed
        ? "Couldn't load the repositories. Close and reopen to try again."
        : "";
    if (!note) return null;
    return (
      <>
        <Divider />
        <Typography
          variant="caption"
          color="text.secondary"
          component="p"
          sx={{ px: 1.5, py: 1 }}
        >
          {note}
        </Typography>
      </>
    );
  }
  const n = scanned.length;
  return (
    <>
      <Divider />
      <Box sx={{ px: 1.5, py: 1 }}>
        <Typography variant="body2" fontWeight={500}>
          Repositories
        </Typography>
        <Typography variant="caption" color="text.secondary" component="p">
          {formatCount(n)} scanned
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
              sx={{ mt: 0.5, px: 0 }}
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
    </>
  );
}

/** The outcome of every platform call of a scan, per connection. */
export function ScanDetailsDrawer({
  scan,
  onClose,
  repositoriesLoad,
}: {
  scan: ShadowScanResponse | null;
  onClose: () => void;
  /** Set when the scan shown may be a list item, which has no repositories. */
  repositoriesLoad?: RepositoriesLoad;
}) {
  return (
    <Drawer anchor="right" open={!!scan} onClose={onClose}>
      <Box sx={{ width: { xs: "100vw", sm: 560 }, p: 3 }} role="dialog" aria-label="Scan details">
        <Stack direction="row" alignItems="center" justifyContent="space-between">
          <Typography variant="h6">Scan details</Typography>
          <IconButton aria-label="Close" onClick={onClose}>
            <X size={18} />
          </IconButton>
        </Stack>
        {scan && (
          <Stack spacing={2} sx={{ mt: 2 }}>
            <Stack direction="row" spacing={1} alignItems="center">
              <ScanStatusChip status={scan.status} />
              <Typography variant="body2" color="text.secondary">
                {scan.connectionIds.length}{" "}
                {scan.connectionIds.length === 1 ? "platform" : "platforms"} scanned
              </Typography>
            </Stack>
            {scan.results.length === 0 && (
              <Typography variant="body2" color="text.secondary">
                The scan has not finished any connection yet.
              </Typography>
            )}
            {scan.results.map((r) => (
              <Box key={r.connectionId} sx={{ border: 1, borderColor: "divider", borderRadius: 1 }}>
                <Stack direction="row" spacing={1} alignItems="center" sx={{ p: 1.5 }}>
                  <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography variant="body2" fontWeight={500} noWrap>
                      {r.connectionName || r.connectionId}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      {providerLabel(r.provider)} · {r.findingsCount}{" "}
                      {r.findingsCount === 1 ? "agent" : "agents"}
                    </Typography>
                  </Box>
                  <OutcomeChip outcome={r.outcome} />
                </Stack>
                {r.message && (
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    component="p"
                    sx={{ px: 1.5, pb: 1 }}
                  >
                    {r.message}
                  </Typography>
                )}
                {(r.calls ?? []).length > 0 && <Divider />}
                <Stack divider={<Divider />}>
                  {(r.calls ?? []).map((c) => (
                    <Box key={c.call} sx={{ px: 1.5, py: 1 }}>
                      <Stack direction="row" spacing={1} alignItems="center">
                        <Typography
                          variant="caption"
                          sx={{ flex: 1, minWidth: 0, ...monoCaptionSx }}
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
                    </Box>
                  ))}
                </Stack>
                <ScanRepositories result={r} load={repositoriesLoad} />
              </Box>
            ))}
          </Stack>
        )}
      </Box>
    </Drawer>
  );
}

/** The platforms a scan covered: their names once it ran, else how many. */
function scannedLabel(s: ShadowScanResponse): string {
  const names = s.results.map((r) => r.connectionName).filter(Boolean);
  if (names.length > 0) return names.join(", ");
  const n = s.connectionIds.length;
  return `${n} ${n === 1 ? "platform" : "platforms"}`;
}

/**
 * The scan history, newest first. A row opens the outcome of every platform
 * call; View agents opens the agents the scan found.
 */
export function ScansTab({
  orgName,
  canManage,
}: {
  orgName: string | undefined;
  canManage: boolean;
}) {
  const navigate = useNavigate();
  const location = useLocation();
  const { data, isLoading, error } = useShadowScans(orgName);
  const clear = useClearShadowScans(orgName);
  const { addConfirmation } = useConfirmationDialog();
  const [open, setOpen] = useState<ShadowScanResponse | null>(null);
  // The list omits a source code scan's repositories; the scan detail has them.
  const detail = useShadowScan(orgName, open?.id);
  const scans = data?.scans ?? [];
  const current = open
    ? detail.data?.id === open.id
      ? detail.data
      : (scans.find((s) => s.id === open.id) ?? open)
    : null;
  // Clear scans keeps a scan that is queued, running or pending (waiting for
  // its scan command to publish results).
  const finished = scans.filter(
    (s) => !isShadowScanActive(s.status) && s.status !== "pending",
  ).length;
  const active = scans.length - finished;

  const handleClear = () =>
    addConfirmation({
      title: "Clear all scans?",
      description:
        `Every finished scan is deleted, with its call results and its list of agents.${
          active > 0 ? " Scans that are queued, running or pending are kept." : ""
        } Unregistered Agents and their evidence are kept, and the next scan updates them as usual. ` +
        "This can't be undone.",
      confirmButtonText: "Clear scans",
      confirmButtonColor: "error",
      onConfirm: () => {
        setOpen(null);
        clear.mutate();
      },
    });

  const toolbar = canManage ? (
    <Stack direction="row" justifyContent="flex-end" sx={{ width: "100%" }}>
      <Button
        variant="outlined"
        color="error"
        startIcon={<Trash2 size={16} />}
        disabled={finished === 0 || clear.isPending}
        onClick={handleClear}
      >
        Clear scans
      </Button>
    </Stack>
  ) : (
    <span />
  );

  return (
    <>
      <ResourceListShell
        resourceName="shadow-agent-scans"
        disablePaper
        searchValue=""
        onSearchChange={() => undefined}
        toolbar={toolbar}
        error={error ? new Error("Couldn't load scans. Reload the page to try again.") : undefined}
        isLoading={isLoading}
        isEmpty={isListEmpty({ isLoading, count: scans.length, isFiltered: false })}
        emptyState={{
          illustration: <History size={64} />,
          title: "No scans yet",
          description: "Run a scan from the top of this page once a platform is added.",
        }}
      >
        <ListingTable>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell>Started</ListingTable.Cell>
              <ListingTable.Cell>Status</ListingTable.Cell>
              <ListingTable.Cell>Platforms scanned</ListingTable.Cell>
              <ListingTable.Cell>Agents found</ListingTable.Cell>
              <ListingTable.Cell align="right" />
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {scans.map((s) => (
              <ListingTable.Row
                key={s.id}
                hover
                sx={{ cursor: "pointer" }}
                onClick={() => setOpen(s)}
              >
                <ListingTable.Cell>
                  <Typography variant="body2">
                    {formatRelativeTime(s.startedAt ?? s.createdAt)}
                  </Typography>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <ScanStatusChip status={s.status} />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Typography variant="body2">{scannedLabel(s)}</Typography>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Typography variant="body2">{s.findingsCount}</Typography>
                </ListingTable.Cell>
                <ListingTable.Cell align="right">
                  <Button
                    variant="outlined"
                    size="small"
                    aria-label={`View agents of the scan started ${formatRelativeTime(
                      s.startedAt ?? s.createdAt,
                    )}`}
                    onClick={(e) => {
                      e.stopPropagation();
                      navigate(scanFindingsPath(orgName ?? "", s.id), {
                        state: returnToState(location),
                      });
                    }}
                  >
                    View agents
                  </Button>
                </ListingTable.Cell>
              </ListingTable.Row>
            ))}
          </ListingTable.Body>
        </ListingTable>
      </ResourceListShell>

      <ScanDetailsDrawer
        scan={current}
        onClose={() => setOpen(null)}
        repositoriesLoad={{ isLoading: detail.isLoading, failed: !!detail.error }}
      />
    </>
  );
}

export default ScansTab;
