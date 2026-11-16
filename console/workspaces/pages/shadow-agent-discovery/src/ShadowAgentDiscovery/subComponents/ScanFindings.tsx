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
import { Button, Skeleton, Stack, Typography } from "@wso2/oxygen-ui";
import { SearchX } from "@wso2/oxygen-ui-icons-react";
import { useParams } from "react-router-dom";
import { PageLayout, StateNotice, useReturnTo } from "@agent-management-platform/views";
import { useShadowScan } from "@agent-management-platform/api-client";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import { SourceRepoPreview } from "../../sourceRepoScan/SourceRepoPreview";
import { ScanStatusChip, SOURCE_HOSTS } from "../shadowUi";
import { FindingsTab } from "./FindingsTab";
import { ScanDetailsDrawer } from "./ScansTab";

/** The agents one scan found (scans/:scanId), with the scan's call details. */
export function ScanFindings({
  orgName,
  listPath,
}: {
  orgName: string | undefined;
  listPath: string;
}) {
  const { scanId } = useParams<{ scanId: string }>();
  const returnTo = useReturnTo(`${listPath}?tab=scans`);
  const scan = useShadowScan(orgName, scanId);
  const [detailsOpen, setDetailsOpen] = useState(false);
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
  const sourceHosts = s.results
    .filter((r) => SOURCE_HOSTS.includes(r.provider))
    .map((r) => ({ connectionName: r.connectionName || r.connectionId, provider: r.provider }));

  return (
    <PageLayout
      title={`Scan — ${started}`}
      description="The agents this scan found, each once. An agent's details and review status are the latest."
      disableIcon
      {...back}
      actions={
        <Button variant="outlined" onClick={() => setDetailsOpen(true)}>
          Scan details
        </Button>
      }
    >
      <Stack spacing={3}>
        <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
          <ScanStatusChip status={s.status} />
          <Typography variant="body2" color="text.secondary">
            {s.connectionIds.length} {s.connectionIds.length === 1 ? "platform" : "platforms"}{" "}
            scanned
          </Typography>
        </Stack>
        <FindingsTab
          orgName={orgName}
          scanId={s.id}
          emptyText={
            s.findingsCount > 0
              ? {
                  title: "Agents were not recorded for this scan",
                  description:
                    "This scan ran before Agent Manager recorded each scan's agents, so they cannot be listed here. Scan details has its calls; the next scan lists its agents.",
                }
              : undefined
          }
        />
        <SourceRepoPreview hosts={sourceHosts} />
      </Stack>
      <ScanDetailsDrawer scan={detailsOpen ? s : null} onClose={() => setDetailsOpen(false)} />
    </PageLayout>
  );
}

export default ScanFindings;
