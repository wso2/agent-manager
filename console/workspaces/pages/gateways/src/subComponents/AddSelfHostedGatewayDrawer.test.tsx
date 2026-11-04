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
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AddSelfHostedGatewayDrawer } from "./AddSelfHostedGatewayDrawer";
import { deriveGatewayName } from "../form/schema";

const hoisted = vi.hoisted(() => ({
  createGateway: vi.fn(),
  createToken: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useListEnvironments: () => ({ data: [{ name: "dev", displayName: "Development" }] }),
  useCreateSelfHostedGateway: () => ({ mutateAsync: hoisted.createGateway, isPending: false }),
  useCreateSelfHostedGatewayToken: () => ({ mutateAsync: hoisted.createToken, isPending: false }),
}));

vi.mock("@agent-management-platform/shared-component", async () => {
  const { deriveResourceName } = await vi.importActual<
    typeof import("@agent-management-platform/shared-component")
  >("@agent-management-platform/shared-component");
  return {
    deriveResourceName,
    getErrorMessage: (err: unknown) => (err instanceof Error ? err.message : ""),
  };
});

vi.mock("./SelfHostedSetupSteps", () => ({ SelfHostedSetupSteps: () => null }));

const displayNameField = () => screen.getByPlaceholderText("e.g., Production Gateway");
const nameField = () => screen.getByPlaceholderText("e.g., production-gateway");

const type = (field: HTMLElement, value: string) =>
  fireEvent.change(field, { target: { value } });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.createGateway.mockResolvedValue({ uuid: "gw-1", displayName: "Edge One" });
  hoisted.createToken.mockResolvedValue({ token: "t" });
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

describe("AddSelfHostedGatewayDrawer name", () => {
  it("follows the display name until the user edits it", () => {
    render(<AddSelfHostedGatewayDrawer open onClose={vi.fn()} orgId="acme" />);

    type(displayNameField(), "Edge One");
    expect(nameField()).toHaveValue("edge-one");

    type(nameField(), "my-edge");
    type(displayNameField(), "Edge Two");
    expect(nameField()).toHaveValue("my-edge");
  });

  it("is sent as the handle", async () => {
    render(<AddSelfHostedGatewayDrawer open onClose={vi.fn()} orgId="acme" />);

    type(displayNameField(), "Edge One");
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Environment" }));
    fireEvent.click(screen.getByRole("option", { name: "Development" }));
    type(screen.getByPlaceholderText("https://gateway.example.com"), "https://edge.example.com");
    fireEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(hoisted.createGateway).toHaveBeenCalledTimes(1));
    expect(hoisted.createGateway.mock.calls[0][0].body).toMatchObject({
      displayName: "Edge One",
      handle: "edge-one",
      environment: "dev",
    });
  });

  it("blocks a name the API would refuse", () => {
    render(<AddSelfHostedGatewayDrawer open onClose={vi.fn()} orgId="acme" />);

    type(displayNameField(), "Edge One");
    type(nameField(), "Edge_One");
    fireEvent.click(screen.getByRole("button", { name: "Create" }));

    expect(
      screen.getByText(/Use lowercase letters, numbers and hyphens/),
    ).toBeInTheDocument();
    expect(hoisted.createGateway).not.toHaveBeenCalled();
  });
});
