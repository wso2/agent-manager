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
import { httpDELETE, httpGET, httpPUT } from "../utils";
import { deleteMonitorAlert, getMonitorAlert, putMonitorAlert } from "./monitor-alerts";

vi.mock("../utils", () => ({
  httpGET: vi.fn(), httpPUT: vi.fn(), httpDELETE: vi.fn(), SERVICE_BASE: "/api/v1",
}));
const mockGET = vi.mocked(httpGET);
const mockPUT = vi.mocked(httpPUT);
const mockDELETE = vi.mocked(httpDELETE);
const params = {
  orgName: "my org", projName: "project/name", agentName: "agent #1", monitorName: "monitor/name",
};
const path = "/api/v1/orgs/my%20org/projects/project%2Fname/agents/agent%20%231/monitors/monitor%2Fname/alerting";
const settings = {
  destinationIds: ["destination-one", "destination-two"],
  notifyOnFailure: true,
  notifyOnSuccess: false,
  thresholds: [],
};

beforeEach(() => { vi.resetAllMocks(); });

describe("shared monitor alerting", () => {
  it("loads shared enabled state and organization destination IDs from the monitor endpoint", async () => {
    const response = { ...settings, enabled: true, breached: [] };
    mockGET.mockResolvedValue(new Response(JSON.stringify(response)));
    await expect(getMonitorAlert(params, async () => "test-token")).resolves.toMatchObject(response);
    expect(mockGET).toHaveBeenCalledExactlyOnceWith(path, { token: "test-token" });
  });

  it("saves destination IDs and alert conditions without a console inbox recipient", async () => {
    const response = { ...settings, enabled: true };
    mockPUT.mockResolvedValue(new Response(JSON.stringify(response)));
    await expect(putMonitorAlert(params, settings, async () => "test-token")).resolves.toMatchObject(response);
    expect(mockPUT).toHaveBeenCalledExactlyOnceWith(path, settings, { token: "test-token" });
  });

  it("disables shared alerting on the monitor", async () => {
    mockDELETE.mockResolvedValue(new Response(null, { status: 204 }));
    await expect(deleteMonitorAlert(params, async () => "test-token")).resolves.toBeUndefined();
    expect(mockDELETE).toHaveBeenCalledExactlyOnceWith(path, { token: "test-token" });
  });

  it("retains disabled state when no alerting configuration exists", async () => {
    const response = { ...settings, enabled: false, destinationIds: [] };
    mockGET.mockResolvedValue(new Response(JSON.stringify(response)));
    const result = await getMonitorAlert({ ...params, orgName: undefined });
    expect(result).toMatchObject(response);
    expect(mockGET).toHaveBeenCalledExactlyOnceWith(
      path.replace("my%20org", "default"), { token: undefined },
    );
  });

  it.each(["projName", "agentName", "monitorName"] as const)("rejects a missing %s before sending a request", async (key) => {
    const incomplete = { ...params, [key]: undefined };
    await expect(getMonitorAlert(incomplete)).rejects.toThrow("required");
    await expect(putMonitorAlert(incomplete, settings)).rejects.toThrow("required");
    await expect(deleteMonitorAlert(incomplete)).rejects.toThrow("required");
    expect(mockGET).not.toHaveBeenCalled();
    expect(mockPUT).not.toHaveBeenCalled();
    expect(mockDELETE).not.toHaveBeenCalled();
  });

  it("propagates authorization failures without replacing them with disabled settings", async () => {
    const denied = new Error("Forbidden");
    mockGET.mockRejectedValue(denied);
    mockPUT.mockRejectedValue(denied);
    mockDELETE.mockRejectedValue(denied);
    await expect(getMonitorAlert(params)).rejects.toBe(denied);
    await expect(putMonitorAlert(params, settings)).rejects.toBe(denied);
    await expect(deleteMonitorAlert(params)).rejects.toBe(denied);
  });
});
