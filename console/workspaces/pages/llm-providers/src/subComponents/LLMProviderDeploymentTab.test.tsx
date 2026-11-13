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

import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import {
  type GatewayResponse,
  type LLMDeploymentResponse,
  type LLMProviderResponse,
} from "@agent-management-platform/types";

// The tab renders the connected EgressGatewaySelector, whose api-client
// import crashes at import time outside a configured app shell. Stub the
// module boundary and feed data through the hooks the tab and selector call.
vi.mock("@agent-management-platform/api-client", () => ({
  useListGateways: vi.fn(),
  useListLLMDeployments: vi.fn(),
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
}));

import {
  useListGateways,
  useListLLMDeployments,
} from "@agent-management-platform/api-client";
import { ConfirmationDialogProvider } from "@agent-management-platform/shared-component";
import {
  LLMProviderDeploymentTab,
  type LLMProviderDeploymentTabProps,
} from "./LLMProviderDeploymentTab";

const mockUseListGateways = vi.mocked(useListGateways);
const mockUseListLLMDeployments = vi.mocked(useListLLMDeployments);

const makeGateway = (uuid: string): GatewayResponse => ({
  uuid,
  organizationName: "org",
  name: uuid,
  displayName: `Gateway ${uuid}`,
  gatewayType: "EGRESS",
  hosting: "MANAGED",
  vhost: "example.com",
  isCritical: false,
  status: "ACTIVE",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
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
  mockUseListGateways.mockReturnValue({
    data: {
      gateways: [makeGateway("gw-a"), makeGateway("gw-b")],
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

const getCheckbox = () =>
  screen.getByRole("checkbox", { name: "Deploy to an egress gateway", hidden: true }) as HTMLInputElement;
const chooseGateway = (optionName: string) => {
  fireEvent.mouseDown(screen.getByRole("combobox", { name: "Egress gateway", hidden: true }));
  const listboxes = screen.getAllByRole("listbox", { hidden: true });
  fireEvent.click(
    within(listboxes[listboxes.length - 1]).getByRole("option", { name: optionName, hidden: true }),
  );
};
const getSaveButton = () => screen.getByRole("button", { name: /save/i });

describe("LLMProviderDeploymentTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    seedSelectorData();
    seedDeployments([]);
  });

  it("seeds from providerData.gateways and keeps Save disabled while clean", () => {
    renderTab({ providerData: makeProvider(["gw-a"]) });
    expect(getCheckbox()).toBeChecked();
    expect(getSaveButton()).toBeDisabled();
  });

  it("saves the chosen gateway as exactly { gateways: [...] }", async () => {
    const onUpdate = vi.fn().mockResolvedValue(makeProvider(["gw-b"]));
    renderTab({ providerData: makeProvider([]), onUpdate });
    expect(getCheckbox()).not.toBeChecked();
    expect(getSaveButton()).toBeDisabled();

    fireEvent.click(getCheckbox());
    chooseGateway("Gateway gw-b");
    expect(getSaveButton()).toBeEnabled();
    fireEvent.click(getSaveButton());

    await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
    expect(onUpdate.mock.calls[0][0]).toEqual({ gateways: ["gw-b"] });
  });

  it("keeps an unsaved selection across a background refetch that leaves the gateway set unchanged", () => {
    const { rerenderTab } = renderTab({ providerData: makeProvider(["gw-a"]) });
    fireEvent.click(getCheckbox());
    expect(getSaveButton()).toBeEnabled();

    rerenderTab({ providerData: makeProvider(["gw-a"]) });

    expect(getCheckbox()).not.toBeChecked();
    expect(getSaveButton()).toBeEnabled();
  });

  it("keeps a FAILED gateway checked and unlocked", () => {
    seedDeployments([makeDeployment("gw-a", "FAILED", "E")]);
    renderTab({ providerData: makeProvider(["gw-a"]) });

    expect(getCheckbox()).toBeChecked();
    expect(getCheckbox()).toBeEnabled();
    expect(screen.getByText("Failed")).toBeInTheDocument();
    expect(screen.queryByText("Deployed")).not.toBeInTheDocument();
  });

  it("locks a DEPLOYED gateway and asks before undeploying it", async () => {
    seedDeployments([makeDeployment("gw-a", "DEPLOYED")]);
    const onUpdate = vi.fn().mockResolvedValue(makeProvider([]));
    renderTab({ providerData: makeProvider(["gw-a"]), onUpdate });

    expect(getCheckbox()).toBeChecked();
    expect(screen.getByText("Deployed")).toBeInTheDocument();

    fireEvent.click(getCheckbox());
    fireEvent.click(getSaveButton());
    expect(await screen.findByText("Undeploy from all gateways?")).toBeInTheDocument();
    expect(onUpdate).not.toHaveBeenCalled();
  });
});
