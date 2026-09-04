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
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { SnackBarProvider } from "@agent-management-platform/views";
import { ConfirmationDialogProvider } from "@agent-management-platform/shared-component";
import { vi } from "vitest";
import { SettingsOrganization } from "./index";

// SettingsOrganization renders the identities package's UsersPage by
// default, which calls real TanStack Query hooks needing a QueryClientProvider
// the real app only supplies at the shell level. Stub the api-client module
// boundary instead.
vi.mock("@agent-management-platform/api-client", () => ({
  useListUsers: vi.fn(() => ({ data: { users: [] }, isLoading: false, error: null })),
  useDeleteUser: vi.fn(() => ({ mutateAsync: vi.fn() })),
}));

const route = "/org/org1/settings";

function renderWithRouter(initialEntry = route) {
  return render(
    <SnackBarProvider>
      <ConfirmationDialogProvider>
        <MemoryRouter
          future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
          initialEntries={[initialEntry]}
        >
          <Routes>
            <Route path="/org/:orgId/settings/*" element={<SettingsOrganization />} />
          </Routes>
        </MemoryRouter>
      </ConfirmationDialogProvider>
    </SnackBarProvider>,
  );
}

describe("SettingsOrganization", () => {
  it("renders the page title and the User Management sidebar section", () => {
    renderWithRouter();

    expect(screen.getByText("Settings")).toBeInTheDocument();
    expect(screen.getByText("User Management")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Users/i })).toBeInTheDocument();
  });

  it("redirects the index route to the identities users list", () => {
    renderWithRouter();

    expect(screen.getByPlaceholderText("Search users...")).toBeInTheDocument();
    expect(screen.getByText("No users yet")).toBeInTheDocument();
  });

  it("marks the Users nav item active while on the users route", () => {
    renderWithRouter("/org/org1/settings/identities/users");

    const usersLink = screen.getByRole("link", { name: /Users/i });
    expect(usersLink.querySelector(".Mui-selected")).toBeInTheDocument();
  });
});
