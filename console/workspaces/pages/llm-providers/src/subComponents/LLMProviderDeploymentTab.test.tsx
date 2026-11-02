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

import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import type {
  Environment,
  GatewayResponse,
  LLMDeploymentResponse,
  LLMProviderResponse,
} from "@agent-management-platform/types";

// The tab renders the connected EnvironmentGatewaySelector, whose api-client
// import crashes at import time outside a configured app shell. Stub the
// module boundary and feed data through the hooks the tab and selector call.
vi.mock("@agent-management-platform/api-client", () => ({
  useListEnvironments: vi.fn(),
  useListGateways: vi.fn(),
  useListLLMDeployments: vi.fn(),
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
}));

import {
  useListEnvironments,
  useListGateways,
  useListLLMDeployments,
} from "@agent-management-platform/api-client";
import { ConfirmationDialogProvider } from "@agent-management-platform/shared-component";
import {
  LLMProviderDeploymentTab,
  type LLMProviderDeploymentTabProps,
} from "./LLMProviderDeploymentTab";

const mockUseListEnvironments = vi.mocked(useListEnvironments);
const mockUseListGateways = vi.mocked(useListGateways);
const mockUseListLLMDeployments = vi.mocked(useListLLMDeployments);

const makeEnvironment = (id: string, name: string): Environment => ({
  id,
  name,
  displayName: name,
  dataplaneRef: "dp-1",
  isProduction: false,
  createdAt: "2026-01-01T00:00:00Z",
});

const makeGateway = (uuid: string, envIds: string[]): GatewayResponse => ({
  uuid,
  organizationName: "org",
  name: uuid,
  displayName: `Gateway ${uuid}`,
  gatewayType: "EGRESS",
  vhost: "example.com",
  isCritical: false,
  status: "ACTIVE",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
  environments: envIds.map((envId) => ({
    id: envId,
    organizationName: "org",
    name: envId,
    displayName: envId,
    dataplaneRef: "dp-1",
    dnsPrefix: envId,
    isProduction: false,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  })),
});

const makeProvider = (gateways: string[]): LLMProviderResponse => ({
  uuid: "provider-1",
  id: "provider-1",
  name: "provider-1",
  version: "v1",
  context: "/llm/provider-1",
  template: "openai",
  upstream: { main: { url: "https://api.example.com" } },
  status: "deployed",
  gateways,
});

const seedSelectorData = () => {
  mockUseListEnvironments.mockReturnValue({
    data: [makeEnvironment("env-a", "Alpha"), makeEnvironment("env-b", "Beta")],
    isLoading: false,
  } as ReturnType<typeof useListEnvironments>);
  mockUseListGateways.mockReturnValue({
    data: {
      gateways: [makeGateway("gw-a", ["env-a"]), makeGateway("gw-b", ["env-b"])],
    },
    isLoading: false,
  } as ReturnType<typeof useListGateways>);
};

const makeDeployment = (
  gatewayId: string,
  status: LLMDeploymentResponse["status"],
  statusReason?: string,
): LLMDeploymentResponse => ({
  deploymentId: `dep-${gatewayId}`,
  agentName: "",
  projectName: "",
  imageId: "",
  environment: "",
  gatewayId,
  createdAt: "2026-01-01T00:00:00Z",
  status,
  statusReason,
});

const seedDeployments = (deployments: LLMDeploymentResponse[]) => {
  mockUseListLLMDeployments.mockReturnValue({
    data: deployments,
    isLoading: false,
  } as ReturnType<typeof useListLLMDeployments>);
};

const renderTab = (props: Partial<LLMProviderDeploymentTabProps> = {}) => {
  const defaultProps: LLMProviderDeploymentTabProps = {
    providerData: makeProvider([]),
    orgName: "org",
    providerId: "provider-1",
    onUpdate: vi.fn().mockResolvedValue(makeProvider([])),
    isUpdating: false,
  };
  const merged = { ...defaultProps, ...props };
  const wrap = (p: LLMProviderDeploymentTabProps) => (
    <MemoryRouter>
      <ThemeProvider theme={createTheme()}>
        <ConfirmationDialogProvider>
          <LLMProviderDeploymentTab {...p} />
        </ConfirmationDialogProvider>
      </ThemeProvider>
    </MemoryRouter>
  );
  const view = render(wrap(merged));
  return {
    ...view,
    rerenderTab: (next: Partial<LLMProviderDeploymentTabProps>) =>
      view.rerender(wrap({ ...merged, ...next })),
  };
};

const getCheckbox = (name: string) =>
  screen.getByRole("checkbox", { name }) as HTMLInputElement;
const getSaveButton = () => screen.getByRole("button", { name: /save/i });

