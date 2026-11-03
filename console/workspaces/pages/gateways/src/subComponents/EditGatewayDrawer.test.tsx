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

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { globalConfig, type GatewayResponse } from "@agent-management-platform/types";
import { EditGatewayDrawer } from "./EditGatewayDrawer";

const hoisted = vi.hoisted(() => ({
  updateManaged: vi.fn(),
  updateSelfHosted: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => {
  const mutation = (mutateAsync: () => unknown) => ({
    mutateAsync,
    isPending: false,
    error: null,
    reset: () => undefined,
  });
  const managed = mutation(hoisted.updateManaged);
  const selfHosted = mutation(hoisted.updateSelfHosted);
  return {
    useUpdateGateway: () => managed,
    useUpdateSelfHostedGateway: () => selfHosted,
  };
});

vi.mock("@agent-management-platform/shared-component", () => ({
  useConfirmIfUnsaved: () => (fn: () => void) => fn(),
  useUnsavedChangesGuard: () => undefined,
}));

const gateway = (hosting: "MANAGED" | "SELF_HOSTED") =>
  ({
    uuid: "gw-1",
    name: "gw-1",
    displayName: "Gateway 1",
    isCritical: false,
    hosting,
  }) as GatewayResponse;

const submit = (hosting: "MANAGED" | "SELF_HOSTED") => {
  render(
    <EditGatewayDrawer open onClose={vi.fn()} gateway={gateway(hosting)} orgId="acme" />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
};

const expectedCall = {
  params: { orgName: "acme", gatewayId: "gw-1" },
  body: { displayName: "Gateway 1", isCritical: false },
};

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(() => {
  delete globalConfig.selfHostedGatewaysEnabled;
});

describe("EditGatewayDrawer with selfHostedGatewaysEnabled off", () => {
  it("submits a self-hosted gateway to the self-hosted update API", async () => {
    globalConfig.selfHostedGatewaysEnabled = false;
    submit("SELF_HOSTED");

    await waitFor(() => expect(hoisted.updateSelfHosted).toHaveBeenCalledWith(expectedCall));
    expect(hoisted.updateManaged).not.toHaveBeenCalled();
  });

  it("submits a managed gateway to the managed update API", async () => {
    globalConfig.selfHostedGatewaysEnabled = false;
    submit("MANAGED");

    await waitFor(() => expect(hoisted.updateManaged).toHaveBeenCalledWith(expectedCall));
    expect(hoisted.updateSelfHosted).not.toHaveBeenCalled();
  });
});
