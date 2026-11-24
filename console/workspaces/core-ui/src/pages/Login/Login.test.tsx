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

import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { OxygenUIThemeProvider } from "@wso2/oxygen-ui";
import { Login } from "./Login";


const mocks = vi.hoisted(() => ({
  auth: {} as Record<string, unknown>,
  fileMode: false,
}));

vi.mock("@agent-management-platform/auth", async (importOriginal) => {
  const actual = await importOriginal<object>();
  return {
    ...actual,
    useAuthHooks: () => mocks.auth,
    get isFileAuthMode() {
      return mocks.fileMode;
    },
    LoginForm: () => <div>credential form</div>,
  };
});

const runtimeConfig = (window as unknown as { __RUNTIME_CONFIG__: Record<string, unknown> })
  .__RUNTIME_CONFIG__;

const login = vi.fn();
const assign = vi.fn();
const originalLocation = window.location;

function renderLogin(from?: string, search = "") {
  window.history.replaceState({}, "", `/login${search}`);
  return render(
    <OxygenUIThemeProvider>
      <MemoryRouter
        initialEntries={[{ pathname: "/login", state: from ? { from: { pathname: from } } : null }]}
      >
        <Login />
      </MemoryRouter>
    </OxygenUIThemeProvider>,
  );
}

beforeEach(() => {
  login.mockReset();
  assign.mockReset();
  mocks.fileMode = false;
  mocks.auth = {
    isAuthenticated: false,
    isLoadingIsAuthenticated: false,
    isLoadingUserInfo: false,
    userInfo: undefined,
    login,
  };
  delete runtimeConfig.basePath;
  // jsdom's location.assign is not configurable; swap the whole object.
  Object.defineProperty(window, "location", {
    configurable: true,
    value: {
      ...originalLocation,
      origin: originalLocation.origin,
      search: originalLocation.search,
      assign,
    },
  });
});

afterEach(() => {
  Object.defineProperty(window, "location", { configurable: true, value: originalLocation });
});

describe("Login", () => {
  it("starts the sign-in redirect for a signed-out user", () => {
    renderLogin();
    expect(login).toHaveBeenCalledTimes(1);
    expect(assign).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Loading")).toBeInTheDocument();
  });

  it("waits while auth state is loading", () => {
    mocks.auth = { ...mocks.auth, isLoadingIsAuthenticated: true };
    renderLogin();
    expect(login).not.toHaveBeenCalled();
    expect(assign).not.toHaveBeenCalled();
  });

  it("does not restart sign-in while handling the OAuth callback", () => {
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { ...originalLocation, origin: originalLocation.origin, search: "?code=abc", assign },
    });
    renderLogin();
    expect(login).not.toHaveBeenCalled();
  });

  it("sends an already signed-in user back to where they were going", () => {
    mocks.auth = { ...mocks.auth, isAuthenticated: true };
    renderLogin("/org/acme/project/p1?tab=x");
    expect(assign).toHaveBeenCalledWith("/org/acme/project/p1?tab=x");
    expect(login).not.toHaveBeenCalled();
  });

  it("treats a user with a subject as signed in and prefixes the base path", () => {
    runtimeConfig.basePath = "/console/";
    mocks.auth = { ...mocks.auth, userInfo: { sub: "u1" } };
    renderLogin("/org/acme");
    expect(assign).toHaveBeenCalledWith("/console/org/acme");
  });

  it.each(["https://evil.example.com/phish", "//evil.example.com/x"])(
    "never redirects off-site (%s)",
    (from) => {
      mocks.auth = { ...mocks.auth, isAuthenticated: true };
      renderLogin(from);
      expect(assign).toHaveBeenCalledWith("/");
    },
  );

  it("defaults to the root when there is no previous location", () => {
    mocks.auth = { ...mocks.auth, isAuthenticated: true };
    renderLogin();
    expect(assign).toHaveBeenCalledWith("/");
  });

  it("shows the credential form in file auth mode instead of redirecting", () => {
    mocks.fileMode = true;
    renderLogin();
    expect(screen.getByText("credential form")).toBeInTheDocument();
    expect(login).not.toHaveBeenCalled();
  });

  it("does not show the credential form to a signed-in user in file mode", () => {
    mocks.fileMode = true;
    mocks.auth = { ...mocks.auth, isAuthenticated: true };
    renderLogin("/org/acme");
    expect(screen.queryByText("credential form")).not.toBeInTheDocument();
    expect(assign).toHaveBeenCalledWith("/org/acme");
  });
});
