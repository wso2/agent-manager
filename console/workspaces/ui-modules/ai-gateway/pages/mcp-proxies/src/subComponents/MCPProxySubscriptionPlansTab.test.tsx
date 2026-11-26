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


import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
  MCPProxy,
  MCPProxyPolicy,
  SubscriptionPlan,
} from "@agent-management-platform/types";
import {
  MCPProxySubscriptionPlansTab,
  deprecatedBlockingSave,
  planOptions,
  samePlans,
  shortDate,
  togglePlan,
} from "./MCPProxySubscriptionPlansTab";

const state = vi.hoisted(() => ({
  canWrite: true,
  canReadPlans: true,
  plans: [] as SubscriptionPlan[],
  isLoading: false,
  error: null as unknown,
}));

vi.mock("@agent-management-platform/views", () => ({
  StateNotice: ({ title, description }: { title: string; description: string }) => (
    <div>
      <p>{title}</p>
      <p>{description}</p>
    </div>
  ),
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canRead: true, canWrite: state.canWrite }),
  useFeatureAccess: (key: string) => (key === "subscription_plan_read" ? state.canReadPlans : false),
}));
vi.mock("@agent-management-platform/api-client", () => ({
  useListSubscriptionPlans: () => ({
    data: { list: state.plans, count: state.plans.length },
    isLoading: state.isLoading,
    error: state.error,
  }),
  extractServerErrorMessage: (e: unknown) => (e instanceof Error ? e.message : undefined),
}));

const plan = (
  id: string,
  displayName: string,
  extra: Partial<SubscriptionPlan> = {},
): SubscriptionPlan => ({
  id,
  displayName,
  status: "ACTIVE",
  conditions: [],
  deprecated: false,
  ...extra,
});

const proxy = (
  subscriptionPlans?: string[],
  policies: MCPProxyPolicy[] = [],
): MCPProxy =>
  ({
    id: "weather",
    name: "Weather",
    version: "v1",
    subscriptionPlans,
    endpoints: [{ id: "main", policies }, { id: "eu", policies }],
  }) as unknown as MCPProxy;

const toolLimit = [{ type: "tool-call-count", params: {} }] as SubscriptionPlan["conditions"];
const allPolicies: MCPProxyPolicy[] = [
  { name: "subscription-validation", version: "v2" },
  { name: "subscription-tool-ratelimit", version: "v1" },
];

const renderTab = (p: MCPProxy, onSave = vi.fn()) =>
  render(
    <MemoryRouter>
      <MCPProxySubscriptionPlansTab proxy={p} orgName="default" onSave={onSave} isSaving={false} />
    </MemoryRouter>,
  );

describe("plan helpers", () => {
  it("offers active plans, and keeps offered ones whatever their status", () => {
    const plans = [
      plan("gold", "Gold"),
      plan("old", "Old", { status: "INACTIVE" }),
      plan("kept", "Kept", { status: "INACTIVE" }),
    ];
    expect(planOptions(plans, ["kept", "gone"]).map((o) => o.id)).toEqual(["gold", "kept", "gone"]);
  });

  it("leaves out a deprecated plan unless the proxy already offers it", () => {
    const plans = [
      plan("gold", "Gold"),
      plan("retired", "Retired", { deprecated: true }),
      plan("offered", "Offered", { deprecated: true }),
    ];
    const options = planOptions(plans, ["offered"]);
    expect(options.map((o) => o.id)).toEqual(["gold", "offered"]);
    expect(options.find((o) => o.id === "offered")?.deprecated).toBe(true);
  });

  it("a deprecated plan blocks only a changed selection", () => {
    const options = planOptions(
      [plan("gold", "Gold"), plan("retired", "Retired", { deprecated: true })],
      ["retired"],
    );
    expect(deprecatedBlockingSave(["retired"], ["retired"], options)).toEqual([]);
    expect(deprecatedBlockingSave(["retired", "gold"], ["retired"], options).map((o) => o.id)).toEqual([
      "retired",
    ]);
    expect(deprecatedBlockingSave(["gold"], ["retired"], options)).toEqual([]);
  });

  it("compares and toggles plans", () => {
    expect(samePlans(["a", "b"], ["b", "a"])).toBe(true);
    expect(samePlans([], ["a"])).toBe(false);
    expect(togglePlan(["a"], "b")).toEqual(["a", "b"]);
    expect(togglePlan(["a", "b"], "a")).toEqual(["b"]);
  });
});

