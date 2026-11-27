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
  project: {
    data: undefined as unknown,
    isLoading: false,
    error: null as unknown,
    refetch: vi.fn(),
  },
  update: { mutate: vi.fn(), isPending: false },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useGetProject: () => hoisted.project,
  useUpdateProject: () => hoisted.update,
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canWrite: hoisted.canWrite }),
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));

import { EditProject } from "./EditProject";

const Destination = () => <div>{`at ${useLocation().pathname}`}</div>;

const renderPage = () =>
  render(
    <MemoryRouter
      initialEntries={["/org/acme/project/shop/edit"]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route path="/org/:orgId/project/:projectId/edit" element={<EditProject />} />
        <Route path="*" element={<Destination />} />
      </Routes>
    </MemoryRouter>,
  );

const nameInput = () => screen.getByPlaceholderText("e.g., Customer Support Platform");
const descriptionInput = () => screen.getByPlaceholderText("What is this project for?");
const updateButton = () => screen.getByRole("button", { name: "Update project" });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.canWrite = true;
  hoisted.project = {
    data: { metadata: { name: "shop" }, spec: { displayName: "Shop", description: "Storefront agents" } },
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  };
  hoisted.update = { mutate: vi.fn(), isPending: false };
});

describe("EditProject", () => {
  it("sends users without project write access back to the project", () => {
    hoisted.canWrite = false;
    renderPage();

    expect(screen.getByText("at /org/acme/project/shop")).toBeInTheDocument();
  });

  it("fills the form with the project's current values", () => {
    renderPage();

    expect(screen.getByText("Edit Project")).toBeInTheDocument();
    expect(nameInput()).toHaveValue("Shop");
    expect(descriptionInput()).toHaveValue("Storefront agents");
  });

  it("saves the edited project and returns to it", () => {
    hoisted.update.mutate.mockImplementation((_body: unknown, opts: MutateOptions) =>
      opts.onSuccess?.(),
    );
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "Shop Floor" } });
    fireEvent.change(descriptionInput(), { target: { value: "In-store agents" } });
    fireEvent.click(updateButton());

    expect(hoisted.update.mutate).toHaveBeenCalledWith(
      { displayName: "Shop Floor", description: "In-store agents" },
      expect.anything(),
    );
    expect(screen.getByText("at /org/acme/project/shop")).toBeInTheDocument();
  });

  it("rejects a display name shorter than three characters", () => {
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "Sh" } });

    expect(screen.getByText("Display name must be at least 3 characters")).toBeInTheDocument();
    expect(updateButton()).toBeDisabled();
  });

  it("shows Updating while the save is in flight", () => {
    hoisted.update.isPending = true;
    renderPage();

    expect(screen.getByRole("button", { name: "Updating..." })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
  });

  it("returns to the project on Cancel without saving", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(hoisted.update.mutate).not.toHaveBeenCalled();
    expect(screen.getByText("at /org/acme/project/shop")).toBeInTheDocument();
  });

  it("shows the load error with retry when the project cannot be loaded", () => {
    hoisted.project = {
      data: undefined,
      isLoading: false,
      error: new Error("Not found"),
      refetch: vi.fn(),
    };
    renderPage();

    expect(screen.getByText("Couldn't load this project")).toBeInTheDocument();
    expect(screen.getByText("Not found")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(hoisted.project.refetch).toHaveBeenCalled();

    fireEvent.click(screen.getByRole("link", { name: "Back to projects" }));
    expect(screen.getByText("at /org/acme/projects")).toBeInTheDocument();
  });

  it("shows no form while the project loads", () => {
    hoisted.project = { data: undefined, isLoading: true, error: null, refetch: vi.fn() };
    renderPage();

    expect(
      screen.queryByPlaceholderText("e.g., Customer Support Platform"),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("Couldn't load this project")).not.toBeInTheDocument();
  });
});
