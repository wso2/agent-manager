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

import { useEffect, type ComponentType } from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ExternalModuleProvider,
  SnackBarProvider,
  type ReplicaPolicy,
  type ReplicaPolicyProps,
} from "@agent-management-platform/views";
import type { AgentResourceConfigsResponse } from "@agent-management-platform/types";
import { EditResourceConfigsDrawer } from "./EditResourceConfigsDrawer";

const { updateConfigs } = vi.hoisted(() => ({ updateConfigs: vi.fn() }));

vi.mock("@agent-management-platform/api-client", () => ({
  useUpdateAgentResourceConfigs: () => ({ mutate: updateConfigs, isPending: false }),
}));

vi.mock("@agent-management-platform/shared-component", () => ({
  useUnsavedChangesGuard: () => ({ allowNavigation: (action: () => void) => action() }),
}));

function renderDrawer(
  Policy: ComponentType<ReplicaPolicyProps> | undefined,
  resourceConfigs?: AgentResourceConfigsResponse,
) {
  const drawer = (component: ComponentType<ReplicaPolicyProps> | undefined) => (
    <ExternalModuleProvider
      externalPageModules={
        component
          ? [
              {
                kind: "component",
                mountPoint: "replica-policy",
                moduleName: "Test replica policy",
                component: component as unknown as ComponentType<Record<string, unknown>>,
              },
            ]
          : []
      }
    >
      <EditResourceConfigsDrawer
        open
        onClose={vi.fn()}
        orgName="org"
        projName="project"
        agentName="agent"
        resourceConfigs={resourceConfigs}
      />
    </ExternalModuleProvider>
  );
  const { rerender } = render(drawer(Policy), { wrapper: SnackBarProvider });
  return (nextPolicy: ComponentType<ReplicaPolicyProps> | undefined) =>
    rerender(drawer(nextPolicy));
}

function reportingPolicy(policy: ReplicaPolicy | null) {
  return function ReportingPolicy({ onPolicyChange }: ReplicaPolicyProps) {
    useEffect(() => onPolicyChange(policy), [onPolicyChange]);
    return null;
  };
}

function saveButton() {
  return screen.getByRole("button", { name: "Save" });
}

beforeEach(() => updateConfigs.mockClear());
afterEach(() => vi.useRealTimers());

