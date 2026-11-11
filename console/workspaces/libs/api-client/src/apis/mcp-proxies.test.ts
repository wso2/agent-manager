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
import { httpGET } from "../utils";
import { getProjectMCPProxy, listAllMCPProxies, listMCPProxies } from "./mcp-proxies";

vi.mock("../utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../utils")>()),
  httpGET: vi.fn(),
}));

const mockGET = vi.mocked(httpGET);

beforeEach(() => {
  mockGET.mockReset();
  mockGET.mockResolvedValue({ ok: true, json: async () => ({ list: [] }) } as Response);
});

describe("listMCPProxies", () => {
  it("sends the project query param when provided", async () => {
    await listMCPProxies({ orgName: "org" }, { limit: 50, offset: 0, project: "p1" });
    expect(mockGET.mock.calls[0][1]?.searchParams).toEqual({
      limit: "50",
      offset: "0",
      project: "p1",
    });
  });

  it("omits project when not provided (org-only list)", async () => {
    await listMCPProxies({ orgName: "org" }, { limit: 50, offset: 0 });
    expect(mockGET.mock.calls[0][1]?.searchParams).toEqual({ limit: "50", offset: "0" });
  });
});

describe("listAllMCPProxies", () => {
  it("reads page after page until it has the total", async () => {
    const page = (n: number, from: number) =>
      Array.from({ length: n }, (_, i) => ({ id: `p${from + i}` }));
    mockGET
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ list: page(100, 0), pagination: { count: 150 } }),
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ list: page(50, 100), pagination: { count: 150 } }),
      } as Response);

    const all = await listAllMCPProxies({ orgName: "org" }, { project: "p1" });

    expect(all.list).toHaveLength(150);
    expect(mockGET).toHaveBeenCalledTimes(2);
    expect(mockGET.mock.calls[1][1]?.searchParams).toEqual({
      limit: "100",
      offset: "100",
      project: "p1",
    });
  });

  it("stops on an empty page", async () => {
    mockGET.mockResolvedValue({
      ok: true,
      json: async () => ({ list: [], pagination: { count: 10 } }),
    } as Response);
    const all = await listAllMCPProxies({ orgName: "org" });
    expect(all.list).toEqual([]);
    expect(mockGET).toHaveBeenCalledTimes(1);
  });
});

describe("getProjectMCPProxy", () => {
  it("reads the proxy from its project route", async () => {
    await getProjectMCPProxy({ orgName: "org", projName: "p 1", proxyId: "orders" });
    expect(String(mockGET.mock.calls[0][0])).toMatch(
      /\/orgs\/org\/projects\/p%201\/mcp-proxies\/orders$/,
    );
  });

  it("refuses a missing project rather than calling the org route", async () => {
    await expect(
      getProjectMCPProxy({ orgName: "org", projName: undefined, proxyId: "orders" }),
    ).rejects.toThrow();
    expect(mockGET).not.toHaveBeenCalled();
  });
});
