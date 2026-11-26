/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { afterEach, describe, expect, it, vi } from "vitest";
import type * as Utils from "../../shared/utils";

const http = vi.hoisted(() => ({
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));
vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof Utils>()),
  ...http,
}));

import {
  generateResourceName,
  getOrganization,
  listOrganizations,
} from "./organizations";

const getToken = async () => "tok";
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });

afterEach(() => vi.resetAllMocks());

describe("listOrganizations", () => {
  it("sends the defined query values as strings with the token", async () => {
    http.httpGET.mockResolvedValue(json({ organizations: [{ name: "acme" }] }));

    await expect(
      listOrganizations({ limit: 10, offset: undefined } as never, getToken),
    ).resolves.toEqual({ organizations: [{ name: "acme" }] });

    expect(http.httpGET).toHaveBeenCalledWith("/api/v1/organizations", {
      searchParams: { limit: "10" },
      token: "tok",
    });
  });

  it("sends no query or token when given none", async () => {
    http.httpGET.mockResolvedValue(json({}));
    await listOrganizations();
    expect(http.httpGET).toHaveBeenCalledWith("/api/v1/organizations", { searchParams: undefined, token: undefined });
  });

  it("throws the server's error body on a non-OK response", async () => {
    http.httpGET.mockResolvedValue(json({ code: "X" }, 500));
    await expect(listOrganizations()).rejects.toEqual({ code: "X" });
  });
});

describe("getOrganization", () => {
  it("gets the caller's org", async () => {
    http.httpGET.mockResolvedValue(json({ name: "a b" }));
    await expect(getOrganization({ orgName: "a b" }, getToken)).resolves.toEqual({ name: "a b" });
    expect(http.httpGET).toHaveBeenCalledWith("/api/v1/organizations/current", { token: "tok" });
  });

  it("throws a non-OK body", async () => {
    http.httpGET.mockResolvedValue(json({ code: "NOT_FOUND" }, 404));
    await expect(getOrganization({ orgName: "x" })).rejects.toEqual({ code: "NOT_FOUND" });
  });
});

describe("generateResourceName", () => {
  it("posts the display name to the name generator", async () => {
    http.httpPOST.mockResolvedValue(json({ name: "my-agent" }));
    const body = { displayName: "My Agent", resourceType: "agent" } as never;
    await expect(generateResourceName({ orgName: "a/b" }, body, getToken)).resolves.toEqual({
      name: "my-agent",
    });
    expect(http.httpPOST).toHaveBeenCalledWith("/api/v1/utils/generate-name", body, {
      token: "tok",
    });
  });

  it("throws a non-OK body", async () => {
    http.httpPOST.mockResolvedValue(json({ code: "BAD" }, 400));
    await expect(generateResourceName({ orgName: "o" }, {} as never)).rejects.toEqual({ code: "BAD" });
  });
});
