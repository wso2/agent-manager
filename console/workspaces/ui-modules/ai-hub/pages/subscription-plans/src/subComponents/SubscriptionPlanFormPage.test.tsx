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

import type { ChangeEvent, ReactNode } from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SubscriptionPlan } from "@agent-management-platform/types";
import { slugify, SubscriptionPlanFormPage } from "./SubscriptionPlanFormPage";
import { toExpiryTime } from "./expiryTime";

type MutateOptions = { onError?: (error: unknown) => void; onSuccess?: () => void };

const state = vi.hoisted(() => ({
  plan: undefined as unknown,
  isLoading: false,
  error: null as unknown,
  createPending: false,
  updatePending: false,
  createMutate: (() => undefined) as (...args: unknown[]) => void,
  updateMutate: (() => undefined) as (...args: unknown[]) => void,
  guardArgs: [] as boolean[],
}));

vi.mock("@agent-management-platform/api-client", () => ({
  SUBSCRIPTION_PLAN_EXISTS: "SUBSCRIPTION_PLAN_EXISTS",
  subscriptionPlanErrorCode: (error: unknown) => (error as { code?: string } | null)?.code,
  useGetSubscriptionPlan: () => ({
    data: state.plan,
    error: state.error,
    isLoading: state.isLoading,
  }),
  useCreateSubscriptionPlan: () => ({
    isPending: state.createPending,
    mutate: (...args: unknown[]) => state.createMutate(...args),
  }),
  useUpdateSubscriptionPlan: () => ({
    isPending: state.updatePending,
    mutate: (...args: unknown[]) => state.updateMutate(...args),
  }),
}));

vi.mock("@agent-management-platform/shared-component", () => ({
  useUnsavedChangesGuard: (dirty: boolean) => {
    state.guardArgs.push(dirty);
    return { allowNavigation: (go: () => void) => go() };
  },
}));

type TextInputProps = {
  label: string;
  value: string;
  onChange: (event: ChangeEvent<HTMLInputElement>) => void;
  onBlur?: () => void;
  disabled?: boolean;
  helperText?: ReactNode;
  multiline?: boolean;
};

vi.mock("@agent-management-platform/views", () => ({
  useReturnTo: (fallback: string) => fallback,
  PageLayout: ({
    title,
    description,
    isLoading,
    children,
  }: {
    title: string;
    description?: string;
    isLoading?: boolean;
    children: ReactNode;
  }) => (
    <div>
      <h1>{title}</h1>
      {description && <p>{description}</p>}
      {isLoading ? <p>Loading…</p> : children}
    </div>
  ),
  StateNotice: ({
    title,
    description,
    actions,
  }: {
    title: string;
    description: string;
    actions?: ReactNode;
  }) => (
    <div>
      <p>{title}</p>
      <p>{description}</p>
      {actions}
    </div>
  ),
  TextInput: ({ label, value, onChange, onBlur, disabled, helperText }: TextInputProps) => (
    <div>
      <label>
        {label}
        <input disabled={disabled} onBlur={onBlur} onChange={onChange} value={value} />
      </label>
      {helperText && <span>{helperText}</span>}
    </div>
  ),
}));

const LIST = "/org/acme/subscription-plans";

const makePlan = (extra: Partial<SubscriptionPlan> = {}): SubscriptionPlan => ({
  id: "gold",
  displayName: "Gold",
  description: "The gold plan",
  status: "INACTIVE",
  conditions: [
    { type: "request-count", params: { limits: [{ count: 10, timeAmount: 1, timeUnit: "MINUTE" }] } },
  ],
  deprecated: false,
  ...extra,
});

const renderAt = (path: string) =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route element={<p>The plan list</p>} path="/org/:orgId/subscription-plans" />
        <Route
          element={<SubscriptionPlanFormPage />}
          path="/org/:orgId/subscription-plans/add"
        />
        <Route
          element={<SubscriptionPlanFormPage />}
          path="/org/:orgId/subscription-plans/edit/:planId"
        />
      </Routes>
    </MemoryRouter>,
  );

const renderAdd = () => renderAt(`${LIST}/add`);
const renderEdit = () => renderAt(`${LIST}/edit/gold`);

