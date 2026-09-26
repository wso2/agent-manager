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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useMemo, useState } from "react";
import {
  Box,
  Button,
  IconButton,
  ListingTable,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { formatDistanceToNow } from "date-fns";
import {
  AlertTriangle,
  Plus,
  ServerCog,
  Trash,
} from "@wso2/oxygen-ui-icons-react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
// This table is generic over an agent's configs (LLM, MCP, ...). The types lib
// intentionally aliases the MCP list response to the model-config one, so the
// item shape is genuinely shared; alias it to a neutral name for readability.
import { type AgentModelConfigListItem as AgentConfigListItem } from "@agent-management-platform/types";
import {
  ConfigTableEmptyState,
  ConfigTableSection,
  EmptyStateContent,
} from "./ConfigTableSection";

const COLUMN_COUNT = 3;
const ROWS_PER_PAGE_OPTIONS = [10, 25, 50];

/** Copy that distinguishes one agent-config listing (LLM, MCP, ...) from another. */
export interface AgentConfigTableLabels {
  title: string;
  searchPlaceholder: string;
  addButtonLabel: string;
  emptyTitle: string;
  emptyDescription: string;
  errorTitle: string;
  errorFallback: string;
  searchEmptyTitle: string;
  searchEmptyDescription: string;
  removeTitle: string;
  removeTooltip: string;
  removeConfirmation: (config: AgentConfigListItem) => string;
  removeAriaLabel: (config: AgentConfigListItem) => string;
}

interface AgentConfigTableSectionProps {
  /** Configs of a single type, sliced from the page's one model-config call. */
  configs: AgentConfigListItem[];
  isLoading: boolean;
  error: unknown;
  labels: AgentConfigTableLabels;
  /** Absolute path to the "add" page for this config type. Ignored when `onAdd` is set. */
  addPath?: string;
  /**
   * When provided, the Add button triggers this callback (e.g. to open an inline panel)
   * instead of navigating to `addPath`. Used by the MCP tab's master-detail flow.
   */
  onAdd?: () => void;
  /** Builds the absolute path to a config's detail page. */
  getViewPath: (configId: string) => string;
  onRemove: (configId: string) => void;
  /** Disables the row remove action while a delete is in flight. */
  isRemoving?: boolean;
  /**
   * Render the section heading above the table. Set false when the section sits
   * under a tab whose label already names it. Defaults to true.
   */
  showTitle?: boolean;
  /**
   * Blocks the Add button for a reason of the caller's own (e.g. the agent's
   * first build has not produced an image yet). Independent of the internal
   * route-param check, which disables the button regardless.
   */
  addDisabled?: boolean;
  /** Tooltip explaining `addDisabled`. Required for the block to be discoverable. */
  addDisabledReason?: string;
}

/**
 * A searchable, paginated listing of an agent's model configs of a given type.
 * The LLM Configurations and MCP Servers tables are identical apart from their copy
 * and routes, so both render through this component.
 */
export function AgentConfigTableSection({
  configs,
  isLoading,
  error,
  labels,
  addPath,
  onAdd,
  getViewPath,
  onRemove,
  isRemoving = false,
  showTitle = true,
  addDisabled = false,
  addDisabledReason,
}: AgentConfigTableSectionProps) {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();
  const navigate = useNavigate();
  const { addConfirmation } = useConfirmationDialog();

  const [searchValue, setSearchValue] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);

  const canAdd = Boolean(orgId && projectId && agentId) && !addDisabled;

  const filteredConfigs = useMemo(() => {
    if (!searchValue.trim()) return configs;
    const lower = searchValue.toLowerCase();
    return configs.filter(
      (c) =>
        c.name.toLowerCase().includes(lower) ||
        (c.description ?? "").toLowerCase().includes(lower) ||
        c.type.toLowerCase().includes(lower),
    );
  }, [configs, searchValue]);

  const totalCount = filteredConfigs.length;

  // Pagination is client-side: the page fetches all configs in one call and
  // splits them between the tables by type. Clamp the page so it stays valid
  // when the list shrinks (e.g. after a delete or a search).
  const pageCount = Math.max(1, Math.ceil(totalCount / rowsPerPage));
  const safePage = Math.min(page, pageCount - 1);
  const paginatedConfigs = useMemo(
    () =>
      filteredConfigs.slice(
        safePage * rowsPerPage,
        safePage * rowsPerPage + rowsPerPage,
      ),
    [filteredConfigs, safePage, rowsPerPage],
  );

  const handleSearchChange = (value: string) => {
    setSearchValue(value);
    setPage(0);
  };

  const handleDelete = (config: AgentConfigListItem) => {
    addConfirmation({
      analytics: { entity: "agent-config", action: "remove" },
      title: labels.removeTitle,
      description: labels.removeConfirmation(config),
      confirmButtonText: "Remove",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () => onRemove(config.uuid),
    });
  };

  const addButton = (variant: "contained" | "outlined") => {
    const button = onAdd ? (
      <Button
        variant={variant}
        color="primary"
        size="small"
        startIcon={<Plus size={16} />}
        disabled={!canAdd}
        onClick={onAdd}
      >
        {labels.addButtonLabel}
      </Button>
    ) : (
      <Button
        component={Link}
        to={addPath ?? "#"}
        variant={variant}
        color="primary"
        size="small"
        startIcon={<Plus size={16} />}
        disabled={!canAdd}
      >
        {labels.addButtonLabel}
      </Button>
    );
    if (!addDisabled || !addDisabledReason) return button;
    // A disabled MUI Button drops pointer events, so the tooltip has to listen
    // on a wrapper — otherwise the one explanation the user needs never shows.
    return (
      <Tooltip title={addDisabledReason}>
        <span>{button}</span>
      </Tooltip>
    );
  };

  const toolbar = (
    <ListingTable.Toolbar
      showSearch
      searchValue={searchValue}
      onSearchChange={handleSearchChange}
      searchPlaceholder={labels.searchPlaceholder}
      actions={addButton("contained")}
    />
  );

  const tableHeader = (
    <ListingTable.Head>
      <ListingTable.Row>
        <ListingTable.Cell width="70%">Name</ListingTable.Cell>
        <ListingTable.Cell width="20%">Created</ListingTable.Cell>
        <ListingTable.Cell width="10%" align="right">
          Actions
        </ListingTable.Cell>
      </ListingTable.Row>
    </ListingTable.Head>
  );

  // A genuinely empty list (no configs at all, before any search) renders as a
  // standalone centered empty state with no table header or pagination around it.
  const isGenuinelyEmpty = !error && configs.length === 0;
  const standaloneEmptyState = isGenuinelyEmpty ? (
    <EmptyStateContent
      illustration={<ServerCog size={64} />}
      title={labels.emptyTitle}
      description={labels.emptyDescription}
      action={addButton("outlined")}
    />
  ) : undefined;

  // The error and zero-result-search cases keep the table chrome (header + search
  // box) in place, so they stay rendered inside the table body.
  const getEmptyState = () => {
    if (error) {
      return (
        <ConfigTableEmptyState
          colSpan={COLUMN_COUNT}
          illustration={
            <Box component="span" sx={{ color: "error.main" }}>
              <AlertTriangle size={64} />
            </Box>
          }
          title={labels.errorTitle}
          description={
            error instanceof Error ? error.message : labels.errorFallback
          }
        />
      );
    }
    return (
      <ConfigTableEmptyState
        colSpan={COLUMN_COUNT}
        illustration={<ServerCog size={64} />}
        title={labels.searchEmptyTitle}
        description={labels.searchEmptyDescription}
      />
    );
  };

  return (
    <ConfigTableSection
      title={showTitle ? labels.title : undefined}
      toolbar={toolbar}
      // Keep the search/toolbar visible whenever the agent has configs of this
      // type, so filtering down to zero results doesn't hide the search input.
      showToolbar={configs.length > 0}
      tableHeader={tableHeader}
      isLoading={isLoading}
      hasRows={filteredConfigs.length > 0}
      emptyState={getEmptyState()}
      standaloneEmptyState={standaloneEmptyState}
      pagination={
        <TablePagination
          rowsPerPageOptions={ROWS_PER_PAGE_OPTIONS}
          component="div"
          count={totalCount}
          rowsPerPage={rowsPerPage}
          page={safePage}
          onPageChange={(_, newPage) => setPage(newPage)}
          onRowsPerPageChange={(e) => {
            setRowsPerPage(parseInt(e.target.value, 10));
            setPage(0);
          }}
        />
      }
    >
      {paginatedConfigs.map((config) => (
        <ListingTable.Row
          key={config.uuid}
          hover
          clickable
          onClick={() => navigate(getViewPath(config.uuid))}
        >
          <ListingTable.Cell>
            <Typography variant="body2">{config.name}</Typography>
          </ListingTable.Cell>
          <ListingTable.Cell>
            {config.createdAt
              ? formatDistanceToNow(new Date(config.createdAt), {
                  addSuffix: true,
                })
              : "-"}
          </ListingTable.Cell>
          <ListingTable.Cell align="right">
            <Tooltip title={labels.removeTooltip}>
              <IconButton
                color="error"
                size="small"
                disabled={isRemoving}
                onClick={(e: React.MouseEvent) => {
                  e.stopPropagation();
                  handleDelete(config);
                }}
                aria-label={labels.removeAriaLabel(config)}
              >
                <Trash size={16} />
              </IconButton>
            </Tooltip>
          </ListingTable.Cell>
        </ListingTable.Row>
      ))}
    </ConfigTableSection>
  );
}
