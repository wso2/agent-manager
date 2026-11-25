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

const hoisted = vi.hoisted(() => ({
  access: new Set<string>(),
}));

vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureManagement: () => ({ hasAccess: (key: string) => hoisted.access.has(key) }),
}));
vi.mock("@agent-management-platform/identities", () => ({
  IdentitiesOrganization: () => <div>{`identities at ${useLocation().pathname}`}</div>,
}));

import { SettingsOrganization } from "./Settings.Organization";

const renderAt = (path: string) =>
  render(
    // No v7_relativeSplatPath: the console's BrowserRouter doesn't opt in, and
    // under it the page's relative <Navigate> would keep re-resolving.
    <MemoryRouter initialEntries={[path]} future={{ v7_startTransition: true }}>
      <Routes>
        <Route path="/org/:orgId/settings/*" element={<SettingsOrganization />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  hoisted.access = new Set(["member_read", "role_read", "group_read"]);
});

describe("SettingsOrganization", () => {
  it("opens the users page by default", () => {
    renderAt("/org/acme/settings");

    expect(screen.getByText("Settings")).toBeInTheDocument();
    expect(screen.getByText("identities at /org/acme/settings/identities/users")).toBeInTheDocument();
  });

  it("sends unknown settings paths to the users page", () => {
    renderAt("/org/acme/settings/appearance");

    expect(screen.getByText("identities at /org/acme/settings/identities/users")).toBeInTheDocument();
  });

  it("lists the identity pages the user can read and marks the open one", () => {
    renderAt("/org/acme/settings/identities/groups");

    expect(screen.getByText("User management")).toBeInTheDocument();
    const links = screen.getAllByRole("link");
    expect(links.map((l) => [l.textContent, l.getAttribute("href")])).toEqual([
      ["Users", "/org/acme/settings/identities/users"],
      ["Groups", "/org/acme/settings/identities/groups"],
      ["Roles", "/org/acme/settings/identities/roles"],
    ]);
    expect(screen.getByRole("button", { name: "Groups" })).toHaveClass("Mui-selected");
    expect(screen.getByRole("button", { name: "Users" })).not.toHaveClass("Mui-selected");

    fireEvent.click(screen.getByRole("link", { name: "Roles" }));
    expect(screen.getByText("identities at /org/acme/settings/identities/roles")).toBeInTheDocument();
  });

  it("hides identity pages the user cannot read", () => {
    hoisted.access = new Set(["role_read"]);
    renderAt("/org/acme/settings/identities/roles");

    expect(screen.getAllByRole("link").map((l) => l.textContent)).toEqual(["Roles"]);
  });

  it("shows no user management section without identity access", () => {
    hoisted.access = new Set();
    renderAt("/org/acme/settings/identities/users");

    expect(screen.queryByText("User management")).not.toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });
});
