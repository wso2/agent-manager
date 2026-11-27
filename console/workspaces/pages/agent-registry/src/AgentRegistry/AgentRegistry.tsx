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

import { useMemo } from "react";
import { Skeleton, Stack } from "@wso2/oxygen-ui";
import { Bot } from "@wso2/oxygen-ui-icons-react";
import { PageLayout } from "@agent-management-platform/views";
import {
  useListAgentRegistry,
  useListUnmanagedAgents,
} from "@agent-management-platform/api-client";
import { ResourceListShell } from "@agent-management-platform/shared-component";
import { useParams, useSearchParams } from "react-router-dom";
import {
  type AgentRegistryFilters,
  buildFilterOptions,
  countByManagement,
  filterAgents,
  filtersFromSearchParams,
  filtersToSearchParams,
  isFiltered,
} from "./agentRegistryFilters";
import { AgentRegistryTable } from "./subComponents/AgentRegistryTable";
import { AgentRegistryTiles } from "./subComponents/AgentRegistryTiles";
import {
  AgentRegistryToolbar,
  type AgentRegistryView,
} from "./subComponents/AgentRegistryToolbar";

const VIEW_PARAM = "view";

function AgentRegistrySkeleton() {
  return (
    <Stack spacing={2}>
      <Skeleton variant="rounded" width={400} height={40} />
      {Array.from({ length: 6 }).map((_, index) => (
        <Skeleton key={index} variant="rounded" width="100%" height={64} />
      ))}
    </Stack>
  );
}

/**
 * Org-level registry: every agent in the organization, from every project.
 * Filters and the view live in the URL, so a filtered list can be shared as a link.
 */
export function AgentRegistry() {
  const { orgId = "" } = useParams<{ orgId: string }>();
  const managed = useListAgentRegistry({ orgName: orgId });
  const unmanaged = useListUnmanagedAgents({ orgName: orgId });
  const [searchParams, setSearchParams] = useSearchParams();

  const filters = useMemo(() => filtersFromSearchParams(searchParams), [searchParams]);
  const view: AgentRegistryView = searchParams.get(VIEW_PARAM) === "tiles" ? "tiles" : "table";

  // Replace, not push: typing in the search shouldn't fill the back button's history.
  const setFilters = (next: AgentRegistryFilters) =>
    setSearchParams((current) => filtersToSearchParams(next, current), { replace: true });
  const setView = (next: AgentRegistryView) =>
    setSearchParams(
      (current) => {
        const params = new URLSearchParams(current);
        if (next === "table") params.delete(VIEW_PARAM);
        else params.set(VIEW_PARAM, next);
        return params;
      },
      { replace: true },
    );

  // The two lists come from different APIs; each tab waits for and reports only its own.
  const shownList = filters.management === "unmanaged" ? unmanaged : managed;
  const agents = useMemo(
    () => [...(managed.data?.agents ?? []), ...(unmanaged.data?.agents ?? [])],
    [managed.data?.agents, unmanaged.data?.agents],
  );
  const filterOptions = useMemo(() => buildFilterOptions(agents), [agents]);
  const shownAgents = useMemo(() => filterAgents(agents, filters), [agents, filters]);
  const counts = useMemo(() => countByManagement(agents), [agents]);
  // With nothing filtered, an empty Unmanaged list means discovery found nothing.
  const noShadowAgents = filters.management === "unmanaged" && !isFiltered(filters);

  return (
    <PageLayout
      title="Agent Registry"
      avatar={null}
    >
      <ResourceListShell
        resourceName="agent-registry"
        searchValue={filters.search}
        onSearchChange={(search) => setFilters({ ...filters, search })}
        toolbar={
          <AgentRegistryToolbar
            filters={filters}
            options={filterOptions}
            onChange={setFilters}
            view={view}
            onViewChange={setView}
            counts={counts}
            shownCount={shownAgents.length}
          />
        }
        error={shownList.error}
        isLoading={shownList.isLoading}
        isEmpty={agents.length === 0}
        isSearchEmpty={shownAgents.length === 0}
        loadingPlaceholder={<AgentRegistrySkeleton />}
        emptyState={{
          illustration: <Bot size={64} />,
          title: "No agents yet",
          description: "Agents created or registered in any project of this organization appear here.",
        }}
        searchEmptyState={
          noShadowAgents
            ? {
                illustration: <Bot size={64} />,
                title: "No unmanaged agents",
                description:
                  "Shadow Agent Discovery has not found any agents outside Agent Manager.",
              }
            : {
                illustration: <Bot size={64} />,
                title: "No agents match",
                description: "No agents match the search and filters. Change or clear the filters.",
              }
        }
      >
        {view === "tiles" ? (
          <AgentRegistryTiles orgId={orgId} agents={shownAgents} />
        ) : (
          <AgentRegistryTable
            orgId={orgId}
            management={filters.management}
            agents={shownAgents}
          />
        )}
      </ResourceListShell>
    </PageLayout>
  );
}

export default AgentRegistry;
