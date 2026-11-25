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
import type { ReactNode } from "react";

const hoisted = vi.hoisted(() => ({
  canWrite: true,
  agent: { data: undefined as unknown, isLoading: false },
  displayName: undefined as string | undefined,
  environments: { data: undefined as unknown, isLoading: false, isError: false },
  panelProps: [] as unknown[],
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  useGetAgent: () => hoisted.agent,
  useUserDisplayName: () => hoisted.displayName,
  useListEnvironments: () => hoisted.environments,
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canWrite: hoisted.canWrite }),
  useFeatureAccess: () => true,
}));
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  EnvironmentCard: ({
    environment,
    tabsHeader,
    actions,
    bottomContent,
  }: {
    environment: { name: string };
    tabsHeader?: ReactNode;
    actions?: ReactNode;
    bottomContent?: ReactNode;
  }) => (
    <section>
      <h2>{`environment ${environment.name}`}</h2>
      {tabsHeader}
      {actions}
      {bottomContent}
    </section>
  ),
  OverviewPanels: ({ props }: { props: unknown }) => {
    hoisted.panelProps.push(props);
    return <div>environment panels</div>;
  },
}));
vi.mock("./SetupAgentDrawer", () => ({
  SetupAgentDrawer: ({
    open,
    environment,
    onClose,
  }: {
    open: boolean;
    environment?: string;
    onClose: () => void;
  }) =>
    open ? (
      <div>
        {`setup drawer for ${environment}`}
        <button type="button" onClick={onClose}>
          close setup
        </button>
      </div>
    ) : null,
}));

import { OverviewComponent } from "../Overview.Component";

const Destination = () => {
  const location = useLocation();
  return <div>{`at ${location.pathname}${location.search}`}</div>;
};

const Location = () => {
  const location = useLocation();
  return <div data-testid="location">{`${location.pathname}${location.search}`}</div>;
};

const renderPage = (path = "/org/acme/project/shop/agents/support-bot") =>
  render(
    <MemoryRouter
      initialEntries={[path]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route
          path="/org/:orgId/project/:projectId/agents/:agentId"
          element={
            <>
              <OverviewComponent />
              <Location />
            </>
          }
        />
        <Route path="*" element={<Destination />} />
      </Routes>
    </MemoryRouter>,
  );

const env = (name: string, displayName: string) => ({ id: `${name}-id`, name, displayName });

const externalAgent = {
  name: "support-bot",
  displayName: "Support Bot",
  description: "Answers support tickets",
  createdAt: "2026-01-01T00:00:00Z",
  createdBy: { id: "u-1", display: "jdoe" },
  provisioning: { type: "external" },
  labels: { team: "support" },
};

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.canWrite = true;
  hoisted.agent = { data: externalAgent, isLoading: false };
  hoisted.displayName = undefined;
  hoisted.environments = {
    data: [env("development", "Development"), env("production", "Production")],
    isLoading: false,
    isError: false,
  };
  hoisted.panelProps = [];
});

// Renders the real page layout and environment tabs, which is slow under load.
describe("AgentOverview", { timeout: 30000 }, () => {
  it("shows the agent's name, type, description, tags and creator", () => {
    renderPage();

    expect(screen.getByText("Support Bot")).toBeInTheDocument();
    expect(screen.getByText("External")).toBeInTheDocument();
    expect(screen.getByText("Answers support tickets")).toBeInTheDocument();
    expect(screen.getByLabelText("Custom tags")).toBeInTheDocument();
    expect(screen.getByText("jdoe")).toBeInTheDocument();
  });

  it("prefers the creator's resolved profile name", () => {
    hoisted.displayName = "Jane Doe";
    renderPage();

    expect(screen.getByText("Jane Doe")).toBeInTheDocument();
    expect(screen.queryByText("jdoe")).not.toBeInTheDocument();
  });

  it("opens the edit page from Edit agent", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Edit agent" }));

    expect(
      screen.getByText("at /org/acme/project/shop/agents/support-bot/edit"),
    ).toBeInTheDocument();
  });

  it("offers no Edit agent without write access", () => {
    hoisted.canWrite = false;
    renderPage();

    expect(screen.getByText("Support Bot")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit agent" })).not.toBeInTheDocument();
  });

  it("shows no environments for a platform-hosted agent", () => {
    hoisted.agent = {
      data: {
        ...externalAgent,
        provisioning: { type: "internal" },
        agentType: { subType: "chat-api" },
        labels: {},
      },
      isLoading: false,
    };
    renderPage();

    expect(screen.getByText("Chat")).toBeInTheDocument();
    expect(screen.queryByLabelText("Custom tags")).not.toBeInTheDocument();
    expect(screen.queryByText(/^environment /)).not.toBeInTheDocument();
  });

  it("shows the last environment by default, with its panels", () => {
    renderPage();

    expect(screen.getByText("environment production")).toBeInTheDocument();
    expect(screen.getByText("Environments")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Development/ })).toBeInTheDocument();
    expect(hoisted.panelProps.at(-1)).toEqual({
      orgId: "acme",
      projectId: "shop",
      agentId: "support-bot",
      envId: "production",
      external: true,
    });
  });

  it("switches environment from the tabs and keeps it in the URL", () => {
    renderPage();

    fireEvent.click(screen.getByRole("tab", { name: /Development/ }));

    expect(screen.getByText("environment development")).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("?env=development");
  });

  it("opens the environment named in the URL", () => {
    renderPage("/org/acme/project/shop/agents/support-bot?env=development");

    expect(screen.getByText("environment development")).toBeInTheDocument();
  });

  it("hides the tabs and heading for a single environment", () => {
    hoisted.environments = {
      data: [env("production", "Production")],
      isLoading: false,
      isError: false,
    };
    renderPage();

    expect(screen.getByText("environment production")).toBeInTheDocument();
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();
    expect(screen.queryByText("Environments")).not.toBeInTheDocument();
  });

  it("says when there are no environments", () => {
    hoisted.environments = { data: [], isLoading: false, isError: false };
    renderPage();

    expect(screen.getByText("No environments found")).toBeInTheDocument();
  });

  it("opens and closes the setup drawer for the selected environment", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Set up agent" }));

    expect(screen.getByText("setup drawer for production")).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("env=production&setup=true");

    fireEvent.click(screen.getByRole("button", { name: "close setup" }));

    expect(screen.queryByText("setup drawer for production")).not.toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("?env=production");
  });

  it("opens the setup drawer straight away from ?setup=true", () => {
    renderPage("/org/acme/project/shop/agents/support-bot?setup=true");

    expect(screen.getByText("setup drawer for production")).toBeInTheDocument();
  });

  it("shows neither content nor environments while the agent loads", () => {
    hoisted.agent = { data: undefined, isLoading: true };
    renderPage();

    expect(screen.queryByText("Answers support tickets")).not.toBeInTheDocument();
    expect(screen.queryByText(/^environment /)).not.toBeInTheDocument();
  });
});
