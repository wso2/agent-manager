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

import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Box, ListingTable, SearchBar, Skeleton, Stack, TablePagination } from "@wso2/oxygen-ui";
import { CircleIcon, Search as SearchIcon } from "@wso2/oxygen-ui-icons-react";
import type { AgentKindResponse } from "@agent-management-platform/types";
import { CatalogKindCard } from "./CatalogKindCard";

const DEFAULT_ROWS_PER_PAGE = 6;
const ROWS_PER_PAGE_OPTIONS = [6, 12, 24];
const SEARCH_DEBOUNCE_MS = 300;

export interface CatalogKindListingProps {
  items: AgentKindResponse[];
  getViewPath: (item: AgentKindResponse) => string;
  isLoading?: boolean;
}

export const CatalogKindListing: React.FC<CatalogKindListingProps> = ({
  items,
  getViewPath,
  isLoading,
}) => {
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(DEFAULT_ROWS_PER_PAGE);
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const debounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (debounceTimer.current) clearTimeout(debounceTimer.current);
    },
    [],
  );

  const handleSearchChange = useCallback((event: React.ChangeEvent<HTMLInputElement>) => {
    const value = event.target.value;
    setSearch(value);
    if (debounceTimer.current) clearTimeout(debounceTimer.current);
    debounceTimer.current = setTimeout(() => {
      setDebouncedSearch(value);
      setPage(0);
    }, SEARCH_DEBOUNCE_MS);
  }, []);

  const filteredItems = useMemo(() => {
    const term = debouncedSearch.trim().toLowerCase();
    if (!term) return items;
    return items.filter(
      (item) =>
        item.displayName.toLowerCase().includes(term) ||
        item.name.toLowerCase().includes(term) ||
        (item.description ?? "").toLowerCase().includes(term),
    );
  }, [items, debouncedSearch]);

  const paginatedItems = useMemo(
    () => filteredItems.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage),
    [filteredItems, page, rowsPerPage],
  );

  const handlePageChange = (
    _event: React.MouseEvent<HTMLButtonElement> | null,
    newPage: number,
  ) => {
    setPage(newPage);
  };

  const handleRowsPerPageChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setRowsPerPage(parseInt(event.target.value, 10));
    setPage(0);
  };

  return (
    <Stack spacing={2}>
      <Stack direction="row" justifyContent="flex-end">
        <SearchBar
          placeholder="Search agent kinds"
          size="small"
          value={search}
          onChange={handleSearchChange}
        />
      </Stack>

      {isLoading ? (
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: {
              xs: "repeat(auto-fill, minmax(260px, 1fr))",
              md: "repeat(auto-fill, minmax(300px, 1fr))",
            },
            gap: 2,
          }}
        >
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} variant="rounded" height={160} />
          ))}
        </Box>
      ) : items.length === 0 ? (
        <ListingTable.Container sx={{ my: 3 }}>
          <ListingTable.EmptyState
            illustration={<CircleIcon size={64} />}
            title="No agent kinds available"
            description="No agent kinds have been added to the catalog yet."
          />
        </ListingTable.Container>
      ) : filteredItems.length === 0 ? (
        <ListingTable.Container sx={{ my: 3 }}>
          <ListingTable.EmptyState
            illustration={<SearchIcon size={64} />}
            title="No agent kinds match your search"
            description="Try a different keyword or clear the search filter."
          />
        </ListingTable.Container>
      ) : (
        <>
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: {
                xs: "repeat(auto-fill, minmax(260px, 1fr))",
                md: "repeat(auto-fill, minmax(300px, 1fr))",
              },
              gap: 2,
            }}
          >
            {paginatedItems.map((item) => (
              <CatalogKindCard key={item.name} item={item} viewPath={getViewPath(item)} />
            ))}
          </Box>
          {filteredItems.length > DEFAULT_ROWS_PER_PAGE && (
            <TablePagination
              component="div"
              count={filteredItems.length}
              page={page}
              rowsPerPage={rowsPerPage}
              onPageChange={handlePageChange}
              onRowsPerPageChange={handleRowsPerPageChange}
              rowsPerPageOptions={ROWS_PER_PAGE_OPTIONS}
            />
          )}
        </>
      )}
    </Stack>
  );
};

export default CatalogKindListing;
