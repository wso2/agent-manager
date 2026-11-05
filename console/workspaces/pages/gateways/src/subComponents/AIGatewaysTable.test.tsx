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

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { globalConfig } from "@agent-management-platform/types";
import { AIGatewaysTable } from "./AIGatewaysTable";

const gateway = (uuid: string, hosting: "MANAGED" | "SELF_HOSTED") => ({
  uuid,
  name: uuid,
  displayName: `Gateway ${uuid}`,
  hosting,
  status: "ACTIVE",
  gatewayType: "AI",
  updatedAt: new Date().toISOString(),
  environments: [],
});

const hoisted = vi.hoisted(() => ({
  deleteManaged: vi.fn(),
  deleteSelfHosted: vi.fn(),
  addConfirmation: vi.fn(),
  gateways: [] as ReturnType<typeof gateway>[],
}));

vi.mock("@agent-management-platform/api-client", () => {
  return {
    useListGateways: () => ({
      data: { gateways: hoisted.gateways },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    }),
    useListEnvironments: () => ({ data: [], isLoading: false, error: null }),
    useDeleteGateway: () => ({ mutateAsync: hoisted.deleteManaged }),
    useDeleteSelfHostedGateway: () => ({ mutateAsync: hoisted.deleteSelfHosted }),
    useTrack: () => ({ track: vi.fn() }),
    ConsoleAction: {},
  };
});

vi.mock("@agent-management-platform/shared-component", async () => {
  const React = await import("react");
  const { gatewayHostingLabel } = await import("@agent-management-platform/types");
  return {
    GatewayTypeChip: () => React.createElement("span", null, "type"),
    GatewayHostingChip: (props: { gateway: Parameters<typeof gatewayHostingLabel>[0] }) =>
      React.createElement("span", null, gatewayHostingLabel(props.gateway)),
    ResourceListShell: ({
      isEmpty,
      emptyState,
      children,
    }: {
      isEmpty?: boolean;
      emptyState: { action?: React.ReactNode };
      children: React.ReactNode;
    }) => React.createElement("div", null, isEmpty ? emptyState.action : children),
    formatRelativeTime: () => "now",
    getAvatarInitials: () => "G",
    useConfirmationDialog: () => ({ addConfirmation: hoisted.addConfirmation }),
  };
});

const renderTable = () =>
  render(
    <MemoryRouter
      initialEntries={["/org/acme"]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <AIGatewaysTable onAddGateway={vi.fn()} />
    </MemoryRouter>,
  );

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.gateways = [gateway("managed-1", "MANAGED"), gateway("self-1", "SELF_HOSTED")];
});

afterEach(() => {
  delete globalConfig.featureFlags?.enableSelfHostedGateways;
});

describe("AIGatewaysTable self-hosted gating", () => {
  it("shows an existing self-hosted gateway's hosting but hides Add Gateway when disabled", () => {
    globalConfig.featureFlags = { ...globalConfig.featureFlags, enableSelfHostedGateways: false };
    renderTable();

    expect(screen.queryByText("Add Gateway")).not.toBeInTheDocument();
    expect(screen.getByText("Hosting")).toBeInTheDocument();
    expect(screen.getByText("Self-hosted")).toBeInTheDocument();
    expect(screen.getByText("Managed")).toBeInTheDocument();
  });

  it("shows no hosting with only managed gateways when disabled", () => {
    globalConfig.featureFlags = { ...globalConfig.featureFlags, enableSelfHostedGateways: false };
    hoisted.gateways = [gateway("managed-1", "MANAGED"), gateway("managed-2", "MANAGED")];
    renderTable();

    expect(screen.queryByText("Add Gateway")).not.toBeInTheDocument();
    expect(screen.queryByText("Hosting")).not.toBeInTheDocument();
    expect(screen.queryByText("Self-hosted")).not.toBeInTheDocument();
    expect(screen.queryByText("Managed")).not.toBeInTheDocument();
  });

  it("offers Add Gateway in the empty state when enabled", () => {
    globalConfig.featureFlags = { ...globalConfig.featureFlags, enableSelfHostedGateways: true };
    hoisted.gateways = [];
    renderTable();

    expect(screen.getByText("Add Gateway")).toBeInTheDocument();
  });

  it("shows hosting chips when enabled", () => {
    globalConfig.featureFlags = { ...globalConfig.featureFlags, enableSelfHostedGateways: true };
    renderTable();

    expect(screen.getByText("Self-hosted")).toBeInTheDocument();
    expect(screen.getByText("Managed")).toBeInTheDocument();
  });

  describe.each([true, false])("row actions with enableSelfHostedGateways=%s", (enabled) => {
    it("lets managed rows delete through the managed hooks", async () => {
      globalConfig.featureFlags = {
        ...globalConfig.featureFlags,
        enableSelfHostedGateways: enabled,
      };
      renderTable();

      fireEvent.click(screen.getAllByRole("button", { name: "Delete", hidden: true })[0]);
      await hoisted.addConfirmation.mock.calls[0][0].onConfirm();
      expect(hoisted.deleteManaged).toHaveBeenCalledWith({ orgName: "", gatewayId: "managed-1" });
      expect(hoisted.deleteSelfHosted).not.toHaveBeenCalled();
    });
  });

  it("lets self-hosted rows delete through the self-hosted hooks", async () => {
    globalConfig.featureFlags = { ...globalConfig.featureFlags, enableSelfHostedGateways: true };
    renderTable();

    fireEvent.click(screen.getAllByRole("button", { name: "Delete", hidden: true })[1]);
    await hoisted.addConfirmation.mock.calls[0][0].onConfirm();
    await waitFor(() =>
      expect(hoisted.deleteSelfHosted).toHaveBeenCalledWith({ orgName: "", gatewayId: "self-1" }),
    );
    expect(hoisted.deleteManaged).not.toHaveBeenCalled();
  });

  it("shows Delete on every row when disabled", () => {
    globalConfig.featureFlags = { ...globalConfig.featureFlags, enableSelfHostedGateways: false };
    renderTable();
    expect(screen.getAllByRole("button", { name: "Delete", hidden: true })).toHaveLength(2);
  });
});
