/*
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

import type { ReactNode } from "react";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SubscriptionPlan } from "@agent-management-platform/types";
import { SubscriptionPlanTable } from "./SubscriptionPlanTable";

type ListQuery = { limit: number; offset: number; query?: string };

const state = vi.hoisted(() => ({
  plans: [] as SubscriptionPlan[],
  total: 0,
  isLoading: false,
  error: null as unknown,
  isPlaceholderData: false,
  canUpdate: true,
  canDeprecate: true,
  queries: [] as Array<{ orgName: string } & ListQuery>,
  mutateAsync: vi.fn(),
  deprecate: vi.fn(),
  addConfirmation: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useListSubscriptionPlans: (params: { orgName: string }, query: ListQuery) => {
    state.queries.push({ ...params, ...query });
    return {
      data: { list: state.plans, pagination: { total: state.total } },
      error: state.error,
      isLoading: state.isLoading,
      isPlaceholderData: state.isPlaceholderData,
    };
  },
  useUpdateSubscriptionPlan: () => ({ mutateAsync: state.mutateAsync }),
  useDeprecateSubscriptionPlan: () => ({ mutate: state.deprecate }),
}));

vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureAccess: (key: string) =>
    key === "subscription_plan_update"
      ? state.canUpdate
      : key === "subscription_plan_delete"
        ? state.canDeprecate
        : false,
}));

vi.mock("@agent-management-platform/views", () => ({
  returnToState: (location: { pathname: string }) => ({ returnTo: location.pathname }),
}));

type EmptyState = { title: string; description: string; action?: ReactNode };
vi.mock("@agent-management-platform/shared-component", () => ({
  useConfirmationDialog: () => ({ addConfirmation: state.addConfirmation }),
  // The shell's states, top-down, as the real one shows them.
  ResourceListShell: ({
    children,
    emptyState,
    error,
    isEmpty,
    isLoading,
    isSearchEmpty,
    searchEmptyState,
    toolbar,
  }: {
    children: ReactNode;
    emptyState: EmptyState;
    error?: unknown;
    isEmpty?: boolean;
    isLoading?: boolean;
    isSearchEmpty?: boolean;
    searchEmptyState?: EmptyState;
    toolbar?: ReactNode;
  }) => {
    const shown = (s: EmptyState) => (
      <div>
        <h2>{s.title}</h2>
        <p>{s.description}</p>
        {s.action}
      </div>
    );
    if (error) return <div role="alert">{(error as Error).message}</div>;
    if (isLoading) return <div>Loading plans</div>;
    if (isEmpty) return shown(emptyState);
    return (
      <div>
        {toolbar}
        {isSearchEmpty && searchEmptyState ? shown(searchEmptyState) : children}
      </div>
    );
  },
}));

const plan = (id: string, displayName: string, extra: Partial<SubscriptionPlan> = {}) =>
  ({
    conditions: [],
    deprecated: false,
    displayName,
    id,
    status: "ACTIVE",
    ...extra,
  }) as SubscriptionPlan;

function EditPage() {
  const location = useLocation();
  return (
    <p>
      Editing {location.pathname} from {(location.state as { returnTo: string }).returnTo}
    </p>
  );
}

const renderTable = (createAction?: ReactNode) =>
  render(
    <MemoryRouter initialEntries={["/org/acme/subscription-plans"]}>
      <Routes>
        <Route
          element={<SubscriptionPlanTable createAction={createAction} />}
          path="/org/:orgId/subscription-plans"
        />
        <Route element={<EditPage />} path="/org/:orgId/subscription-plans/edit/:planId" />
      </Routes>
    </MemoryRouter>,
  );

const lastQuery = () => state.queries[state.queries.length - 1];

beforeEach(() => {
  // The condition chips watch their cell; jsdom has no layout to watch.
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  state.plans = [plan("gold", "Gold"), plan("silver", "Silver", { status: "INACTIVE" })];
  state.total = 2;
  state.isLoading = false;
  state.error = null;
  state.isPlaceholderData = false;
  state.canUpdate = true;
  state.canDeprecate = true;
  state.queries = [];
  state.mutateAsync.mockReset();
  state.deprecate.mockReset();
  state.addConfirmation.mockReset();
});

describe("SubscriptionPlanTable: the list", () => {
  it("asks for the organization's first page and lists its plans with their conditions", () => {
    state.plans = [
      plan("gold", "Gold", {
        conditions: [
          {
            id: "requests",
            params: { limits: [{ count: 100, timeAmount: 1, timeUnit: "MINUTE" }] },
            type: "request-count",
          },
        ],
      }),
    ];
    state.total = 1;
    renderTable();
    expect(lastQuery()).toEqual({ limit: 5, offset: 0, orgName: "acme", query: undefined });
    expect(screen.getByText("Gold")).toBeInTheDocument();
    // The chip, and its hidden copy measured for the one-line cell.
    expect(screen.getAllByText("100 / min")).toHaveLength(2);
    expect(screen.getByLabelText("Gold, active")).toBeChecked();
    // One page only: no pager.
    expect(screen.queryByText("Plans per page")).not.toBeInTheDocument();
  });

  it("shows the create prompt, with the page's action, for an organization with no plans", () => {
    state.plans = [];
    state.total = 0;
    renderTable(<button type="button">Create plan</button>);
    expect(screen.getByText("Create your first subscription plan")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create plan" })).toBeInTheDocument();
  });

  it("shows the loading state and the load error", () => {
    state.isLoading = true;
    const { unmount } = renderTable();
    expect(screen.getByText("Loading plans")).toBeInTheDocument();
    unmount();
    state.isLoading = false;
    state.error = new Error("Server down");
    renderTable();
    expect(screen.getByRole("alert")).toHaveTextContent("Server down");
  });

  it("searches after typing stops, then offers to clear an empty search", async () => {
    renderTable();
    fireEvent.change(screen.getByPlaceholderText("Search plans"), {
      target: { value: "  bronze " },
    });
    // Not asked for at once: the search waits for typing to stop.
    expect(lastQuery().query).toBeUndefined();
    await waitFor(() => expect(lastQuery().query).toBe("bronze"));

    state.plans = [];
    state.total = 0;
    fireEvent.change(screen.getByPlaceholderText("Search plans"), {
      target: { value: "bronzer" },
    });
    // A search with no matches is not the create prompt.
    expect(await screen.findByText("No matching plans")).toBeInTheDocument();
    expect(screen.queryByText("Create your first subscription plan")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    expect(screen.getByPlaceholderText("Search plans")).toHaveValue("");
    await waitFor(() => expect(lastQuery().query).toBeUndefined());
  });
});

describe("SubscriptionPlanTable: paging", () => {
  beforeEach(() => {
    state.plans = Array.from({ length: 5 }, (_, i) => plan(`p${i}`, `Plan ${i}`));
    state.total = 12;
  });

  it("pages on the server", () => {
    renderTable();
    expect(screen.getByText("1–5 of 12")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Next page" }));
    expect(lastQuery()).toMatchObject({ limit: 5, offset: 5 });
    expect(screen.getByText("6–10 of 12")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Previous page" }));
    expect(lastQuery()).toMatchObject({ offset: 0 });
  });

  it("changes the page size and goes back to the first page", async () => {
    renderTable();
    fireEvent.click(screen.getByRole("button", { name: "Next page" }));
    fireEvent.mouseDown(screen.getByRole("combobox"));
    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).getAllByRole("option").map((o) => o.textContent)).toEqual([
      "5",
      "10",
      "15",
      "20",
    ]);
    fireEvent.click(within(listbox).getByRole("option", { name: "10" }));
    expect(lastQuery()).toMatchObject({ limit: 10, offset: 0 });
  });

  it("starts a new search from the first page", async () => {
    renderTable();
    fireEvent.click(screen.getByRole("button", { name: "Next page" }));
    expect(lastQuery()).toMatchObject({ offset: 5 });
    fireEvent.change(screen.getByPlaceholderText("Search plans"), { target: { value: "plan" } });
    await waitFor(() => expect(lastQuery()).toMatchObject({ offset: 0, query: "plan" }));
  });

  it("goes back to the last page when the list shrinks under the current one", () => {
    const { rerender } = renderTable();
    fireEvent.click(screen.getByRole("button", { name: "Next page" }));
    fireEvent.click(screen.getByRole("button", { name: "Next page" }));
    expect(lastQuery()).toMatchObject({ offset: 10 });
    state.total = 7;
    rerender(
      <MemoryRouter initialEntries={["/org/acme/subscription-plans"]}>
        <Routes>
          <Route element={<SubscriptionPlanTable />} path="/org/:orgId/subscription-plans" />
        </Routes>
      </MemoryRouter>,
    );
    expect(lastQuery()).toMatchObject({ offset: 5 });
  });
});

describe("SubscriptionPlanTable: the status switch", () => {
  it("deactivates a plan with a partial update, waiting while it saves", async () => {
    let finish: () => void = () => undefined;
    state.mutateAsync.mockReturnValue(
      new Promise<void>((resolve) => {
        finish = resolve;
      }),
    );
    renderTable();
    fireEvent.click(screen.getByLabelText("Gold, active"));
    expect(state.mutateAsync).toHaveBeenCalledWith({
      body: { displayName: "Gold", status: "INACTIVE" },
      params: { orgName: "acme", planId: "gold" },
      successText: 'Deactivated plan "Gold".',
    });
    // The switch shows the status it is being set to, and waits.
    const pending = screen.getByLabelText("Gold, inactive");
    expect(pending).not.toBeChecked();
    expect(pending).toBeDisabled();
    // The other plans' switches stay usable.
    expect(screen.getByLabelText("Silver, inactive")).toBeEnabled();

    await act(async () => finish());
    expect(screen.getByLabelText("Gold, active")).toBeEnabled();
  });

  it("activates a plan, and puts the switch back when saving fails", async () => {
    state.mutateAsync.mockRejectedValue(new Error("nope"));
    renderTable();
    fireEvent.click(screen.getByLabelText("Silver, inactive"));
    expect(state.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        body: { displayName: "Silver", status: "ACTIVE" },
        successText: 'Activated plan "Silver".',
      }),
    );
    await waitFor(() => expect(screen.getByLabelText("Silver, inactive")).toBeEnabled());
    expect(screen.getByLabelText("Silver, inactive")).not.toBeChecked();
  });

  it("turns the switches and actions off while the rows are stale", () => {
    state.isPlaceholderData = true;
    renderTable();
    expect(screen.getByLabelText("Gold, active")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Edit Gold" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Deprecate Gold" })).toBeDisabled();
  });
});

describe("SubscriptionPlanTable: row actions", () => {
  it("opens a plan's edit page, remembering where it came from", () => {
    renderTable();
    fireEvent.click(screen.getByRole("button", { name: "Edit Silver" }));
    expect(
      screen.getByText(
        "Editing /org/acme/subscription-plans/edit/silver from /org/acme/subscription-plans",
      ),
    ).toBeInTheDocument();
  });

  it("deprecates a plan once its name is typed to confirm", () => {
    renderTable();
    fireEvent.click(screen.getByRole("button", { name: "Deprecate Gold" }));
    expect(state.addConfirmation).toHaveBeenCalledTimes(1);
    const confirmation = state.addConfirmation.mock.calls[0][0];
    expect(confirmation).toMatchObject({
      confirmButtonColor: "error",
      confirmButtonText: "Deprecate",
      confirmationText: "Gold",
      title: "Deprecate plan",
    });
    expect(confirmation.description).toContain('"Gold" stays for the subscriptions on it');
    expect(state.deprecate).not.toHaveBeenCalled();
    confirmation.onConfirm();
    expect(state.deprecate).toHaveBeenCalledWith({
      displayName: "Gold",
      orgName: "acme",
      planId: "gold",
    });
  });

  it("lists a deprecated plan read only", () => {
    state.plans = [plan("old", "Old", { deprecated: true })];
    state.total = 1;
    renderTable();
    expect(screen.getByText("Deprecated")).toBeInTheDocument();
    expect(screen.queryByLabelText(/^Old, /)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit Old" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Deprecate Old" })).not.toBeInTheDocument();
  });

  it("offers only the actions the user may take", () => {
    state.canDeprecate = false;
    const { unmount } = renderTable();
    expect(screen.getByRole("button", { name: "Edit Gold" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Deprecate Gold" })).not.toBeInTheDocument();
    unmount();

    state.canDeprecate = true;
    state.canUpdate = false;
    renderTable();
    expect(screen.queryByRole("button", { name: "Edit Gold" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Deprecate Gold" })).toBeInTheDocument();
    // Without update access the status can't be switched either.
    expect(screen.getByLabelText("Gold, active")).toBeDisabled();
  });

  it("shows no actions to a user who may neither edit nor deprecate", () => {
    state.canUpdate = false;
    state.canDeprecate = false;
    renderTable();
    expect(screen.queryByRole("button", { name: /^(Edit|Deprecate) / })).not.toBeInTheDocument();
  });
});