const nameInput = () => screen.getByLabelText("Plan name") as HTMLInputElement;
const descriptionInput = () => screen.getByLabelText("Description (optional)") as HTMLInputElement;
const lastGuard = () => state.guardArgs[state.guardArgs.length - 1];
const type = (input: HTMLElement, value: string) => fireEvent.change(input, { target: { value } });

let createMutate: ReturnType<typeof vi.fn>;
let updateMutate: ReturnType<typeof vi.fn>;

beforeEach(() => {
  state.plan = undefined;
  state.isLoading = false;
  state.error = null;
  state.createPending = false;
  state.updatePending = false;
  state.guardArgs = [];
  createMutate = vi.fn();
  updateMutate = vi.fn();
  state.createMutate = createMutate;
  state.updateMutate = updateMutate;
});

describe("slugify", () => {
  it("makes a lower-case dashed handle of the name", () => {
    expect(slugify("  Gold Plan (2026)! ")).toBe("gold-plan-2026");
  });

  it("keeps at most 40 characters, with no trailing dash", () => {
    const slug = slugify(`${"a".repeat(39)} b`);
    expect(slug).toBe("a".repeat(39));
    expect(slugify("!!!")).toBe("");
  });
});

describe("SubscriptionPlanFormPage: add", () => {
  it("shows the add page with an empty form", () => {
    renderAdd();
    expect(screen.getByRole("heading", { name: "Add subscription plan" })).toBeInTheDocument();
    expect(
      screen.getByText("Define the conditions and availability of this plan."),
    ).toBeInTheDocument();
    expect(nameInput().value).toBe("");
    expect(screen.getByRole("button", { name: "Add plan" })).toBeEnabled();
    expect(
      screen.getByText(
        "The plan stops accepting subscriptions at the end of this day, in your time zone.",
      ),
    ).toBeInTheDocument();
  });

  it("reports a missing name on submit and doesn't create the plan", () => {
    renderAdd();
    fireEvent.click(screen.getByRole("button", { name: "Add plan" }));
    expect(screen.getByText("Enter a display name")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add plan" })).toBeDisabled();
    expect(createMutate).not.toHaveBeenCalled();
  });

  it("reports a name without letters or numbers once the field is left", () => {
    renderAdd();
    type(nameInput(), "!!!");
    expect(screen.queryByText("Use letters or numbers in the name")).not.toBeInTheDocument();
    fireEvent.blur(nameInput());
    expect(screen.getByText("Use letters or numbers in the name")).toBeInTheDocument();
  });

  it("reports a name that is too long", () => {
    renderAdd();
    type(nameInput(), "x".repeat(256));
    fireEvent.blur(nameInput());
    expect(screen.getByText("Use at most 255 characters")).toBeInTheDocument();
  });

  it("reports a description that is too long", () => {
    renderAdd();
    type(descriptionInput(), "d".repeat(1024));
    expect(screen.getByText("Use at most 1,023 characters (1,024 now).")).toBeInTheDocument();
  });

  it("creates an active plan with the slug as its id and goes back to the list", () => {
    renderAdd();
    type(nameInput(), "  Gold Plan ");
    type(descriptionInput(), " For the best agents ");
    fireEvent.click(screen.getByRole("button", { name: "Add plan" }));

    expect(createMutate).toHaveBeenCalledTimes(1);
    const [variables, options] = createMutate.mock.calls[0] as [unknown, MutateOptions];
    expect(variables).toEqual({
      body: {
        conditions: [],
        description: "For the best agents",
        displayName: "Gold Plan",
        expiryTime: undefined,
        id: "gold-plan",
        status: "ACTIVE",
      },
      params: { orgName: "acme" },
    });

    act(() => options.onSuccess?.());
    expect(screen.getByText("The plan list")).toBeInTheDocument();
  });

  it("leaves out an empty description", () => {
    renderAdd();
    type(nameInput(), "Silver");
    fireEvent.click(screen.getByRole("button", { name: "Add plan" }));
    const [variables] = createMutate.mock.calls[0] as [{ body: Record<string, unknown> }];
    expect(variables.body).not.toHaveProperty("description");
  });

  it("sends a condition picked in the editor", () => {
    renderAdd();
    type(nameInput(), "Gold");
    fireEvent.click(screen.getByRole("button", { name: /^Request count/ }));
    // An empty count is a condition error: nothing is sent.
    fireEvent.click(screen.getByRole("button", { name: "Add plan" }));
    expect(createMutate).not.toHaveBeenCalled();

    type(screen.getByLabelText("Count"), "100");
    fireEvent.click(screen.getByRole("button", { name: "Add plan" }));
    const [variables] = createMutate.mock.calls[0] as [{ body: { conditions: unknown[] } }];
    expect(variables.body.conditions).toEqual([
      expect.objectContaining({
        type: "request-count",
        params: expect.objectContaining({
          limits: [{ count: 100, timeAmount: 1, timeUnit: "MINUTE" }],
        }),
      }),
    ]);
  });

  it("shows a taken name as a conflict on the name, cleared once the name changes", () => {
    renderAdd();
    type(nameInput(), "Gold");
    fireEvent.click(screen.getByRole("button", { name: "Add plan" }));
    const [, options] = createMutate.mock.calls[0] as [unknown, MutateOptions];

    act(() => options.onError?.({ code: "SUBSCRIPTION_PLAN_EXISTS", message: "exists" }));
    expect(screen.getByText("Plan not created")).toBeInTheDocument();
    expect(
      screen.getByText(/A plan named "Gold" already exists in this organization/),
    ).toBeInTheDocument();
    expect(screen.getByText("A plan with this name already exists")).toBeInTheDocument();
    expect(screen.queryByText("exists")).not.toBeInTheDocument();

    type(nameInput(), "Gold 2");
    expect(screen.queryByText("Plan not created")).not.toBeInTheDocument();
    expect(screen.queryByText("A plan with this name already exists")).not.toBeInTheDocument();
  });

  it("shows the server's reason for any other failure", () => {
    renderAdd();
    type(nameInput(), "Gold");
    fireEvent.click(screen.getByRole("button", { name: "Add plan" }));
    const [, options] = createMutate.mock.calls[0] as [unknown, MutateOptions];
    act(() => options.onError?.(new Error("Quota exceeded")));
    expect(screen.getByText("Quota exceeded")).toBeInTheDocument();
  });

  it("falls back to a generic failure message", () => {
    renderAdd();
    type(nameInput(), "Gold");
    fireEvent.click(screen.getByRole("button", { name: "Add plan" }));
    const [, options] = createMutate.mock.calls[0] as [unknown, MutateOptions];
    act(() => options.onError?.(null));
    expect(screen.getByText("Failed to create the plan.")).toBeInTheDocument();
  });

  it("guards unsaved changes only once the form is edited", () => {
    renderAdd();
    expect(lastGuard()).toBe(false);
    type(nameInput(), "Gold");
    expect(lastGuard()).toBe(true);
  });

  it("goes back to the list on Cancel", () => {
    renderAdd();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("The plan list")).toBeInTheDocument();
  });

  it("disables the form while the plan is being added", () => {
    state.createPending = true;
    renderAdd();
    expect(screen.getByRole("button", { name: "Adding…" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    expect(nameInput()).toBeDisabled();
    expect(lastGuard()).toBe(false);
  });
});

describe("SubscriptionPlanFormPage: edit", () => {
  it("shows the loading state while the plan loads", () => {
    state.isLoading = true;
    renderEdit();
    expect(screen.getByText("Loading…")).toBeInTheDocument();
  });

  it("shows plan not found when the plan can't be loaded, with a way back", () => {
    state.error = new Error("404");
    renderEdit();
    expect(screen.getByText("Plan not found")).toBeInTheDocument();
    expect(
      screen.getByText("The plan could not be loaded. Check that it exists in this organization."),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Back to Subscription Plans" }));
    expect(screen.getByText("The plan list")).toBeInTheDocument();
  });

  it("fills the form from the plan", () => {
    state.plan = makePlan();
    renderEdit();
    expect(screen.getByRole("heading", { name: "Edit subscription plan" })).toBeInTheDocument();
    expect(
      screen.getByText("Update the conditions and availability of this plan."),
    ).toBeInTheDocument();
    expect(nameInput().value).toBe("Gold");
    expect(descriptionInput().value).toBe("The gold plan");
    expect(nameInput()).toBeEnabled();
    expect(lastGuard()).toBe(false);
  });

  it("waits for the plan with the route's id before filling the form", () => {
    state.plan = makePlan({ id: "silver", displayName: "Silver" });
    renderEdit();
    expect(nameInput().value).toBe("");
  });

  it("saves the editable fields, keeping the status, and goes back", () => {
    state.plan = makePlan();
    renderEdit();
    type(nameInput(), "Gold+");
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(updateMutate).toHaveBeenCalledTimes(1);
    const [variables, options] = updateMutate.mock.calls[0] as [unknown, MutateOptions];
    expect(variables).toEqual({
      body: {
        conditions: [
          expect.objectContaining({
            type: "request-count",
            params: expect.objectContaining({
              limits: [{ count: 10, timeAmount: 1, timeUnit: "MINUTE" }],
            }),
          }),
        ],
        displayName: "Gold+",
        expiryTime: null,
        status: "INACTIVE",
      },
      params: { orgName: "acme", planId: "gold" },
    });
    act(() => options.onSuccess?.());
    expect(screen.getByText("The plan list")).toBeInTheDocument();
  });

  it("sends a changed description, and an empty one to clear it", () => {
    state.plan = makePlan();
    renderEdit();
    type(descriptionInput(), "  ");
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    const [variables] = updateMutate.mock.calls[0] as [{ body: Record<string, unknown> }];
    expect(variables.body.description).toBe("");
  });

  it("keeps the saved expiry, and sends null once it is cleared", () => {
    const expiry = new Date();
    expiry.setFullYear(expiry.getFullYear() + 1);
    const expiryTime = toExpiryTime(expiry);
    state.plan = makePlan({ expiryTime });
    renderEdit();

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    const [kept] = updateMutate.mock.calls[0] as [{ body: Record<string, unknown> }];
    expect(kept.body.expiryTime).toBe(expiryTime);

    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    expect(lastGuard()).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    const [cleared] = updateMutate.mock.calls[1] as [{ body: Record<string, unknown> }];
    expect(cleared.body.expiryTime).toBeNull();
  });

  it("reports an invalid expiry date and doesn't save", () => {
    state.plan = makePlan({ expiryTime: "not a date" });
    renderEdit();
    expect(screen.getByText("Enter a valid date")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    expect(updateMutate).not.toHaveBeenCalled();
  });

  it("locks the name, conditions and expiry of a plan in use", () => {
    state.plan = makePlan({ usage: { inUse: true } } as Partial<SubscriptionPlan>);
    renderEdit();
    expect(
      screen.getByText(
        "This plan has subscriptions. To change its name, conditions or expiry, create a new plan.",
      ),
    ).toBeInTheDocument();
    expect(nameInput()).toBeDisabled();
    expect(descriptionInput()).toBeEnabled();
    expect(screen.queryByRole("button", { name: "Add condition" })).not.toBeInTheDocument();

    type(descriptionInput(), "Updated");
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    const [variables] = updateMutate.mock.calls[0] as [unknown];
    expect(variables).toEqual({
      body: { description: "Updated", displayName: "Gold", status: "INACTIVE" },
      params: { orgName: "acme", planId: "gold" },
    });
  });

  it("shows a deprecated plan read only, with only a Back button", () => {
    state.plan = makePlan({ deprecated: true });
    renderEdit();
    expect(screen.getByRole("heading", { name: "Subscription plan" })).toBeInTheDocument();
    expect(
      screen.getByText("A deprecated plan, kept for the subscriptions on it."),
    ).toBeInTheDocument();
    expect(screen.getByText(/This plan is deprecated/)).toBeInTheDocument();
    expect(screen.queryByText(/This plan has subscriptions/)).not.toBeInTheDocument();
    expect(nameInput()).toBeDisabled();
    expect(descriptionInput()).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(screen.getByText("The plan list")).toBeInTheDocument();
  });

  it("shows Saving… while the plan is being saved", () => {
    state.plan = makePlan();
    state.updatePending = true;
    renderEdit();
    expect(screen.getByRole("button", { name: "Saving…" })).toBeDisabled();
  });
});
