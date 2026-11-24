/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { type ReactNode } from "react";
import { fireEvent, render, renderHook, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { FeatureManagementProvider } from "@agent-management-platform/feature-management";

let extraContent: ExternalComponentModule[] = [];
vi.mock("@agent-management-platform/views", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@agent-management-platform/views")>()),
  useExternalComponentModulesByMountPoint: (mount: string) =>
    mount === "add-new-agent-options" ? extraContent : [],
}));

import {
  AgentTypesProvider,
  type AgentTypeOption,
  type ExternalComponentModule,
} from "@agent-management-platform/views";
import { connectAgentSchema } from "./schema";
import { RequireAgentCreateAccess } from "./RequireAgentCreateAccess";
import { useAgentWizardReturn } from "./useAgentWizardReturn";
import { useAgentPickerBack, useAgentTypePicker } from "./useAgentTypePicker";
import { CreateButtons } from "./components/CreateButtons";
import { AgentHandleField } from "./components/AgentHandleField";
import { buildAgentCreationPayload } from "./buildAgentPayload";

const agentType = (key: string) => ({ key, pluginId: "p" }) as AgentTypeOption;

type Entry = string | { pathname: string; state: unknown };

function wrapperFor(
  entry: Entry,
  state: { agentTypes: AgentTypeOption[]; isPending: boolean } = {
    agentTypes: [],
    isPending: false,
  },
) {
  return ({ children }: { children: ReactNode }) => (
    <MemoryRouter initialEntries={[entry]}>
      <AgentTypesProvider value={state}>
        <Routes>
          <Route path="/org/:orgId/project/:projectId/*" element={children} />
          <Route path="/org/:orgId/*" element={children} />
        </Routes>
      </AgentTypesProvider>
    </MemoryRouter>
  );
}

describe("connectAgentSchema", () => {
  const valid = { displayName: "Support bot", name: "support-bot" };
  const firstError = (over: Record<string, unknown>) => {
    const res = connectAgentSchema.safeParse({ ...valid, ...over });
    return res.success ? undefined : res.error.issues[res.error.issues.length - 1].message;
  };

  it.each([
    [{}, undefined],
    [{ name: "" }, undefined],
    [{ name: "a" }, undefined],
    [{ labels: { env: "prod" }, description: "d" }, undefined],
    [{ displayName: "ab" }, "Name must be at least 3 characters"],
    [{ displayName: "x".repeat(101) }, "Name must be at most 100 characters"],
    [{ name: "x".repeat(26) }, "Name must be at most 25 characters"],
    [{ name: "Bad_Name" }, expect.stringContaining("lowercase alphanumeric")],
    [{ name: "ends-" }, expect.stringContaining("lowercase alphanumeric")],
  ])("%j -> %j", (over, message) => expect(firstError(over)).toEqual(message));
});

describe("buildAgentCreationPayload labels", () => {
  const params = { orgName: "o", projName: "p" } as never;
  it("sends labels only when there are some", () => {
    const withLabels = buildAgentCreationPayload(
      { name: "a", displayName: "A", labels: { env: "prod" } },
      params,
    );
    expect(withLabels.body.labels).toEqual({ env: "prod" });
    const without = buildAgentCreationPayload({ name: "a", displayName: "A", labels: {} }, params);
    expect("labels" in without.body).toBe(false);
  });
});

describe("RequireAgentCreateAccess", () => {
  const renderAt = (canCreate: boolean, path: string) =>
    render(
      <FeatureManagementProvider featureAccess={{ agent_create: canCreate }}>
        <MemoryRouter initialEntries={[path]}>
          <Routes>
            <Route
              path="/org/:orgId/project/:projectId/newAgent/connect"
              element={
                <RequireAgentCreateAccess>
                  <span>create form</span>
                </RequireAgentCreateAccess>
              }
            />
            <Route
              path="/org/:orgId/newAgent"
              element={
                <RequireAgentCreateAccess>
                  <span>create form</span>
                </RequireAgentCreateAccess>
              }
            />
            <Route path="/org/:orgId/project/:projectId" element={<span>project home</span>} />
          </Routes>
        </MemoryRouter>
      </FeatureManagementProvider>,
    );

  it("renders the form for users who may create agents", () => {
    renderAt(true, "/org/acme/project/p1/newAgent/connect");
    expect(screen.getByText("create form")).toBeInTheDocument();
  });

  it("sends everyone else back to the project", () => {
    renderAt(false, "/org/acme/project/p1/newAgent/connect");
    expect(screen.queryByText("create form")).not.toBeInTheDocument();
    expect(screen.getByText("project home")).toBeInTheDocument();
  });

  it("falls back to the default project", () => {
    renderAt(false, "/org/acme/newAgent");
    expect(screen.getByText("project home")).toBeInTheDocument();
  });
});

