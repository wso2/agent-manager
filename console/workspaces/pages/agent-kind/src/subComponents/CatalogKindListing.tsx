import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Box, ListingTable, SearchBar, Stack, TablePagination } from "@wso2/oxygen-ui";
import { CircleIcon, Search as SearchIcon } from "@wso2/oxygen-ui-icons-react";
import type { CatalogItem } from "../catalog.mock";
import { CatalogKindCard } from "./CatalogKindCard";

const DEFAULT_ROWS_PER_PAGE = 6;
const ROWS_PER_PAGE_OPTIONS = [6, 12, 24];
const SEARCH_DEBOUNCE_MS = 300;

export interface CatalogKindListingProps {
  items: CatalogItem[];
  getViewPath: (item: CatalogItem) => string;
}

export const CatalogKindListing: React.FC<CatalogKindListingProps> = ({ items, getViewPath }) => {
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
        item.title.toLowerCase().includes(term) ||
        item.tags.some((tag) => tag.toLowerCase().includes(term)),
    );
  }, [items, debouncedSearch]);

  const paginatedItems = useMemo(
    () => filteredItems.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage),
    [filteredItems, page, rowsPerPage],
  );

  const handlePageChange = (_event: React.MouseEvent<HTMLButtonElement> | null, newPage: number) => {
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

      {items.length === 0 && (
        <ListingTable.Container sx={{ my: 3 }}>
          <ListingTable.EmptyState
            illustration={<CircleIcon size={64} />}
            title="No agent kinds available"
            description="No agent kinds have been added to the catalog yet."
          />
        </ListingTable.Container>
      )}

      {items.length > 0 && filteredItems.length === 0 && (
        <ListingTable.Container sx={{ my: 3 }}>
          <ListingTable.EmptyState
            illustration={<SearchIcon size={64} />}
            title="No agent kinds match your search"
            description="Try a different keyword or clear the search filter."
          />
        </ListingTable.Container>
      )}

      {filteredItems.length > 0 && (
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
              <CatalogKindCard key={item.id} item={item} viewPath={getViewPath(item)} />
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
