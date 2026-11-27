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

import { useEffect, useRef, useState } from "react";
import { Button, Skeleton, Stack } from "@wso2/oxygen-ui";
import { SearchX, Upload } from "@wso2/oxygen-ui-icons-react";
import { useParams } from "react-router-dom";
import { PageLayout, StateNotice, useReturnTo } from "@agent-management-platform/views";
import {
  useRefreshShadowFindings,
  useShadowConnections,
  useShadowScan,
} from "@agent-management-platform/api-client";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import { FindingsTab } from "./FindingsTab";
import { isCancellable } from "./ScansTab";
import { finishedCount, ScanProgressCard } from "./ScanProgress";
import { UploadResultsDialog, useConfirmCancelScan } from "./UploadResultsDialog";

/** One scan (scans/:scanId): its progress per connector, kept after it ends, and its agents. */
export function ScanFindings({
  orgName,
  listPath,
  canManage = false,
}: {
  orgName: string | undefined;
  listPath: string;
  /** May upload results to a scan waiting for results, or cancel a scan that has not finished. */
  canManage?: boolean;
}) {
  const { scanId } = useParams<{ scanId: string }>();
  const returnTo = useReturnTo(`${listPath}?tab=scans`);
  // Polls while the scan is queued or running.
  const scan = useShadowScan(orgName, scanId);
  // A queued connector may have no name or provider in the scan yet.
  const connections = useShadowConnections(orgName);
  const [uploadOpen, setUploadOpen] = useState(false);
  const confirmCancel = useConfirmCancelScan(orgName);
  // Only the scan is polled: refresh its agents (and the summary) when a connector
  // finishes or the scan ends, so they show up below without a reload.
  const refreshFindings = useRefreshShadowFindings(orgName);
  const progress = scan.data ? `${scan.data.status}:${finishedCount(scan.data)}` : undefined;
  const lastProgress = useRef(progress);
  useEffect(() => {
    const before = lastProgress.current;
    lastProgress.current = progress;
    if (before !== undefined && progress !== undefined && before !== progress) refreshFindings();
  }, [progress, refreshFindings]);
  const back = { backHref: returnTo, backLabel: "Back to scans" };

  if (scan.isLoading) {
    return (
      <PageLayout title="Scan" disableIcon {...back}>
        <Skeleton variant="rounded" height={320} />
      </PageLayout>
    );
  }
  if (scan.error || !scan.data) {
    return (
      <PageLayout title="Scan" disableIcon {...back}>
        <StateNotice
          icon={SearchX}
          title="Scan not found"
          description="The scan may have been cleared. Go back to the scans to pick another one."
        />
      </PageLayout>
    );
  }
  const s = scan.data;
  const started = formatRelativeTime(s.startedAt ?? s.createdAt);
  const waiting = canManage && s.status === "pending";
  const cancellable = canManage && isCancellable(s);

  return (
    <PageLayout
      title={`Scan — ${started}`}
      description="The agents this scan found, each once. An agent's details and review status are the latest."
      disableIcon
      {...back}
      actions={
        waiting || cancellable ? (
          <Stack direction="row" spacing={1}>
            {waiting && (
              <Button
                variant="contained"
                startIcon={<Upload size={16} />}
                onClick={() => setUploadOpen(true)}
              >
                Upload results
              </Button>
            )}
            {cancellable && (
              <Button variant="outlined" color="error" onClick={() => confirmCancel(s)}>
                Cancel scan
              </Button>
            )}
          </Stack>
        ) : undefined
      }
    >
      <Stack spacing={3}>
        <ScanProgressCard scan={s} connections={connections.data?.connections ?? []} />
        <FindingsTab
          orgName={orgName}
          scanId={s.id}
          emptyText={
            s.findingsCount > 0
              ? {
                  title: "Agents were not recorded for this scan",
                  description:
                    "This scan ran before Agent Manager recorded each scan's agents, so they cannot be listed here. Progress above has its steps; the next scan lists its agents.",
                }
              : undefined
          }
        />
      </Stack>
      <UploadResultsDialog
        orgName={orgName}
        open={uploadOpen}
        scan={s}
        scans={[s]}
        onClose={() => setUploadOpen(false)}
      />
    </PageLayout>
  );
}

export default ScanFindings;
