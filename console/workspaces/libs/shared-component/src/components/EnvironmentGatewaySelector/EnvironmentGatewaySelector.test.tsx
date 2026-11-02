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
import { describe, it, expect, vi } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import type {
  Environment,
  GatewayDeploymentStatus,
  GatewayResponse,
  GatewayType,
} from "@agent-management-platform/types";
// Only EnvironmentGatewaySelectorView is under test, but importing the module
// drags in api-client, whose auth dependency crashes at import time outside a
// configured app shell. Stub the module boundary; no hook behavior is mocked.
vi.mock("@agent-management-platform/api-client", () => ({
  useListEnvironments: vi.fn(() => ({ data: [], isLoading: false })),
  useListGateways: vi.fn(() => ({ data: undefined, isLoading: false })),
}));

import { EnvironmentGatewaySelectorView } from "./EnvironmentGatewaySelector";

const renderWithTheme = (component: React.ReactElement) =>
  render(<ThemeProvider theme={createTheme()}>{component}</ThemeProvider>);

const makeEnvironment = (id: string, name: string): Environment => ({
  id,
  name,
  displayName: name,
  dataplaneRef: "dp-1",
  isProduction: false,
  createdAt: "2026-01-01T00:00:00Z",
});

// A gateway belongs to exactly one environment; the wire shape is an array.
const makeGateway = (
  uuid: string,
  envId: string,
  gatewayType: GatewayType = "EGRESS",
): GatewayResponse => ({
  uuid,
  organizationName: "org",
  name: uuid,
  displayName: `Gateway ${uuid}`,
  gatewayType,
  vhost: "example.com",
  isCritical: false,
  status: "ACTIVE",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
  environments: [
    {
      id: envId,
      organizationName: "org",
      name: envId,
      displayName: envId,
      dataplaneRef: "dp-1",
      dnsPrefix: envId,
      isProduction: false,
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    },
  ],
});

// Controlled wrapper so onChange results flow back into value, mirroring how
// the create form and Deployment tab own the selection state.
const ControlledSelector: React.FC<{
  environments: Environment[];
  gateways: GatewayResponse[];
  initialValue?: string[];
  lockedGatewayIds?: string[];
  gatewayStatuses?: Map<string, GatewayDeploymentStatus>;
  disabled?: boolean;
  onChangeSpy?: (ids: string[]) => void;
  onValidityChange?: (isValid: boolean) => void;
  onSingleChoiceChange?: (isSingleChoice: boolean) => void;
}> = ({
  environments,
  gateways,
  initialValue = [],
  lockedGatewayIds,
  gatewayStatuses,
  disabled,
  onChangeSpy,
  onValidityChange,
  onSingleChoiceChange,
}) => {
  const [value, setValue] = useState<string[]>(initialValue);
  return (
    <EnvironmentGatewaySelectorView
      environments={environments}
      gateways={gateways}
      value={value}
      onChange={(ids) => {
        setValue(ids);
        onChangeSpy?.(ids);
      }}
      lockedGatewayIds={lockedGatewayIds}
      gatewayStatuses={gatewayStatuses}
      onValidityChange={onValidityChange}
      onSingleChoiceChange={onSingleChoiceChange}
      disabled={disabled}
    />
  );
};

// `hidden: true` throughout: while an MUI Select menu is open (or still
// unmounting), the Modal marks the rest of the app aria-hidden, which would
// otherwise make every row invisible to role queries.
const getCheckbox = (name: string) =>
  screen.getByRole("checkbox", { name, hidden: true }) as HTMLInputElement;

const chooseOption = (selectName: string, optionName: string) => {
  fireEvent.mouseDown(
    screen.getByRole("combobox", { name: selectName, hidden: true }),
  );
  const listboxes = screen.getAllByRole("listbox", { hidden: true });
  const listbox = listboxes[listboxes.length - 1];
  fireEvent.click(
    within(listbox).getByRole("option", { name: optionName, hidden: true }),
  );
};

