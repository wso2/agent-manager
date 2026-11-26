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
import { EntityAvatar } from "@agent-management-platform/views";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import type {
  AgentRegistryEntry,
  AgentRegistryManagement,
} from "@agent-management-platform/types";
import { lastDeployedAt } from "../agentRegistryFilters";
import { agentTypeText, shadowSourceText, useOpenAgent } from "./agentRow";
import { UnknownOwnerLabel } from "./UnknownOwnerLabel";

const ROWS_PER_PAGE_OPTIONS = [10, 25, 50];

/** Avatar, name and one line under it. */
function AgentCell({ agent, detail }: { agent: AgentRegistryEntry; detail?: string }) {
  return (
    <Stack direction="row" alignItems="center" spacing={2} sx={{ minWidth: 0 }}>
      <EntityAvatar
        name={agent.displayName}
        color="primary.main"
        shape="circular"
        size={32}
        sx={{ flexShrink: 0 }}
      />
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="body1" noWrap title={agent.displayName} sx={{ maxWidth: 260 }}>
          {agent.displayName}
        </Typography>
        {detail && (
          <Typography
            variant="caption"
            color="text.secondary"
            noWrap
            component="div"
            title={detail}
            sx={{ maxWidth: 260 }}
          >
            {detail}
          </Typography>
        )}
      </Box>
    </Stack>
  );
}

function TimeText({ value, empty }: { value: string | null | undefined; empty: string }) {
  return (
    <Typography variant="body2" color="text.secondary" noWrap>
      {value ? formatRelativeTime(value) : empty}
    </Typography>
  );
}

/** Managed agents: name, project, owner (the creator) and when it was last deployed. */
function ManagedRows({ orgId, agents }: { orgId: string; agents: AgentRegistryEntry[] }) {
  const openAgent = useOpenAgent(orgId);
  return (
    <ListingTable variant="card">
      <ListingTable.Head>
        <ListingTable.Row>
          <ListingTable.Cell>Agent</ListingTable.Cell>
          <ListingTable.Cell>Project</ListingTable.Cell>
          <ListingTable.Cell>Created By</ListingTable.Cell>
          <ListingTable.Cell align="right">Last deployed</ListingTable.Cell>
        </ListingTable.Row>
      </ListingTable.Head>
      <ListingTable.Body>
        {agents.map((agent) => (
          <ListingTable.Row
            key={`${agent.projectName}/${agent.name}`}
            variant="card"
            hover
            clickable
            onClick={() => openAgent(agent)}
          >
            <ListingTable.Cell>
              <AgentCell agent={agent} detail={agentTypeText(agent)} />
            </ListingTable.Cell>
            <ListingTable.Cell>
              <Typography variant="body2" noWrap>
                {agent.projectDisplayName}
              </Typography>
            </ListingTable.Cell>
            <ListingTable.Cell>
              <Typography variant="body2" noWrap>
                {agent.createdBy ? agent.createdBy.displayName : <UnknownOwnerLabel />}
              </Typography>
            </ListingTable.Cell>
            <ListingTable.Cell align="right">
              {/* External agents are never deployed by Agent Manager. */}
              <TimeText
                value={lastDeployedAt(agent)}
                empty={agent.hosting === "external" ? "—" : "Never"}
              />
            </ListingTable.Cell>
          </ListingTable.Row>
        ))}
      </ListingTable.Body>
    </ListingTable>
  );
}

/**
 * Unmanaged (shadow) agents: the agent (its scope under it), its source (the platform it
 * was found on), the connections that found it (Connection) and when a scan last saw it.
 */
function UnmanagedRows({ orgId, agents }: { orgId: string; agents: AgentRegistryEntry[] }) {
  const openAgent = useOpenAgent(orgId);
  return (
    <ListingTable variant="card">
      <ListingTable.Head>
        <ListingTable.Row>
          <ListingTable.Cell>Agent</ListingTable.Cell>
          <ListingTable.Cell>Source</ListingTable.Cell>
          <ListingTable.Cell>Connection</ListingTable.Cell>
          <ListingTable.Cell align="right">Last seen</ListingTable.Cell>
        </ListingTable.Row>
      </ListingTable.Head>
      <ListingTable.Body>
        {agents.map((agent) => {
          const discovery = agent.discovery;
          const detail = discovery
            ? discovery.scope
            : undefined;
          return (
            <ListingTable.Row
              key={discovery?.findingId ?? agent.name}
              variant="card"
              hover
              clickable
              onClick={() => openAgent(agent)}
            >
              <ListingTable.Cell>
                <AgentCell agent={agent} detail={detail} />
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography variant="body2" noWrap>
                  {discovery?.providerDisplayName ?? "—"}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography variant="body2" color="text.secondary" noWrap>
                  {discovery ? shadowSourceText(discovery) : "—"}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell align="right">
                <TimeText value={discovery?.lastSeenAt} empty="—" />
              </ListingTable.Cell>
            </ListingTable.Row>
          );
        })}
      </ListingTable.Body>
    </ListingTable>
  );
}

export interface AgentRegistryTableProps {
  orgId: string;
  management: AgentRegistryManagement;
  agents: AgentRegistryEntry[];
}

/** The registry's agents, one row each, with the columns of the chosen management. */
export function AgentRegistryTable({ orgId, management, agents }: AgentRegistryTableProps) {
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
    // Scrolls inside its own box on narrow screens, so the page never scrolls sideways.
    <Box sx={{ overflowX: "auto" }}>
      <Box sx={{ minWidth: 720 }}>
        {management === "managed" ? (
          <ManagedRows orgId={orgId} agents={pageOfAgents} />
        ) : (
          <UnmanagedRows orgId={orgId} agents={pageOfAgents} />
        )}
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
