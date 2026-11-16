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
  Box,
  Button,
  Chip,
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
import { Pencil, PlugZap, Plus, Radar, Trash2, TriangleAlert } from "@wso2/oxygen-ui-icons-react";
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
import type {
  ShadowConnectionResponse,
  ShadowConnectionTest,
} from "@agent-management-platform/types";
import { OutcomeChip, providerLabel, SOURCE_HOSTS, unlockLabels } from "../shadowUi";
import { scopedKeyCount } from "../capabilities";

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

/** GitHub and Azure DevOps: the credential in use, and that the analysis is not available yet. */
export function SourceHostChips({ connection }: { connection: ShadowConnectionResponse }) {
  if (connection.provider !== "github" && connection.provider !== "azure_devops") return null;
  const entra = connection.provider === "azure_devops" && connection.settings?.useEntra === true;
  return (
    <>
      <Chip
        size="small"
        variant="outlined"
        label={entra ? "Entra service principal" : "Personal access token"}
      />
      <Tooltip title="Test connection checks the credential now. Scans list this connection as skipped until source code analysis is added.">
        <Chip size="small" variant="outlined" color="info" label="Analysis coming soon" />
      </Tooltip>
    </>
  );
}

interface ConnectionsTabProps {
  orgName: string | undefined;
  canManage: boolean;
  onAdd: () => void;
  onEdit: (c: ShadowConnectionResponse) => void;
}

/**
 * The platforms: which accounts are scanned, how their last test went, and
 * a Scan action to scan one platform on its own.
 */
export function ConnectionsTab({ orgName, canManage, onAdd, onEdit }: ConnectionsTabProps) {
  const [search, setSearch] = useState("");
  const [testResult, setTestResult] = useState<{
    name: string;
    result: ShadowConnectionTest;
  } | null>(null);
  const { data, isLoading, error } = useShadowConnections(orgName);
  const remove = useDeleteShadowConnection(orgName);
  const test = useTestShadowConnection(orgName);
  const scan = useStartShadowScan(orgName);
  const summary = useShadowSummary(orgName);
  const scanActive = isShadowScanActive(summary.data?.latestScan?.status);
  const providers = useShadowProviders(orgName);
  const signalNames = useMemo(
    () => new Map((providers.data?.signals ?? []).map((d) => [d.code, d.name])),
    [providers.data],
  );
  const { addConfirmation } = useConfirmationDialog();
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
      Add platform
    </Button>
  ) : undefined;

  const handleDelete = (c: ShadowConnectionResponse) =>
    addConfirmation({
      title: `Delete ${c.name}?`,
      description:
        "The stored credential and every agent this platform found are deleted from Agent Manager. The platform account is not changed.",
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      onConfirm: () => remove.mutate(c.id),
    });

  const handleTest = (c: ShadowConnectionResponse) =>
    test.mutate(c.id, { onSuccess: (result) => setTestResult({ name: c.name, result }) });

  const toolbar = (
    <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5} alignItems={{ sm: "center" }}>
      <Box sx={{ width: { xs: "100%", sm: 400 } }}>
        <SearchBar
          placeholder="Search platforms"
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
            ? new Error("Couldn't load the platforms. Reload the page to try again.")
            : undefined
        }
        isLoading={isLoading}
        isEmpty={isListEmpty({ isLoading, count: connections.length, isFiltered: !!search })}
        isSearchEmpty={!isLoading && connections.length > 0 && filtered.length === 0}
        emptyState={{
          illustration: <PlugZap size={64} />,
          title: "No platforms added",
          description:
            "Connect OpenAI, Anthropic, Amazon Bedrock, Google Agent Platform, Microsoft Foundry, GitHub or Azure DevOps with a read-only credential.",
          action: addButton,
        }}
        searchEmptyState={{
          illustration: <PlugZap size={64} />,
          title: "No platforms match your search",
          description: "Try another name or platform.",
        }}
      >
        <ListingTable>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell>Name</ListingTable.Cell>
              <ListingTable.Cell>Platform</ListingTable.Cell>
              <ListingTable.Cell>Last test</ListingTable.Cell>
              <ListingTable.Cell>Created</ListingTable.Cell>
              <ListingTable.Cell align="right" />
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {filtered.map((c) => (
              <ListingTable.Row key={c.id}>
                <ListingTable.Cell>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <Typography variant="body2" fontWeight={500}>
                      {c.name}
                    </Typography>
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
                  <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
                    <Typography variant="body2">{providerLabel(c.provider)}</Typography>
                    <ScopedKeysChip provider={c.provider} secretHints={c.secretHints} />
                    <SourceHostChips connection={c} />
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
                      {!SOURCE_HOSTS.includes(c.provider) && (
                        <Tooltip title={scanActive ? "A scan is running" : "Scan this platform"}>
                          <span>
                            <IconButton
                              size="small"
                              aria-label={`Scan ${c.name}`}
                              disabled={scanActive || scan.isPending}
                              onClick={() => scan.mutate({ connectionIds: [c.id] })}
                            >
                              <Radar size={18} />
                            </IconButton>
                          </span>
                        </Tooltip>
                      )}
                      <Tooltip title="Test connection">
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
      </ResourceListShell>

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
