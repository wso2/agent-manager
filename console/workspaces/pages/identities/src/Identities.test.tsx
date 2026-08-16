/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { SnackBarProvider } from "@agent-management-platform/views";
import { ConfirmationDialogProvider } from "@agent-management-platform/shared-component";
import { vi } from "vitest";
import { IdentitiesOrganization } from "./index";

// IdentitiesOrganization (via UsersPage) calls real TanStack Query hooks,
// which need a QueryClientProvider the real app only supplies at the shell
// level. Stub the api-client module boundary instead.
vi.mock("@agent-management-platform/api-client", () => ({
  useListUsers: vi.fn(() => ({ data: undefined, isLoading: false, error: null })),
  useDeleteUser: vi.fn(() => ({ mutateAsync: vi.fn() })),
}));

import { useListUsers } from "@agent-management-platform/api-client";

const mockUseListUsers = vi.mocked(useListUsers);

const route = "/org/org1/settings/identities";

function renderWithRouter(initialEntry = route) {
  return render(
    <SnackBarProvider>
      <ConfirmationDialogProvider>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }} initialEntries={[initialEntry]}>
          <Routes>
            <Route
              path="/org/:orgId/settings/identities/*"
              element={<IdentitiesOrganization />}
            />
          </Routes>
        </MemoryRouter>
      </ConfirmationDialogProvider>
    </SnackBarProvider>,
  );
}

describe("IdentitiesOrganization", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("redirects the index route to the users list and shows its empty state", () => {
    mockUseListUsers.mockReturnValue({
      data: { users: [] },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useListUsers>);

    renderWithRouter();

    expect(screen.getByPlaceholderText("Search users...")).toBeInTheDocument();
    expect(screen.getByText("No users yet")).toBeInTheDocument();
  });

  it("lists users by username once data loads", () => {
    mockUseListUsers.mockReturnValue({
      data: {
        users: [
          { id: "user-1", attributes: { username: "jane.doe" } },
        ],
      },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useListUsers>);

    renderWithRouter();

    expect(screen.getByText("jane.doe")).toBeInTheDocument();
  });

  it("shows an error alert when the users list fails to load", () => {
    mockUseListUsers.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("network down"),
    } as unknown as ReturnType<typeof useListUsers>);

    renderWithRouter();

    expect(screen.getByText("Failed to load users")).toBeInTheDocument();
  });
});
