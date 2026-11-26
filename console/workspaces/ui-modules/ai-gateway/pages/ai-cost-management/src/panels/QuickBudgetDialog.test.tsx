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

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { beforeEach, describe, expect, it, vi } from "vitest";

const hoisted = vi.hoisted(() => ({
  create: vi.fn(),
  navigate: vi.fn(),
  refetch: vi.fn(),
  pending: false,
  budgets: [] as unknown[],
  destinations: {
    data: { destinations: [{ name: "finance-alerts" }] } as unknown,
    isLoading: false,
    error: undefined as unknown,
  },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  isNetworkError: (e: unknown) =>
    (e as { network?: boolean })?.network === true,
  useListBudgets: () => ({ data: { budgets: hoisted.budgets } }),
  useListEventDestinations: () => ({
    ...hoisted.destinations,
    refetch: hoisted.refetch,
  }),
  useCreateBudget: () => ({
    mutate: hoisted.create,
    isPending: hoisted.pending,
  }),
}));
vi.mock("react-router-dom", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  useNavigate: () => hoisted.navigate,
}));

import { QuickBudgetDialog } from "./QuickBudgetDialog";

vi.setConfig({ testTimeout: 60_000 });

const Where = () => {
  const l = useLocation();
  return <span data-testid="where">{l.pathname}</span>;
};

const renderDialog = (
  props: Partial<React.ComponentProps<typeof QuickBudgetDialog>> = {},
) => {
  const onClose = vi.fn();
  render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter initialEntries={["/org/acme/providers/azure-openai"]}>
        <QuickBudgetDialog
          open
          onClose={onClose}
          orgName="acme"
          start={{ provider: "azure-openai-eu" }}
          kind="LLM provider"
          label="Azure OpenAI EU"
          createHref="/org/acme/budgets/create"
          viewHref={(n) => `/org/acme/budgets/${n}`}
          {...props}
        />
        <Where />
      </MemoryRouter>
    </ThemeProvider>,
  );
  return { onClose };
};

const type = (label: string | RegExp, value: string) =>
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
const create = () =>
  fireEvent.click(screen.getByRole("button", { name: "Create budget" }));

beforeEach(() => {
  hoisted.create.mockClear();
  hoisted.navigate.mockClear();
  hoisted.refetch.mockClear();
  hoisted.pending = false;
  hoisted.budgets = [];
  hoisted.destinations = {
    data: { destinations: [{ name: "finance-alerts" }] },
    isLoading: false,
    error: undefined,
  };
});

