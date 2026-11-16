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

import { useMemo, useState } from "react";
import {
  Alert,
  Box,
  Chip,
  Collapse,
  IconButton,
  ListingTable,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown, ChevronRight, KeyRound } from "@wso2/oxygen-ui-icons-react";
import { getSourceRepoScanData, type RepoSuspect } from "./dummyData";

const VERDICT_COLOR: Record<RepoSuspect["verdict"], "error" | "warning" | "default"> = {
  Agent: "error",
  "Possible agent": "warning",
  "LLM consumer": "default",
};

function SuspectRow({ s, detectorName }: { s: RepoSuspect; detectorName: (id: string) => string }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <ListingTable.Row hover>
        <ListingTable.Cell sx={{ width: 40 }}>
          <IconButton
            size="small"
            aria-label={open ? `Hide hits for ${s.repository}` : `Show hits for ${s.repository}`}
            onClick={() => setOpen(!open)}
          >
            {open ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          </IconButton>
        </ListingTable.Cell>
        <ListingTable.Cell>
          <Typography variant="body2" fontWeight={500}>
            {s.repository}
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {s.host} · {s.servicePath} · {s.commitSha}
          </Typography>
        </ListingTable.Cell>
        <ListingTable.Cell>{s.score}</ListingTable.Cell>
        <ListingTable.Cell>
          <Chip
            size="small"
            variant="outlined"
            color={VERDICT_COLOR[s.verdict]}
            label={s.verdict}
          />
        </ListingTable.Cell>
        <ListingTable.Cell>
          {s.secretFindings.length > 0 ? (
            <Tooltip title="Hard-coded secret found (location only)">
              <Chip
                size="small"
                color="error"
                icon={<KeyRound size={14} />}
                label={s.secretFindings.length}
              />
            </Tooltip>
          ) : (
            "—"
          )}
        </ListingTable.Cell>
        <ListingTable.Cell>
          <Typography variant="body2" color="text.secondary">
            {s.lastCommitter}
          </Typography>
        </ListingTable.Cell>
      </ListingTable.Row>
      <ListingTable.Row>
        <ListingTable.Cell colSpan={6} sx={{ py: 0, borderBottom: open ? undefined : "none" }}>
          <Collapse in={open} unmountOnExit>
            <Stack spacing={0.5} sx={{ py: 1.5 }}>
              {s.hits.map((h) => (
                <Typography key={`${h.detectorId}|${h.path}|${h.line}`} variant="caption">
                  <strong>{h.detectorId}</strong> {detectorName(h.detectorId)} — {h.path}:{h.line} (
                  {h.symbol})
                </Typography>
              ))}
              {s.secretFindings.map((x) => (
                <Typography key={`${x.path}|${x.line}`} variant="caption" color="error.main">
                  {x.type} at {x.path}:{x.line} (value not stored)
                </Typography>
              ))}
            </Stack>
          </Collapse>
        </ListingTable.Cell>
      </ListingTable.Row>
    </>
  );
}

/** A GitHub or Azure DevOps connection a scan included. */
export interface ScannedSourceHost {
  connectionName: string;
  provider: string;
}

const HOST_OF: Record<string, RepoSuspect["host"]> = {
  github: "GitHub",
  azure_devops: "Azure DevOps",
};

/**
 * Source repository suspects of a scan: SAMPLE DATA. Source code analysis
 * (Method 2) is built separately; until its API exists, a scan records the
 * GitHub and Azure DevOps connections as skipped and this preview shows
 * what the analysis will report. Replace dummyData.ts with its API.
 */
export function SourceRepoPreview({ hosts }: { hosts: ScannedSourceHost[] }) {
  const data = useMemo(() => getSourceRepoScanData(), []);
  const kinds = new Set(hosts.map((h) => HOST_OF[h.provider]).filter(Boolean));
  const suspects = data.suspects.filter((s) => kinds.has(s.host));
  const detectorName = (id: string) => data.detectors.find((d) => d.id === id)?.name ?? id;
  if (hosts.length === 0) return null;
  return (
    <Stack spacing={1.5}>
      <Stack direction="row" spacing={1} alignItems="center">
        <Typography variant="subtitle1">Source repositories</Typography>
        <Chip size="small" label="Sample data" />
      </Stack>
      <Alert severity="info">
        Source code analysis is not available yet, so this scan skipped{" "}
        {hosts.map((h) => h.connectionName).join(", ")}. The suspects below are sample data that
        show what the analysis will report; none of them comes from your repositories. Code will be
        read in memory and never stored; only the repository, path, line, matched symbol and commit
        are kept.
      </Alert>
      <Box sx={{ overflowX: "auto" }}>
        <ListingTable>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell />
              <ListingTable.Cell>Suspect (repository and service folder)</ListingTable.Cell>
              <ListingTable.Cell>Score</ListingTable.Cell>
              <ListingTable.Cell>Verdict</ListingTable.Cell>
              <ListingTable.Cell>Secrets</ListingTable.Cell>
              <ListingTable.Cell>Last committer</ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {suspects.map((s) => (
              <SuspectRow key={s.id} s={s} detectorName={detectorName} />
            ))}
          </ListingTable.Body>
        </ListingTable>
      </Box>
      <Typography variant="caption" color="text.secondary">
        Verdict rule: Agent — score of 60 or more and at least one High detector or D7; Possible
        agent — 30 to 59 with the same; otherwise LLM consumer.
      </Typography>
    </Stack>
  );
}

export default SourceRepoPreview;
