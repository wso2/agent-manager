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
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

type MutateOptions = { onSuccess?: (data: unknown) => void; onError?: (e: unknown) => void };

const hoisted = vi.hoisted(() => ({
  canCreate: true,
  create: { mutate: vi.fn(), isPending: false, error: null as unknown },
  generateName: vi.fn(),
  generating: false,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useCreateAgent: () => hoisted.create,
  useGenerateResourceName: () => ({ mutate: hoisted.generateName, isPending: hoisted.generating }),
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureAccess: () => hoisted.canCreate,
}));
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
  useAgentPickerBack: () => ({
    href: "/org/acme/project/shop/newAgent",
    label: "Back to add new agent",
  }),
  MarkdownEditor: ({
    id,
    value,
    onChange,
    placeholder,
  }: {
    id: string;
    value: string;
    onChange: (v: string) => void;
    placeholder?: string;
  }) => (
    <textarea
      id={id}
      value={value}
      placeholder={placeholder}
      onChange={(e) => onChange(e.target.value)}
    />
  ),
  LabelsEditor: ({
    onChange,
  }: {
    onChange: (labels: Record<string, string>) => void;
  }): ReactNode => (
    <button type="button" onClick={() => onChange({ team: "support" })}>
      add team label
    </button>
  ),
}));

import { ExternalAgentPage } from "./ExternalAgentPage";

const Destination = () => {
  const location = useLocation();
  return <div>{`at ${location.pathname}${location.search}`}</div>;
};

const renderPage = (state?: unknown) =>
  render(
    <MemoryRouter
      initialEntries={[{ pathname: "/org/acme/project/shop/newAgent/connect", state }]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route
          path="/org/:orgId/project/:projectId/newAgent/connect"
          element={<ExternalAgentPage />}
        />
        <Route path="*" element={<Destination />} />
      </Routes>
    </MemoryRouter>,
  );

const nameInput = () => screen.getByPlaceholderText("e.g., Customer Support");
const handleInput = () => screen.getByPlaceholderText("e.g., customer-support-agent");
const registerButton = () => screen.getByRole("button", { name: "Register" });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.canCreate = true;
  hoisted.generating = false;
  hoisted.create = { mutate: vi.fn(), isPending: false, error: null };
  hoisted.generateName.mockImplementation((body: { displayName: string }, opts: MutateOptions) =>
    opts.onSuccess?.({ name: body.displayName.toLowerCase().replace(/\s+/g, "-") }),
  );
});

describe("ExternalAgentPage", () => {
  it("sends users who cannot create agents back to the project", () => {
    hoisted.canCreate = false;
    renderPage();

    expect(screen.getByText("at /org/acme/project/shop")).toBeInTheDocument();
  });

  it("shows the register form with Register disabled until a handle exists", () => {
    renderPage();

    expect(screen.getByText("Register an Externally-Hosted Agent")).toBeInTheDocument();
    expect(screen.getByText("Back to add new agent")).toBeInTheDocument();
    expect(registerButton()).toBeDisabled();
  });

  it("generates the agent handle from the name in this project", async () => {
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "Support Bot" } });

    await waitFor(() => expect(handleInput()).toHaveValue("support-bot"));
    expect(hoisted.generateName).toHaveBeenCalledWith(
      { displayName: "Support Bot", resourceType: "agent", projectName: "shop" },
      expect.anything(),
    );
    expect(registerButton()).toBeEnabled();
  });

  it("flags a name that is too short and generates no handle", async () => {
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "ab" } });

    expect(await screen.findByText("Name must be at least 3 characters")).toBeInTheDocument();
    await new Promise((r) => setTimeout(r, 600));
    expect(hoisted.generateName).not.toHaveBeenCalled();
  });

  it("registers an external agent with its details and opens its setup", async () => {
    hoisted.create.mutate.mockImplementation((_body: unknown, opts: MutateOptions) =>
      opts.onSuccess?.({}),
    );
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "Support Bot" } });
    fireEvent.change(screen.getByPlaceholderText(/Short description/), {
      target: { value: "Answers tickets" },
    });
    fireEvent.click(screen.getByRole("button", { name: "add team label" }));
    await waitFor(() => expect(handleInput()).toHaveValue("support-bot"));
    fireEvent.click(registerButton());

    expect(hoisted.create.mutate).toHaveBeenCalledWith(
      {
        params: { orgName: "acme", projName: "shop" },
        body: {
          name: "support-bot",
          displayName: "Support Bot",
          description: "Answers tickets",
          labels: { team: "support" },
          provisioning: { type: "external" },
          agentType: { type: "external-agent-api" },
        },
      },
      expect.anything(),
    );
    expect(
      screen.getByText("at /org/acme/project/shop/agents/support-bot?setup=true"),
    ).toBeInTheDocument();
  });

  it("stays on the form when registering fails", async () => {
    hoisted.create.mutate.mockImplementation((_body: unknown, opts: MutateOptions) =>
      opts.onError?.(new Error("conflict")),
    );
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "Support Bot" } });
    await waitFor(() => expect(handleInput()).toHaveValue("support-bot"));
    fireEvent.click(registerButton());

    expect(screen.getByText("Register an Externally-Hosted Agent")).toBeInTheDocument();
    expect(consoleError).toHaveBeenCalledWith("Failed to register agent:", expect.any(Error));
    consoleError.mockRestore();
  });

  it("rejects an invalid custom handle", async () => {
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "Support Bot" } });
    await waitFor(() => expect(handleInput()).toHaveValue("support-bot"));
    fireEvent.click(screen.getByRole("button", { name: "Edit agent handle" }));
    fireEvent.change(handleInput(), { target: { value: "Bad_Handle" } });

    const message =
      'Name must be lowercase alphanumeric or "-", start with a letter and end with a letter or digit';
    expect(screen.getByText(message)).toBeInTheDocument();

    fireEvent.click(registerButton());

    expect(hoisted.create.mutate).not.toHaveBeenCalled();
    expect(screen.getAllByText(message)).toHaveLength(2);
  });

  it("shows the server's error message", () => {
    hoisted.create.error = new Error("Agent already exists");
    renderPage();

    expect(screen.getByText("Agent already exists")).toBeInTheDocument();
  });

  it("shows a generic message for a non-Error failure", () => {
    hoisted.create.error = { status: 500 };
    renderPage();

    expect(screen.getByText("Failed to register agent")).toBeInTheDocument();
  });

  it("shows that the name is being validated", () => {
    hoisted.generating = true;
    renderPage();

    expect(screen.getByText("Validating name...")).toBeInTheDocument();
  });

  it("returns to the page that opened the wizard on Cancel", () => {
    renderPage({ from: "/org/acme/project/shop/agents" });

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.getByText("at /org/acme/project/shop/agents")).toBeInTheDocument();
  });
});
