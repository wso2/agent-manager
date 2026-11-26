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

import { useEffect, useState } from "react";
import {
  ListingTable,
  MenuItem,
  SearchBar,
  Stack,
  TablePagination,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Coins } from "@wso2/oxygen-ui-icons-react";
import { usePricing } from "@agent-management-platform/api-client";
import {
  isListEmpty,
  ResourceListShell,
} from "@agent-management-platform/shared-component";
import { formatPrice, formatUpdated, NO_VALUE } from "./pricingFormat";
import { TOOLBAR_FIELD_SX } from "../AICostManagement/controlSizes";

/** Page sizes, as the console's other lists. */
const ROWS_PER_PAGE_OPTIONS = [10, 25, 50];

/** The model prices the gateways use, read-only: search, a provider filter and paging. */
export function PricingList({ orgName }: { orgName?: string }) {
  const [search, setSearch] = useState("");
  const [provider, setProvider] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(ROWS_PER_PAGE_OPTIONS[1]);
  // The server searches; ask it once typing pauses, from the first page.
  const [query, setQuery] = useState("");
  useEffect(() => {
    const t = setTimeout(() => {
      setQuery(search.trim());
      setPage(0);
    }, 300);
    return () => clearTimeout(t);
  }, [search]);

  const { data, isLoading, error } = usePricing(
    { orgName: orgName ?? "" },
    {
      search: query || undefined,
      provider: provider || undefined,
      sort: "provider",
      limit: rowsPerPage,
      offset: page * rowsPerPage,
    },
  );
  const models = data?.models ?? [];
  const total = data?.total ?? 0;
  const isFiltered = !!query || !!provider;
  const isEmpty = isListEmpty({ isLoading, count: total, isFiltered });
  const updated = formatUpdated(data?.updatedAt);

  const providerSelect = (
    <TextField
      select
      size="small"
      value={provider}
      onChange={(e) => {
        setProvider(e.target.value);
        setPage(0);
      }}
      sx={{ minWidth: 200, flexShrink: 0, ...TOOLBAR_FIELD_SX }}
      slotProps={{
        htmlInput: { "aria-label": "Filter by provider" },
        select: { displayEmpty: true },
      }}
    >
      <MenuItem value="">All providers</MenuItem>
      {(data?.providers ?? []).map((p) => (
        <MenuItem key={p} value={p}>
          {p}
        </MenuItem>
      ))}
    </TextField>
  );

  return (
    <ResourceListShell
      resourceName="models"
      disablePaper
      searchValue={search}
      onSearchChange={setSearch}
      searchPlaceholder="Search models"
      toolbar={
        <Stack
          direction={{ xs: "column", md: "row" }}
          spacing={1.5}
          alignItems={{ xs: "stretch", md: "center" }}
          justifyContent="space-between"
        >
          <SearchBar
            placeholder="Search models"
            size="small"
            width="standard"
            value={search}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
              setSearch(e.target.value)
            }
          />
          {providerSelect}
        </Stack>
      }
      error={
        error
          ? new Error("Couldn't load pricing. Reload to try again.")
          : undefined
      }
      isLoading={isLoading}
      isEmpty={isEmpty}
      isSearchEmpty={!isLoading && isFiltered && total === 0}
      emptyState={{
        illustration: <Coins size={64} />,
        title: "No pricing yet",
        description: "Gateways send it when they connect.",
      }}
      searchEmptyState={{
        illustration: <Coins size={64} />,
        title: "No models match",
        description: "Try other search keywords or another provider.",
      }}
    >
      <ListingTable.Container>
        <ListingTable>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell>Model</ListingTable.Cell>
              <ListingTable.Cell>Provider</ListingTable.Cell>
              <ListingTable.Cell align="right">Input</ListingTable.Cell>
              <ListingTable.Cell align="right">Output</ListingTable.Cell>
              <ListingTable.Cell align="right">Cached input</ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {models.map((m) => (
              <ListingTable.Row key={m.model}>
                <ListingTable.Cell sx={{ maxWidth: 360 }}>
                  {/* One line, cut short: a row is as tall for any model name. */}
                  <Typography
                    variant="body2"
                    fontWeight={600}
                    noWrap
                    title={m.model}
                  >
                    {m.model}
                  </Typography>
                </ListingTable.Cell>
                <ListingTable.Cell>{m.provider ?? NO_VALUE}</ListingTable.Cell>
                <ListingTable.Cell align="right">
                  {formatPrice(m.inputPer1M)}
                </ListingTable.Cell>
                <ListingTable.Cell align="right">
                  {formatPrice(m.outputPer1M)}
                </ListingTable.Cell>
                <ListingTable.Cell align="right">
                  {formatPrice(m.cachedInputPer1M)}
                </ListingTable.Cell>
              </ListingTable.Row>
            ))}
          </ListingTable.Body>
        </ListingTable>
        {total > 0 && (
          <TablePagination
            rowsPerPageOptions={ROWS_PER_PAGE_OPTIONS}
            component="div"
            count={total}
            rowsPerPage={rowsPerPage}
            page={page}
            onPageChange={(_, next) => setPage(next)}
            onRowsPerPageChange={(e) => {
              setRowsPerPage(parseInt(e.target.value, 10));
              setPage(0);
            }}
          />
        )}
      </ListingTable.Container>
      {total > 0 && (
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ display: "block", mt: 1 }}
        >
          {updated
            ? `USD per 1M tokens. Updated ${updated}.`
            : "USD per 1M tokens."}
        </Typography>
      )}
    </ResourceListShell>
  );
}
