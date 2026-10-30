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
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The types package reads window.__RUNTIME_CONFIG__ when it loads, and nothing
// injects it under Vitest.
vi.hoisted(() => {
  (window as unknown as { __RUNTIME_CONFIG__: Record<string, unknown> }).__RUNTIME_CONFIG__ ??= {};
});

vi.mock("@agent-management-platform/api-client", () => ({
  useListEnvironments: vi.fn(),
}));

vi.mock("@agent-management-platform/views", () => ({
  PageLayout: ({ children }: { children?: React.ReactNode }) => <div>{children}</div>,
}));

import { useListEnvironments } from "@agent-management-platform/api-client";
import { AgentIdentityEnvironmentProvider } from "../../context/AgentIdentityEnvironmentContext";
import { AgentIdentityEnvironmentGate } from "./AgentIdentityEnvironmentGate";

const config = (window as unknown as { __RUNTIME_CONFIG__: { featureFlags?: unknown } })
  .__RUNTIME_CONFIG__;

function Search() {
  const { search } = useLocation();
  return <span data-testid="search">{search}</span>;
}

const renderGate = (entry: string, environmentAgnostic: boolean) =>
  render(
    <MemoryRouter initialEntries={[entry]}>
      <AgentIdentityEnvironmentProvider>
        <Routes>
          <Route
            path="/org/:orgId/roles"
            element={
              <>
                <Search />
                <AgentIdentityEnvironmentGate title="Roles" environmentAgnostic={environmentAgnostic}>
                  {(envName) => <div>rendered for {envName}</div>}
                </AgentIdentityEnvironmentGate>
              </>
            }
          />
        </Routes>
      </AgentIdentityEnvironmentProvider>
    </MemoryRouter>,
  );

beforeEach(() => {
  vi.mocked(useListEnvironments).mockReturnValue({
    data: [{ name: "development" }, { name: "staging" }],
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  } as unknown as ReturnType<typeof useListEnvironments>);
});

afterEach(() => {
  delete config.featureFlags;
});

describe("AgentIdentityEnvironmentGate", () => {
  it("keeps the chosen environment for roles and groups when the promotion UI is off", () => {
    config.featureFlags = { enableAgentIdentity: true };
    renderGate("/org/acme/roles?envName=staging", true);
    expect(screen.getByText("rendered for staging")).toBeInTheDocument();
  });

  it("pins roles and groups to the default environment when the promotion UI is on", () => {
    config.featureFlags = { enableAgentIdentity: true, enableIdentityPromotionUi: true };
    renderGate("/org/acme/roles?envName=staging", true);
    expect(screen.getByText("rendered for development")).toBeInTheDocument();
    expect(screen.getByTestId("search")).toHaveTextContent("envName=development");
  });

  it("leaves environment wise pages such as Agents alone when the promotion UI is on", () => {
    config.featureFlags = { enableAgentIdentity: true, enableIdentityPromotionUi: true };
    renderGate("/org/acme/roles?envName=staging", false);
    expect(screen.getByText("rendered for staging")).toBeInTheDocument();
  });
});
