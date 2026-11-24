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

import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation, useParams } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Each page is replaced by a stub that names itself and the id it was given,
// so these tests cover only which route renders what.
const hoisted = vi.hoisted(() => ({
  access: {} as Record<string, boolean>,
  stub: (name: string) => {
    const Stub = () => {
      const params = useParams();
      const location = useLocation();
      const id = params.userId ?? params.roleId ?? params.groupId;
      return <div>{`${name}${id ? ` ${id}` : ""} @ ${location.pathname}`}</div>;
    };
    return Stub;
  },
}));

vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureAccess: (key: string) => hoisted.access[key] ?? false,
  useResourceAccess: (resource: string) => ({
    canRead: true,
    canWrite: hoisted.access[`${resource}_write`] ?? false,
  }),
}));

vi.mock("./UsersPage", () => ({ UsersPage: hoisted.stub("UsersPage") }));
vi.mock("./UserInvitePage", () => ({ UserInvitePage: hoisted.stub("UserInvitePage") }));
vi.mock("./UserAddPage", () => ({ UserAddPage: hoisted.stub("UserAddPage") }));
vi.mock("./UserEditPage", () => ({ UserEditPage: hoisted.stub("UserEditPage") }));
vi.mock("./RolesPage", () => ({ RolesPage: hoisted.stub("RolesPage") }));
vi.mock("./RoleCreatePage", () => ({ RoleCreatePage: hoisted.stub("RoleCreatePage") }));
vi.mock("./RoleEditPage", () => ({ RoleEditPage: hoisted.stub("RoleEditPage") }));
vi.mock("./GroupsPage", () => ({ GroupsPage: hoisted.stub("GroupsPage") }));
vi.mock("./GroupCreatePage", () => ({ GroupCreatePage: hoisted.stub("GroupCreatePage") }));
vi.mock("./GroupEditPage", () => ({ GroupEditPage: hoisted.stub("GroupEditPage") }));

import { IdentitiesOrganization } from "./Identities.Organization";
import { metaData } from "./index";

const BASE = "/org/acme/settings/identities";

const renderAt = (path: string) =>
  render(
    <MemoryRouter
      initialEntries={[path]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route path="/org/:orgId/settings/identities/*" element={<IdentitiesOrganization />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  hoisted.access = {};
});

describe("IdentitiesOrganization", () => {
  it("registers the page at /identities for the organization level", () => {
    expect(metaData.path).toBe("/identities");
    expect(metaData.levels?.organization).toBe(IdentitiesOrganization);
  });

  it("redirects the index to the users list", () => {
    renderAt(BASE);
    expect(screen.getByText(`UsersPage @ ${BASE}/users`)).toBeInTheDocument();
  });

  it("redirects unknown paths to the users list", () => {
    renderAt(`${BASE}/nope`);
    expect(screen.getByText(`UsersPage @ ${BASE}/users`)).toBeInTheDocument();
  });

  it.each([
    ["users", "UsersPage"],
    ["roles", "RolesPage"],
    ["groups", "GroupsPage"],
  ])("renders the %s list", (list, page) => {
    renderAt(`${BASE}/${list}`);
    expect(screen.getByText(`${page} @ ${BASE}/${list}`)).toBeInTheDocument();
  });

  it.each([
    ["users/u1", "UserEditPage u1"],
    ["roles/r1", "RoleEditPage r1"],
    ["groups/g1", "GroupEditPage g1"],
  ])("opens the edit page for %s", (path, text) => {
    renderAt(`${BASE}/${path}`);
    expect(screen.getByText(`${text} @ ${BASE}/${path}`)).toBeInTheDocument();
  });

  it.each([
    ["users/invite", "UserInvitePage", { member_invite: true }],
    ["users/add", "UserAddPage", { member_invite: true }],
    ["roles/create", "RoleCreatePage", { role_write: true }],
    ["groups/create", "GroupCreatePage", { group_write: true }],
  ])("opens %s with the right access", (path, page, access) => {
    hoisted.access = access;
    renderAt(`${BASE}/${path}`);
    expect(screen.getByText(`${page} @ ${BASE}/${path}`)).toBeInTheDocument();
  });

  it.each([
    ["users/invite", "UsersPage @ /org/acme/settings/identities/users"],
    ["users/add", "UsersPage @ /org/acme/settings/identities/users"],
    ["roles/create", "RolesPage @ /org/acme/settings/identities/roles"],
    ["groups/create", "GroupsPage @ /org/acme/settings/identities/groups"],
  ])("sends %s back to its list without access instead of treating it as an id", (path, text) => {
    renderAt(`${BASE}/${path}`);
    expect(screen.getByText(text)).toBeInTheDocument();
  });
});
