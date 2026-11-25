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

import { fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

type Confirmation = { title: string; description: string; onConfirm: () => void };

const hoisted = vi.hoisted(() => ({
  canWrite: true,
  projects: { data: undefined as unknown, isPending: false },
  deleteProject: vi.fn(),
  isDeleting: false,
  addConfirmation: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useListProjects: () => hoisted.projects,
  useDeleteProject: () => ({ mutate: hoisted.deleteProject, isPending: hoisted.isDeleting }),
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canWrite: hoisted.canWrite }),
}));
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useConfirmationDialog: () => ({ addConfirmation: hoisted.addConfirmation }),
}));

import { ProjectListing } from "./ProjectListing";

const Destination = () => {
  const location = useLocation();
  return <div>{`at ${location.pathname} ${JSON.stringify(location.state)}`}</div>;
};

const renderPage = () =>
  render(
    <MemoryRouter
      initialEntries={["/org/acme/projects"]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route path="/org/:orgId/projects" element={<ProjectListing />} />
        <Route path="*" element={<Destination />} />
      </Routes>
    </MemoryRouter>,
  );

const project = (name: string, displayName: string, description = "") => ({
  name,
  displayName,
  description,
  createdAt: "2026-01-01T00:00:00Z",
});

const twoProjects = {
  data: {
    projects: [
      project("shop", "Shop", "Storefront agents"),
      project("support", "Support Desk", "Ticket agents"),
    ],
  },
  isPending: false,
};

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.canWrite = true;
  hoisted.isDeleting = false;
  hoisted.projects = twoProjects;
});

describe("ProjectListing", () => {
  it("shows a card for each project", () => {
    renderPage();

    expect(screen.getByText("All Projects")).toBeInTheDocument();
    expect(screen.getByText("Shop")).toBeInTheDocument();
    expect(screen.getByText("Storefront agents")).toBeInTheDocument();
    expect(screen.getByText("Support Desk")).toBeInTheDocument();
  });

  it("opens a project when its card is clicked", () => {
    renderPage();

    fireEvent.click(screen.getByText("Support Desk"));

    expect(screen.getByText(/^at \/org\/acme\/project\/support/)).toBeInTheDocument();
  });

  it("links Add project to the create page with a way back", () => {
    renderPage();

    fireEvent.click(screen.getByRole("link", { name: "Add project" }));

    expect(
      screen.getByText('at /org/acme/newProject {"from":"/org/acme/projects"}'),
    ).toBeInTheDocument();
  });

  it("filters projects by display name", () => {
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Search projects"), { target: { value: "supp" } });

    expect(screen.queryByText("Shop")).not.toBeInTheDocument();
    expect(screen.getByText("Support Desk")).toBeInTheDocument();
  });

  it("says when no project matches the search", () => {
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Search projects"), { target: { value: "zzz" } });

    expect(screen.getByText("No projects match your search")).toBeInTheDocument();
  });

  it("asks before deleting a project and deletes it on confirm", () => {
    renderPage();

    fireEvent.click(screen.getAllByRole("button", { name: "Delete project" })[0]);

    const confirmation = hoisted.addConfirmation.mock.calls[0][0] as Confirmation;
    expect(confirmation.title).toBe("Delete Project?");
    expect(confirmation.description).toContain('"Shop"');
    expect(hoisted.deleteProject).not.toHaveBeenCalled();
    expect(screen.getByText("Shop")).toBeInTheDocument();

    confirmation.onConfirm();

    expect(hoisted.deleteProject).toHaveBeenCalledWith({ orgName: "acme", projName: "shop" });
  });

  it("offers no create or delete without project write access", () => {
    hoisted.canWrite = false;
    renderPage();

    expect(screen.getByText("Shop")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Add project" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete project" })).not.toBeInTheDocument();
  });

  it("shows an empty state with Add project when there are no projects", () => {
    hoisted.projects = { data: { projects: [] }, isPending: false };
    renderPage();

    expect(screen.getByText("No projects yet")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add project" })).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search projects")).not.toBeInTheDocument();
  });

  it("shows no create action in the empty state without write access", () => {
    hoisted.canWrite = false;
    hoisted.projects = { data: { projects: [] }, isPending: false };
    renderPage();

    const emptyState = screen.getByText("No projects yet").parentElement as HTMLElement;
    expect(within(emptyState).queryByRole("link", { name: "Add project" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Add project" })).not.toBeInTheDocument();
  });

  it("shows placeholders instead of cards while loading", () => {
    hoisted.projects = { data: undefined, isPending: true };
    renderPage();

    expect(screen.queryByText("No projects yet")).not.toBeInTheDocument();
    expect(screen.queryByText("Shop")).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Add project" })).not.toBeInTheDocument();
  });

  it("hides the cards while a project is being deleted", () => {
    hoisted.isDeleting = true;
    renderPage();

    expect(screen.queryByText("Shop")).not.toBeInTheDocument();
  });
});
