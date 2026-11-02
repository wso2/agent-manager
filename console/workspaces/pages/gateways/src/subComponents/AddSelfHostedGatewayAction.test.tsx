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
import { fireEvent, render, screen } from "@testing-library/react";
import { globalConfig } from "@agent-management-platform/types";
import { AddSelfHostedGatewayAction } from "./AddSelfHostedGatewayAction";

const drawerProps = vi.hoisted(() => ({ last: null as Record<string, unknown> | null }));

const listEnvironments = vi.hoisted(() => vi.fn());

vi.mock("@agent-management-platform/api-client", () => ({
  useListEnvironments: listEnvironments,
}));

vi.mock("./AddSelfHostedGatewayDrawer", () => ({
  AddSelfHostedGatewayDrawer: (props: Record<string, unknown>) => {
    drawerProps.last = props;
    return props.open ? <div>drawer open</div> : null;
  },
}));

describe("AddSelfHostedGatewayAction", () => {
  beforeEach(() => {
    drawerProps.last = null;
    listEnvironments.mockReset();
    listEnvironments.mockReturnValue({ data: [{ id: "e1", name: "dev" }] });
    globalConfig.selfHostedGatewaysEnabled = true;
  });

  afterEach(() => {
    delete globalConfig.selfHostedGatewaysEnabled;
  });

  it("no add-self-hosted action when selfHostedGatewaysEnabled is false", () => {
    globalConfig.selfHostedGatewaysEnabled = false;
    render(<AddSelfHostedGatewayAction orgId="acme" />);
    expect(screen.queryByText("Add self-hosted gateway")).toBeNull();
    expect(drawerProps.last).toBeNull();
    expect(listEnvironments).not.toHaveBeenCalled();
  });

  it("opens the drawer preset with the first environment and APIP's default name", () => {
    render(<AddSelfHostedGatewayAction orgId="acme" />);
    fireEvent.click(screen.getByText("Add self-hosted gateway"));
    expect(screen.getByText("drawer open")).toBeTruthy();
    expect(drawerProps.last).toMatchObject({
      presetEnvironment: "dev",
      presetDisplayName: "AI Self-Hosted",
      presetDescription: "Default self-hosted gateway",
    });
  });

  it("prefers an explicit preset environment", () => {
    render(<AddSelfHostedGatewayAction orgId="acme" presetEnvironment="prod" />);
    expect(drawerProps.last).toMatchObject({ presetEnvironment: "prod" });
  });
});
