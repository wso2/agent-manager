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

import { useEffect, useState, type ReactNode } from "react";
import {
  Button,
  IconButton,
  ListingTable,
  SearchBar,
  Stack,
  Switch,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Pencil, ReceiptText, SearchX, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useLocation, useNavigate, useParams } from "react-router-dom";
import {
  useDeleteSubscriptionPlan,
  useListSubscriptionPlans,
  useUpdateSubscriptionPlan,
} from "@agent-management-platform/api-client";
import {
  ResourceListShell,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";
import { returnToState } from "@agent-management-platform/views";
import { useFeatureAccess } from "@agent-management-platform/feature-management";
import { absoluteRouteMap, type SubscriptionPlan } from "@agent-management-platform/types";
import { PlanConditionChips } from "./PlanConditionChips";
import { PAGE_SIZE_OPTIONS } from "./pageSize";

const SEARCH_DEBOUNCE_MS = 300;

/** The value, once it has stopped changing for `delay` ms. */
function useDebouncedValue<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

interface SubscriptionPlanTableProps {
  /** Shown in the empty state; the page owns how a plan gets created. */
  createAction?: ReactNode;
}

/** The organization's plans: a full-width search, then a table
 * of name, conditions, pricing, an active switch and edit and delete, paged on the server. */
export function SubscriptionPlanTable({ createAction }: SubscriptionPlanTableProps) {
  const { orgId = "" } = useParams<{ orgId: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const canUpdate = useFeatureAccess("subscription_plan_update");
  const canDelete = useFeatureAccess("subscription_plan_delete");
  const { addConfirmation } = useConfirmationDialog();
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(PAGE_SIZE_OPTIONS[0]);
  const debouncedSearch = useDebouncedValue(search.trim(), SEARCH_DEBOUNCE_MS);

  const { data, isLoading, error, isPlaceholderData } = useListSubscriptionPlans(
    { orgName: orgId },
    { limit: rowsPerPage, offset: page * rowsPerPage, query: debouncedSearch || undefined },
  );
  // While the next page, search or organization loads, the rows on screen are the previous ones:
  // their actions are off, so none acts on a row that is no longer there.
  const stale = isPlaceholderData;
  const updatePlan = useUpdateSubscriptionPlan();
  const deletePlan = useDeleteSubscriptionPlan();
  // Status switches being saved, keyed `${orgId}:${planId}`, with the status they are being set
  // to. Only those switches wait; the others stay usable. A Map, so no plan id hits an inherited
  // property.
  const [saving, setSaving] = useState<ReadonlyMap<string, SubscriptionPlan["status"]>>(
    () => new Map(),
  );
  const savingKey = (planId: string) => `${orgId}:${planId}`;
  // Switches of another organization are not on screen: drop them.
  useEffect(() => {
    setSaving(new Map());
  }, [orgId]);

  const plans = data?.list ?? [];
  const total = data?.pagination.total ?? 0;
  const lastPage = Math.max(0, Math.ceil(total / rowsPerPage) - 1);
  const currentPage = Math.min(page, lastPage);
  // The create prompt is for an organization with no plans, not for an empty search.
  const isFirstRun = !isLoading && total === 0 && debouncedSearch === "" && search.trim() === "";

  // A new search starts from the first page.
  useEffect(() => setPage(0), [debouncedSearch]);
  // Deleting the only row on the last page leaves `page` past the end.
  useEffect(() => {
    if (page > lastPage) setPage(lastPage);
  }, [page, lastPage]);

  const toggleStatus = (plan: SubscriptionPlan) => {
    const status = plan.status === "ACTIVE" ? "INACTIVE" : "ACTIVE";
    const key = savingKey(plan.id);
    // Removes only this call's own entry: a later toggle of the same switch has replaced it.
    const done = () =>
      setSaving((current) => {
        if (current.get(key) !== status) return current;
        const next = new Map(current);
        next.delete(key);
        return next;
      });
    setSaving((current) => new Map(current).set(key, status));
    // Each call's own promise releases its switch: a second toggle before this one finishes
    // replaces `mutate`'s per-call callbacks, which would leave this switch waiting.
    // On success the list already holds the new status; on failure the switch goes back. The
    // failure itself is shown by the hook.
    updatePlan
      .mutateAsync({
        // A partial update, so a stale row can't overwrite the plan's conditions.
        body: { displayName: plan.displayName, id: plan.id, status },
        params: { orgName: orgId, planId: plan.id },
        successText: `${status === "ACTIVE" ? "Activated" : "Deactivated"} plan "${plan.displayName}".`,
      })
      .catch(() => undefined)
      .finally(done);
  };

  // The name must be typed to confirm. A plan in use can't be deleted: the server refuses it and
  // the hook shows its reason.
  const confirmDelete = (plan: SubscriptionPlan) =>
    addConfirmation({
      analytics: { action: "delete", entity: "subscription-plan" },
      confirmButtonColor: "error",
      confirmButtonText: "Delete",
      confirmationInputLabel: "Type the plan name to confirm",
      confirmationText: plan.displayName,
      description: `This permanently deletes "${plan.displayName}". This can’t be undone.`,
      onConfirm: () =>
        deletePlan.mutate({ displayName: plan.displayName, orgName: orgId, planId: plan.id }),
      title: "Delete plan",
    });

  const editPath = (plan: SubscriptionPlan) =>
    generatePath(absoluteRouteMap.children.org.children.subscriptionPlans.children.edit.path, {
      orgId,
      planId: plan.id,
    });

  return (
    <ResourceListShell
      emptyState={{
        action: createAction,
        description:
          "Plans set the conditions an agent subscribes to, such as how many requests, tool " +
          "calls or tokens it can use in a time window. Offer them on the MCP servers you manage.",
        illustration: <ReceiptText size={64} />,
        title: "Create your first subscription plan",
      }}
      error={error}
      isEmpty={isFirstRun}
      isLoading={isLoading}
      isSearchEmpty={plans.length === 0}
      onSearchChange={setSearch}
      resourceName="subscription-plans"
      searchEmptyState={{
        action: (
          <Button onClick={() => setSearch("")} variant="outlined">
            Clear search
          </Button>
        ),
        description: "Try a different search term.",
        illustration: <SearchX size={64} />,
        title: "No matching plans",
      }}
      searchPlaceholder="Search plans"
      searchValue={search}
      toolbar={
        <SearchBar
          fullWidth
          onChange={(event) => setSearch(event.target.value)}
          placeholder="Search plans"
          value={search}
        />
      }
    >
      <Stack spacing={2.5}>
        <ListingTable.Provider>
          <ListingTable.Container>
            <ListingTable>
              <ListingTable.Head>
                <ListingTable.Row>
                  <ListingTable.Cell>Name</ListingTable.Cell>
                  <ListingTable.Cell>Conditions</ListingTable.Cell>
                  <ListingTable.Cell>Pricing</ListingTable.Cell>
                  <ListingTable.Cell align="center" sx={{ width: 96 }}>
                    Status
                  </ListingTable.Cell>
                  <ListingTable.Cell align="center" sx={{ width: 112 }}>
                    Actions
                  </ListingTable.Cell>
                </ListingTable.Row>
              </ListingTable.Head>
              <ListingTable.Body>
                {plans.map((plan) => {
                  const pendingStatus = saving.get(savingKey(plan.id));
                  const status = pendingStatus ?? plan.status;
                  return (
                    <ListingTable.Row key={plan.id}>
                      <ListingTable.Cell sx={{ maxWidth: 240 }}>
                        <Typography
                          noWrap
                          sx={{ fontWeight: 600 }}
                          title={plan.displayName}
                          variant="body2"
                        >
                          {plan.displayName}
                        </Typography>
                      </ListingTable.Cell>
                      {/* maxWidth 0 with a width share keeps the chips to their column. */}
                      <ListingTable.Cell sx={{ maxWidth: 0, width: "45%" }}>
                        {/* One line in every row: three chips at most, the rest in "+N". */}
                        <PlanConditionChips conditions={plan.conditions} singleLine />
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        {/* Agent Manager plans are free: pricing is not supported. */}
                        <Typography color="text.secondary" noWrap variant="body2">
                          Free
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell align="center">
                        <Switch
                          checked={status === "ACTIVE"}
                          disabled={!canUpdate || stale || pendingStatus !== undefined}
                          onChange={() => toggleStatus(plan)}
                          size="small"
                          slotProps={{
                            input: {
                              "aria-label": `${plan.displayName}, ${
                                status === "ACTIVE" ? "active" : "inactive"
                              }`,
                            },
                          }}
                        />
                      </ListingTable.Cell>
                      <ListingTable.Cell align="center">
                        {canUpdate || canDelete ? (
                          <Stack direction="row" justifyContent="center" spacing={0.5}>
                            {canUpdate && (
                              <Tooltip title={`Edit ${plan.displayName}`}>
                                <IconButton
                                  aria-label={`Edit ${plan.displayName}`}
                                  disabled={stale}
                                  onClick={() =>
                                    navigate(editPath(plan), { state: returnToState(location) })
                                  }
                                  size="small"
                                >
                                  <Pencil size={16} />
                                </IconButton>
                              </Tooltip>
                            )}
                            {canDelete && (
                              <Tooltip title={`Delete ${plan.displayName}`}>
                                <IconButton
                                  aria-label={`Delete ${plan.displayName}`}
                                  disabled={stale}
                                  color="error"
                                  onClick={() => confirmDelete(plan)}
                                  size="small"
                                >
                                  <Trash2 size={16} />
                                </IconButton>
                              </Tooltip>
                            )}
                          </Stack>
                        ) : null}
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  );
                })}
              </ListingTable.Body>
            </ListingTable>
          </ListingTable.Container>
        </ListingTable.Provider>

        {total > PAGE_SIZE_OPTIONS[0] && (
          <TablePagination
            component="div"
            count={total}
            getItemAriaLabel={(type) => (type === "next" ? "Next page" : "Previous page")}
            labelDisplayedRows={({ count, from, to }) =>
              `${from}–${to} of ${count.toLocaleString()}`
            }
            labelRowsPerPage="Plans per page"
            onPageChange={(_, next) => setPage(next)}
            onRowsPerPageChange={(event) => {
              setRowsPerPage(parseInt(event.target.value, 10));
              setPage(0);
            }}
            page={currentPage}
            rowsPerPage={rowsPerPage}
            rowsPerPageOptions={PAGE_SIZE_OPTIONS}
          />
        )}
      </Stack>
    </ResourceListShell>
  );
}