describe("EnvironmentGatewaySelectorView", () => {
  it("auto-selects and renders nothing when there's only one environment with a single candidate gateway", () => {
    const onChangeSpy = vi.fn();
    const onSingleChoiceChange = vi.fn();
    const { container } = renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[makeGateway("gw-1", "env-a")]}
        onChangeSpy={onChangeSpy}
        onSingleChoiceChange={onSingleChoiceChange}
      />,
    );
    expect(onSingleChoiceChange).toHaveBeenLastCalledWith(true);
    expect(onChangeSpy).toHaveBeenCalledWith(["gw-1"]);
    expect(container).toBeEmptyDOMElement();
  });

  it("does not auto-select or hide a 1-candidate row when it's one of several environments", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[
          makeEnvironment("env-a", "Alpha"),
          makeEnvironment("env-b", "Beta"),
        ]}
        gateways={[
          makeGateway("gw-1", "env-a"),
          makeGateway("gw-2", "env-b"),
        ]}
        onChangeSpy={onChangeSpy}
      />,
    );
    expect(getCheckbox("Alpha")).not.toBeChecked();
    expect(onChangeSpy).not.toHaveBeenCalled();
    fireEvent.click(getCheckbox("Alpha"));
    expect(onChangeSpy).toHaveBeenCalledWith(["gw-1"]);
  });

  it("renders a Select for 2 candidates and reports invalid until one is chosen", () => {
    const onChangeSpy = vi.fn();
    const onValidityChange = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[
          makeGateway("gw-1", "env-a"),
          makeGateway("gw-2", "env-a"),
        ]}
        onChangeSpy={onChangeSpy}
        onValidityChange={onValidityChange}
      />,
    );
    expect(onValidityChange).toHaveBeenLastCalledWith(true);
    // The picker for an ambiguous row stays hidden until the row is checked —
    // nothing to choose between until the user opts into this environment.
    expect(
      screen.queryByRole("combobox", { hidden: true }),
    ).not.toBeInTheDocument();
    fireEvent.click(getCheckbox("Alpha"));
    expect(onChangeSpy).not.toHaveBeenCalled();
    expect(onValidityChange).toHaveBeenLastCalledWith(false);
    expect(
      screen.getByText("Select an egress gateway for this environment."),
    ).toBeInTheDocument();
    chooseOption("Egress gateway for Alpha", "Gateway gw-2");
    expect(onChangeSpy).toHaveBeenLastCalledWith(["gw-2"]);
    expect(onValidityChange).toHaveBeenLastCalledWith(true);
  });

  it("does not block validity while a 2-candidate row is unchecked and unresolved", () => {
    const onValidityChange = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[
          makeGateway("gw-1", "env-a"),
          makeGateway("gw-2", "env-a"),
        ]}
        onValidityChange={onValidityChange}
      />,
    );
    expect(onValidityChange).toHaveBeenLastCalledWith(true);
    expect(onValidityChange).not.toHaveBeenCalledWith(false);
  });

  it("disables the checkbox of a 0-candidate row and explains why", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[]}
        onChangeSpy={onChangeSpy}
      />,
    );
    expect(getCheckbox("Alpha")).toBeDisabled();
    expect(
      screen.getByText(
        "No egress-capable gateway is attached to this environment.",
      ),
    ).toBeInTheDocument();
    expect(onChangeSpy).not.toHaveBeenCalled();
  });

  it("still reports single-choice for a locked environment, but renders the deployed row instead of hiding it", () => {
    // Unlike the unlocked single-candidate case, a locked row is an existing
    // fact ("this is deployed here") rather than an unmade choice, so it must
    // stay visible even though there's nothing left to decide.
    const onSingleChoiceChange = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[
          makeGateway("gw-1", "env-a"),
          makeGateway("gw-2", "env-a"),
        ]}
        initialValue={["gw-1"]}
        lockedGatewayIds={["gw-1"]}
        onSingleChoiceChange={onSingleChoiceChange}
      />,
    );
    expect(onSingleChoiceChange).toHaveBeenLastCalledWith(true);
    expect(getCheckbox("Alpha")).toBeChecked();
    expect(screen.getByText("Gateway gw-1")).toBeInTheDocument();
    expect(screen.getByText("Deployed")).toBeInTheDocument();
    // Another candidate (gw-2) exists for this environment, so the caption
    // must offer switching to it rather than the sole-candidate wording.
    expect(
      screen.getByText(/select the new gateway and save again/),
    ).toBeInTheDocument();
  });

  it("shows a locked sole-candidate row as a fixed fact — no caption, no undeploy affordance", () => {
    // A sole candidate has nowhere to switch to, and undeploying it would
    // strand the provider: re-deploying to the same sole gateway would
    // immediately auto-hide again (see the "auto-selects and renders
    // nothing" test above), leaving no way back into this row. So unlike the
    // 2+-candidate case, this row is purely informational.
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[makeGateway("gw-1", "env-a")]}
        initialValue={["gw-1"]}
        lockedGatewayIds={["gw-1"]}
        onChangeSpy={onChangeSpy}
      />,
    );
    expect(getCheckbox("Alpha")).toBeChecked();
    expect(getCheckbox("Alpha")).toBeDisabled();
    expect(screen.getByText("Gateway gw-1")).toBeInTheDocument();
    expect(screen.getByText("Deployed")).toBeInTheDocument();
    expect(
      screen.queryByText(/Placement is fixed once deployed/),
    ).not.toBeInTheDocument();
    fireEvent.click(getCheckbox("Alpha"));
    expect(onChangeSpy).not.toHaveBeenCalled();
    expect(getCheckbox("Alpha")).toBeChecked();
  });

  it("renders a locked row checked with the deployed gateway name and a Deployed chip, and drops it when unchecked, when it's one of several environments", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[
          makeEnvironment("env-a", "Alpha"),
          makeEnvironment("env-b", "Beta"),
        ]}
        gateways={[
          makeGateway("gw-1", "env-a"),
          makeGateway("gw-2", "env-a"),
          makeGateway("gw-3", "env-b"),
        ]}
        initialValue={["gw-1"]}
        lockedGatewayIds={["gw-1"]}
        onChangeSpy={onChangeSpy}
      />,
    );
    expect(getCheckbox("Alpha")).toBeChecked();
    expect(
      screen.queryByRole("combobox", { hidden: true }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("Gateway gw-1")).toBeInTheDocument();
    expect(screen.getByText("Deployed")).toBeInTheDocument();

    fireEvent.click(getCheckbox("Alpha"));
    expect(onChangeSpy).toHaveBeenLastCalledWith([]);
    fireEvent.click(getCheckbox("Alpha"));
    expect(onChangeSpy).toHaveBeenLastCalledWith(["gw-1"]);
  });

  it("shows a locked gateway whose deploy failed as Failed, with its reason on hover, and lets it be unchecked", async () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[makeGateway("gw-1", "env-a")]}
        initialValue={["gw-1"]}
        lockedGatewayIds={["gw-1"]}
        gatewayStatuses={
          new Map([
            ["gw-1", { status: "FAILED", statusReason: "gateway unreachable" }],
          ])
        }
        onChangeSpy={onChangeSpy}
      />,
    );
    expect(screen.queryByText("Deployed")).not.toBeInTheDocument();
    fireEvent.mouseOver(screen.getByText("Failed"));
    expect(await screen.findByText("gateway unreachable")).toBeInTheDocument();

    expect(getCheckbox("Alpha")).toBeChecked();
    expect(getCheckbox("Alpha")).toBeEnabled();
    fireEvent.click(getCheckbox("Alpha"));
    expect(onChangeSpy).toHaveBeenLastCalledWith([]);
    expect(getCheckbox("Alpha")).not.toBeChecked();
  });

  it("shows a locked gateway whose undeploy failed as Undeploy failed", () => {
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[makeGateway("gw-1", "env-a")]}
        initialValue={["gw-1"]}
        lockedGatewayIds={["gw-1"]}
        gatewayStatuses={
          new Map([["gw-1", { status: "FAILED", statusDesired: "UNDEPLOYED" }]])
        }
      />,
    );
    expect(screen.getByText("Undeploy failed")).toBeInTheDocument();
    expect(screen.queryByText("Failed")).not.toBeInTheDocument();
  });

  it("shows Deploying for a selected gateway whose deploy is in progress", () => {
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[makeGateway("gw-1", "env-a")]}
        initialValue={["gw-1"]}
        gatewayStatuses={new Map([["gw-1", { status: "DEPLOYING" }]])}
      />,
    );
    expect(screen.getByText("Deploying")).toBeInTheDocument();
    expect(getCheckbox("Alpha")).toBeChecked();
  });

  it("keeps a DEPLOYED locked gateway locked with a Deployed chip", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[makeGateway("gw-1", "env-a")]}
        initialValue={["gw-1"]}
        lockedGatewayIds={["gw-1"]}
        gatewayStatuses={new Map([["gw-1", { status: "DEPLOYED" }]])}
        onChangeSpy={onChangeSpy}
      />,
    );
    expect(screen.getByText("Deployed")).toBeInTheDocument();
    expect(getCheckbox("Alpha")).toBeChecked();
    expect(getCheckbox("Alpha")).toBeDisabled();
    fireEvent.click(getCheckbox("Alpha"));
    expect(onChangeSpy).not.toHaveBeenCalled();
  });

  it("evicts the previously selected gateway when a different candidate is chosen in the same environment", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[
          makeEnvironment("env-a", "Alpha"),
          makeEnvironment("env-b", "Beta"),
        ]}
        gateways={[
          makeGateway("gw-1", "env-a"),
          makeGateway("gw-2", "env-a"),
          makeGateway("gw-3", "env-b"),
        ]}
        onChangeSpy={onChangeSpy}
      />,
    );
    fireEvent.click(getCheckbox("Alpha"));
    chooseOption("Egress gateway for Alpha", "Gateway gw-1");
    expect(onChangeSpy).toHaveBeenLastCalledWith(["gw-1"]);

    // A gateway in a different environment coexists.
    fireEvent.click(getCheckbox("Beta"));
    expect(onChangeSpy).toHaveBeenLastCalledWith(["gw-1", "gw-3"]);

    // Switching Alpha's candidate replaces gw-1 rather than joining it.
    chooseOption("Egress gateway for Alpha", "Gateway gw-2");
    expect(onChangeSpy).toHaveBeenLastCalledWith(["gw-3", "gw-2"]);
  });

  it("renders an unmapped selected gateway as a removable locked row, and reports invalid while it remains", () => {
    // Two environments, so removing the unmapped entry can't cascade into the
    // unrelated single-choice auto-select feature — this test is only about
    // the unmapped row itself.
    const onChangeSpy = vi.fn();
    const onValidityChange = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[
          makeEnvironment("env-a", "Alpha"),
          makeEnvironment("env-b", "Beta"),
        ]}
        gateways={[
          makeGateway("gw-1", "env-a"),
          makeGateway("gw-2", "env-b"),
        ]}
        initialValue={["ghost-gw"]}
        onChangeSpy={onChangeSpy}
        onValidityChange={onValidityChange}
      />,
    );
    expect(screen.getByText("Unmapped")).toBeInTheDocument();
    // A stale reference to a gateway that no longer resolves to any candidate
    // is left over work, not a valid final state — even though every real row
    // (here, none checked) is individually fine on its own.
    expect(onValidityChange).toHaveBeenLastCalledWith(false);
    const ghostCheckbox = getCheckbox("ghost-gw");
    expect(ghostCheckbox).toBeChecked();
    fireEvent.click(ghostCheckbox);
    expect(onChangeSpy).toHaveBeenLastCalledWith([]);
    expect(screen.queryByText("Unmapped")).not.toBeInTheDocument();
    expect(onValidityChange).toHaveBeenLastCalledWith(true);
  });

  it("keeps an unmapped selection visible instead of collapsing to the single-choice environment", () => {
    // Only one real environment, but an orphaned reference is still selected —
    // there's something to clean up here, so auto-hide must not kick in.
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[makeGateway("gw-1", "env-a")]}
        initialValue={["ghost-gw"]}
      />,
    );
    expect(screen.getByText("Unmapped")).toBeInTheDocument();
    expect(getCheckbox("Alpha")).not.toBeChecked();
  });

  it("shows the selection footer only when there is more than one environment", () => {
    const { unmount } = renderWithTheme(
      <ControlledSelector
        environments={[
          makeEnvironment("env-a", "Alpha"),
          makeEnvironment("env-b", "Beta"),
        ]}
        gateways={[
          makeGateway("gw-1", "env-a"),
          makeGateway("gw-2", "env-b"),
        ]}
        initialValue={["gw-1"]}
      />,
    );
    expect(
      screen.getByText("1 of 2 environments selected."),
    ).toBeInTheDocument();
    unmount();

    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[makeGateway("gw-1", "env-a")]}
        initialValue={["gw-1"]}
      />,
    );
    expect(screen.queryByText(/environments selected/)).not.toBeInTheDocument();
  });
});

