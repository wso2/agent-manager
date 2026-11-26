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
  Button,
  Chip,
  ListingTable,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  SearchBar,
  TablePagination,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  ChevronDown,
  Plus,
  Trash2,
  Upload,
  Wallet,
} from "@wso2/oxygen-ui-icons-react";
import { useFeatureManagement } from "@agent-management-platform/feature-management";
import { useListBudgets } from "@agent-management-platform/api-client";
import {
  isListEmpty,
  ResourceListShell,
} from "@agent-management-platform/shared-component";
import { useConfirmDeleteBudget } from "./useConfirmDeleteBudget";
import { type Budget } from "@agent-management-platform/types";
import { amount, formatUsd } from "../costFormat";
import { PERIOD_LABELS } from "./budgetLabels";
import { ImportBudgetsDialog } from "./ImportBudgetsDialog";
import { TOOLBAR_FIELD_SX } from "../controlSizes";

/** Delete shows on hover or focus, as in the console's other lists; touch shows it always. */
const ROW_HOVER_SX = {
  "& .budget-actions": { visibility: "hidden" },
  "&:hover .budget-actions, &:focus-within .budget-actions": {
    visibility: "visible",
  },
  "@media (hover: none)": { "& .budget-actions": { visibility: "visible" } },
} as const;

interface BudgetsTabProps {
  orgName?: string;
  /** Set on a project's page: only the budgets that count that project's usage. */
  projectName?: string;
  /** Set on an agent's page (with its project): the budgets that count its usage. */
  agentName?: string;
  /** Opens a budget's detail page. */
  onOpen: (budgetName: string) => void;
  /** The create action, shown in the empty state; the page header has it otherwise. */
  createAction?: React.ReactNode;
  /** Told whether the list is empty, so the page shows its header action only when not. */
  onEmptyChange?: (empty: boolean) => void;
}

/** What a budget's limit is split by (each agent, each API key, ...), if anything. */
export function separateKind(b: Budget): string | undefined {
  const t = b.spec.limit.splitBy?.type;
  if (!t) return undefined;
  return t === "llmProvider"
    ? "LLM provider"
    : t === "apiKey"
      ? "API key"
      : t === "attribute"
        ? "value"
        : t;
}

/** A budget's name is cut at this width, so the list does not depend on its length. */
const NAME_MAX_WIDTH = 360;

/** Page sizes, as the console's other lists. */
const ROWS_PER_PAGE_OPTIONS = [5, 10, 25];
/** Warning chips shown in a row; the rest are one "+N" chip, so rows keep their height. */
const WARNING_CHIPS_SHOWN = 3;

function WarningChips({ thresholds }: { thresholds: number[] }) {
  if (!thresholds.length) return <>None</>;
  const shown = thresholds.slice(0, WARNING_CHIPS_SHOWN);
  const rest = thresholds.slice(WARNING_CHIPS_SHOWN);
  return (
    <Stack direction="row" spacing={0.5} sx={{ flexWrap: "nowrap" }}>
      {shown.map((t) => (
        <Chip key={t} size="small" label={`${t}%`} />
      ))}
      {rest.length > 0 && (
        <Tooltip title={rest.map((t) => `${t}%`).join(", ")}>
          <Chip size="small" variant="outlined" label={`+${rest.length}`} />
        </Tooltip>
      )}
    </Stack>
  );
}

type StatusFilter = "all" | "active" | "inactive" | "over";
const STATUS_FILTERS: { value: StatusFilter; label: string }[] = [
  { value: "all", label: "All budgets" },
  { value: "active", label: "Active" },
  { value: "inactive", label: "Inactive" },
  { value: "over", label: "Limit reached" },
];

/** This period's spend and its share of the limit. */
function Usage({ budget }: { budget: Budget }) {
  const period = budget.status?.currentPeriod;
  if (!period) {
    return (
      <Typography variant="body2" color="text.secondary">
        No spend yet
      </Typography>
    );
  }
  const spent = amount(period.spent);
  const limit = amount(budget.spec.limit.amount);
  // Counted separately, each usage has the whole limit: the total is not a share of it.
  const each = separateKind(budget);
  const share =
    limit > 0 && !each ? Math.round((spent / limit) * 100) : undefined;
  const inactive = budget.spec.status === "inactive";
  return (
    <Stack spacing={0}>
      <Typography variant="body2">{formatUsd(spent)}</Typography>
      <Typography
        variant="caption"
        color={
          !inactive && share !== undefined && share >= 100
            ? "error.main"
            : "text.secondary"
        }
      >
        {each
          ? `Each ${each} has its own limit`
          : share !== undefined
            ? `${share}% of the limit`
            : ""}
      </Typography>
    </Stack>
  );
}

