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

import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  apiState,
  mutation,
  renderAt,
  resetApiState,
  sharedState,
  last,
} from "../../__test__/mocks";
import { GroupCreatePage } from "./GroupCreatePage";

vi.mock(
  "@agent-management-platform/api-client",
  async () => (await import("../../__test__/mocks")).apiClientMock,
);
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  ...(await import("../../__test__/mocks")).sharedComponentOverrides,
}));

// Oxygen UI pages are slow to render under a loaded CI runner.
vi.setConfig({ testTimeout: 60_000 });

const renderPage = () =>
  renderAt(
    <GroupCreatePage />,
    "/org/:orgId/thunder-instances/groups/create",
    "/org/acme/thunder-instances/groups/create?envName=dev",
  );

const nameInput = () => screen.getByPlaceholderText("on-call-agents");
const descriptionInput = () => screen.getByPlaceholderText("Describe the group's purpose");
const createButton = () => screen.getByRole("button", { name: "Create group" });

beforeEach(() => {
  vi.clearAllMocks();
  resetApiState();
  sharedState.guardDirty = [];
});

describe("GroupCreatePage", () => {
  it("keeps Create disabled until a name is entered", () => {
    renderPage();

    expect(createButton()).toBeDisabled();
    fireEvent.change(nameInput(), { target: { value: "on-call" } });
    expect(createButton()).toBeEnabled();
    // Typing makes the form dirty, so leaving is guarded.
    expect(last(sharedState.guardDirty)).toBe(true);
  });

  it("creates the group with trimmed values and opens it", async () => {
    mutation("useCreateAgentIdentityGroup").mockResolvedValue({ id: "g-new" });
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "  on-call  " } });
    fireEvent.change(descriptionInput(), { target: { value: " Pager rotation " } });
    fireEvent.click(createButton());

    await waitFor(() =>
      expect(mutation("useCreateAgentIdentityGroup")).toHaveBeenCalledWith({
        params: { orgName: "acme", envName: "dev" },
        body: { name: "on-call", description: "Pager rotation" },
      }),
    );
    expect(
      await screen.findByText("at /org/acme/thunder-instances/groups/g-new?envName=dev"),
    ).toBeInTheDocument();
  });

  it("sends no description when it is left blank", async () => {
    mutation("useCreateAgentIdentityGroup").mockResolvedValue({ id: "g-new" });
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "on-call" } });
    fireEvent.change(descriptionInput(), { target: { value: "   " } });
    fireEvent.click(createButton());

    await waitFor(() =>
      expect(mutation("useCreateAgentIdentityGroup")).toHaveBeenCalledWith({
        params: { orgName: "acme", envName: "dev" },
        body: { name: "on-call", description: undefined },
      }),
    );
  });

  it("validates the name as it is typed and blocks an invalid submit", async () => {
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "x".repeat(51) } });
    expect(screen.getAllByText("Name must be at most 50 characters").length).toBeGreaterThan(0);

    fireEvent.click(createButton());
    await waitFor(() =>
      expect(screen.getAllByText("Name must be at most 50 characters")).toHaveLength(2),
    );
    expect(mutation("useCreateAgentIdentityGroup")).not.toHaveBeenCalled();
  });

  it("shows the API error and stays on the form when creation fails", async () => {
    apiState.mutations.useCreateAgentIdentityGroup.error = new Error("Group already exists");
    mutation("useCreateAgentIdentityGroup").mockRejectedValue(new Error("Group already exists"));
    renderPage();

    expect(screen.getByText("Group already exists")).toBeInTheDocument();
    fireEvent.change(nameInput(), { target: { value: "on-call" } });
    fireEvent.click(createButton());

    await waitFor(() => expect(mutation("useCreateAgentIdentityGroup")).toHaveBeenCalled());
    expect(screen.queryByText(/^at /)).not.toBeInTheDocument();
  });

  it("disables Create while the request is in flight", () => {
    apiState.mutations.useCreateAgentIdentityGroup.isPending = true;
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "on-call" } });
    expect(createButton()).toBeDisabled();
  });

  it("goes back to the groups list on Cancel", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(
      screen.getByText("at /org/acme/thunder-instances/groups?envName=dev"),
    ).toBeInTheDocument();
  });
});
