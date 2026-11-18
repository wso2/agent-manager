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
import {
  EntityAvatar,
  displayAgentSubType,
} from "@agent-management-platform/views";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import { absoluteRouteMap, type AgentRegistryEntry } from "@agent-management-platform/types";
import { generatePath, useNavigate } from "react-router-dom";
import { EnvironmentStatusChips } from "./EnvironmentStatusChips";
import { LifecycleChip } from "./LifecycleChip";

const ROWS_PER_PAGE_OPTIONS = [10, 25, 50];

/** Hosting, interface and kind in one line, e.g. "Platform · A2A · Kind reviewer 0.4.1". */
function typeText(agent: AgentRegistryEntry): string {
  const hosting = agent.hosting === "platform" ? "Platform" : "External";
  const kind = agent.kindName
    ? `Kind ${agent.kindName}${agent.kindVersion ? ` ${agent.kindVersion}` : ""}`
    : undefined;
  return [hosting, displayAgentSubType(agent.interface), kind].filter(Boolean).join(" · ");
}

function OwnerCell({ agent }: { agent: AgentRegistryEntry }) {
  if (!agent.owner) {
    return (
      <Typography variant="body2" color="warning.main" noWrap>
        No owner
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
  const navigate = useNavigate();
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

  const openAgent = (agent: AgentRegistryEntry) =>
    navigate(
      generatePath(absoluteRouteMap.children.org.children.projects.children.agents.path, {
        orgId,
        projectId: agent.projectName,
        agentId: agent.name,
      }),
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
                hover
                clickable
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
                      <Typography variant="body1" noWrap>
                        {agent.displayName}
                      </Typography>
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        noWrap
                        component="div"
                        sx={{ maxWidth: 180 }}
                      >
                        {typeText(agent)}
                      </Typography>
                    </Box>
                  </Stack>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Typography variant="body2" noWrap>
                    {agent.projectDisplayName}
                  </Typography>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <OwnerCell agent={agent} />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <LifecycleChip state={agent.lifecycleState} />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <EnvironmentStatusChips environments={agent.environments} />
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
