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
import { createPortal } from "react-dom";
import {
  Button,
  IconButton,
  ListingTable,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Ban, History, Library, Trash2, Upload } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useLocation, useNavigate } from "react-router-dom";
import { returnToState } from "@agent-management-platform/views";
import {
  isShadowScanActive,
  useClearShadowScans,
  useShadowScans,
} from "@agent-management-platform/api-client";
import {
  formatRelativeTime,
  isListEmpty,
  ResourceListShell,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";
import { absoluteRouteMap, type ShadowScanResponse } from "@agent-management-platform/types";
import { ScanStatusChip, unmanagedAgentsPath } from "../shadowUi";
import { UploadResultsDialog, useConfirmCancelScan } from "./UploadResultsDialog";
import { finishedCount, isCancelledScan } from "./ScanProgress";

/** The path of a scan's page: its progress and its agents. */
export function scanFindingsPath(orgName: string, scanId: string): string {
  return generatePath(
    absoluteRouteMap.children.org.children.shadowAgentDiscovery.children.scan.path,
    {
      orgId: orgName,
      scanId,
    },
  );
}

/** Whether a scan can be cancelled: waiting for results, queued or running. */
export function isCancellable(s: ShadowScanResponse): boolean {
  return s.status === "pending" || isShadowScanActive(s.status);
}

/**
 * The connectors a scan covered: their names once it ran, else how many. A
 * queued connector may have no name yet.
 */
function scannedLabel(s: ShadowScanResponse): string {
  const names = s.results.map((r) => r.connectionName);
  if (names.length > 0 && names.every(Boolean)) return names.join(", ");
  const n = s.connectionIds.length;
  return `${n} ${n === 1 ? "connector" : "connectors"}`;
}

/**
 * The scan history, newest first. A row opens the scan's page: its progress per
 * connector and the agents it found.
 */
export function ScansTab({
  orgName,
  canManage,
  actionsSlot,
}: {
  orgName: string | undefined;
  canManage: boolean;
  /** Where the tab's actions go (the page's tab row); without it they sit above the table. */
  actionsSlot?: HTMLElement | null;
}) {
  const navigate = useNavigate();
  const location = useLocation();
  const { data, isLoading, error } = useShadowScans(orgName);
  const clear = useClearShadowScans(orgName);
  const { addConfirmation } = useConfirmationDialog();
  const scans = data?.scans ?? [];
  const openScan = (s: ShadowScanResponse) =>
    navigate(scanFindingsPath(orgName ?? "", s.id), { state: returnToState(location) });
  // Clear scans keeps a scan that is queued, running or waiting for results.
  const finished = scans.filter(
    (s) => !isShadowScanActive(s.status) && s.status !== "pending",
  ).length;
  const active = scans.length - finished;
  // Upload results: for one scan (a row), or for the scan the file names.
  const [upload, setUpload] = useState<{ scan: ShadowScanResponse | null } | null>(null);
  const confirmCancel = useConfirmCancelScan(orgName);

  const handleClear = () =>
    addConfirmation({
      title: "Clear all scans?",
      description:
        `Every finished scan is deleted, with its call results and its list of agents.${
          active > 0 ? " Scans that are queued, running or waiting for results are kept." : ""
        } Unmanaged Agents and their evidence are kept, and the next scan updates them as usual. ` +
        "This can't be undone.",
      confirmButtonText: "Clear scans",
      confirmButtonColor: "error",
      onConfirm: () => clear.mutate(),
    });

  // Small actions; Clear scans is quiet (its dialog carries the warning). They sit in the
  // page's tab row when the page gives a slot for them, else above the table.
  const actions = (
    <Stack direction="row" spacing={1} alignItems="center" justifyContent="flex-end">
      {/* The agents the scans found are listed in the agent registry. */}
      <Button
        variant="text"
        size="small"
        startIcon={<Library size={16} />}
        onClick={() => navigate(unmanagedAgentsPath(orgName ?? ""))}
      >
        View unmanaged agents
      </Button>
      {canManage && (
        <>
          <Button
            variant="outlined"
            size="small"
            startIcon={<Upload size={16} />}
            onClick={() => setUpload({ scan: null })}
          >
            Upload results
          </Button>
          <Button
            variant="text"
            size="small"
            startIcon={<Trash2 size={16} />}
            disabled={finished === 0 || clear.isPending}
            onClick={handleClear}
            sx={{ color: "text.secondary" }}
          >
            Clear scans
          </Button>
        </>
      )}
    </Stack>
  );
  // Not undefined when the actions are in the slot: the list shell would show its search bar.
  const toolbar = actionsSlot ? <></> : actions;

  return (
    <>
      {actionsSlot && createPortal(actions, actionsSlot)}
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
          description: "Run a scan from the top of this page once a source is added.",
        }}
      >
        <ListingTable>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell>Started</ListingTable.Cell>
              <ListingTable.Cell>Status</ListingTable.Cell>
              <ListingTable.Cell>Connectors scanned</ListingTable.Cell>
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
                onClick={() => openScan(s)}
              >
                <ListingTable.Cell>
                  <Typography variant="body2">
                    {formatRelativeTime(s.startedAt ?? s.createdAt)}
                  </Typography>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
                    <ScanStatusChip status={s.status} cancelled={isCancelledScan(s)} />
                    {s.status === "running" && (
                      <Typography variant="caption" color="text.secondary">
                        {finishedCount(s)} of {s.connectionIds.length}{" "}
                        {s.connectionIds.length === 1 ? "connector" : "connectors"} done
                      </Typography>
                    )}
                  </Stack>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Typography variant="body2">{scannedLabel(s)}</Typography>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Typography variant="body2">{s.findingsCount}</Typography>
                </ListingTable.Cell>
                <ListingTable.Cell align="right">
                  <Stack
                    direction="row"
                    spacing={0.5}
                    justifyContent="flex-end"
                    alignItems="center"
                  >
                    {canManage && s.status === "pending" && (
                      <Tooltip title="Upload results">
                        <IconButton
                          size="small"
                          aria-label={`Upload results of the scan started ${formatRelativeTime(
                          s.startedAt ?? s.createdAt,
                        )}`}
                          onClick={(e) => {
                            e.stopPropagation();
                            setUpload({ scan: s });
                          }}
                        >
                          <Upload size={16} />
                        </IconButton>
                      </Tooltip>
                    )}
                    {canManage && isCancellable(s) && (
                      <Tooltip title="Cancel scan">
                        <IconButton
                          size="small"
                          aria-label={`Cancel the scan started ${formatRelativeTime(
                          s.startedAt ?? s.createdAt,
                        )}`}
                          onClick={(e) => {
                            e.stopPropagation();
                            confirmCancel(s);
                          }}
                        >
                          <Ban size={16} />
                        </IconButton>
                      </Tooltip>
                    )}
                    <Button
                      variant="outlined"
                      size="small"
                      sx={{ whiteSpace: "nowrap", flexShrink: 0 }}
                      aria-label={`View the scan started ${formatRelativeTime(
                        s.startedAt ?? s.createdAt,
                      )}`}
                      onClick={(e) => {
                        e.stopPropagation();
                        openScan(s);
                      }}
                    >
                      View scan
                    </Button>
                  </Stack>
                </ListingTable.Cell>
              </ListingTable.Row>
            ))}
          </ListingTable.Body>
        </ListingTable>
      </ResourceListShell>

      <UploadResultsDialog
        orgName={orgName}
        open={!!upload}
        scan={upload?.scan}
        scans={scans}
        onClose={() => setUpload(null)}
      />
    </>
  );
}

export default ScansTab;
