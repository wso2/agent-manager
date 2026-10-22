/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { type ReactNode, useEffect, useState } from 'react';
import { Box, Skeleton, TablePagination } from '@wso2/oxygen-ui';
import { Bot, Search } from '@wso2/oxygen-ui-icons-react';
import {
  useListA2ACatalog,
  useListEnvironments,
  useListProjects,
} from '@agent-management-platform/api-client';
import { ResourceListShell } from '@agent-management-platform/shared-component';
import { useReplaceSearchParams } from '../utils/useReplaceSearchParams';
import { useEnvironmentLabel } from '../utils/useEnvironmentLabel';
import { A2AAgentTile } from './A2AAgentTile';
import { A2AAgentFilterBar, type AdvancedFilters } from './A2AAgentFilterBar';
import {
  ADVANCED_FILTER_KEYS,
  type AdvancedFilterKey,
  PAGE_SIZE,
  catalogueListQuery,
  isFiltering,
  lastPageIndex,
  parsePageParam,
  readCatalogueFilters,
  setOrDelete,
  writePage,
} from './catalogueQuery';

const SEARCH_DEBOUNCE_MS = 300;
const SKELETON_COUNT = 4;

const TileGrid: React.FC<{ children: ReactNode }> = ({ children }) => (
  <Box
    sx={{
      display: 'grid',
      gap: 2,
      gridTemplateColumns: {
        xs: 'repeat(auto-fill, minmax(260px, 1fr))',
        md: 'repeat(auto-fill, minmax(300px, 1fr))',
      },
    }}
  >
    {children}
  </Box>
);

const loadingTiles = (
  <TileGrid>
    {Array.from({ length: SKELETON_COUNT }, (_, index) => (
      <Skeleton
        key={index}
        variant="rounded"
        height={200}
        data-testid="a2a-tile-skeleton"
      />
    ))}
  </TileGrid>
);

const emptyState = {
  illustration: <Bot size={64} />,
  title: 'No A2A agents yet',
  description: 'Agents that expose an A2A card show up here once deployed.',
};

const noMatchState = {
  illustration: <Search size={64} />,
  title: 'No A2A agents match your search',
  description: 'Try a different keyword or clear the filters.',
};

/** Any search or filter write starts again from the first page. */
const dropPage = (params: URLSearchParams) => params.delete('page');

export interface A2AAgentCatalogueProps {
  orgId: string;
}

/** The list keeps its applied search, filters and page in the URL. */
export const A2AAgentCatalogue: React.FC<A2AAgentCatalogueProps> = ({ orgId }) => {
  const [params, replaceParams] = useReplaceSearchParams();
  const filters = readCatalogueFilters(params);
  const page = parsePageParam(params.get('page'));
  const urlSearch = params.get('q') ?? '';

  const [search, setSearch] = useState(urlSearch);
  const [debouncedSearch, setDebouncedSearch] = useState(() => urlSearch.trim());

  useEffect(() => {
    const timer = setTimeout(
      () => setDebouncedSearch(search.trim()),
      SEARCH_DEBOUNCE_MS,
    );
    return () => clearTimeout(timer);
  }, [search]);

  useEffect(() => {
    if (debouncedSearch === urlSearch) return;
    replaceParams((p) => {
      setOrDelete(p, 'q', debouncedSearch);
      dropPage(p);
    });
  }, [debouncedSearch, urlSearch, replaceParams]);

  const query = useListA2ACatalog(
    { orgName: orgId },
    catalogueListQuery(filters, debouncedSearch, page),
  );
  const { data: environments } = useListEnvironments({ orgName: orgId });
  const { data: projectList } = useListProjects({ orgName: orgId });
  const environmentLabel = useEnvironmentLabel(orgId);

  const agents = query.data?.agents ?? [];
  const total = query.data?.total ?? 0;
  const filtering = isFiltering(filters, debouncedSearch);

  // A page past the end (the catalogue shrank) moves to the last page with agents.
  useEffect(() => {
    const data = query.data;
    if (!data || data.agents.length > 0 || data.total === 0) return;
    const lastPage = lastPageIndex(data.total);
    if (page === 0 || page === lastPage) return;
    replaceParams((p) => writePage(p, lastPage));
  }, [query.data, page, replaceParams]);

  const changeKind = (kind: string | undefined) =>
    replaceParams((p) => {
      setOrDelete(p, 'kind', kind ?? '');
      dropPage(p);
    });
  const applyAdvanced = (advanced: AdvancedFilters) =>
    replaceParams((p) => {
      for (const key of ADVANCED_FILTER_KEYS) setOrDelete(p, key, advanced[key]);
      dropPage(p);
    });
  const removeFilter = (key: AdvancedFilterKey) =>
    replaceParams((p) => {
      p.delete(key);
      dropPage(p);
    });
  const clearAll = () =>
    replaceParams((p) => {
      for (const key of ['kind', ...ADVANCED_FILTER_KEYS]) p.delete(key);
      dropPage(p);
    });

  return (
    <ResourceListShell
      resourceName="a2a-agents"
      searchValue={search}
      onSearchChange={setSearch}
      toolbar={
        <A2AAgentFilterBar
          search={search}
          onSearchChange={setSearch}
          filters={filters}
          onKindChange={changeKind}
          onApply={applyAdvanced}
          onRemove={removeFilter}
          onClearAll={clearAll}
          environments={environments ?? []}
          projects={projectList?.projects ?? []}
        />
      }
      error={query.error}
      isLoading={query.isLoading}
      isEmpty={total === 0 && !filtering}
      isSearchEmpty={total === 0 && filtering}
      loadingPlaceholder={loadingTiles}
      emptyState={emptyState}
      searchEmptyState={noMatchState}
    >
      <TileGrid>
        {agents.map((agent) => (
          <A2AAgentTile
            key={`${agent.projectName}/${agent.agentName}`}
            orgId={orgId}
            agent={agent}
            emphasised={
              debouncedSearch !== '' &&
              agent.environments.some((environment) => environment.matched)
            }
            environmentLabel={environmentLabel}
          />
        ))}
      </TileGrid>
      {total > PAGE_SIZE && (
        <TablePagination
          component="div"
          rowsPerPageOptions={[]}
          count={total}
          rowsPerPage={PAGE_SIZE}
          page={page}
          onPageChange={(_, next) => replaceParams((p) => writePage(p, next))}
        />
      )}
    </ResourceListShell>
  );
};
