/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */


import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { EditGateway } from "./EditGateway";

const hoisted = vi.hoisted(() => ({
  gatewayQuery: {} as Record<string, unknown>,
  refetch: vi.fn(),
  updateSelfHosted: vi.fn(),
  updateError: null as Error | null,
}));

// MUI renders are slow on a busy machine; leave headroom over the 5s default.
vi.setConfig({ testTimeout: 30_000 });

vi.mock("@agent-management-platform/api-client", () => ({
  useGetGateway: () => ({ refetch: hoisted.refetch, ...hoisted.gatewayQuery }),
  useUpdateSelfHostedGateway: () => ({
    mutateAsync: hoisted.updateSelfHosted,
    isPending: false,
    error: hoisted.updateError,
  }),
}));

vi.mock("@agent-management-platform/shared-component", () => ({
  useUnsavedChangesGuard: () => ({ allowNavigation: (go: () => void) => go() }),
}));

const gateway = (hosting: "MANAGED" | "SELF_HOSTED" = "SELF_HOSTED") => ({
  uuid: "gw-uuid",
  name: "edge",
  displayName: "Edge Gateway",
  isCritical: true,
  hosting,
  environments: [],
});

const Location = () => <div data-testid="location">{useLocation().pathname}</div>;

const renderEdit = () =>
  render(
    <MemoryRouter
      initialEntries={["/org/acme/gateways/edit/gw-uuid"]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route path="/org/:orgId/gateways/edit/:gatewayId" element={<EditGateway />} />
        <Route path="*" element={null} />
      </Routes>
      <Location />
    </MemoryRouter>,
  );

const nameField = () => screen.getByPlaceholderText("e.g., Production Gateway");
const VIEW_PATH = "/org/acme/gateways/view/gw-uuid";

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.updateError = null;
  hoisted.gatewayQuery = { data: gateway(), isLoading: false, error: null };
  hoisted.updateSelfHosted.mockResolvedValue({});
});

describe("EditGateway", () => {
  it("shows no form while the gateway loads", () => {
    hoisted.gatewayQuery = { data: undefined, isLoading: true, error: null };
    renderEdit();
    expect(screen.queryByPlaceholderText("e.g., Production Gateway")).not.toBeInTheDocument();
    expect(screen.queryByText(/couldn't load/i)).not.toBeInTheDocument();
  });

  it("offers a retry when the gateway fails to load", () => {
    hoisted.gatewayQuery = { data: undefined, isLoading: false, error: new Error("not found") };
    renderEdit();
    expect(screen.getByText("Couldn't load this gateway")).toBeInTheDocument();
    expect(screen.getByText("not found")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(hoisted.refetch).toHaveBeenCalled();
    expect(screen.getByRole("link", { name: "Back to gateways" })).toHaveAttribute(
      "href",
      "/org/acme/gateways",
    );
  });

  it("saves the trimmed display name and returns to the gateway", async () => {
    renderEdit();
    expect(nameField()).toHaveValue("Edge Gateway");

    fireEvent.change(nameField(), { target: { value: "  Renamed Gateway  " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(
      () => expect(screen.getByTestId("location")).toHaveTextContent(VIEW_PATH),
      { timeout: 10_000 },
    );
    expect(hoisted.updateSelfHosted).toHaveBeenCalledWith({
      params: { orgName: "acme", gatewayId: "gw-uuid" },
      body: { displayName: "Renamed Gateway", isCritical: true },
    });
  });

  it("shows a managed gateway as read-only, with no form", () => {
    hoisted.gatewayQuery = { data: gateway("MANAGED"), isLoading: false, error: null };
    renderEdit();
    expect(screen.getByText(/managed by the platform and can't be edited/)).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("e.g., Production Gateway")).not.toBeInTheDocument();
  });

  it("blocks saving an empty display name", async () => {
    renderEdit();
    fireEvent.change(nameField(), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect((await screen.findAllByText("Display name is required")).length).toBeGreaterThan(0);
    expect(hoisted.updateSelfHosted).not.toHaveBeenCalled();
    expect(screen.getByTestId("location")).toHaveTextContent("/org/acme/gateways/edit/gw-uuid");
  });

  it("stays on the form when saving fails", async () => {
    hoisted.updateSelfHosted.mockRejectedValue(new Error("conflict"));
    renderEdit();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(hoisted.updateSelfHosted).toHaveBeenCalled());
    expect(screen.getByTestId("location")).toHaveTextContent("/org/acme/gateways/edit/gw-uuid");
  });

  it("shows the update error", () => {
    hoisted.updateError = new Error("Display name already taken");
    renderEdit();
    expect(screen.getByText("Display name already taken")).toBeInTheDocument();
  });

  it("returns to the gateway on cancel", () => {
    renderEdit();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByTestId("location")).toHaveTextContent(VIEW_PATH);
  });
});