describe("QuickBudgetDialog", () => {
  it("shows the entity the budget is for and the defaults", () => {
    renderDialog();
    expect(
      screen.getByText("Add budget for Azure OpenAI EU"),
    ).toBeInTheDocument();
    expect(screen.getByText("LLM provider")).toBeInTheDocument();
    expect(
      screen.getByText("Refuse requests at the limit."),
    ).toBeInTheDocument();
    expect(screen.getByText("USD")).toBeInTheDocument();
    expect(screen.queryByText("Notification channels")).not.toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /Advanced create/ }),
    ).toHaveAttribute(
      "href",
      "/org/acme/budgets/create?provider=azure-openai-eu",
    );
  });

  it("explains Allow as warn-only", () => {
    renderDialog();
    fireEvent.click(screen.getByRole("button", { name: "Allow" }));
    expect(screen.getByText("Only warns.")).toBeInTheDocument();
  });

  it("asks for a name and an amount before creating", () => {
    renderDialog();
    create();
    expect(screen.getByText("Enter a name.")).toBeInTheDocument();
    expect(screen.getByText("Enter an amount.")).toBeInTheDocument();
    expect(screen.getByText("Fix the highlighted fields.")).toBeInTheDocument();
    expect(hoisted.create).not.toHaveBeenCalled();
  });

  it("rejects an amount with too many decimals", () => {
    renderDialog();
    type(/^Name/, "Finance monthly");
    type(/^Amount/, "12.345");
    create();
    expect(
      screen.getByText("Enter an amount above 0, up to 2 decimals."),
    ).toBeInTheDocument();
  });

  it("rejects a name another budget already uses", () => {
    hoisted.budgets = [
      {
        metadata: { name: "finance-monthly" },
        spec: { displayName: "Finance monthly" },
      },
    ];
    renderDialog();
    type(/^Name/, "finance Monthly");
    type(/^Amount/, "500");
    create();
    expect(
      screen.getByText("A budget named finance Monthly already exists."),
    ).toBeInTheDocument();
    expect(hoisted.create).not.toHaveBeenCalled();
  });

  it("creates the budget with the entity filter, warnings and channel", () => {
    hoisted.budgets = [
      { metadata: { name: "finance-monthly" }, spec: { displayName: "Other" } },
    ];
    renderDialog();
    type(/^Name/, "Finance monthly");
    type(/^Amount/, "1500");
    const warn = screen.getByLabelText("Warn at");
    fireEvent.change(warn, { target: { value: "80" } });
    fireEvent.keyDown(warn, { key: "Enter" });
    fireEvent.change(screen.getByLabelText("Warn at"), {
      target: { value: "100" },
    });
    fireEvent.keyDown(screen.getByLabelText("Warn at"), { key: "Enter" });
    const channels = screen.getByLabelText("Notification channels");
    fireEvent.mouseDown(channels);
    fireEvent.click(screen.getByRole("option", { name: "finance-alerts" }));
    create();
    expect(hoisted.create).toHaveBeenCalledTimes(1);
    const [vars] = hoisted.create.mock.calls[0];
    expect(vars.params).toEqual({ orgName: "acme" });
    expect(vars.body.metadata.name).toBe("finance-monthly-2");
    expect(vars.body.spec).toMatchObject({
      displayName: "Finance monthly",
      limit: {
        amount: "1500.00",
        period: "month",
        action: "block",
        blockAt: 100,
      },
      filters: [{ type: "llmProvider", id: "azure-openai-eu" }],
      warnings: { thresholds: [80, 100], channels: ["finance-alerts"] },
    });
  });

  it("sends a week allow budget without blockAt", () => {
    renderDialog({ start: {} });
    type(/^Name/, "Marketing weekly");
    type(/^Amount/, "250.5");
    // The select shows the current period, "Monthly", as its value.
    fireEvent.mouseDown(screen.getByText("Monthly"));
    fireEvent.click(screen.getByRole("option", { name: "Weekly" }));
    fireEvent.click(screen.getByRole("button", { name: "Allow" }));
    create();
    const [vars] = hoisted.create.mock.calls[0];
    expect(vars.body.spec.limit).toEqual({
      amount: "250.50",
      period: "week",
      action: "allow",
    });
    expect(vars.body.spec.filters).toEqual([]);
    expect(vars.body.spec.warnings).toBeUndefined();
  });

  it("closes and opens the new budget after it is created", () => {
    const { onClose } = renderDialog();
    type(/^Name/, "Finance monthly");
    type(/^Amount/, "1500");
    create();
    const [, opts] = hoisted.create.mock.calls[0];
    opts.onSuccess({ metadata: { name: "finance-monthly" } });
    expect(onClose).toHaveBeenCalled();
    expect(hoisted.navigate).toHaveBeenCalledWith(
      "/org/acme/budgets/finance-monthly",
    );
  });

  it("shows a server error on the field it names", async () => {
    renderDialog();
    type(/^Name/, "Finance monthly");
    type(/^Amount/, "1500");
    create();
    const [, opts] = hoisted.create.mock.calls[0];
    opts.onError({
      body: {
        message: "Amount exceeds the organization limit.",
        additionalData: { field: "spec.limit.amount" },
      },
    });
    expect(
      await screen.findByText("Amount exceeds the organization limit."),
    ).toBeInTheDocument();
  });

  it("shows an error for a field the dialog lacks above the buttons", async () => {
    renderDialog();
    type(/^Name/, "Finance monthly");
    type(/^Amount/, "1500");
    create();
    const [, opts] = hoisted.create.mock.calls[0];
    opts.onError({ network: true });
    expect(
      await screen.findByText("Couldn't reach the server. Try again."),
    ).toBeInTheDocument();
    type(/^Name/, "Finance monthly 2");
    await waitFor(() =>
      expect(
        screen.queryByText("Couldn't reach the server. Try again."),
      ).not.toBeInTheDocument(),
    );
  });

  it("closes with Cancel and the close button", () => {
    const { onClose } = renderDialog();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("keeps typed input when Escape is pressed", () => {
    const { onClose } = renderDialog();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
    type(/^Name/, "Finance monthly");
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("disables the buttons while the budget is being created", () => {
    hoisted.pending = true;
    renderDialog();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Create budget" }),
    ).toBeDisabled();
  });

  it("builds the project and agent filters from the start", () => {
    renderDialog({
      start: { project: "online-shop", agent: "online-shop/support-bot" },
      kind: "Agent",
      label: "support-bot",
    });
    expect(
      screen.getByRole("link", { name: /Advanced create/ }),
    ).toHaveAttribute(
      "href",
      "/org/acme/budgets/create?project=online-shop&agent=online-shop%2Fsupport-bot",
    );
    type(/^Name/, "Support bot");
    type(/^Amount/, "75");
    create();
    expect(hoisted.create.mock.calls[0][0].body.spec.filters).toHaveLength(2);
  });

  it("offers a retry when notification channels fail to load", () => {
    hoisted.destinations = {
      data: undefined,
      isLoading: false,
      error: new Error("boom"),
    };
    renderDialog();
    const warn = screen.getByLabelText("Warn at");
    fireEvent.change(warn, { target: { value: "90" } });
    fireEvent.keyDown(warn, { key: "Enter" });
    fireEvent.mouseDown(screen.getByLabelText("Notification channels"));
    fireEvent.mouseDown(screen.getByRole("button", { name: "Retry" }));
    expect(hoisted.refetch).toHaveBeenCalled();
  });
});
