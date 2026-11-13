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

import React, { useState } from "react";
import { render, screen, fireEvent, within } from "@testing-library/react";
import { describe, it, expect, vi, afterEach } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import {
  FeatureManagementProvider,
  type FeatureFlags,
} from "@agent-management-platform/feature-management";
import type {
  GatewayDeploymentStatus,
  GatewayHosting,
  GatewayResponse,
  GatewayType,
} from "@agent-management-platform/types";
// Only the view is under test, but importing the module drags in api-client,
// whose auth dependency crashes at import time outside a configured app shell.
vi.mock("@agent-management-platform/api-client", () => ({
  useListGateways: vi.fn(() => ({ data: undefined, isLoading: false })),
}));

import { EgressGatewaySelectorView } from "./EgressGatewaySelector";

let featureFlags: Partial<FeatureFlags> = {};

afterEach(() => {
  featureFlags = {};
});

const renderWithTheme = (component: React.ReactElement) =>
  render(
    <FeatureManagementProvider featureFlags={featureFlags}>
      <ThemeProvider theme={createTheme()}>{component}</ThemeProvider>
    </FeatureManagementProvider>,
  );

const makeGateway = (
  uuid: string,
  gatewayType: GatewayType = "EGRESS",
  hosting: GatewayHosting = "MANAGED",
): GatewayResponse => ({
  uuid,
  organizationName: "org",
  name: uuid,
  displayName: `Gateway ${uuid}`,
  gatewayType,
  hosting,
  vhost: "example.com",
  isCritical: false,
  status: "ACTIVE",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
});

// Controlled wrapper so onChange results flow back into value.
const ControlledSelector: React.FC<{
  gateways: GatewayResponse[];
  initialValue?: string[];
  lockedGatewayIds?: string[];
  gatewayStatuses?: Map<string, GatewayDeploymentStatus>;
  onChangeSpy?: (ids: string[]) => void;
  onValidityChange?: (isValid: boolean) => void;
  onSingleChoiceChange?: (isSingleChoice: boolean) => void;
}> = ({ gateways, initialValue = [], onChangeSpy, ...rest }) => {
  const [value, setValue] = useState<string[]>(initialValue);
  return (
    <EgressGatewaySelectorView
      gateways={gateways}
      value={value}
      onChange={(ids) => {
        setValue(ids);
        onChangeSpy?.(ids);
      }}
      {...rest}
    />
  );
};

const CHECKBOX = "Deploy to an egress gateway";
const getCheckbox = () =>
  screen.getByRole("checkbox", { name: CHECKBOX, hidden: true }) as HTMLInputElement;

const chooseOption = (optionName: string) => {
  fireEvent.mouseDown(screen.getByRole("combobox", { name: "Egress gateway", hidden: true }));
  const listboxes = screen.getAllByRole("listbox", { hidden: true });
  fireEvent.click(
    within(listboxes[listboxes.length - 1]).getByRole("option", {
      name: optionName,
      hidden: true,
    }),
  );
};

