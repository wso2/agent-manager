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
import { Box, ListingTable, Stack, TablePagination, Typography } from "@wso2/oxygen-ui";
import { Users } from "@wso2/oxygen-ui-icons-react";
import { EntityAvatar } from "@agent-management-platform/views";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import type { AgentRegistryEntry } from "@agent-management-platform/types";
import { agentTypeText, canOpenAgent, useOpenAgent } from "./agentRow";
import { EnvironmentStatusChips } from "./EnvironmentStatusChips";
import { LifecycleChip } from "./LifecycleChip";
import { NoOwnerLabel } from "./NoOwnerLabel";
import { UnregisteredBadge } from "./UnregisteredBadge";

const ROWS_PER_PAGE_OPTIONS = [10, 25, 50];

function OwnerCell({ agent }: { agent: AgentRegistryEntry }) {
  // A shadow agent has no owner yet; show who made it when its platform says.
  const ownerHint = agent.registration === "unregistered" ? agent.discovery?.ownerHint : undefined;
  if (ownerHint) {
    return (
      <Typography
        variant="body2"
        color="text.secondary"
        noWrap
        title={ownerHint}
        sx={{ maxWidth: 150 }}
      >
        Owner hint: {ownerHint}
      </Typography>
    );
  }
  if (!agent.owner) {
    return (
      <Typography variant="body2" noWrap>
        <NoOwnerLabel />
      </Typography>
    );
  }
  return (
    <Stack direction="row" spacing={0.5} alignItems="center" sx={{ minWidth: 0 }}>
      {agent.owner.type === "group" && <Users size={14} aria-label="Group" />}
      <Typography variant="body2" noWrap>
        {agent.owner.displayName}
      </Typography>
    </Stack>
  );
}

export interface AgentRegistryTableProps {
  orgId: string;
  agents: AgentRegistryEntry[];
}

/** The registry's agents, one row each. A row opens the agent in its project. */
export function AgentRegistryTable({ orgId, agents }: AgentRegistryTableProps) {
  const openAgent = useOpenAgent(orgId);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(ROWS_PER_PAGE_OPTIONS[0]);

  // A new filter can leave the current page past the end, so go back to the first page.
  useEffect(() => {
    setPage(0);
  }, [agents]);

  const pageOfAgents = useMemo(
    () => agents.slice(page * rowsPerPage, (page + 1) * rowsPerPage),
    [agents, page, rowsPerPage],
  );

  return (
    // Wide table: it scrolls inside its own box so the page itself never scrolls sideways.
    <Box sx={{ overflowX: "auto" }}>
      <Box sx={{ minWidth: 900 }}>
        <ListingTable variant="card">
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell>Agent</ListingTable.Cell>
              <ListingTable.Cell>Project</ListingTable.Cell>
              <ListingTable.Cell>Owner</ListingTable.Cell>
              <ListingTable.Cell>Lifecycle</ListingTable.Cell>
              <ListingTable.Cell>Environments</ListingTable.Cell>
              <ListingTable.Cell align="right">Last activity</ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {pageOfAgents.map((agent) => (
              <ListingTable.Row
                key={`${agent.projectName}/${agent.name}`}
                variant="card"
                hover={canOpenAgent(agent)}
                clickable={canOpenAgent(agent)}
                onClick={() => openAgent(agent)}
              >
                <ListingTable.Cell>
                  <Stack direction="row" alignItems="center" spacing={2} sx={{ minWidth: 0 }}>
                    <EntityAvatar
                      name={agent.displayName}
                      color="primary.main"
                      shape="circular"
                      size={32}
                      sx={{ flexShrink: 0 }}
                    />
                    <Box sx={{ minWidth: 0 }}>
                      <Typography
                        variant="body1"
                        noWrap
                        title={agent.displayName}
                        sx={{ maxWidth: 180 }}
                      >
                        {agent.displayName}
                      </Typography>
                      {/* The badge goes on the second line so the column stays narrow. */}
                      <Stack direction="row" spacing={0.75} alignItems="center">
                        {agent.registration === "unregistered" && <UnregisteredBadge />}
                        <Typography
                          variant="caption"
                          color="text.secondary"
                          noWrap
                          component="div"
                          title={agentTypeText(agent)}
                          sx={{ maxWidth: agent.registration === "unregistered" ? 80 : 180 }}
                        >
                          {agentTypeText(agent)}
                        </Typography>
                      </Stack>
                    </Box>
                  </Stack>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Typography
                    variant="body2"
                    noWrap
                    color={agent.projectDisplayName ? "text.primary" : "text.secondary"}
                  >
                    {agent.projectDisplayName || "—"}
                  </Typography>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <OwnerCell agent={agent} />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  {agent.lifecycleState ? (
                    <LifecycleChip state={agent.lifecycleState} />
                  ) : (
                    // Unregistered agents have no lifecycle until they are onboarded.
                    <Typography variant="body2" color="text.secondary">
                      —
                    </Typography>
                  )}
                </ListingTable.Cell>
                <ListingTable.Cell>
                  {agent.registration === "unregistered" ? (
                    <Typography variant="body2" color="text.secondary">
                      —
                    </Typography>
                  ) : (
                    <EnvironmentStatusChips environments={agent.environments} />
                  )}
                </ListingTable.Cell>
                <ListingTable.Cell align="right">
                  <Typography variant="body2" color="text.secondary" noWrap>
                    {agent.lastActivityAt ? formatRelativeTime(agent.lastActivityAt) : "Never"}
                  </Typography>
                </ListingTable.Cell>
              </ListingTable.Row>
            ))}
          </ListingTable.Body>
        </ListingTable>
      </Box>
      {agents.length > ROWS_PER_PAGE_OPTIONS[0] && (
        <TablePagination
          rowsPerPageOptions={ROWS_PER_PAGE_OPTIONS}
          component="div"
          count={agents.length}
          rowsPerPage={rowsPerPage}
          page={page}
          onPageChange={(_, newPage) => setPage(newPage)}
          onRowsPerPageChange={(e) => {
            setRowsPerPage(parseInt(e.target.value, 10));
            setPage(0);
          }}
        />
      )}
    </Box>
  );
}
