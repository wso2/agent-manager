/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useMemo, useState } from "react";
import {
  Alert,
  Avatar,
  Chip,
  ListingTable,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, Search, Users } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  useListAgentIdentityAgents,
  useOrgAgentDisplayNames,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap, type AgentIdentityAgentResponse } from "@agent-management-platform/types";
import { withSearchParams } from "../../utils/withSearchParams";

const AVATAR_SX = { width: 28, height: 28, fontSize: 12 } as const;
const MONO_SX = { fontFamily: "monospace" } as const;

type StatusColor = "success" | "warning" | "error" | "info" | "default";

const STATUS_COLORS: Record<string, StatusColor> = {
  completed: "success",
  in_progress: "info",
  pending: "warning",
  failed: "error",
};

export const AgentsTab: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const [searchParams] = useSearchParams();
  const envName = searchParams.get("envName") ?? "";
  const navigate = useNavigate();
  const [search, setSearch] = useState("");

  const { data, isLoading, error } = useListAgentIdentityAgents({
    orgName: orgId,
    envName,
  });
  const { resolveAgentName, resolveProjectName } = useOrgAgentDisplayNames({ orgName: orgId });

  const agents = useMemo(() => data?.agents ?? [], [data]);

  // Distinct from a search that matched nothing: with no agents at all there is
  // nothing to search, so the toolbar goes away with the table.
  const isEmpty = !isLoading && agents.length === 0 && !search;

  // Resolve each row's display name/project display name once here — every
  // downstream consumer (search filtering, the rendered cells) reads the
  // precomputed fields instead of re-resolving on every render.
  const enrichedAgents = useMemo(
    () =>
      agents.map((agent) => {
        const displayName = resolveAgentName(agent.projectName, agent.agentName);
        const projectDisplayName = resolveProjectName(agent.projectName, agent.agentName);
        return {
          ...agent,
          displayName,
          projectDisplayName,
          filterText: [
            agent.agentName,
            displayName,
            agent.projectName,
            projectDisplayName,
            agent.status,
            agent.thunderAgentId ?? "",
          ]
            .join(" ")
            .toLowerCase(),
        };
      }),
    [agents, resolveAgentName, resolveProjectName],
  );

  const agentsNode =
    absoluteRouteMap.children.org.children.thunderInstances.children.agents;
  const agentDetailPath = (agent: AgentIdentityAgentResponse) =>
    orgId
      ? withSearchParams(
          generatePath(agentsNode.children.detail.path, {
            orgId,
            projectName: agent.projectName,
            agentName: agent.agentName,
          }),
          searchParams,
        )
      : "#";

  const filteredAgents = useMemo(() => {
    const query = search.trim().toLowerCase();
    return query ? enrichedAgents.filter((a) => a.filterText.includes(query)) : enrichedAgents;
  }, [enrichedAgents, search]);

  if (error != null) {
    return (
      <Alert severity="error" icon={<AlertTriangle size={18} />}>
        Failed to load agents. Please try again.
      </Alert>
    );
  }

  return (
    <ListingTable.Provider searchValue={search} onSearchChange={setSearch}>
      <ListingTable.Container>
        {!isEmpty && (
          <ListingTable.Toolbar showSearch searchPlaceholder="Search agents..." />
        )}
        {!isLoading && filteredAgents.length === 0 ? (
          search ? (
            <ListingTable.EmptyState
              illustration={<Search size={64} />}
              title="No agents found"
              description={`No agents match "${search}". Try a different search term.`}
            />
          ) : (
            <ListingTable.EmptyState
              illustration={<Users size={64} />}
              title="No agents yet"
              description="Agents provisioned in this environment will appear here."
            />
          )
        ) : (
          <ListingTable variant="table">
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>Agent</ListingTable.Cell>
                <ListingTable.Cell>Project</ListingTable.Cell>
                <ListingTable.Cell>Status</ListingTable.Cell>
                <ListingTable.Cell>Agent ID</ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {isLoading &&
                Array.from({ length: 5 }).map((_, index) => (
                  <ListingTable.Row key={index} variant="table">
                    <ListingTable.Cell>
                      <Stack direction="row" alignItems="center" spacing={2}>
                        <Skeleton variant="circular" width={28} height={28} />
                        <Skeleton variant="text" width="40%" />
                      </Stack>
                    </ListingTable.Cell>
                    <ListingTable.Cell><Skeleton variant="text" width="60%" /></ListingTable.Cell>
                    <ListingTable.Cell><Skeleton variant="rounded" width={80} height={24} /></ListingTable.Cell>
                    <ListingTable.Cell><Skeleton variant="text" width="70%" /></ListingTable.Cell>
                  </ListingTable.Row>
                ))}
              {!isLoading &&
                filteredAgents.map((agent) => (
                  <ListingTable.Row
                    key={`${agent.projectName}/${agent.agentName}`}
                    variant="table"
                    hover
                    clickable
                    onClick={() => navigate(agentDetailPath(agent))}
                  >
                    <ListingTable.Cell>
                      <ListingTable.CellIcon
                        icon={
                          <Avatar sx={AVATAR_SX}>
                            {agent.displayName.charAt(0).toUpperCase() || "A"}
                          </Avatar>
                        }
                        primary={agent.displayName}
                      />
                    </ListingTable.Cell>
                    <ListingTable.Cell>{agent.projectDisplayName}</ListingTable.Cell>
                    <ListingTable.Cell>
                      <Chip
                        label={agent.status}
                        size="small"
                        color={STATUS_COLORS[agent.status] ?? "default"}
                        variant="outlined"
                      />
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Typography variant="caption" color="text.secondary" sx={MONO_SX}>
                        {agent.thunderAgentId ?? "-"}
                      </Typography>
                    </ListingTable.Cell>
                  </ListingTable.Row>
                ))}
            </ListingTable.Body>
          </ListingTable>
        )}
      </ListingTable.Container>
    </ListingTable.Provider>
  );
};

export default AgentsTab;
