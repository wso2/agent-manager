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

import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import {
  Link,
  MemoryRouter,
  Route,
  Routes,
  useLocation,
  useParams,
} from "react-router-dom";
import { AddGateway } from "./AddGateway";
import { forgetNewGatewayToken, newGatewayToken } from "./newGatewayToken";
import { deriveGatewayName } from "../form/schema";

const DEV = { name: "dev", displayName: "Development" };
const PROD = { name: "prod", displayName: "Production" };

const hoisted = vi.hoisted(() => ({
  createGateway: vi.fn(),
  createToken: vi.fn(),
  environments: [] as Array<{ name: string; displayName: string }> | undefined,
  environmentsError: null as Error | null,
  retryEnvironments: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useListEnvironments: () => ({
    data: hoisted.environments,
    error: hoisted.environmentsError,
    isFetching: false,
    refetch: hoisted.retryEnvironments,
  }),
  useCreateSelfHostedGateway: () => ({ mutateAsync: hoisted.createGateway, isPending: false }),
  useCreateSelfHostedGatewayToken: () => ({ mutateAsync: hoisted.createToken, isPending: false }),
}));

vi.mock("@agent-management-platform/shared-component", async () => {
  const { deriveResourceName, onlyEnvironmentName } = await vi.importActual<
    typeof import("@agent-management-platform/shared-component")
  >("@agent-management-platform/shared-component");
  return {
    deriveResourceName,
    onlyEnvironmentName,
    getErrorMessage: (err: unknown) => (err instanceof Error ? err.message : ""),
    useUnsavedChangesGuard: () => ({ allowNavigation: (go: () => void) => go() }),
  };
});

vi.mock("@agent-management-platform/views", async () => {
  const actual = await vi.importActual<typeof import("@agent-management-platform/views")>(
    "@agent-management-platform/views",
  );
  const React = await import("react");
  return {
    ...actual,
    PageLayout: ({ title, children }: { title: string; children: React.ReactNode }) =>
      React.createElement("div", null, React.createElement("h1", null, title), children),
  };
});


/** Stands in for the setup page: shows what the add page handed over. */
function SetupProbe() {
  const { gatewayId = "" } = useParams();
  const { state } = useLocation() as { state: { tokenFailed?: boolean } | null };
  return (
    <div>
      setup {gatewayId} token={newGatewayToken(gatewayId) ?? "none"} failed=
      {String(!!state?.tokenFailed)}
    </div>
  );
}

/** The add page, with stand-ins for where it can lead. */
const renderPage = (from?: string) =>
  render(
    <MemoryRouter initialEntries={[{ pathname: "/org/acme/gateways/add", state: from ? { from } : null }]}>
      <Routes>
        <Route path="/org/:orgId/gateways/add" element={<AddGateway />} />
        <Route path="/org/:orgId/gateways" element={<div>gateway list</div>} />
        <Route path="/org/:orgId/gateways/setup/:gatewayId" element={<SetupProbe />} />
        <Route path="/somewhere" element={<div>opener</div>} />
      </Routes>
    </MemoryRouter>,
  );

const displayNameField = () => screen.getByPlaceholderText("e.g., Production Gateway");
const nameField = () => screen.getByPlaceholderText("e.g., production-gateway");

const type = (field: HTMLElement, value: string) =>
  fireEvent.change(field, { target: { value } });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.createGateway.mockResolvedValue({ uuid: "gw-1", displayName: "Edge One" });
  hoisted.createToken.mockResolvedValue({ token: "t" });
  hoisted.environments = [DEV];
  hoisted.environmentsError = null;
});

describe("deriveGatewayName", () => {
  it.each([
    ["Edge One", "edge-one"],
    ["  My_Edge  GW! ", "myedge-gw"],
    ["-- Edge --", "edge"],
    ["网关", ""],
    ["a".repeat(70), "a".repeat(64)],
  ])("%j becomes %j", (displayName, name) => {
    expect(deriveGatewayName(displayName)).toBe(name);
  });
});

describe("AddGateway name", () => {
  it("follows the display name until the user edits it", () => {
    renderPage();

    type(displayNameField(), "Edge One");
    expect(nameField()).toHaveValue("edge-one");

    type(nameField(), "my-edge");
    type(displayNameField(), "Edge Two");
    expect(nameField()).toHaveValue("my-edge");
  });

  it("is sent as the handle", async () => {
    renderPage();

    type(displayNameField(), "Edge One");
    type(screen.getByPlaceholderText("https://gateway.example.com"), "https://edge.example.com");
    fireEvent.click(screen.getByRole("button", { name: "Create gateway" }));

    await waitFor(() => expect(hoisted.createGateway).toHaveBeenCalledTimes(1));
    expect(hoisted.createGateway.mock.calls[0][0].body).toMatchObject({
      displayName: "Edge One",
      handle: "edge-one",
      environment: "dev",
    });
  });

  it("blocks a name the API would refuse", () => {
    renderPage();

    type(displayNameField(), "Edge One");
    type(nameField(), "Edge_One");
    fireEvent.click(screen.getByRole("button", { name: "Create gateway" }));

    expect(
      screen.getByText(/Use lowercase letters, numbers and hyphens/),
    ).toBeInTheDocument();
    expect(hoisted.createGateway).not.toHaveBeenCalled();
  });
});

