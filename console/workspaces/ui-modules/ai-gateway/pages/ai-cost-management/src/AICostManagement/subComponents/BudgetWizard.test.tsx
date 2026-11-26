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
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Budget } from "@agent-management-platform/types";

const hoisted = vi.hoisted(() => ({
  existing: {
    data: undefined as unknown,
    isLoading: false,
    error: undefined as unknown,
    refetch: vi.fn(),
  },
  create: vi.fn(),
  update: vi.fn(),
  destinations: [{ name: "finance-alerts" }, { name: "cost-owners-email" }],
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  useGetBudget: () => hoisted.existing,
  useListBudgets: () => ({ data: { budgets: [] } }),
  useListLLMProviders: () => ({
    data: {
      providers: [
        { id: "openai-production", uuid: "u1", name: "OpenAI Production" },
      ],
    },
  }),
  useListProjects: () => ({ data: { projects: [] } }),
  useListOrgAgents: () => ({ data: { agents: [] } }),
  useListEventDestinations: () => ({
    data: { destinations: hoisted.destinations },
    isLoading: false,
    error: undefined,
    refetch: vi.fn(),
  }),
  useListLLMProvidersAPIKeys: () => ({ keys: [], isLoading: false }),
  useCreateBudget: () => ({ mutate: hoisted.create, isPending: false }),
  useUpdateBudget: () => ({ mutate: hoisted.update, isPending: false }),
}));
vi.mock(
  "@agent-management-platform/shared-component",
  async (importOriginal) => ({
    ...(await importOriginal<Record<string, unknown>>()),
    // The unsaved-changes guard needs a data router; the form is what is under test.
    useUnsavedChangesGuard: () => ({
      allowNavigation: (go: () => void) => go(),
    }),
  }),
);

import { BudgetWizard } from "./BudgetForm";

// The wizard is driven one field at a time, which is slow on a busy runner.
vi.setConfig({ testTimeout: 60_000 });

const renderWizard = (
  props: Partial<React.ComponentProps<typeof BudgetWizard>> = {},
) => {
  const onSaved = vi.fn();
  render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter>
        <BudgetWizard
          orgName="acme"
          backHref="/budgets"
          backLabel="Back to budgets"
          onSaved={onSaved}
          {...props}
        />
      </MemoryRouter>
    </ThemeProvider>,
  );
  return { onSaved };
};

const type = (label: string | RegExp, value: string) =>
  fireEvent.change(screen.getByLabelText(label), { target: { value } });

/** Opens a select by its label: the label sits on the field, the menu opens from its button. */
const openSelect = (label: string) => {
  const el = screen.getByLabelText(label);
  const field = (el.closest(".MuiFormControl-root") ??
    el.parentElement) as HTMLElement;
  const box =
    el.getAttribute("role") === "combobox"
      ? el
      : (field.querySelector('[role="combobox"]') as HTMLElement);
  fireEvent.mouseDown(box);
};

const next = () =>
  fireEvent.click(screen.getByRole("button", { name: /^Next/ }));

beforeEach(() => {
  // jsdom has no layout: the form scrolls a field in error into view.
  window.scrollTo = vi.fn();
  Element.prototype.scrollIntoView = vi.fn();
  hoisted.create.mockClear();
  hoisted.update.mockClear();
  hoisted.existing = {
    data: undefined,
    isLoading: false,
    error: undefined,
    refetch: vi.fn(),
  };
});

