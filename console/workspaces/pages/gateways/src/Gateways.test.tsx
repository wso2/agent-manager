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
import { GatewaysOrganization } from "./index";

// GatewaysOrganization (via AIGatewaysTable) calls real TanStack Query
// hooks, which need a QueryClientProvider the real app only supplies at the
// shell level. Stub the api-client module boundary instead.
vi.mock("@agent-management-platform/api-client", () => ({
  useListGateways: vi.fn(() => ({ data: undefined, isLoading: false, error: null })),
  useListEnvironments: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useDeleteGateway: vi.fn(() => ({ mutateAsync: vi.fn() })),
}));

import { useListGateways } from "@agent-management-platform/api-client";

const mockUseListGateways = vi.mocked(useListGateways);

const route = "/org/org1/gateways";

function renderWithRouter(initialEntry = route) {
  return render(
    <SnackBarProvider>
      <ConfirmationDialogProvider>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }} initialEntries={[initialEntry]}>
          <Routes>
            <Route path="/org/:orgId/gateways/*" element={<GatewaysOrganization />} />
          </Routes>
        </MemoryRouter>
      </ConfirmationDialogProvider>
    </SnackBarProvider>,
  );
}

describe("GatewaysOrganization", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the page title and the empty state when there are no gateways", () => {
    mockUseListGateways.mockReturnValue({
      data: { gateways: [] },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useListGateways>);

    renderWithRouter();

    expect(screen.getByText("Gateways")).toBeInTheDocument();
    expect(screen.getByText("No available gateway")).toBeInTheDocument();
  });

  it("lists gateways by display name once data loads", () => {
    mockUseListGateways.mockReturnValue({
      data: {
        gateways: [
          {
            uuid: "gw-1",
            name: "gw-prod",
            displayName: "Production Gateway",
            status: "ACTIVE",
            gatewayType: "internal",
            updatedAt: "2026-01-01T00:00:00Z",
          },
        ],
      },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useListGateways>);

    renderWithRouter();

    expect(screen.getByText("Production Gateway")).toBeInTheDocument();
  });
});
