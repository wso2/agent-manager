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

import { useDeferredValue, useMemo, useState } from "react";
import {
  Box,
  Button,
  Chip,
  IconButton,
  ListingTable,
  MenuItem,
  SearchBar,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { ScanSearch, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useLocation, useNavigate } from "react-router-dom";
import { returnToState, TextInput } from "@agent-management-platform/views";
import {
  useDeleteShadowFinding,
  useDeleteShadowFindings,
  useShadowFindings,
} from "@agent-management-platform/api-client";
import {
  formatRelativeTime,
  isListEmpty,
  ResourceListShell,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  type ShadowFindingListQuery,
  type ShadowFindingSummary,
} from "@agent-management-platform/types";
import { AGENT_PLATFORMS, kindLabel, providerLabel, StatusChip } from "../shadowUi";

const PAGE_SIZE = 50;

interface FindingsTabProps {
  orgName: string | undefined;
  /** Only the agents this scan found. Without it, only the agents still listed. */
  scanId?: string;
  /** The platform filter, when the parent keeps it (e.g. in the URL). */
  provider?: string;
  onProviderChange?: (provider: string) => void;
  /** Replaces the empty state's text (e.g. a scan from before agents were recorded per scan). */
  emptyText?: { title: string; description: string };
  /** Shown in the empty state when there is nothing to scan yet. */
  emptyAction?: React.ReactNode;
  /** Shows the delete actions: one per agent, and Delete all (not for a scan's agents). */
  canManage?: boolean;
}

/** What makes two rows look alike: same platform, type, name and scope. */
function lookalikeKey(f: ShadowFindingSummary): string {
  return [f.provider, f.kind, f.displayName, f.scope].join("\u0000");
}

/** The connections that found an agent: one name, or "A, B" for an agent found twice. */
function foundByLabel(f: ShadowFindingSummary): string {
  const names = (f.foundBy ?? []).map((s) => s.connectionName);
  return names.length > 0 ? names.join(", ") : f.connectionName;
}

/**
 * The unregistered agents: every agent the platforms list, once however many
 * connections found it, by name. With scanId, the agents one scan found.
 */
export function FindingsTab({
  orgName,
  scanId,
  provider: controlledProvider,
  onProviderChange,
  emptyText,
  emptyAction,
  canManage = false,
}: FindingsTabProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const [search, setSearch] = useState("");
  const [ownProvider, setOwnProvider] = useState("");
  const [status, setStatus] = useState("");
  const [offset, setOffset] = useState(0);
  const provider = controlledProvider ?? ownProvider;
  const deferredSearch = useDeferredValue(search.trim());
  const query = useMemo<ShadowFindingListQuery>(
    () => ({ scanId, search: deferredSearch, provider, status, limit: PAGE_SIZE, offset }),
    [scanId, deferredSearch, provider, status, offset],
  );
  const { data, isLoading, error } = useShadowFindings(orgName, query);
  const findings = useMemo(() => data?.findings ?? [], [data]);
  const total = data?.total ?? 0;
  const isFiltered = !!(deferredSearch || provider || status);
  // Distinct agents can share a name (a platform allows it): their rows also
  // show the platform ID, so they do not read as duplicates.
  const lookalikes = useMemo(() => {
    const seen = new Map<string, number>();
    for (const f of findings) seen.set(lookalikeKey(f), (seen.get(lookalikeKey(f)) ?? 0) + 1);
    return new Set([...seen].filter(([, n]) => n > 1).map(([k]) => k));
  }, [findings]);
  const canDelete = canManage && !scanId;
  const remove = useDeleteShadowFinding(orgName);
  const removeAll = useDeleteShadowFindings(orgName);
  const { addConfirmation } = useConfirmationDialog();

  const handleDelete = (f: ShadowFindingSummary) =>
    addConfirmation({
      title: `Delete ${f.displayName}?`,
      description:
        `The agent and its evidence are deleted from Agent Manager, for every platform that found it (${foundByLabel(f)}). ` +
        "Nothing changes on the platform: a later scan lists the agent again if the platform still has it.",
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      onConfirm: () => remove.mutate(f.id),
    });
  const handleDeleteAll = () =>
    addConfirmation({
      title: "Delete all unregistered agents?",
      description:
        "Every agent and its evidence is deleted from Agent Manager, on every platform, whatever the filters show, " +
        "including the agents their platforms no longer list. " +
        "Platforms and scan history are kept, and nothing changes on the platforms: a later scan lists the agents they still have again. " +
        "This can't be undone.",
      confirmButtonText: "Delete all",
      confirmButtonColor: "error",
      onConfirm: () => {
        setOffset(0);
        removeAll.mutate();
      },
    });

  const setProvider = (v: string) => {
    if (onProviderChange) onProviderChange(v);
    else setOwnProvider(v);
    setOffset(0);
  };
  const setFilter = (set: (v: string) => void) => (v: string) => {
    set(v);
    setOffset(0);
  };

  const toolbar = (
    <Stack direction={{ xs: "column", md: "row" }} spacing={1.5} alignItems={{ md: "center" }}>
      <Box sx={{ width: { xs: "100%", md: 400 } }}>
        <SearchBar
          placeholder="Search by name, platform ID or scope"
          size="small"
          value={search}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
            setFilter(setSearch)(e.target.value)
          }
          fullWidth
        />
      </Box>
      <Box sx={{ flex: 1 }} />
      <Box sx={{ minWidth: 150 }}>
        <TextInput
          select
          aria-label="Status"
          value={status}
          onChange={(e) => setFilter(setStatus)(e.target.value)}
          size="small"
          slotProps={{ select: { displayEmpty: true, inputProps: { "aria-label": "Status" } } }}
        >
          <MenuItem value="">All statuses</MenuItem>
          <MenuItem value="new">New</MenuItem>
          <MenuItem value="reviewed">Reviewed</MenuItem>
          <MenuItem value="dismissed">Dismissed</MenuItem>
        </TextInput>
      </Box>
      <Box sx={{ minWidth: 220 }}>
        <TextInput
          select
          aria-label="Platform"
          value={provider}
          onChange={(e) => setProvider(e.target.value)}
          size="small"
          slotProps={{ select: { displayEmpty: true, inputProps: { "aria-label": "Platform" } } }}
        >
          <MenuItem value="">All platforms</MenuItem>
          {AGENT_PLATFORMS.map((p) => (
            <MenuItem key={p} value={p}>
              {providerLabel(p)}
            </MenuItem>
          ))}
        </TextInput>
      </Box>
      {canDelete && (
        <Button
          variant="outlined"
          color="error"
          startIcon={<Trash2 size={16} />}
          disabled={(total === 0 && !isFiltered) || removeAll.isPending}
          onClick={handleDeleteAll}
        >
          Delete all
        </Button>
      )}
    </Stack>
  );

  return (
    <ResourceListShell
      resourceName="shadow-agent-unregistered-agents"
      disablePaper
      searchValue={search}
      onSearchChange={setFilter(setSearch)}
      toolbar={toolbar}
      error={
        error ? new Error("Couldn't load the agents. Reload the page to try again.") : undefined
      }
      isLoading={isLoading}
      isEmpty={isListEmpty({ isLoading, count: findings.length, isFiltered })}
      isSearchEmpty={!isLoading && isFiltered && findings.length === 0}
      emptyState={{
        illustration: <ScanSearch size={64} />,
        title:
          emptyText?.title ?? (scanId ? "This scan found no agents" : "No unregistered agents yet"),
        description: emptyText
          ? emptyText.description
          : scanId
            ? "No platform of this scan listed an agent. Open the scan details to see each call."
            : "Add a platform and run a scan. The agents your platforms list show up here, once each.",
        action: emptyAction,
      }}
      searchEmptyState={{
        illustration: <ScanSearch size={64} />,
        title: "No agents match your filters",
        description: "Try other search keywords or clear the filters.",
      }}
    >
      <ListingTable>
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell>Agent</ListingTable.Cell>
            <ListingTable.Cell>Platform</ListingTable.Cell>
            <ListingTable.Cell>Found by</ListingTable.Cell>
            <ListingTable.Cell>State</ListingTable.Cell>
            <ListingTable.Cell>Last seen</ListingTable.Cell>
            <ListingTable.Cell>Status</ListingTable.Cell>
            {canDelete && <ListingTable.Cell align="right">Actions</ListingTable.Cell>}
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {findings.map((f) => (
            <ListingTable.Row
              key={f.id}
              hover
              sx={{ cursor: "pointer" }}
              onClick={() =>
                navigate(
                  generatePath(
                    absoluteRouteMap.children.org.children.shadowAgentDiscovery.children.finding
                      .path,
                    { orgId: orgName ?? "", findingId: f.id },
                  ),
                  { state: returnToState(location) },
                )
              }
            >
              <ListingTable.Cell>
                <Typography variant="body2" fontWeight={500}>
                  {f.displayName}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {kindLabel(f.kind, f.provider)}
                  {f.scope ? ` · ${f.scope}` : ""}
                  {lookalikes.has(lookalikeKey(f)) ? ` · ${f.externalId}` : ""}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography variant="body2">{providerLabel(f.provider)}</Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography variant="body2" color="text.secondary">
                  {foundByLabel(f)}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Stack direction="row" spacing={1} alignItems="center">
                  <Typography variant="body2" color="text.secondary">
                    {f.state || "—"}
                  </Typography>
                  {!f.present && (
                    <Chip
                      size="small"
                      variant="outlined"
                      color="warning"
                      label="No longer listed"
                    />
                  )}
                </Stack>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography variant="body2" color="text.secondary">
                  {formatRelativeTime(f.lastSeenAt)}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <StatusChip status={f.status} />
              </ListingTable.Cell>
              {canDelete && (
                <ListingTable.Cell align="right" onClick={(e) => e.stopPropagation()}>
                  <Tooltip title="Delete agent">
                    <span>
                      <IconButton
                        size="small"
                        color="error"
                        aria-label={`Delete ${f.displayName}`}
                        disabled={remove.isPending}
                        onClick={() => handleDelete(f)}
                      >
                        <Trash2 size={16} />
                      </IconButton>
                    </span>
                  </Tooltip>
                </ListingTable.Cell>
              )}
            </ListingTable.Row>
          ))}
        </ListingTable.Body>
      </ListingTable>
      {total > PAGE_SIZE && (
        <Stack direction="row" spacing={1} alignItems="center" sx={{ pt: 2 }}>
          <Button
            variant="outlined"
            size="small"
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
          >
            Previous
          </Button>
          <Button
            variant="outlined"
            size="small"
            disabled={offset + PAGE_SIZE >= total}
            onClick={() => setOffset(offset + PAGE_SIZE)}
          >
            Next
          </Button>
          <Typography variant="body2" color="text.secondary">
            {offset + 1}–{Math.min(offset + PAGE_SIZE, total)} of {total}
          </Typography>
        </Stack>
      )}
    </ResourceListShell>
  );
}

export default FindingsTab;
