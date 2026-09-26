/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useEffect, useRef } from "react";
import { ConsoleAction, useTrack } from "@agent-management-platform/api-client";
import {
  Alert,
  Box,
  Button,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
} from "@wso2/oxygen-ui";
import { AlertTriangle, Plus, Search } from "@wso2/oxygen-ui-icons-react";

export interface ResourceListEmptyState {
  illustration: React.ReactNode;
  title: string;
  description: string;
}

export interface ResourceListShellProps {
  /**
   * What this list holds, e.g. "agents", "gateways". Used as the analytics
   * label for searches and empty states; defaults to "unspecified".
   */
  resourceName?: string;
  /** Search input value. */
  searchValue: string;
  onSearchChange: (value: string) => void;
  searchPlaceholder?: string;
  /** Right-aligned button rendered next to the search bar. */
  addButton?: {
    label: string;
    to?: string;
    onClick?: () => void;
    component?: React.ElementType;
  };
  /** A render override for the toolbar — when provided, replaces the default toolbar entirely. */
  toolbar?: React.ReactNode;

  /** Mutually exclusive states evaluated top-down. */
  error?: unknown;
  isLoading?: boolean;
  isEmpty?: boolean;
  isSearchEmpty?: boolean;

  /** Optional override for the loading placeholder; defaults to generic skeleton rows. */
  loadingPlaceholder?: React.ReactNode;
  /** Empty-state shown when there are no items at all. */
  emptyState: ResourceListEmptyState;
  /** Empty-state shown when the search filter returns no results. */
  searchEmptyState?: ResourceListEmptyState;

  /** The "happy path" content (typically a ListingTable). */
  children: React.ReactNode;
}

const DEFAULT_LOADING_ROWS = 5;

/** Long enough that typing a word reports one search, not eight. */
const SEARCH_TRACK_DEBOUNCE_MS = 1200;

function DefaultLoadingRows() {
  return (
    <Stack spacing={1} mt={1}>
      {Array.from({ length: DEFAULT_LOADING_ROWS }).map((_, index) => (
        <Stack
          key={index}
          direction="row"
          alignItems="center"
          spacing={2}
          sx={{
            px: 2,
            py: 1.5,
            borderRadius: 1,
            border: "1px solid",
            borderColor: "divider",
            bgcolor: "background.paper",
          }}
        >
          <Skeleton variant="circular" width={36} height={36} />
          <Skeleton variant="text" width={180} height={20} />
          <Skeleton variant="text" sx={{ flex: 1 }} height={18} />
        </Stack>
      ))}
    </Stack>
  );
}

export const ResourceListShell: React.FC<ResourceListShellProps> = ({
  resourceName = "unspecified",
  searchValue,
  onSearchChange,
  searchPlaceholder = "Search...",
  addButton,
  toolbar,
  error,
  isLoading,
  isEmpty,
  isSearchEmpty,
  loadingPlaceholder,
  emptyState,
  searchEmptyState,
  children,
}) => {
  const { track } = useTrack();

  // Searching is a read, so nothing server-side records it. Reported on a
  // debounce rather than per keystroke: the input is controlled, so a raw
  // handler would emit one action per character typed.
  useEffect(() => {
    if (!searchValue) return;
    const timer = setTimeout(() => {
      track(ConsoleAction.Search, {
        scope: resourceName,
        // The query itself is never reported — users search for their own
        // resource names. Only its length, which distinguishes a stray
        // keystroke from a real search.
        query_length: searchValue.length,
        zero_results: isSearchEmpty === true,
      });
    }, SEARCH_TRACK_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [searchValue, isSearchEmpty, resourceName, track]);

  // An empty list is where activation stalls: someone reached the page and had
  // nothing to work with. Reported once per mount per state, not per render.
  const reportedEmpty = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (isLoading || error) return;
    const state = isEmpty ? "empty" : isSearchEmpty ? "search-empty" : undefined;
    if (!state || reportedEmpty.current === state) return;
    reportedEmpty.current = state;
    track(ConsoleAction.EmptyState, { resource: resourceName, state });
  }, [isLoading, error, isEmpty, isSearchEmpty, resourceName, track]);

  const resolvedToolbar = toolbar ?? (
    <Stack direction="row" spacing={1} alignItems="center">
      <Box flexGrow={1}>
        <SearchBar
          key="search-bar"
          placeholder={searchPlaceholder}
          size="small"
          fullWidth
          value={searchValue}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
            onSearchChange(e.target.value)
          }
        />
      </Box>
      {addButton && (
        <Button
          {...(addButton.component
            ? { component: addButton.component, to: addButton.to }
            : {})}
          onClick={addButton.onClick}
          variant="contained"
          color="primary"
          startIcon={<Plus size={16} />}
        >
          {addButton.label}
        </Button>
      )}
    </Stack>
  );

  const renderBody = () => {
    if (error) {
      return (
        <ListingTable.Container>
          <Alert
            severity="error"
            icon={<AlertTriangle size={18} />}
            sx={{ alignSelf: "stretch" }}
          >
            {error instanceof Error
              ? error.message
              : "Failed to load. Please try again."}
          </Alert>
        </ListingTable.Container>
      );
    }

    if (isLoading) {
      return (
        <ListingTable.Container disablePaper>
          {loadingPlaceholder ?? <DefaultLoadingRows />}
        </ListingTable.Container>
      );
    }

    if (isEmpty) {
      return (
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={emptyState.illustration}
            title={emptyState.title}
            description={emptyState.description}
          />
        </ListingTable.Container>
      );
    }

    if (isSearchEmpty) {
      const empty = searchEmptyState ?? {
        illustration: <Search size={64} />,
        title: "No results match your search",
        description: "Try a different keyword or clear the search filter.",
      };
      return (
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={empty.illustration}
            title={empty.title}
            description={empty.description}
          />
        </ListingTable.Container>
      );
    }

    return (
      <ListingTable.Container disablePaper>
        <Stack pt={4}>{children}</Stack>
      </ListingTable.Container>
    );
  };
  return (
    <Stack spacing={1}>
      {resolvedToolbar}
      {renderBody()}
    </Stack>
  );
};

export default ResourceListShell;
