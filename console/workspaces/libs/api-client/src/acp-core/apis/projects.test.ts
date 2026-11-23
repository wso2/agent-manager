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

import { createProject, deleteProject, getProject, listProjects, updateProject } from "./projects";

const getToken = async () => "tok";
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
const params = { orgName: "my org", projName: "p/1" };
const PROJECT = "/api/v1/orgs/my%20org/projects/p%2F1";

afterEach(() => vi.resetAllMocks());

describe("listProjects", () => {
  it("sends the defined paging values with the token", async () => {
    http.httpGET.mockResolvedValue(json({ projects: [], total: 0 }));
    await expect(
      listProjects({ orgName: "my org" }, { limit: 100, offset: 0, x: undefined } as never, getToken),
    ).resolves.toEqual({ projects: [], total: 0 });
    expect(http.httpGET).toHaveBeenCalledWith("/api/v1/orgs/my%20org/projects", {
      searchParams: { limit: "100", offset: "0" },
      token: "tok",
    });
  });

  it("uses the default org with no query", async () => {
    http.httpGET.mockResolvedValue(json({}));
    await listProjects({} as never);
    expect(http.httpGET).toHaveBeenCalledWith("/api/v1/orgs/default/projects", {
      searchParams: undefined,
      token: undefined,
    });
  });
});

type Method = keyof typeof http;
const body = { displayName: "P" } as never;
const calls: [string, Method, () => Promise<unknown>, string, unknown?][] = [
  ["listProjects", "httpGET", () => listProjects({ orgName: "o" }), "/api/v1/orgs/o/projects"],
  ["createProject", "httpPOST", () => createProject({ orgName: "my org" }, body, getToken),
    "/api/v1/orgs/my%20org/projects", body],
  ["getProject", "httpGET", () => getProject(params, getToken), PROJECT],
  ["updateProject", "httpPUT", () => updateProject(params, body, getToken), PROJECT, body],
];

describe.each(calls)("%s", (_name, method, call, path, sentBody) => {
  it("sends the request and returns the body", async () => {
    http[method].mockResolvedValue(json({ name: "p" }));
    await expect(call()).resolves.toEqual({ name: "p" });
    expect(http[method].mock.calls[0][0]).toBe(path);
    if (sentBody !== undefined) expect(http[method].mock.calls[0][1]).toBe(sentBody);
  });

  it("throws the server's error body on a non-OK response", async () => {
    http[method].mockResolvedValue(json({ code: "FORBIDDEN" }, 403));
    await expect(call()).rejects.toEqual({ code: "FORBIDDEN" });
  });
});

describe("defaults", () => {
  it("falls back to the default org and project", async () => {
    http.httpGET.mockResolvedValue(json({}));
    http.httpPOST.mockResolvedValue(json({}));
    http.httpPUT.mockResolvedValue(json({}));
    await getProject({} as never);
    await createProject({} as never, body);
    await updateProject({} as never, body);
    expect(http.httpGET.mock.calls[0][0]).toBe("/api/v1/orgs/default/projects/default");
    expect(http.httpPOST.mock.calls[0][0]).toBe("/api/v1/orgs/default/projects");
    expect(http.httpPUT.mock.calls[0][0]).toBe("/api/v1/orgs/default/projects/default");
  });
});

describe("deleteProject", () => {
  it("resolves to nothing on a 204", async () => {
    http.httpDELETE.mockResolvedValue(new Response(null, { status: 204 }));
    await expect(deleteProject(params, getToken)).resolves.toBeUndefined();
    expect(http.httpDELETE).toHaveBeenCalledWith(PROJECT, { token: "tok" });
  });

  it("resolves to nothing on an empty 200", async () => {
    http.httpDELETE.mockResolvedValue(
      new Response("", { status: 200, headers: { "content-length": "0" } }),
    );
    await expect(deleteProject({} as never)).resolves.toBeUndefined();
    expect(http.httpDELETE.mock.calls[0][0]).toBe("/api/v1/orgs/default/projects/default");
  });

  it("returns a body the server sends", async () => {
    http.httpDELETE.mockResolvedValue(json({ deleted: true }));
    await expect(deleteProject(params)).resolves.toEqual({ deleted: true });
  });

  it("throws the server's error body on a non-OK response", async () => {
    http.httpDELETE.mockResolvedValue(json({ code: "CONFLICT" }, 409));
    await expect(deleteProject(params)).rejects.toEqual({ code: "CONFLICT" });
  });
});
