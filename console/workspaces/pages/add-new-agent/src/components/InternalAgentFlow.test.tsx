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

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import type { AgentSourceSeedState } from "@agent-management-platform/types";
import type { CreateAgentFormValues } from "../form/schema";

const hoisted = vi.hoisted(() => ({
  createAgent: vi.fn(),
  linkFinding: vi.fn(),
  linkArgs: [] as unknown[],
  pushSnackBar: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useAgentBuildOptions: () => ({ data: undefined }),
  useCreateAgent: () => ({ mutate: hoisted.createAgent, isPending: false, error: null }),
  useGetDeploymentPipeline: () => ({ data: undefined, isLoading: false }),
  useOnboardShadowFinding: (...args: unknown[]) => {
    hoisted.linkArgs = args;
    return { mutate: hoisted.linkFinding, isPending: false };
  },
}));

vi.mock("@agent-management-platform/shared-component", () => ({
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
  getScriptRef: () => "",
}));

vi.mock("@agent-management-platform/views", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  useSnackBar: () => ({ pushSnackBar: hoisted.pushSnackBar }),
}));

// The real form is large; the flow only needs its values, and the Python
// version the real form sets from the build options.
vi.mock("../forms/InternalAgentForm", async () => {
  const React = await import("react");
  return {
    InternalAgentForm: ({
      formData,
      setFormData,
    }: {
      formData: CreateAgentFormValues;
      setFormData: React.Dispatch<React.SetStateAction<CreateAgentFormValues>>;
    }) => {
      React.useEffect(() => {
        setFormData((prev) => (prev.languageVersion ? prev : { ...prev, languageVersion: "3.11" }));
      }, [setFormData]);
      return <pre data-testid="form">{JSON.stringify(formData)}</pre>;
    },
  };
});

import { InternalAgentFlow } from "./InternalAgentFlow";

const FINDING_PATH = "/organizations/acme/shadow-agent-discovery/agents/f1";

const seedState = (over: Partial<AgentSourceSeedState> = {}): AgentSourceSeedState => ({
  from: FINDING_PATH,
  agentSourceSeed: {
    displayName: "Customer Support Agent",
    name: "customer-support-agent",
    description: "Discovered by Shadow Agent Discovery: GitHub code agent support-bot:agent.py:x",
    repositoryUrl: "https://github.com/wso2/support-bot",
    branch: "develop",
    appPath: "/samples/support",
    language: "python",
    shadowFinding: { orgName: "acme", findingId: "f1" },
  },
  ...over,
});

function ShowLocation() {
  const l = useLocation();
  return <div data-testid="location">{`${l.pathname}${l.search}`}</div>;
}

const renderFlow = (state: unknown) =>
  render(
    <MemoryRouter
      initialEntries={[
        { pathname: "/organizations/acme/projects/default/newAgent/create/source", state },
      ]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route
          path="/organizations/:orgId/projects/:projectId/newAgent/create/source"
          element={<InternalAgentFlow />}
        />
        <Route path="*" element={<ShowLocation />} />
      </Routes>
    </MemoryRouter>,
  );

const formValues = () =>
  JSON.parse(screen.getByTestId("form").textContent ?? "{}") as CreateAgentFormValues;

const submit = () => fireEvent.click(screen.getByRole("button", { name: "Create and deploy" }));

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.linkArgs = [];
});

describe("InternalAgentFlow with a found code agent", () => {
  it("fills the form in from the router-state seed and says so", () => {
    renderFlow(seedState());
    expect(screen.getByText(/Filled in from a code agent found by Shadow Agent Discovery/))
      .toBeInTheDocument();
    expect(formValues()).toMatchObject({
      displayName: "Customer Support Agent",
      name: "customer-support-agent",
      repositoryUrl: "https://github.com/wso2/support-bot",
      branch: "develop",
      appPath: "/samples/support",
      language: "python",
      runCommand: "python main.py",
    });
    expect(hoisted.linkArgs.slice(0, 2)).toEqual(["acme", "f1"]);
  });

  it("links the found agent after the agent is created, then opens the agent", () => {
    hoisted.createAgent.mockImplementation((_payload, opts: { onSuccess: () => void }) =>
      opts.onSuccess(),
    );
    hoisted.linkFinding.mockImplementation((_body, opts: { onSuccess: () => void }) =>
      opts.onSuccess(),
    );
    renderFlow(seedState());
    submit();
    expect(hoisted.createAgent).toHaveBeenCalledTimes(1);
    expect(hoisted.linkFinding).toHaveBeenCalledWith(
      { projectName: "default", agentName: "customer-support-agent" },
      expect.anything(),
    );
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/organizations/acme/projects/default/agents/customer-support-agent?setup=true",
    );
    expect(hoisted.pushSnackBar).not.toHaveBeenCalled();
  });

  it.each([
    [500, "The agent was created but could not be linked to the found agent.", "error"],
    [
      409,
      "The agent was created. The found agent was already onboarded as another agent, " +
        "so it was not linked.",
      "info",
    ],
  ])("still opens the agent when linking fails with %s", (status, message, type) => {
    hoisted.createAgent.mockImplementation((_payload, opts: { onSuccess: () => void }) =>
      opts.onSuccess(),
    );
    hoisted.linkFinding.mockImplementation((_body, opts: { onError: (e: unknown) => void }) =>
      opts.onError({ status }),
    );
    renderFlow(seedState());
    submit();
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/organizations/acme/projects/default/agents/customer-support-agent?setup=true",
    );
    expect(hoisted.pushSnackBar).toHaveBeenCalledWith({ message, type });
  });

  it("ignores a link to a found agent of another organization", () => {
    const state = seedState();
    state.agentSourceSeed!.shadowFinding = { orgName: "other", findingId: "f1" };
    renderFlow(state);
    expect(hoisted.linkArgs.slice(0, 2)).toEqual([undefined, undefined]);
  });

  it("is a blank form without a seed", () => {
    renderFlow(undefined);
    expect(screen.queryByText(/Filled in from a code agent/)).not.toBeInTheDocument();
    expect(formValues()).toMatchObject({ displayName: "", repositoryUrl: "", branch: "main" });
  });

  it("creates without linking when there is no seed", () => {
    hoisted.createAgent.mockImplementation((_payload, opts: { onSuccess: () => void }) =>
      opts.onSuccess(),
    );
    renderFlow({ agentSourceSeed: { ...seedState().agentSourceSeed, shadowFinding: undefined } });
    submit();
    expect(hoisted.createAgent).toHaveBeenCalledTimes(1);
    expect(hoisted.linkFinding).not.toHaveBeenCalled();
  });
});
