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

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

type MutateOptions = { onSuccess?: (data: unknown) => void; onError?: (e: unknown) => void };

const hoisted = vi.hoisted(() => ({
  canWrite: true,
  create: {
    mutate: vi.fn(),
    isPending: false,
    error: null as unknown,
  },
  generateName: vi.fn(),
  generating: false,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useCreateProject: () => hoisted.create,
  useGenerateResourceName: () => ({ mutate: hoisted.generateName, isPending: hoisted.generating }),
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canWrite: hoisted.canWrite }),
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));

import { AddNewProject } from "./AddNewProject";

const Destination = () => <div>{`at ${useLocation().pathname}`}</div>;

const renderPage = (state?: unknown) =>
  render(
    <MemoryRouter
      initialEntries={[{ pathname: "/org/acme/newProject", state }]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route path="/org/:orgId/newProject" element={<AddNewProject />} />
        <Route path="*" element={<Destination />} />
      </Routes>
    </MemoryRouter>,
  );

const nameInput = () => screen.getByPlaceholderText("e.g. Customer Support Platform");
const idInput = () => screen.getByPlaceholderText("e.g., customer-support-platform");
const descriptionInput = () => screen.getByPlaceholderText("What is this project for?");
const createButton = () => screen.getByRole("button", { name: "Create" });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.canWrite = true;
  hoisted.generating = false;
  hoisted.create = { mutate: vi.fn(), isPending: false, error: null };
  hoisted.generateName.mockImplementation((body: { displayName: string }, opts: MutateOptions) =>
    opts.onSuccess?.({ name: body.displayName.toLowerCase().replace(/\s+/g, "-") }),
  );
});

describe("AddNewProject", () => {
  it("sends users without project write access back to the project list", () => {
    hoisted.canWrite = false;
    renderPage();

    expect(screen.getByText("at /org/acme/projects")).toBeInTheDocument();
  });

  it("keeps Create disabled until a project ID is generated", () => {
    renderPage();

    expect(screen.getByText("Create a Project")).toBeInTheDocument();
    expect(createButton()).toBeDisabled();
    expect(idInput()).toBeDisabled();
  });

  it("generates the project ID from the display name", async () => {
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "Customer Support" } });

    await waitFor(() => expect(idInput()).toHaveValue("customer-support"));
    expect(hoisted.generateName).toHaveBeenCalledWith(
      { displayName: "Customer Support", resourceType: "project" },
      expect.anything(),
    );
    expect(createButton()).toBeEnabled();
  });

  it("does not ask for a name until the display name has three characters", async () => {
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "ab" } });

    expect(
      await screen.findByText("Display name must be at least 3 characters"),
    ).toBeInTheDocument();
    await new Promise((r) => setTimeout(r, 600));
    expect(hoisted.generateName).not.toHaveBeenCalled();
    expect(idInput()).toHaveValue("");
  });

  it("creates the project with the entered values and opens it", async () => {
    hoisted.create.mutate.mockImplementation((_body: unknown, opts: MutateOptions) =>
      opts.onSuccess?.({}),
    );
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "Customer Support" } });
    fireEvent.change(descriptionInput(), { target: { value: "  Help desk agents  " } });
    await waitFor(() => expect(idInput()).toHaveValue("customer-support"));
    fireEvent.click(createButton());

    expect(hoisted.create.mutate).toHaveBeenCalledWith(
      {
        name: "customer-support",
        displayName: "Customer Support",
        description: "Help desk agents",
      },
      expect.anything(),
    );
    expect(screen.getByText("at /org/acme/project/customer-support")).toBeInTheDocument();
  });

  it("omits a blank description", async () => {
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "Shop" } });
    await waitFor(() => expect(idInput()).toHaveValue("shop"));
    fireEvent.click(createButton());

    expect(hoisted.create.mutate).toHaveBeenCalledWith(
      { name: "shop", displayName: "Shop", description: undefined },
      expect.anything(),
    );
  });

  it("lets the user edit the project ID and rejects an invalid one", async () => {
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "Shop" } });
    await waitFor(() => expect(idInput()).toHaveValue("shop"));
    fireEvent.click(screen.getByRole("button", { name: "Edit project ID" }));
    fireEvent.change(idInput(), { target: { value: "Bad_Name" } });

    const message =
      'Name must be lowercase alphanumeric or "-", start with a letter and end with a letter or digit';
    expect(screen.getByText(message)).toBeInTheDocument();

    fireEvent.click(createButton());

    expect(hoisted.create.mutate).not.toHaveBeenCalled();
    expect(screen.getAllByText(message)).toHaveLength(2);
  });

  it("shows the server's error when creating fails", () => {
    hoisted.create.error = new Error("Project already exists");
    renderPage();

    expect(screen.getByText("Project already exists")).toBeInTheDocument();
  });

  it("shows a generic error for a non-Error failure", () => {
    hoisted.create.error = "boom";
    renderPage();

    expect(screen.getByText("Failed to create project")).toBeInTheDocument();
  });

  it("shows that the name is being validated while the ID is generated", () => {
    hoisted.generating = true;
    renderPage();

    expect(screen.getByText("Validating name...")).toBeInTheDocument();
  });

  it("returns to the opening page on Cancel", () => {
    renderPage({ from: "/org/acme/project/shop" });

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.getByText("at /org/acme/project/shop")).toBeInTheDocument();
  });

  it("returns to the project list on Cancel after a direct visit", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.getByText("at /org/acme/projects")).toBeInTheDocument();
  });
});