// Deterministic LCG so any failure reproduces from its seed.
const makeLcg = (seed: number) => {
  let state = seed >>> 0;
  return () => {
    state = (state * 1664525 + 1013904223) >>> 0;
    return state / 2 ** 32;
  };
};

interface Fixture {
  environments: Environment[];
  gateways: GatewayResponse[];
  lockedGatewayIds: string[];
  initialValue: string[];
  envIdByUuid: Record<string, string | undefined>;
}

const generateFixture = (rand: () => number): Fixture => {
  const envCount = 2 + Math.floor(rand() * 4);
  const environments = Array.from({ length: envCount }, (_, i) =>
    makeEnvironment(`env-${i}`, `Env ${i}`),
  );
  const gatewayCount = 2 + Math.floor(rand() * 5);
  const gateways = Array.from({ length: gatewayCount }, (_, i) =>
    makeGateway(
      `gw-${i}`,
      environments[Math.floor(rand() * envCount)].id as string,
    ),
  );

  const envIdByUuid: Record<string, string | undefined> = {};
  gateways.forEach((gateway) => {
    envIdByUuid[gateway.uuid] = gateway.environments?.[0]?.id;
  });

  const lockedGatewayIds: string[] = [];
  const lockedEnvIds = new Set<string>();
  gateways.forEach((gateway) => {
    if (rand() >= 0.35) return;
    const envId = envIdByUuid[gateway.uuid];
    if (!envId || lockedEnvIds.has(envId)) return;
    lockedGatewayIds.push(gateway.uuid);
    lockedEnvIds.add(envId);
  });

  const initialValue = [...lockedGatewayIds];
  if (rand() < 0.3) {
    initialValue.push("ghost-gw");
    envIdByUuid["ghost-gw"] = undefined;
  }

  return { environments, gateways, lockedGatewayIds, initialValue, envIdByUuid };
};

