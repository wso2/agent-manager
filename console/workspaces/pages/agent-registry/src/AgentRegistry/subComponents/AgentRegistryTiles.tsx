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
import {
  Box,
  Divider,
  ListingCard,
  Stack,
  TablePagination,
  Typography,
} from "@wso2/oxygen-ui";
import { Bot } from "@wso2/oxygen-ui-icons-react";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import type { AgentRegistryEntry } from "@agent-management-platform/types";
import { agentTypeText, useOpenAgent } from "./agentRow";
import { EnvironmentStatusChips } from "./EnvironmentStatusChips";
import { LifecycleChip } from "./LifecycleChip";
import { NoOwnerLabel } from "./NoOwnerLabel";

// Multiples of the columns at each breakpoint, so a full page has no ragged last row.
const TILES_PER_PAGE_OPTIONS = [12, 24, 48];

// Same columns as the Projects page.
const GRID_TEMPLATE = {
  xs: "repeat(1, minmax(0, 1fr))",
  md: "repeat(2, minmax(0, 1fr))",
  lg: "repeat(3, minmax(0, 1fr))",
  xl: "repeat(4, minmax(0, 1fr))",
};

function AgentTile({ orgId, agent }: { orgId: string; agent: AgentRegistryEntry }) {
  const openAgent = useOpenAgent(orgId);

  const details = (
    <Stack spacing={1} component="span" sx={{ display: "flex" }}>
      <Typography variant="body2" color="text.secondary" component="span" noWrap>
        {agent.projectDisplayName} ·{" "}
        {agent.owner ? (
          agent.owner.displayName
        ) : (
          <NoOwnerLabel />
        )}
      </Typography>
      <Typography variant="caption" color="text.secondary" component="span" noWrap>
        {agentTypeText(agent)}
      </Typography>
      <Stack
        direction="row"
        spacing={1}
        alignItems="center"
        component="span"
        divider={<Divider orientation="vertical" flexItem />}
        sx={{ display: "flex" }}
      >
        <LifecycleChip state={agent.lifecycleState} variant="filled" />
        <EnvironmentStatusChips environments={agent.environments} />
      </Stack>
    </Stack>
  );

  return (
    <ListingCard
      title={agent.displayName}
      description={details}
      icon={<Bot size={20} />}
      updatedLabel="Last active"
      updatedTime={agent.lastActivityAt ? formatRelativeTime(agent.lastActivityAt) : "never"}
      updatedDateTime={agent.lastActivityAt ?? undefined}
      onClick={() => openAgent(agent)}
    />
  );
}

export interface AgentRegistryTilesProps {
  orgId: string;
  agents: AgentRegistryEntry[];
}

/** The registry's agents as a grid of tiles. A tile opens the agent in its project. */
export function AgentRegistryTiles({ orgId, agents }: AgentRegistryTilesProps) {
  const [page, setPage] = useState(0);
  const [tilesPerPage, setTilesPerPage] = useState(TILES_PER_PAGE_OPTIONS[0]);

  // A new filter can leave the current page past the end, so go back to the first page.
  useEffect(() => {
    setPage(0);
  }, [agents]);

  const pageOfAgents = useMemo(
    () => agents.slice(page * tilesPerPage, (page + 1) * tilesPerPage),
    [agents, page, tilesPerPage],
  );

  return (
    <Box>
      <Box sx={{ display: "grid", gridTemplateColumns: GRID_TEMPLATE, gap: 2 }}>
        {pageOfAgents.map((agent) => (
          <AgentTile key={`${agent.projectName}/${agent.name}`} orgId={orgId} agent={agent} />
        ))}
      </Box>
      {agents.length > TILES_PER_PAGE_OPTIONS[0] && (
        <TablePagination
          rowsPerPageOptions={TILES_PER_PAGE_OPTIONS}
          labelRowsPerPage="Tiles per page:"
          component="div"
          count={agents.length}
          rowsPerPage={tilesPerPage}
          page={page}
          onPageChange={(_, newPage) => setPage(newPage)}
          onRowsPerPageChange={(e) => {
            setTilesPerPage(parseInt(e.target.value, 10));
            setPage(0);
          }}
        />
      )}
    </Box>
  );
}
