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

import type { ReactNode } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentTypeOption } from "@agent-management-platform/views";

const hoisted = vi.hoisted(() => ({
  track: vi.fn(),
  canCreate: true,
  picker: {
    agentTypes: [] as AgentTypeOption[],
    isPending: false,
    extraContent: [] as { moduleName: string; component: () => ReactNode }[],
    soleAgentType: undefined as AgentTypeOption | undefined,
  },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: { WizardStepViewed: "wizard_step_viewed" },
  useTrack: () => ({ track: hoisted.track }),
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  RequireAgentCreateAccess: ({ children }: { children: ReactNode }) =>
    hoisted.canCreate ? <>{children}</> : <div>no create access</div>,
  useAgentWizardReturn: () => ({ returnTo: "/org/acme/project/shop", wizardState: { from: "agents" } }),
  useAgentTypePicker: () => hoisted.picker,
}));

import { AddNewAgent } from "./AddNewAgent";

const agentType = (id: string, overrides: Partial<AgentTypeOption> = {}): AgentTypeOption => ({
  key: `mod:${id}`,
  pluginId: "mod",
  id,
  type: "agent-type",
  title: `${id} agent`,
  description: `Create a ${id} agent`,
  ctaLabel: `Start ${id}`,
  illustration: () => <svg data-testid={`${id}-illustration`} />,
  route: { path: `/org/:orgId/project/:projectId/agents/new/${id}` },
  ...overrides,
} as AgentTypeOption);

// Shows where the page sent the browser, and the state it passed along.
const Destination = () => {
  const location = useLocation();
  return <div>{`at ${location.pathname} ${JSON.stringify(location.state)}`}</div>;
};

const renderPage = (path = "/org/acme/project/shop/agents/new") =>
  render(
    <MemoryRouter
      initialEntries={[path]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route path="/org/:orgId/project/:projectId/agents/new" element={<AddNewAgent />} />
        <Route path="/org/:orgId/agents/new" element={<AddNewAgent />} />
        <Route path="*" element={<Destination />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.canCreate = true;
  hoisted.picker = { agentTypes: [], isPending: false, extraContent: [], soleAgentType: undefined };
});

describe("AddNewAgent", () => {
  it("shows one card per agent type the modules contribute", () => {
    hoisted.picker.agentTypes = [agentType("external"), agentType("a2a")];
    renderPage();

    expect(screen.getByText("Add a New Agent")).toBeInTheDocument();
    expect(screen.getByText("external agent")).toBeInTheDocument();
    expect(screen.getByText("Create a external agent")).toBeInTheDocument();
    expect(screen.getByText("Start external")).toBeInTheDocument();
    expect(screen.getByTestId("a2a-illustration")).toBeInTheDocument();
  });

  it("opens the chosen type's form in this project and records its wizard steps", () => {
    hoisted.picker.agentTypes = [
      agentType("external", { wizardSteps: ["choose-type", "register"] }),
      agentType("a2a"),
    ];
    renderPage();

    fireEvent.click(screen.getByText("external agent"));

    expect(screen.getByText('at /org/acme/project/shop/agents/new/external {"from":"agents"}')).toBeInTheDocument();
    expect(hoisted.track.mock.calls).toEqual([
      ["wizard_step_viewed", { wizard: "create-agent", step: "choose-type", step_index: 1 }],
      ["wizard_step_viewed", { wizard: "create-agent", step: "register", step_index: 2 }],
    ]);
  });

  it("uses the default project on an org-level route", () => {
    hoisted.picker.agentTypes = [agentType("external"), agentType("a2a")];
    renderPage("/org/acme/agents/new");

    fireEvent.click(screen.getByText("a2a agent"));

    expect(
      screen.getByText(/^at \/org\/acme\/project\/default\/agents\/new\/a2a /),
    ).toBeInTheDocument();
    expect(hoisted.track).toHaveBeenCalledWith("wizard_step_viewed", {
      wizard: "create-agent",
      step: "a2a",
      step_index: 1,
    });
  });

  it("goes straight to the form when there is only one type, recording its steps once", () => {
    const only = agentType("external");
    hoisted.picker.agentTypes = [only];
    hoisted.picker.soleAgentType = only;
    renderPage();

    expect(screen.getByText('at /org/acme/project/shop/agents/new/external {"from":"agents"}')).toBeInTheDocument();
    expect(hoisted.track).toHaveBeenCalledTimes(1);
  });

  it("says no agent types are available when no module offers one", () => {
    renderPage();

    expect(screen.getByText("No agent types available")).toBeInTheDocument();
  });

  it("shows neither cards nor the empty state while a type may still activate", () => {
    hoisted.picker.isPending = true;
    hoisted.picker.agentTypes = [agentType("external"), agentType("a2a")];
    renderPage();

    expect(screen.queryByText("external agent")).not.toBeInTheDocument();
    expect(screen.queryByText("No agent types available")).not.toBeInTheDocument();
  });

  it("shows the modules' panels below the cards", () => {
    hoisted.picker.agentTypes = [agentType("external"), agentType("a2a")];
    hoisted.picker.extraContent = [{ moduleName: "samples", component: () => <div>sample agents panel</div> }];
    renderPage();

    expect(screen.getByText("sample agents panel")).toBeInTheDocument();
  });

  it("shows nothing of the page without permission to create agents", () => {
    hoisted.canCreate = false;
    hoisted.picker.agentTypes = [agentType("external"), agentType("a2a")];
    renderPage();

    expect(screen.getByText("no create access")).toBeInTheDocument();
    expect(screen.queryByText("Add a New Agent")).not.toBeInTheDocument();
  });
});