describe("BudgetWizard (create)", () => {
  it("starts on Details, as step 1 of 2", () => {
    renderWizard();
    expect(
      screen.getByRole("heading", { name: "Create budget" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Step 1 of 2 · Details")).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toBeInTheDocument();
    expect(screen.getByLabelText("Description (optional)")).toBeInTheDocument();
  });

  it("does not go on without a name", () => {
    renderWizard();
    next();
    expect(screen.getByText("Step 1 of 2 · Details")).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
  });

  it("goes to the limit page once there is a name, and back again", () => {
    renderWizard();
    type("Name", "UI test budget");
    next();
    expect(
      screen.getByText("Step 2 of 2 · Applies to and limit"),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Amount")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Previous/ }));
    expect(screen.getByText("Step 1 of 2 · Details")).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toHaveValue("UI test budget");
  });

  it("asks for an amount above zero before it creates", () => {
    renderWizard();
    type("Name", "UI test budget");
    next();
    fireEvent.click(screen.getByRole("button", { name: "Create budget" }));
    expect(hoisted.create).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Amount")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
  });

  it("creates a blocking monthly budget, naming it after what was typed", () => {
    const { onSaved } = renderWizard();
    type("Name", "UI test budget");
    next();
    type("Amount", "50");
    fireEvent.click(screen.getByRole("button", { name: "Create budget" }));

    expect(hoisted.create).toHaveBeenCalledTimes(1);
    const { params, body } = hoisted.create.mock.calls[0][0];
    expect(params).toEqual({ orgName: "acme" });
    expect(body.kind).toBe("Budget");
    expect(body.metadata.name).toBe("ui-test-budget");
    expect(body.spec.displayName).toBe("UI test budget");
    expect(body.spec.limit).toMatchObject({
      amount: "50.00",
      period: "month",
      action: "block",
      blockAt: 100,
    });
    expect(body.spec.filters).toEqual([]);

    // Once the server answers, the form hands the saved name on.
    hoisted.create.mock.calls[0][1].onSuccess({
      metadata: { name: "ui-test-budget" },
    });
    expect(onSaved).toHaveBeenCalledWith("ui-test-budget");
  });

  it("an Allow budget has no block share", () => {
    renderWizard();
    type("Name", "Watcher");
    next();
    expect(screen.getByLabelText("Block at")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("radio", { name: /Allow requests/ }));
    expect(screen.queryByLabelText("Block at")).not.toBeInTheDocument();
    type("Amount", "20");
    fireEvent.click(screen.getByRole("button", { name: "Create budget" }));
    const { body } = hoisted.create.mock.calls[0][0];
    expect(body.spec.limit.action).toBe("allow");
    expect(body.spec.limit.blockAt).toBeUndefined();
  });

  it("takes the weekly or the daily period", () => {
    renderWizard();
    type("Name", "Daily cap");
    next();
    type("Amount", "3");
    fireEvent.click(screen.getByRole("button", { name: "Daily" }));
    fireEvent.click(screen.getByRole("button", { name: "Create budget" }));
    expect(hoisted.create.mock.calls[0][0].body.spec.limit.period).toBe("day");
  });

  it("keeps several CEL expressions as alternatives in one filter", () => {
    renderWizard();
    type("Name", "Two paths");
    next();
    type("Amount", "50");
    fireEvent.click(screen.getByText("Add filter"));
    openSelect("Filter 1 type");
    fireEvent.click(screen.getByRole("option", { name: "Custom attribute" }));
    // The expression field is the one text input among the comboboxes (the type is a select).
    const box = () =>
      screen
        .getAllByRole("combobox")
        .find((el) => el.tagName === "INPUT") as HTMLElement;
    for (const expr of [
      "request.Path.startsWith('/lt-mini/')",
      "request.Path.startsWith('/lt-claude/')",
    ]) {
      fireEvent.change(box(), { target: { value: expr } });
      fireEvent.keyDown(box(), { key: "Enter" });
    }
    fireEvent.click(screen.getByRole("button", { name: "Create budget" }));

    const { body } = hoisted.create.mock.calls[0][0];
    expect(body.spec.filters).toHaveLength(1);
    expect(body.spec.filters[0]).toEqual({
      type: "attribute",
      expression:
        "(request.Path.startsWith('/lt-mini/')) || (request.Path.startsWith('/lt-claude/'))",
    });
  });

  it("wants a notification channel when there are warnings", async () => {
    renderWizard();
    type("Name", "With alerts");
    next();
    type("Amount", "50");
    fireEvent.click(screen.getByText("Alerts and exemptions"));
    const warn = screen.getByLabelText("Warn at");
    fireEvent.change(warn, { target: { value: "80" } });
    fireEvent.keyDown(warn, { key: "Enter" });
    fireEvent.click(screen.getByRole("button", { name: "Create budget" }));
    expect(hoisted.create).not.toHaveBeenCalled();
    expect(
      await screen.findByText(/Choose where warnings go/),
    ).toBeInTheDocument();
  });

  it("takes a warning and the channel it goes to", async () => {
    renderWizard();
    type("Name", "With alerts");
    next();
    type("Amount", "50");
    fireEvent.click(screen.getByText("Alerts and exemptions"));
    const warn = screen.getByLabelText("Warn at");
    fireEvent.change(warn, { target: { value: "80" } });
    fireEvent.keyDown(warn, { key: "Enter" });
    fireEvent.mouseDown(screen.getByLabelText("Notification channels"));
    fireEvent.click(
      await screen.findByRole("option", { name: "finance-alerts" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Create budget" }));
    await waitFor(() => expect(hoisted.create).toHaveBeenCalledTimes(1));
    expect(hoisted.create.mock.calls[0][0].body.spec.warnings).toEqual({
      thresholds: [80],
      channels: ["finance-alerts"],
    });
  });

  it("offers every type for an exemption, an LLM provider among them", async () => {
    renderWizard();
    type("Name", "With exemption");
    next();
    fireEvent.click(screen.getByText("Alerts and exemptions"));
    fireEvent.click(screen.getByText("Add exemption"));
    for (const t of [
      "LLM provider",
      "Project",
      "Agent",
      "API key",
      "Custom attribute",
    ]) {
      expect(
        await screen.findByRole("menuitem", { name: t }),
      ).toBeInTheDocument();
    }
    fireEvent.click(screen.getByRole("menuitem", { name: "LLM provider" }));
    expect(
      await screen.findByLabelText("Exemption 1 type"),
    ).toBeInTheDocument();
  });
});

describe("BudgetWizard (edit)", () => {
  const stored: Budget = {
    apiVersion: "gateway.api-platform.wso2.com/v1",
    kind: "Budget",
    metadata: { name: "org-monthly", labels: ["finance"] },
    spec: {
      displayName: "Organization monthly ceiling",
      description: "Keeps spend within the allocation",
      status: "active",
      limit: {
        amount: "1500.00",
        period: "month",
        action: "block",
        blockAt: 95,
      },
      filters: [],
      exemptions: [],
      warnings: { thresholds: [75, 90], channels: ["finance-alerts"] },
    },
    status: {
      origin: "controlPlane",
      createdAt: "2026-11-01T00:00:00Z",
      updatedAt: "2026-11-01T00:00:00Z",
    },
  };

  it("opens with the stored values", async () => {
    hoisted.existing.data = stored;
    renderWizard({ budgetName: "org-monthly" });
    expect(
      screen.getByRole("heading", {
        name: /Edit Organization monthly ceiling/,
      }),
    ).toBeInTheDocument();
    expect(await screen.findByLabelText("Name")).toHaveValue(
      "Organization monthly ceiling",
    );
    expect(screen.getByLabelText("Description (optional)")).toHaveValue(
      "Keeps spend within the allocation",
    );
  });

  it("saves through update, with the budget's own name, not a new one", async () => {
    hoisted.existing.data = stored;
    const { onSaved } = renderWizard({ budgetName: "org-monthly" });
    type("Description (optional)", "Changed in a test");
    fireEvent.click(
      await screen.findByRole("button", { name: /Save changes/ }),
    );
    await waitFor(() => expect(hoisted.update).toHaveBeenCalledTimes(1));
    const { params, body } = hoisted.update.mock.calls[0][0];
    expect(params).toEqual({ orgName: "acme", budgetName: "org-monthly" });
    expect(body.metadata.name).toBe("org-monthly");
    expect(body.spec.description).toBe("Changed in a test");
    expect(body.spec.limit.blockAt).toBe(95);
    expect(hoisted.create).not.toHaveBeenCalled();
    hoisted.update.mock.calls[0][1].onSuccess({});
    expect(onSaved).toHaveBeenCalledWith("org-monthly");
  });

  it("says when there is no budget of that name", () => {
    hoisted.existing = {
      data: undefined,
      isLoading: false,
      error: { status: 404 },
      refetch: vi.fn(),
    };
    renderWizard({ budgetName: "gone" });
    expect(
      screen.getByText("There is no budget named gone"),
    ).toBeInTheDocument();
  });

  it("offers Retry when the budget could not be loaded", () => {
    hoisted.existing = {
      data: undefined,
      isLoading: false,
      error: new Error("boom"),
      refetch: vi.fn(),
    };
    renderWizard({ budgetName: "org-monthly" });
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(hoisted.existing.refetch).toHaveBeenCalledTimes(1);
  });
});
