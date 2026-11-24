/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
import { OxygenUIThemeProvider } from "@wso2/oxygen-ui";
import { Protected } from "./Protected";


type Query = { data?: unknown; isLoading: boolean; error?: unknown };

const mocks = vi.hoisted(() => ({
  auth: {} as Record<string, unknown>,
  orgs: {} as Query,
  projects: {} as Query,
  projectQueries: [] as unknown[],
  setTelemetryTokenProvider: vi.fn(),
  setTelemetrySubject: vi.fn(),
  sessionSubjects: [] as unknown[],
}));

vi.mock("@agent-management-platform/auth", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useAuthHooks: () => mocks.auth,
}));

vi.mock("@agent-management-platform/api-client", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useListOrganizations: () => mocks.orgs,
  useListProjects: (params: unknown) => {
    mocks.projectQueries.push(params);
    return mocks.projects;
  },
  setTelemetryTokenProvider: mocks.setTelemetryTokenProvider,
  setTelemetrySubject: mocks.setTelemetrySubject,
  useSessionAnalytics: (subject: unknown) => {
    mocks.sessionSubjects.push(subject);
  },
}));

function Where() {
  const location = useLocation();
  return (
    <div data-testid="where">
      {location.pathname}|{(location.state as { from?: { pathname: string } })?.from?.pathname}
    </div>
  );
}

function renderAt(path: string) {
  return render(
    <OxygenUIThemeProvider>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/login" element={<Where />} />
          <Route path="/org/:orgId/project/:projectId" element={<Where />} />
          <Route path="/org/:orgId/projects" element={<Where />} />
          {["/", "/org/:orgId"].map((p) => (
            <Route
              key={p}
              path={p}
              element={
                <Protected>
                  <div>protected content</div>
                </Protected>
              }
            />
          ))}
        </Routes>
      </MemoryRouter>
    </OxygenUIThemeProvider>,
  );
}

const logout = vi.fn();
const getToken = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  mocks.projectQueries.length = 0;
  mocks.sessionSubjects.length = 0;
  mocks.auth = {
    isAuthenticated: true,
    isLoadingIsAuthenticated: false,
    userInfo: { sub: "user-1" },
    logout,
    getToken,
  };
  mocks.orgs = {
    data: { organizations: [{ name: "ns-acme", handle: "acme" }, { name: "ns-b" }] },
    isLoading: false,
    error: null,
  };
  mocks.projects = { data: { projects: [{ name: "p1" }, { name: "default" }] }, isLoading: false };
});

describe("Protected", () => {
  it("shows a loader while auth is resolving", () => {
    mocks.auth = { ...mocks.auth, isLoadingIsAuthenticated: true };
    renderAt("/org/acme");
    expect(screen.getByLabelText("Loading")).toBeInTheDocument();
    expect(screen.queryByText("protected content")).not.toBeInTheDocument();
  });

  it("sends signed-out users to login, remembering where they were going", () => {
    mocks.auth = { ...mocks.auth, isAuthenticated: false };
    renderAt("/org/acme");
    expect(screen.getByTestId("where")).toHaveTextContent("/login|/org/acme");
    // No session subject while signed out, and telemetry has no token source.
    expect(mocks.sessionSubjects).toEqual([undefined]);
    expect(mocks.setTelemetryTokenProvider).toHaveBeenCalledWith(undefined);
  });

  it("renders children for a signed-in user inside an org", () => {
    const view = renderAt("/org/acme");
    expect(screen.getByText("protected content")).toBeInTheDocument();
    expect(mocks.sessionSubjects).toContain("user-1");
    expect(mocks.setTelemetryTokenProvider).toHaveBeenCalledWith(getToken);
    expect(mocks.setTelemetrySubject).toHaveBeenCalledWith("user-1");
    // The landing project list is not needed when the URL names an org.
    expect(mocks.projectQueries.at(-1)).toEqual({ orgName: "" });
    view.unmount();
    expect(mocks.setTelemetryTokenProvider).toHaveBeenLastCalledWith(undefined);
  });

  it("shows a loader while organizations load", () => {
    mocks.orgs = { isLoading: true };
    renderAt("/org/acme");
    expect(screen.getByLabelText("Loading")).toBeInTheDocument();
  });

  it("shows a recoverable error when organizations fail to load", () => {
    mocks.orgs = { isLoading: false, error: new Error("upstream down") };
    renderAt("/org/acme");
    expect(screen.getByText("Failed to Load Organizations")).toBeInTheDocument();
    expect(screen.getByText("upstream down")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));
    expect(logout).toHaveBeenCalled();
  });

  it("offers sign-out when the user belongs to no organization", () => {
    mocks.orgs = { data: { organizations: [] }, isLoading: false, error: null };
    renderAt("/org/acme");
    expect(screen.getByText("No Organization Found")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));
    expect(logout).toHaveBeenCalled();
  });

  it("lands on the first org's default project at /", () => {
    renderAt("/");
    expect(screen.getByTestId("where")).toHaveTextContent("/org/acme/project/default|/");
    // Projects are listed for the namespace, not the URL handle.
    expect(mocks.projectQueries.at(-1)).toEqual({ orgName: "ns-acme" });
  });

  it("falls back to the first project when there is no default one", () => {
    mocks.projects = { data: { projects: [{ name: "p1" }, { name: "p2" }] }, isLoading: false };
    renderAt("/");
    expect(screen.getByTestId("where")).toHaveTextContent("/org/acme/project/p1|/");
  });

  it("lands on the project listing when the org has no projects", () => {
    mocks.projects = { data: { projects: [] }, isLoading: false };
    renderAt("/");
    expect(screen.getByTestId("where")).toHaveTextContent("/org/acme/projects|/");
  });

  it("waits for the project list before landing", () => {
    mocks.projects = { isLoading: true };
    renderAt("/");
    expect(screen.getByLabelText("Loading")).toBeInTheDocument();
    expect(screen.queryByTestId("where")).not.toBeInTheDocument();
  });
});