describe("EditResourceConfigsDrawer replica policy", () => {
  it("blocks invalid reports and accepts a corrected report", () => {
    let report: ReplicaPolicyProps["onPolicyChange"] = () => {};
    function Policy({ onPolicyChange }: ReplicaPolicyProps) {
      useEffect(() => {
        report = onPolicyChange;
      }, [onPolicyChange]);
      return null;
    }
    renderDrawer(Policy);
    act(() => report(null));
    expect(saveButton()).toBeEnabled();

    act(() => report({ maxReplicas: NaN, autoScalingAllowed: true }));
    expect(screen.getByText(/replica limits could not be loaded/i)).toBeInTheDocument();
    expect(saveButton()).toBeDisabled();

    act(() => report({ maxReplicas: 1, autoScalingAllowed: false }));
    expect(saveButton()).toBeEnabled();
  });

  it("waits for added and replaced providers and ignores a superseded report", () => {
    let firstReport: ReplicaPolicyProps["onPolicyChange"] = () => {};
    let secondReport: ReplicaPolicyProps["onPolicyChange"] = () => {};
    function FirstPolicy({ onPolicyChange }: ReplicaPolicyProps) {
      useEffect(() => {
        firstReport = onPolicyChange;
      }, [onPolicyChange]);
      return null;
    }
    function SecondPolicy({ onPolicyChange }: ReplicaPolicyProps) {
      useEffect(() => {
        secondReport = onPolicyChange;
      }, [onPolicyChange]);
      return null;
    }
    const changePolicy = renderDrawer(undefined);
    expect(saveButton()).toBeEnabled();

    changePolicy(FirstPolicy);
    expect(saveButton()).toBeDisabled();
    act(() => firstReport(null));
    expect(saveButton()).toBeEnabled();

    changePolicy(SecondPolicy);
    expect(saveButton()).toBeDisabled();
    act(() => firstReport(null));
    expect(saveButton()).toBeDisabled();

    act(() => secondReport({ maxReplicas: 1, autoScalingAllowed: false }));
    expect(saveButton()).toBeEnabled();
    expect(screen.getByLabelText("Enable autoscaling")).toBeDisabled();
    act(() => firstReport(null));
    expect(screen.getByLabelText("Enable autoscaling")).toBeDisabled();

    changePolicy(undefined);
    expect(saveButton()).toBeEnabled();
    expect(screen.getByLabelText("Enable autoscaling")).toBeEnabled();
  });

  it("blocks saving while the policy is pending and shows a timeout", () => {
    vi.useFakeTimers();
    renderDrawer(() => null);

    expect(saveButton()).toBeDisabled();
    expect(screen.queryByText(/replica limits could not be loaded/i)).toBeNull();

    act(() => vi.advanceTimersByTime(10_000));

    expect(screen.getByText(/replica limits could not be loaded/i)).toBeInTheDocument();
    expect(saveButton()).toBeDisabled();
  });

  it("blocks saving and submitting after a reported policy crashes", () => {
    let crash = false;
    function Policy({ onPolicyChange }: ReplicaPolicyProps) {
      useEffect(() => onPolicyChange(null), [onPolicyChange]);
      if (crash) throw new Error("Policy render failed");
      return null;
    }
    renderDrawer(Policy);
    expect(saveButton()).toBeEnabled();

    crash = true;
    const errorLog = vi.spyOn(console, "error").mockImplementation(() => {});
    try {
      // Changing a field renders the drawer and its injected policy again.
      fireEvent.change(screen.getByRole("spinbutton", { name: "Replicas" }), {
        target: { value: "2" },
      });
    } finally {
      errorLog.mockRestore();
    }

    expect(screen.getByText(/replica limits could not be loaded/i)).toBeInTheDocument();
    const save = saveButton();
    expect(save).toBeDisabled();
    fireEvent.submit((save as HTMLButtonElement).form!);
    expect(updateConfigs).not.toHaveBeenCalled();
  });

  it("blocks fixed replicas above the policy cap", () => {
    renderDrawer(reportingPolicy({ maxReplicas: 1, autoScalingAllowed: false }), { replicas: 2 });

    expect(screen.getByText("Replicas must be at most 1")).toBeInTheDocument();
    expect(saveButton()).toBeDisabled();

    fireEvent.change(screen.getByRole("spinbutton", { name: "Replicas" }), {
      target: { value: "1" },
    });
    expect(saveButton()).toBeEnabled();
  });

  it("blocks the autoscaling maximum above the policy cap", () => {
    renderDrawer(reportingPolicy({ maxReplicas: 2, autoScalingAllowed: true }), {
      autoScaling: { enabled: true, minReplicas: 1, maxReplicas: 3 },
    });

    expect(screen.getByText("Max replicas must be at most 2")).toBeInTheDocument();
    expect(saveButton()).toBeDisabled();

    fireEvent.change(screen.getByRole("spinbutton", { name: "Max replicas" }), {
      target: { value: "2" },
    });
    expect(saveButton()).toBeEnabled();
  });

  it.each(["Min replicas", "Max replicas"])(
    "ignores invalid %s after autoscaling is disabled and prevents re-enabling it",
    (field) => {
      renderDrawer(reportingPolicy({ maxReplicas: 1, autoScalingAllowed: false }), {
        replicas: 1,
        autoScaling: { enabled: true, minReplicas: 1, maxReplicas: 3 },
      });

      const autoscaling = screen.getByLabelText("Enable autoscaling");
      expect(autoscaling).toBeChecked();
      expect(autoscaling).toBeEnabled();
      fireEvent.change(screen.getByRole("spinbutton", { name: field }), {
        target: { value: "-1" },
      });
      expect(saveButton()).toBeDisabled();

      fireEvent.click(autoscaling);
      expect(autoscaling).not.toBeChecked();
      expect(autoscaling).toBeDisabled();
      expect(saveButton()).toBeEnabled();

      fireEvent.click(saveButton());
      expect(updateConfigs).toHaveBeenCalledWith(
        expect.objectContaining({
          body: expect.objectContaining({ autoScaling: { enabled: false } }),
        }),
        expect.any(Object),
      );
    },
  );
});