describe("EgressGatewaySelectorView", () => {
  it("auto-selects and renders nothing for a single managed candidate", () => {
    const onChangeSpy = vi.fn();
    const onSingleChoiceChange = vi.fn();
    const { container } = renderWithTheme(
      <ControlledSelector
        gateways={[makeGateway("gw-1")]}
        onChangeSpy={onChangeSpy}
        onSingleChoiceChange={onSingleChoiceChange}
      />,
    );
    expect(onSingleChoiceChange).toHaveBeenLastCalledWith(true);
    expect(onChangeSpy).toHaveBeenCalledWith(["gw-1"]);
    expect(container).toBeEmptyDOMElement();
  });

  it("ignores ingress-only gateways", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        gateways={[makeGateway("gw-1"), makeGateway("gw-in", "INGRESS")]}
        onChangeSpy={onChangeSpy}
      />,
    );
    expect(onChangeSpy).toHaveBeenCalledWith(["gw-1"]);
  });

  it("renders a picker for 2 candidates and reports invalid until one is chosen", () => {
    const onChangeSpy = vi.fn();
    const onValidityChange = vi.fn();
    renderWithTheme(
      <ControlledSelector
        gateways={[makeGateway("gw-1"), makeGateway("gw-2")]}
        onChangeSpy={onChangeSpy}
        onValidityChange={onValidityChange}
      />,
    );
    expect(onValidityChange).toHaveBeenLastCalledWith(true);
    expect(screen.getByText("2 gateways available")).toBeInTheDocument();
    expect(screen.queryByRole("combobox", { hidden: true })).not.toBeInTheDocument();
    fireEvent.click(getCheckbox());
    expect(onChangeSpy).not.toHaveBeenCalled();
    expect(onValidityChange).toHaveBeenLastCalledWith(false);
    expect(screen.getByText("Select an egress gateway.")).toBeInTheDocument();
    chooseOption("Gateway gw-2");
    expect(onChangeSpy).toHaveBeenLastCalledWith(["gw-2"]);
    expect(onValidityChange).toHaveBeenLastCalledWith(true);
  });

  it("disables the checkbox with no candidate and explains why", () => {
    renderWithTheme(<ControlledSelector gateways={[]} />);
    expect(getCheckbox()).toBeDisabled();
    expect(screen.getByText("No egress-capable gateway is available.")).toBeInTheDocument();
  });

  it("shows a locked gateway as deployed and lets it be switched when others exist", () => {
    const onSingleChoiceChange = vi.fn();
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        gateways={[makeGateway("gw-1"), makeGateway("gw-2")]}
        initialValue={["gw-1"]}
        lockedGatewayIds={["gw-1"]}
        onChangeSpy={onChangeSpy}
        onSingleChoiceChange={onSingleChoiceChange}
      />,
    );
    expect(onSingleChoiceChange).toHaveBeenLastCalledWith(true);
    expect(getCheckbox()).toBeChecked();
    expect(screen.getByText("Gateway gw-1")).toBeInTheDocument();
    expect(screen.getByText("Deployed")).toBeInTheDocument();
    expect(screen.getByText(/select the new gateway and save again/)).toBeInTheDocument();
    fireEvent.click(getCheckbox());
    expect(onChangeSpy).toHaveBeenLastCalledWith([]);
  });

  it("keeps a locked sole candidate fixed, with no undeploy affordance", () => {
    renderWithTheme(
      <ControlledSelector
        gateways={[makeGateway("gw-1")]}
        initialValue={["gw-1"]}
        lockedGatewayIds={["gw-1"]}
      />,
    );
    expect(getCheckbox()).toBeChecked();
    expect(getCheckbox()).toBeDisabled();
    expect(screen.queryByText(/select the new gateway/)).not.toBeInTheDocument();
  });

  it("shows a locked gateway whose deploy failed as Failed and leaves it editable", () => {
    renderWithTheme(
      <ControlledSelector
        gateways={[makeGateway("gw-1")]}
        initialValue={["gw-1"]}
        lockedGatewayIds={["gw-1"]}
        gatewayStatuses={
          new Map([["gw-1", { status: "FAILED", statusReason: "boom" } as GatewayDeploymentStatus]])
        }
      />,
    );
    expect(screen.getByText("Failed")).toBeInTheDocument();
    expect(screen.queryByText("Deployed")).not.toBeInTheDocument();
  });

  it("renders an unavailable selected gateway as removable and reports invalid while it remains", () => {
    const onValidityChange = vi.fn();
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        gateways={[makeGateway("gw-1"), makeGateway("gw-2"), makeGateway("gw-x", "INGRESS")]}
        initialValue={["gw-x"]}
        onValidityChange={onValidityChange}
        onChangeSpy={onChangeSpy}
      />,
    );
    expect(onValidityChange).toHaveBeenLastCalledWith(false);
    expect(screen.getByText("Unavailable")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("checkbox", { name: "Gateway gw-x", hidden: true }));
    expect(onChangeSpy).toHaveBeenLastCalledWith([]);
    expect(onValidityChange).toHaveBeenLastCalledWith(true);
  });

  it("never commits a lone self-hosted gateway on the user's behalf", () => {
    featureFlags = { enableSelfHostedGateways: true };
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        gateways={[makeGateway("gw-sh", "EGRESS", "SELF_HOSTED")]}
        onChangeSpy={onChangeSpy}
      />,
    );
    expect(onChangeSpy).not.toHaveBeenCalled();
    expect(getCheckbox()).not.toBeChecked();
  });

  it("preselects the managed gateway when a self-hosted one also exists", () => {
    featureFlags = { enableSelfHostedGateways: true };
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        gateways={[makeGateway("gw-m"), makeGateway("gw-sh", "EGRESS", "SELF_HOSTED")]}
        onChangeSpy={onChangeSpy}
      />,
    );
    fireEvent.click(getCheckbox());
    expect(onChangeSpy).toHaveBeenLastCalledWith(["gw-m"]);
  });
});
