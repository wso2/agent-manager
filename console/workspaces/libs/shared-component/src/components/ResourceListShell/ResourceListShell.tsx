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

import React from "react";
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
import { DocsLink, type DocsTarget } from "@agent-management-platform/views";

export interface ResourceListEmptyState {
  illustration: React.ReactNode;
  title: string;
  description: string;
  /**
   * Documentation for the thing this list holds. An empty list is where
   * someone is most likely to not yet know what they are being asked to
   * create, so the guide is offered right under the prompt to create one.
   */
  docs?: DocsTarget;
  /** Link text for {@link docs}. Defaults to "Learn more". */
  docsLabel?: string;
}

export interface ResourceListShellProps {
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
            action={
              emptyState.docs && (
                <DocsLink docs={emptyState.docs}>
                  {emptyState.docsLabel ?? "Learn more"}
                </DocsLink>
              )
            }
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