/** Every budget of the organization, or of one project, with its spend this period. */
export function BudgetsTab({
  orgName,
  projectName,
  agentName,
  onOpen,
  createAction,
  onEmptyChange,
}: BudgetsTabProps) {
  // Budgets are managed at organization level; a project sees, read-only, the budgets
  // that count its usage.
  const { hasAccess } = useFeatureManagement();
  // Row actions are for the organization's list only.
  const canDelete = hasAccess("cost_delete") && !projectName;
  const { confirmDelete } = useConfirmDeleteBudget(orgName);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
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
  // Search, status and paging are the API's: the list shows one page of the result.
  const { data, isLoading, error } = useListBudgets(
    { orgName },
    {
      filter: !projectName
        ? undefined
        : agentName
          ? `agent:${projectName}/${agentName}`
          : `project:${projectName}`,
      search: query || undefined,
      status:
        statusFilter === "all"
          ? undefined
          : statusFilter === "over"
            ? "limitReached"
            : statusFilter,
      limit: rowsPerPage,
      offset: page * rowsPerPage,
    },
  );
  const budgets = data?.budgets ?? [];
  const total = data?.total ?? 0;
  const isFiltered = !!query || statusFilter !== "all";
  const isEmpty = isListEmpty({ isLoading, count: total, isFiltered });
  useEffect(() => onEmptyChange?.(isEmpty), [isEmpty, onEmptyChange]);
  const statusSelect = (
    <TextField
      select
      size="small"
      value={statusFilter}
      onChange={(e) => {
        setStatusFilter(e.target.value as StatusFilter);
        setPage(0);
      }}
      sx={{ minWidth: 200, flexShrink: 0, ...TOOLBAR_FIELD_SX }}
      slotProps={{ htmlInput: { "aria-label": "Filter by status" } }}
    >
      {STATUS_FILTERS.map((f) => (
        <MenuItem key={f.value} value={f.value}>
          {f.label}
        </MenuItem>
      ))}
    </TextField>
  );

  return (
    <>
      <ResourceListShell
        resourceName="budgets"
        disablePaper
        searchValue={search}
        onSearchChange={setSearch}
        searchPlaceholder="Search budgets"
        toolbar={
          <Stack
            direction={{ xs: "column", md: "row" }}
            spacing={1.5}
            alignItems={{ xs: "stretch", md: "center" }}
            justifyContent="space-between"
          >
            <SearchBar
              placeholder="Search by name or label"
              size="small"
              width="standard"
              value={search}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                setSearch(e.target.value)
              }
            />
            {statusSelect}
          </Stack>
        }
        error={
          error
            ? new Error("Couldn't load budgets. Reload to try again.")
            : undefined
        }
        isLoading={isLoading}
        isEmpty={isEmpty}
        isSearchEmpty={!isLoading && isFiltered && total === 0}
        emptyState={{
          illustration: <Wallet size={64} />,
          title: agentName
            ? "No budgets apply to this agent"
            : projectName
              ? "No budgets apply to this project"
              : "No budgets yet",
          description: agentName
            ? "Budgets for this agent show here."
            : projectName
              ? "Budgets for this project show here."
              : "Cap LLM spend per day, week or month.",
          // While the list is empty the create action is here, not in the page header.
          action: createAction,
        }}
        searchEmptyState={{
          illustration: <Wallet size={64} />,
          title: "No budgets match",
          description: "Try other search keywords or another filter.",
        }}
      >
        {/* The table in its own card, apart from the search and filters above. */}
        <ListingTable.Container>
          <ListingTable>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>Budget</ListingTable.Cell>
                <ListingTable.Cell>Amount (USD)</ListingTable.Cell>
                <ListingTable.Cell>Period</ListingTable.Cell>
                <ListingTable.Cell>At the limit</ListingTable.Cell>
                <ListingTable.Cell>This period</ListingTable.Cell>
                <ListingTable.Cell>Warnings</ListingTable.Cell>
                <ListingTable.Cell>Status</ListingTable.Cell>
                {canDelete && <ListingTable.Cell width="140px" />}
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {budgets.map((b) => {
                const { limit, warnings, status, displayName } = b.spec;
                const inactive = status === "inactive";
                return (
                  <ListingTable.Row
                    key={b.metadata.name}
                    hover
                    onClick={() => onOpen(b.metadata.name)}
                    // Inactive budgets count and block nothing: shown, but in the background.
                    sx={{
                      cursor: "pointer",
                      opacity: inactive ? 0.6 : 1,
                      ...ROW_HOVER_SX,
                    }}
                  >
                    <ListingTable.Cell>
                      {/* One line each, cut short: a row is as tall for any name. */}
                      <Stack spacing={0} sx={{ maxWidth: NAME_MAX_WIDTH }}>
                        <Typography
                          variant="body2"
                          fontWeight={600}
                          noWrap
                          title={displayName ?? b.metadata.name}
                        >
                          {displayName ?? b.metadata.name}
                        </Typography>
                        {displayName && displayName !== b.metadata.name && (
                          <Typography
                            variant="caption"
                            color="text.secondary"
                            noWrap
                            title={b.metadata.name}
                          >
                            {b.metadata.name}
                          </Typography>
                        )}
                      </Stack>
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      {formatUsd(amount(limit.amount))}
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      {PERIOD_LABELS[limit.period]}
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Chip
                        size="small"
                        variant="outlined"
                        color={limit.action === "block" ? "warning" : "default"}
                        label={limit.action === "block" ? "Block" : "Allow"}
                      />
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Usage budget={b} />
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <WarningChips thresholds={warnings?.thresholds ?? []} />
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Chip
                        size="small"
                        color={inactive ? "default" : "success"}
                        variant="outlined"
                        label={inactive ? "Inactive" : "Active"}
                      />
                    </ListingTable.Cell>
                    {canDelete && (
                      <ListingTable.Cell
                        align="right"
                        padding="none"
                        sx={{ pr: 1.5 }}
                      >
                        {/* Gateway-made budgets are deleted on their gateway. */}
                        {b.status?.origin !== "gatewayApi" && (
                          <Stack
                            className="budget-actions"
                            direction="row"
                            justifyContent="flex-end"
                          >
                            <Button
                              variant="outlined"
                              color="error"
                              size="small"
                              startIcon={<Trash2 size={14} />}
                              onClick={(e) => {
                                e.stopPropagation();
                                confirmDelete(
                                  b.metadata.name,
                                  displayName ?? b.metadata.name,
                                );
                              }}
                            >
                              Delete
                            </Button>
                          </Stack>
                        )}
                      </ListingTable.Cell>
                    )}
                  </ListingTable.Row>
                );
              })}
            </ListingTable.Body>
          </ListingTable>
          {/* Always shown with results, so a short search still shows its count and page size. */}
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
      </ResourceListShell>
    </>
  );
}