const performRandomAction = (rand: () => number) => {
  const checkboxes = screen
    .queryAllByRole("checkbox", { hidden: true })
    .filter((checkbox) => !(checkbox as HTMLInputElement).disabled);
  const combos = screen
    .queryAllByRole("combobox", { hidden: true })
    .filter((combo) => combo.getAttribute("aria-disabled") !== "true");
  const total = checkboxes.length + combos.length;
  if (total === 0) return;
  const pick = Math.floor(rand() * total);
  if (pick < checkboxes.length) {
    fireEvent.click(checkboxes[pick]);
    return;
  }
  fireEvent.mouseDown(combos[pick - checkboxes.length]);
  const listboxes = screen.getAllByRole("listbox", { hidden: true });
  const options = within(listboxes[listboxes.length - 1])
    .getAllByRole("option", { hidden: true })
    .filter((option) => option.getAttribute("aria-disabled") !== "true");
  if (options.length > 0) {
    fireEvent.click(options[Math.floor(rand() * options.length)]);
  }
};

describe("EnvironmentGatewaySelectorView placement invariant", () => {
  it.each([7, 42, 1337, 20260807])(
    "never emits two gateways in the same environment (seed %i)",
    (seed) => {
      const rand = makeLcg(seed);
      const fixture = generateFixture(rand);
      const emissions: string[][] = [];
      renderWithTheme(
        <ControlledSelector
          environments={fixture.environments}
          gateways={fixture.gateways}
          initialValue={fixture.initialValue}
          lockedGatewayIds={fixture.lockedGatewayIds}
          onChangeSpy={(ids) => emissions.push(ids)}
        />,
      );

      let verified = 0;
      const verifyNewEmissions = () => {
        for (; verified < emissions.length; verified += 1) {
          const emitted = emissions[verified];
          const coveredEnvIds = new Set<string>();
          emitted.forEach((uuid) => {
            const envId = fixture.envIdByUuid[uuid];
            if (!envId) return;
            if (coveredEnvIds.has(envId)) {
              throw new Error(
                `seed ${seed}, emission ${verified}: env ${envId} covered ` +
                  `twice in [${emitted.join(", ")}]`,
              );
            }
            coveredEnvIds.add(envId);
          });
        }
      };

      for (let step = 0; step < 25; step += 1) {
        performRandomAction(rand);
        verifyNewEmissions();
      }
      expect(emissions.length).toBeGreaterThan(0);
    },
  );
});
