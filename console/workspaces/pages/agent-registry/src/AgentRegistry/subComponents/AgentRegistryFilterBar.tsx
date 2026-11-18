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

import React from "react";
import { Button, SearchBar, Stack } from "@wso2/oxygen-ui";
import {
  type AgentRegistryFilterOptions,
  type AgentRegistryFilters,
  isFiltered,
} from "../agentRegistryFilters";
import { FilterSelect } from "./FilterSelect";

export interface AgentRegistryFilterBarProps {
  filters: AgentRegistryFilters;
  options: AgentRegistryFilterOptions;
  onChange: (filters: AgentRegistryFilters) => void;
  onClear: () => void;
}

/** Search on the first row; one dropdown per filter on the next, wrapping on narrow screens. */
export function AgentRegistryFilterBar({
  filters,
  options,
  onChange,
  onClear,
}: AgentRegistryFilterBarProps) {
  const setFilter = (key: keyof AgentRegistryFilters) => (value: string) =>
    onChange({ ...filters, [key]: value });

  return (
    <Stack spacing={1.5}>
      <SearchBar
        placeholder="Search by name, project, owner or tag"
        size="small"
        width="standard"
        value={filters.search}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => setFilter("search")(e.target.value)}
      />
      <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
        <FilterSelect
          allLabel="All projects"
          ariaLabel="Filter by project"
          value={filters.project}
          options={options.projects}
          onChange={setFilter("project")}
        />
        <FilterSelect
          allLabel="All environments"
          ariaLabel="Filter by environment"
          value={filters.environment}
          options={options.environments}
          onChange={setFilter("environment")}
        />
        <FilterSelect
          allLabel="All owners"
          ariaLabel="Filter by owner"
          value={filters.owner}
          options={options.owners}
          onChange={setFilter("owner")}
        />
        <FilterSelect
          allLabel="All lifecycle states"
          ariaLabel="Filter by lifecycle state"
          value={filters.lifecycleState}
          options={options.lifecycleStates}
          onChange={setFilter("lifecycleState")}
        />
        <FilterSelect
          allLabel="All tags"
          ariaLabel="Filter by tag"
          value={filters.tag}
          options={options.tags}
          onChange={setFilter("tag")}
        />
        <FilterSelect
          allLabel="All LLM providers"
          ariaLabel="Filter by LLM provider"
          value={filters.llmProvider}
          options={options.llmProviders}
          onChange={setFilter("llmProvider")}
        />
        <FilterSelect
          allLabel="All MCP servers"
          ariaLabel="Filter by MCP server"
          value={filters.mcpServer}
          options={options.mcpServers}
          onChange={setFilter("mcpServer")}
        />
        {isFiltered(filters) && (
          <Button variant="text" size="small" onClick={onClear}>
            Clear filters
          </Button>
        )}
      </Stack>
    </Stack>
  );
}