describe("AddGateway environment loading", () => {
  it("offers a retry and blocks creation when the environments fail to load", () => {
    hoisted.environments = undefined;
    hoisted.environmentsError = new Error("boom");
    renderPage();

    type(displayNameField(), "Edge One");
    expect(screen.getByRole("button", { name: "Create gateway" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));

    expect(hoisted.retryEnvironments).toHaveBeenCalled();
    expect(displayNameField()).toHaveValue("Edge One");
  });
});

describe("AddGateway environment", () => {
  const submit = (url = "https://edge.example.com") => {
    type(displayNameField(), "Edge One");
    type(screen.getByPlaceholderText("https://gateway.example.com"), url);
    fireEvent.click(screen.getByRole("button", { name: "Create gateway" }));
  };

  it("hides the field and uses the only environment", async () => {
    renderPage();

    expect(screen.queryByRole("combobox", { name: "Environment" })).not.toBeInTheDocument();
    submit();

    await waitFor(() => expect(hoisted.createGateway).toHaveBeenCalledTimes(1));
    expect(hoisted.createGateway.mock.calls[0][0].body).toMatchObject({ environment: "dev" });
  });

  it("asks for an environment when there is more than one", async () => {
    hoisted.environments = [DEV, PROD];
    renderPage();

    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Environment" }));
    fireEvent.click(screen.getByRole("option", { name: "Production" }));
    submit();

    await waitFor(() => expect(hoisted.createGateway).toHaveBeenCalledTimes(1));
    expect(hoisted.createGateway.mock.calls[0][0].body).toMatchObject({ environment: "prod" });
  });
});

describe("AddGateway environment refresh", () => {
  it("does not submit an environment the form no longer offers", async () => {
    hoisted.environments = [DEV, PROD];
    renderPage();
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Environment" }));
    fireEvent.click(screen.getByRole("option", { name: "Production" }));

    // Production is removed; with one environment left the field hides.
    hoisted.environments = [DEV];
    type(displayNameField(), "Edge One");
    type(screen.getByPlaceholderText("https://gateway.example.com"), "https://edge.example.com");
    fireEvent.click(screen.getByRole("button", { name: "Create gateway" }));

    await waitFor(() => expect(hoisted.createGateway).toHaveBeenCalledTimes(1));
    expect(hoisted.createGateway.mock.calls[0][0].body).toMatchObject({ environment: "dev" });
  });
});

describe("AddGateway leaving mid-submit", () => {
  it("keeps the token but does not pull the user back to setup", async () => {
    let finishToken: (value: { token: string }) => void = () => {};
    hoisted.createToken.mockReturnValue(
      new Promise((resolve) => {
        finishToken = resolve;
      }),
    );
    forgetNewGatewayToken("gw-1");
    render(
      <MemoryRouter initialEntries={["/org/acme/gateways/add"]}>
        <Link to="/somewhere">leave</Link>
        <Routes>
          <Route path="/org/:orgId/gateways/add" element={<AddGateway />} />
          <Route path="/org/:orgId/gateways/setup/:gatewayId" element={<div>setup page</div>} />
          <Route path="/somewhere" element={<div>elsewhere</div>} />
        </Routes>
      </MemoryRouter>,
    );
    type(displayNameField(), "Edge One");
    type(screen.getByPlaceholderText("https://gateway.example.com"), "https://edge.example.com");
    fireEvent.click(screen.getByRole("button", { name: "Create gateway" }));
    await waitFor(() => expect(hoisted.createToken).toHaveBeenCalled());

    fireEvent.click(screen.getByText("leave"));
    await act(async () => finishToken({ token: "late" }));

    expect(screen.getByText("elsewhere")).toBeInTheDocument();
    expect(screen.queryByText("setup page")).not.toBeInTheDocument();
    expect(newGatewayToken("gw-1")).toBe("late");
  });
});

describe("AddGateway navigation", () => {
  const create = async () => {
    type(displayNameField(), "Edge One");
    type(screen.getByPlaceholderText("https://gateway.example.com"), "https://edge.example.com");
    fireEvent.click(screen.getByRole("button", { name: "Create gateway" }));
    await screen.findByText(/^setup gw-1/);
  };

  it("opens the setup page with the new token kept in memory", async () => {
    renderPage();
    await create();

    expect(
      screen.getByText("setup gw-1 token=t failed=false", { normalizer: (t) => t.replace(/\s+/g, " ") }),
    ).toBeInTheDocument();
    expect(hoisted.createToken).toHaveBeenCalledWith({ orgName: "acme", gatewayId: "gw-1" });
  });

  it("tells the setup page when the token could not be minted", async () => {
    forgetNewGatewayToken("gw-1");
    hoisted.createToken.mockRejectedValueOnce(new Error("boom"));
    renderPage();
    await create();

    expect(
      screen.getByText("setup gw-1 token=none failed=true", { normalizer: (t) => t.replace(/\s+/g, " ") }),
    ).toBeInTheDocument();
  });

  it("returns to the page that opened it on Cancel", () => {
    renderPage("/somewhere");

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("opener")).toBeInTheDocument();
  });

  it("returns to the gateway list on Cancel after a direct visit", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("gateway list")).toBeInTheDocument();
  });
});