describe("useAgentWizardReturn", () => {
  it("returns to the agent listing by default", () => {
    const { result } = renderHook(() => useAgentWizardReturn(), {
      wrapper: wrapperFor("/org/acme/project/p1/newAgent"),
    });
    expect(result.current.returnTo).toBe("/org/acme/project/p1/agents");
    expect(result.current.wizardState).toEqual({ from: "/org/acme/project/p1/agents" });
  });

  it("returns to the page that opened the wizard", () => {
    const { result } = renderHook(() => useAgentWizardReturn(), {
      wrapper: wrapperFor({ pathname: "/org/acme/project/p1/newAgent", state: { from: "/org/acme/home" } }),
    });
    expect(result.current.returnTo).toBe("/org/acme/home");
  });
});

describe("useAgentTypePicker / useAgentPickerBack", () => {
  beforeEach(() => {
    extraContent = [];
  });

  const run = (state: { agentTypes: AgentTypeOption[]; isPending: boolean }) =>
    renderHook(() => ({ picker: useAgentTypePicker(), back: useAgentPickerBack() }), {
      wrapper: wrapperFor("/org/acme/project/p1/newAgent/connect", state),
    }).result.current;

  it("skips the picker when there is a single type and nothing else", () => {
    const { picker, back } = run({ agentTypes: [agentType("ext")], isPending: false });
    expect(picker.soleAgentType?.key).toBe("ext");
    expect(back).toEqual({ href: "/org/acme/project/p1/agents", label: "Back" });
  });

  it("leaves the wizard while types are still settling", () => {
    const { picker, back } = run({ agentTypes: [agentType("ext")], isPending: true });
    expect(picker.soleAgentType).toBeUndefined();
    expect(back.label).toBe("Back");
  });

  it("goes back to the picker when it offers a choice", () => {
    const { picker, back } = run({ agentTypes: [agentType("a"), agentType("b")], isPending: false });
    expect(picker.soleAgentType).toBeUndefined();
    expect(back).toEqual({ href: "/org/acme/project/p1/newAgent", label: "Back to add new agent" });
  });

  it("treats module content as a second choice", () => {
    extraContent = [{ moduleName: "extra" } as ExternalComponentModule];
    const { picker, back } = run({ agentTypes: [agentType("a")], isPending: false });
    expect(picker.extraContent).toHaveLength(1);
    expect(picker.soleAgentType).toBeUndefined();
    expect(back.label).toBe("Back to add new agent");
  });
});

describe("CreateButtons", () => {
  it("lists submitted errors and wires cancel and register", () => {
    const onCancel = vi.fn();
    const onSubmit = vi.fn();
    render(
      <CreateButtons
        lastSubmittedValidationErrors={{ name: "Name is required", description: undefined, x: "Too long" }}
        isPending={false}
        onCancel={onCancel}
        onSubmit={onSubmit}
      />,
    );
    expect(screen.getByText("Name is required")).toBeInTheDocument();
    expect(screen.getByText("Too long")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.click(screen.getByRole("button", { name: "Register" }));
    expect(onCancel).toHaveBeenCalled();
    expect(onSubmit).toHaveBeenCalled();
  });

  it.each([
    [{ isPending: true }],
    [{ isPending: false, isNameEmpty: true }],
  ])("disables register for %j", (props) => {
    render(
      <CreateButtons
        lastSubmittedValidationErrors={{}}
        onCancel={vi.fn()}
        onSubmit={vi.fn()}
        {...props}
      />,
    );
    expect(screen.getByRole("button", { name: "Register" })).toBeDisabled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

describe("AgentHandleField", () => {
  it("is read-only with an edit button until unlocked", () => {
    const onEdit = vi.fn();
    render(<AgentHandleField value="support-bot" editable={false} onEdit={onEdit} onChange={vi.fn()} />);
    const input = document.getElementById("name") as HTMLInputElement;
    expect(input).toHaveAttribute("readonly");
    expect(input).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Edit agent handle" }));
    expect(onEdit).toHaveBeenCalled();
  });

  it("is disabled while there is no generated handle yet", () => {
    render(<AgentHandleField value="" editable={false} onEdit={vi.fn()} onChange={vi.fn()} />);
    expect(document.getElementById("name")).toBeDisabled();
  });

  it("edits freely once unlocked and shows errors over the hint", () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <AgentHandleField value="bot" editable onEdit={vi.fn()} onChange={onChange} />,
    );
    expect(screen.getByText("Unique and cannot be changed later")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit agent handle" })).not.toBeInTheDocument();
    fireEvent.change(document.getElementById("name")!, { target: { value: "bot-2" } });
    expect(onChange).toHaveBeenCalledWith("bot-2");
    rerender(
      <AgentHandleField
        value="Bad"
        editable
        onEdit={vi.fn()}
        onChange={onChange}
        error="Invalid handle"
      />,
    );
    expect(screen.getByText("Invalid handle")).toBeInTheDocument();
    expect(screen.queryByText("Unique and cannot be changed later")).not.toBeInTheDocument();
  });
});