/**
 * The create action of a budgets list: on the organization's list, Create budget with
 * its menu (the form, or a CSV import); on a project's or agent's page, Add budget for
 * it. Nothing without budgets.create.
 */
export function BudgetCreateAction({
  orgName,
  projectName,
  onCreate,
}: {
  orgName?: string;
  projectName?: string;
  onCreate: () => void;
}) {
  const canCreate = useFeatureManagement().hasAccess("cost_write");
  const [importing, setImporting] = useState(false);
  if (!canCreate) return null;
  if (projectName) {
    return (
      <Button
        variant="contained"
        startIcon={<Plus size={16} />}
        onClick={onCreate}
        sx={{ flexShrink: 0, whiteSpace: "nowrap" }}
      >
        Add budget
      </Button>
    );
  }
  return (
    <>
      <CreateBudgetMenu
        onCreate={onCreate}
        onImport={() => setImporting(true)}
      />
      <ImportBudgetsDialog
        open={importing}
        orgName={orgName}
        onClose={() => setImporting(false)}
      />
    </>
  );
}

/** Create budget, with a menu right under it: the form, or an import from a CSV file. */
function CreateBudgetMenu({
  onCreate,
  onImport,
}: {
  onCreate: () => void;
  onImport: () => void;
}) {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const pick = (then: () => void) => () => {
    setAnchor(null);
    then();
  };
  return (
    <>
      <Button
        variant="contained"
        startIcon={<Plus size={16} />}
        endIcon={<ChevronDown size={16} />}
        onClick={(e) => setAnchor(e.currentTarget)}
        aria-haspopup="menu"
        aria-expanded={anchor ? "true" : undefined}
        sx={{ whiteSpace: "nowrap", flexShrink: 0 }}
      >
        Create budget
      </Button>
      <Menu
        open={!!anchor}
        anchorEl={anchor}
        onClose={() => setAnchor(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
        transformOrigin={{ vertical: "top", horizontal: "right" }}
      >
        <MenuItem onClick={pick(onCreate)}>
          <ListItemIcon>
            <Plus size={16} />
          </ListItemIcon>
          <ListItemText
            primary="Create manually"
            secondary="Configure a new budget"
          />
        </MenuItem>
        <MenuItem onClick={pick(onImport)}>
          <ListItemIcon>
            <Upload size={16} />
          </ListItemIcon>
          <ListItemText
            primary="Import from CSV"
            secondary="Create or update budgets in bulk"
          />
        </MenuItem>
      </Menu>
    </>
  );
}
