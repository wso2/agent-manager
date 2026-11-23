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

import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

type MutateOptions = { onSuccess?: () => void };

const hoisted = vi.hoisted(() => ({
  canWrite: true,
  agent: {
    data: undefined as unknown,
    isLoading: false,
    error: null as unknown,
    refetch: vi.fn(),
  },
  update: { mutate: vi.fn(), isPending: false },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: () => hoisted.agent,
  useUpdateAgent: () => hoisted.update,
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canWrite: hoisted.canWrite }),
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
  MarkdownEditor: ({
    value,
    onChange,
    placeholder,
  }: {
    value: string;
    onChange: (v: string) => void;
    placeholder?: string;
  }) => (
    <textarea value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} />
  ),
  LabelsEditor: ({
    value,
    onChange,
  }: {
    value: Record<string, string>;
    onChange: (labels: Record<string, string>) => void;
  }) => (
    <div>
      <span>{`labels ${JSON.stringify(value)}`}</span>
      <button type="button" onClick={() => onChange({})}>
        clear labels
      </button>
    </div>
  ),
}));

import { EditAgent } from "./EditAgent";

const Destination = () => <div>{`at ${useLocation().pathname}`}</div>;

const renderPage = () =>
  render(
    <MemoryRouter
      initialEntries={["/org/acme/project/shop/agents/support-bot/edit"]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route path="/org/:orgId/project/:projectId/agents/:agentId/edit" element={<EditAgent />} />
        <Route path="*" element={<Destination />} />
      </Routes>
    </MemoryRouter>,
  );

const nameInput = () => screen.getByPlaceholderText("e.g., Customer Support Agent");
const descriptionInput = () => screen.getByPlaceholderText(/Short description/);
const updateButton = () => screen.getByRole("button", { name: "Update agent" });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.canWrite = true;
  hoisted.agent = {
    data: {
      name: "support-bot",
      displayName: "Support Bot",
      description: "Answers tickets",
      labels: { env: "prod" },
    },
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  };
  hoisted.update = { mutate: vi.fn(), isPending: false };
});

// Renders the real page layout and form inputs, which is slow under load.
describe("EditAgent", { timeout: 30000 }, () => {
  it("sends users without agent write access back to the agent", () => {
    hoisted.canWrite = false;
    renderPage();

    expect(screen.getByText("at /org/acme/project/shop/agents/support-bot")).toBeInTheDocument();
  });

  it("fills the form with the agent's current values and labels", () => {
    renderPage();

    expect(screen.getByText("Edit Agent")).toBeInTheDocument();
    expect(nameInput()).toHaveValue("Support Bot");
    expect(descriptionInput()).toHaveValue("Answers tickets");
    expect(screen.getByText('labels {"env":"prod"}')).toBeInTheDocument();
  });

  it("saves the edits, sending cleared labels as an empty set, and returns to the agent", () => {
    hoisted.update.mutate.mockImplementation((_body: unknown, opts: MutateOptions) =>
      opts.onSuccess?.(),
    );
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "Help Desk Bot" } });
    fireEvent.change(descriptionInput(), { target: { value: "Routes tickets" } });
    fireEvent.click(screen.getByRole("button", { name: "clear labels" }));
    fireEvent.click(updateButton());

    expect(hoisted.update.mutate).toHaveBeenCalledWith(
      {
        params: { orgName: "acme", projName: "shop", agentName: "support-bot" },
        body: { displayName: "Help Desk Bot", description: "Routes tickets", labels: {} },
      },
      expect.anything(),
    );
    expect(screen.getByText("at /org/acme/project/shop/agents/support-bot")).toBeInTheDocument();
  });

  it("requires a display name", () => {
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "" } });

    expect(screen.getByText("Display name is required")).toBeInTheDocument();
    expect(updateButton()).toBeDisabled();
  });

  it("shows Updating while the save is in flight", () => {
    hoisted.update.isPending = true;
    renderPage();

    expect(screen.getByRole("button", { name: "Updating..." })).toBeDisabled();
  });

  it("returns to the agent on Cancel without saving", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(hoisted.update.mutate).not.toHaveBeenCalled();
    expect(screen.getByText("at /org/acme/project/shop/agents/support-bot")).toBeInTheDocument();
  });

  it("shows the load error with retry and a way back to the agents", () => {
    hoisted.agent = {
      data: undefined,
      isLoading: false,
      error: new Error("Agent gone"),
      refetch: vi.fn(),
    };
    renderPage();

    expect(screen.getByText("Couldn't load this agent")).toBeInTheDocument();
    expect(screen.getByText("Agent gone")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(hoisted.agent.refetch).toHaveBeenCalled();

    fireEvent.click(screen.getByRole("link", { name: "Back to agents" }));
    expect(screen.getByText("at /org/acme/project/shop/agents")).toBeInTheDocument();
  });

  it("shows no form while the agent loads", () => {
    hoisted.agent = { data: undefined, isLoading: true, error: null, refetch: vi.fn() };
    renderPage();

    expect(screen.queryByPlaceholderText("e.g., Customer Support Agent")).not.toBeInTheDocument();
    expect(screen.queryByText("Couldn't load this agent")).not.toBeInTheDocument();
  });
});
