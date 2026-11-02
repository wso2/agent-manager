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

import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
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

vi.mock("@agent-management-platform/api-client", () => {
  const mutation = { mutateAsync: vi.fn(), isPending: false };
  return {
    useListGateways: () => ({
      data: { gateways: [gateway("managed-1", "MANAGED"), gateway("self-1", "SELF_HOSTED")] },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    }),
    useListEnvironments: () => ({ data: [], isLoading: false, error: null }),
    useDeleteGateway: () => mutation,
    useDeleteSelfHostedGateway: () => mutation,
    useTrack: () => ({ track: vi.fn() }),
    ConsoleAction: {},
  };
});

vi.mock("@agent-management-platform/shared-component", async () => {
  const React = await import("react");
  return {
    GatewayTypeChip: () => React.createElement("span", null, "type"),
    ResourceListShell: ({
      addButton,
      children,
    }: {
      addButton?: { label: string; onClick?: () => void };
      children: React.ReactNode;
    }) =>
      React.createElement(
        "div",
        null,
        addButton && React.createElement("button", { onClick: addButton.onClick }, addButton.label),
        children,
      ),
    formatRelativeTime: () => "now",
    getAvatarInitials: () => "G",
    useConfirmationDialog: () => ({ addConfirmation: vi.fn() }),
  };
});

const renderTable = () =>
  render(
    <MemoryRouter
      initialEntries={["/org/acme"]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <AIGatewaysTable onEditGateway={vi.fn()} onAddGateway={vi.fn()} />
    </MemoryRouter>,
  );

afterEach(() => {
  delete globalConfig.selfHostedGatewaysEnabled;
});

describe("AIGatewaysTable self-hosted gating", () => {
  it("hides self-hosted actions when selfHostedGatewaysEnabled is false", () => {
    globalConfig.selfHostedGatewaysEnabled = false;
    renderTable();

    expect(screen.queryByText("Add Gateway")).not.toBeInTheDocument();
    expect(screen.queryByText("Self-hosted")).not.toBeInTheDocument();
    expect(screen.queryByText("Managed")).not.toBeInTheDocument();
  });

  it("offers Add Gateway and hosting chips when enabled", () => {
    globalConfig.selfHostedGatewaysEnabled = true;
    renderTable();

    expect(screen.getByText("Add Gateway")).toBeInTheDocument();
    expect(screen.getByText("Self-hosted")).toBeInTheDocument();
    expect(screen.getByText("Managed")).toBeInTheDocument();
  });
});