describe("MCPProxySubscriptionPlansTab", () => {
  beforeEach(() => {
    state.canWrite = true;
    state.canReadPlans = true;
    state.isLoading = false;
    state.error = null;
    state.plans = [
      plan("gold", "Gold", {
        description: "For production agents",
        expiryTime: "2030-01-01T00:00:00Z",
        conditions: toolLimit,
      }),
      plan("silver", "Silver", { conditions: toolLimit }),
    ];
  });

  it("shows each plan as a checkbox card: name, expiry date when set, then description", () => {
    renderTab(proxy(["gold"], allPolicies));
    const gold = screen.getByRole("checkbox", { name: "Gold" });
    expect(gold).toHaveAttribute("aria-checked", "true");
    expect(
      within(gold).getByText(`Expires ${shortDate("2030-01-01T00:00:00Z")}`),
    ).toBeInTheDocument();
    expect(within(gold).getByText("For production agents")).toBeInTheDocument();
    const silver = screen.getByRole("checkbox", { name: "Silver" });
    expect(silver).toHaveAttribute("aria-checked", "false");
    expect(within(silver).queryByText(/Expires/)).not.toBeInTheDocument();
  });

  it("marks an offered deprecated plan, which can still be unticked", () => {
    state.plans = [...state.plans, plan("retired", "Retired", { deprecated: true })];
    renderTab(proxy(["retired"], allPolicies));
    const retired = screen.getByRole("checkbox", { name: "Retired" });
    expect(retired).toHaveAttribute("aria-checked", "true");
    expect(within(retired).getByText("Deprecated")).toBeInTheDocument();
    fireEvent.click(retired);
    expect(retired).toHaveAttribute("aria-checked", "false");
  });

  it("won't save another plan beside an offered deprecated one until it is removed", () => {
    state.plans = [...state.plans, plan("retired", "Retired", { deprecated: true })];
    const onSave = vi.fn().mockResolvedValue(undefined);
    renderTab(proxy(["retired"], allPolicies), onSave);
    const save = screen.getByRole("button", { name: "Save" });

    fireEvent.click(screen.getByRole("checkbox", { name: "Gold" }));
    const dialog = screen.queryByRole("dialog");
    if (dialog) fireEvent.click(within(dialog).getByRole("button", { name: /skip/i }));
    expect(save).toBeDisabled();
    expect(screen.getByText(/"Retired" is deprecated\. Remove it to save/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("checkbox", { name: "Retired" }));
    expect(save).toBeEnabled();
    expect(screen.queryByText(/is deprecated\. Remove it/)).not.toBeInTheDocument();
  });

  it("warns when an offered plan's policies are not attached", () => {
    renderTab(proxy(["gold"]));
    expect(
      screen.getByText(/not yet attached the following required policies/),
    ).toBeInTheDocument();
    expect(screen.getByText("subscription-validation v2")).toBeInTheDocument();
    expect(screen.getByText("subscription-tool-ratelimit v1")).toBeInTheDocument();
  });

  it("picking a plan offers its policies; attaching saves them on every endpoint", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    renderTab(proxy([]), onSave);
    fireEvent.click(screen.getByRole("checkbox", { name: "Silver" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Attach required policies?")).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Attach policies" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.queryByText(/not yet attached/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(onSave).toHaveBeenCalled());
    const [plans, endpoints] = onSave.mock.calls[0];
    expect(plans).toEqual(["silver"]);
    for (const ep of endpoints) {
      expect(ep.policies.map((p: MCPProxyPolicy) => `${p.name} ${p.version}`)).toEqual([
        "subscription-validation v2",
        "subscription-tool-ratelimit v1",
      ]);
    }
  });

  it("the warning is hidden while the dialog is open and shows after skip", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    renderTab(proxy([]), onSave);
    fireEvent.click(screen.getByRole("checkbox", { name: "Silver" }));
    await screen.findByRole("dialog");
    // No warning behind the dialog while it is open.
    expect(screen.queryByText(/not yet attached/)).not.toBeInTheDocument();
    fireEvent.click(await screen.findByRole("button", { name: "Skip" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.getByText(/not yet attached/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(onSave).toHaveBeenCalledWith(["silver"], undefined));
  });

  it("no popup when the policies are already attached", () => {
    renderTab(proxy([], allPolicies));
    fireEvent.click(screen.getByRole("checkbox", { name: "Silver" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("shows why a save was refused", async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('subscription plan "silver" is not active'));
    renderTab(proxy([], allPolicies), onSave);
    fireEvent.click(screen.getByRole("checkbox", { name: "Silver" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText(/is not active/)).toBeInTheDocument();
  });

  it("discard puts back the offered plans", () => {
    renderTab(proxy(["gold"], allPolicies));
    fireEvent.click(screen.getByRole("checkbox", { name: "Gold" }));
    fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(screen.getByRole("checkbox", { name: "Gold" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("without write access, shows only the offered plans", () => {
    state.canWrite = false;
    renderTab(proxy(["gold"], allPolicies));
    expect(screen.getByRole("checkbox", { name: "Gold" })).toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "Silver" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("says when there is nothing to offer", () => {
    state.plans = [];
    renderTab(proxy([]));
    expect(screen.getByText("No active subscription plans")).toBeInTheDocument();
  });

  it("says a read-only viewer the server offers nothing", () => {
    state.plans = [];
    state.canWrite = false;
    renderTab(proxy([]));
    expect(screen.getByText("No plans offered")).toBeInTheDocument();
    expect(
      screen.getByText("This MCP server doesn't offer any subscription plans."),
    ).toBeInTheDocument();
  });

  it("shows placeholders while the plans load, and an error when they don't", () => {
    state.isLoading = true;
    const { container, unmount } = renderTab(proxy(["gold"]));
    expect(container.querySelectorAll(".MuiSkeleton-root")).toHaveLength(3);
    unmount();
    state.isLoading = false;
    state.error = new Error("down");
    renderTab(proxy(["gold"]));
    expect(screen.getByText(/Couldn.t load the subscription plans/)).toBeInTheDocument();
  });

  it("marks an offered plan that has since been deactivated", () => {
    state.plans = [...state.plans, plan("paused", "Paused", { status: "INACTIVE" })];
    renderTab(proxy(["paused"], allPolicies));
    const paused = screen.getByRole("checkbox", { name: "Paused" });
    expect(within(paused).getByText("Inactive")).toBeInTheDocument();
  });

  it("names every deprecated plan that blocks a save", () => {
    state.plans = [
      ...state.plans,
      plan("old-a", "Old A", { deprecated: true }),
      plan("old-b", "Old B", { deprecated: true }),
    ];
    renderTab(proxy(["old-a", "old-b"], allPolicies));
    fireEvent.click(screen.getByRole("checkbox", { name: "Gold" }));
    const dialog = screen.queryByRole("dialog");
    if (dialog) fireEvent.click(within(dialog).getByRole("button", { name: /skip/i }));
    expect(
      screen.getByText(/"Old A", "Old B" are deprecated\. Remove them to save/),
    ).toBeInTheDocument();
  });

  it("checks on hover whether a name or description is cut short", () => {
    renderTab(proxy(["gold"], allPolicies));
    const gold = screen.getByRole("checkbox", { name: "Gold" });
    fireEvent.mouseEnter(within(gold).getByText("Gold"));
    fireEvent.mouseEnter(within(gold).getByText("For production agents"));
    expect(gold).toBeInTheDocument();
  });
});