describe("LLMProviderDeploymentTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    seedSelectorData();
    seedDeployments([]);
  });

  it("seeds from providerData.gateways and keeps Save disabled while clean", () => {
    renderTab({ providerData: makeProvider(["gw-a"]) });
    expect(getCheckbox("Alpha")).toBeChecked();
    expect(getCheckbox("Beta")).not.toBeChecked();
    expect(getSaveButton()).toBeDisabled();
  });

  it("saves the toggled selection as exactly { gateways: [...] }", async () => {
    const onUpdate = vi
      .fn()
      .mockResolvedValue(makeProvider(["gw-a", "gw-b"]));
    renderTab({ providerData: makeProvider(["gw-a"]), onUpdate });

    fireEvent.click(getCheckbox("Beta"));
    expect(getSaveButton()).toBeEnabled();
    fireEvent.click(getSaveButton());

    await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
    expect(onUpdate.mock.calls[0][0]).toEqual({ gateways: ["gw-a", "gw-b"] });
  });

  it("renders every row undeployed for a gateway-less provider and enables Save once a choice is made", () => {
    renderTab({ providerData: makeProvider([]) });
    expect(getCheckbox("Alpha")).not.toBeChecked();
    expect(getCheckbox("Beta")).not.toBeChecked();
    expect(getSaveButton()).toBeDisabled();

    fireEvent.click(getCheckbox("Alpha"));
    expect(getSaveButton()).toBeEnabled();
  });

  it("keeps an unsaved selection across a background refetch that leaves the gateway set unchanged", () => {
    const { rerenderTab } = renderTab({ providerData: makeProvider(["gw-a"]) });
    fireEvent.click(getCheckbox("Beta"));
    expect(getSaveButton()).toBeEnabled();

    // New object identity, same gateway set — e.g. a window-focus refetch
    // that changed an unrelated provider field.
    rerenderTab({ providerData: makeProvider(["gw-a"]) });

    expect(getCheckbox("Beta")).toBeChecked();
    expect(getSaveButton()).toBeEnabled();
  });

  it("reconciles the selection down to the post-save refetch when a deploy fails", async () => {
    const onUpdate = vi
      .fn()
      .mockResolvedValue(makeProvider(["gw-a", "gw-b"]));
    const { rerenderTab } = renderTab({
      providerData: makeProvider(["gw-a"]),
      onUpdate,
    });

    fireEvent.click(getCheckbox("Beta"));
    fireEvent.click(getSaveButton());
    await screen.findByText("Deployment updated successfully.");
    expect(onUpdate.mock.calls[0][0]).toEqual({ gateways: ["gw-a", "gw-b"] });

    // The refetched GET only returns gw-a: the server dropped gw-b.
    rerenderTab({ providerData: makeProvider(["gw-a"]) });

    expect(getCheckbox("Alpha")).toBeChecked();
    expect(getCheckbox("Beta")).not.toBeChecked();
    expect(getSaveButton()).toBeDisabled();
  });

  it("keeps a FAILED gateway checked and unlocked, and an unchanged save sends it back", async () => {
    seedDeployments([makeDeployment("gw-a", "FAILED", "E")]);
    const onUpdate = vi
      .fn()
      .mockResolvedValue(makeProvider(["gw-a", "gw-b"]));
    renderTab({ providerData: makeProvider(["gw-a"]), onUpdate });

    expect(getCheckbox("Alpha")).toBeChecked();
    expect(getCheckbox("Alpha")).toBeEnabled();
    expect(screen.getByText("Failed")).toBeInTheDocument();
    expect(screen.queryByText("Deployed")).not.toBeInTheDocument();

    fireEvent.click(getCheckbox("Alpha"));
    expect(getCheckbox("Alpha")).not.toBeChecked();
    fireEvent.click(getCheckbox("Alpha"));
    expect(getCheckbox("Alpha")).toBeChecked();
    fireEvent.click(getCheckbox("Beta"));
    fireEvent.click(getSaveButton());

    await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
    expect(onUpdate.mock.calls[0][0]).toEqual({ gateways: ["gw-a", "gw-b"] });
  });

  it("locks only DEPLOYED gateways", () => {
    // gw-b has no deployment entry yet (just saved), so it counts as DEPLOYING.
    seedDeployments([makeDeployment("gw-a", "DEPLOYED")]);
    renderTab({ providerData: makeProvider(["gw-a", "gw-b"]) });

    expect(getCheckbox("Alpha")).toBeChecked();
    expect(getCheckbox("Alpha")).toBeDisabled();
    expect(screen.getByText("Deployed")).toBeInTheDocument();

    expect(getCheckbox("Beta")).toBeChecked();
    expect(getCheckbox("Beta")).toBeEnabled();
    expect(screen.getByText("Deploying")).toBeInTheDocument();
  });
});
