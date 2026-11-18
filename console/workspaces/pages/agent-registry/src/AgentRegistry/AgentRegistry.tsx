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
import { Skeleton, Stack, Typography } from "@wso2/oxygen-ui";
import { Bot } from "@wso2/oxygen-ui-icons-react";
import { PageLayout } from "@agent-management-platform/views";
import { useListAgentRegistry } from "@agent-management-platform/api-client";
import { ResourceListShell } from "@agent-management-platform/shared-component";
import { useParams } from "react-router-dom";
import {
  EMPTY_FILTERS,
  buildFilterOptions,
  filterAgents,
  isFiltered,
  type AgentRegistryFilters,
} from "./agentRegistryFilters";
import { AgentRegistryFilterBar } from "./subComponents/AgentRegistryFilterBar";
import { AgentRegistryTable } from "./subComponents/AgentRegistryTable";

function AgentRegistrySkeleton() {
  return (
    <Stack spacing={2}>
      <Skeleton variant="rounded" width={400} height={40} />
      <Skeleton variant="rounded" width="100%" height={40} />
      {Array.from({ length: 6 }).map((_, index) => (
        <Skeleton key={index} variant="rounded" width="100%" height={64} />
      ))}
    </Stack>
  );
}

/** Org-level registry: every agent in the organization, from every project. */
export function AgentRegistry() {
  const { orgId = "" } = useParams<{ orgId: string }>();
  const { data, isLoading, error } = useListAgentRegistry({ orgName: orgId });
  const [filters, setFilters] = useState<AgentRegistryFilters>(EMPTY_FILTERS);

  const agents = useMemo(() => data?.agents ?? [], [data?.agents]);
  const filterOptions = useMemo(() => buildFilterOptions(agents), [agents]);
  const shownAgents = useMemo(() => filterAgents(agents, filters), [agents, filters]);

  return (
    <PageLayout
      title="Agent registry"
      description="Every agent in the organization, across all projects, with its owner, lifecycle state and where it runs."
      avatar={null}
    >
      <ResourceListShell
        resourceName="agent-registry"
        searchValue={filters.search}
        onSearchChange={(search) => setFilters({ ...filters, search })}
        toolbar={
          <Stack spacing={1}>
            <AgentRegistryFilterBar
              filters={filters}
              options={filterOptions}
              onChange={setFilters}
              onClear={() => setFilters(EMPTY_FILTERS)}
            />
            {isFiltered(filters) && (
              <Typography variant="body2" color="text.secondary">
                Showing {shownAgents.length} of {agents.length} agents
              </Typography>
            )}
          </Stack>
        }
        error={error}
        isLoading={isLoading}
        isEmpty={agents.length === 0}
        isSearchEmpty={shownAgents.length === 0}
        loadingPlaceholder={<AgentRegistrySkeleton />}
        emptyState={{
          illustration: <Bot size={64} />,
          title: "No agents yet",
          description: "Agents created or registered in any project of this organization appear here.",
        }}
        searchEmptyState={{
          illustration: <Bot size={64} />,
          title: "No agents match",
          description: "No agents match the search and filters. Change or clear the filters.",
        }}
      >
        <AgentRegistryTable orgId={orgId} agents={shownAgents} />
      </ResourceListShell>
    </PageLayout>
  );
}

export default AgentRegistry;
