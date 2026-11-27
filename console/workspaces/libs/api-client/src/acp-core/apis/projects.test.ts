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
const PROJECTS = "/api/v1/projects";
const PROJECT = "/api/v1/projects/p%2F1";
const doc = (name: string, spec: object) => ({
  apiVersion: "ai-control-plane.choreolabs.com/v1",
  kind: "Project",
  metadata: { name },
  spec,
});

afterEach(() => vi.resetAllMocks());

describe("listProjects", () => {
  it("sends the defined paging values with the token, without the org", async () => {
    http.httpGET.mockResolvedValue(json({ list: [], pagination: { total: 0 } }));
    await expect(
      listProjects({ orgName: "my org" }, { limit: 100, offset: 0, x: undefined } as never, getToken),
    ).resolves.toEqual({ list: [], pagination: { total: 0 } });
    expect(http.httpGET).toHaveBeenCalledWith(PROJECTS, {
      searchParams: { limit: "100", offset: "0" },
      token: "tok",
    });
  });

  it("sends no query when none is given", async () => {
    http.httpGET.mockResolvedValue(json({}));
    await listProjects({} as never);
    expect(http.httpGET).toHaveBeenCalledWith(PROJECTS, {
      searchParams: undefined,
      token: undefined,
    });
  });
});

type Method = keyof typeof http;
const created = { name: "p", displayName: "P", description: "d" };
const spec = { displayName: "Q" };
const calls: [string, Method, () => Promise<unknown>, string, unknown?][] = [
  ["listProjects", "httpGET", () => listProjects({ orgName: "o" }), PROJECTS],
  ["createProject", "httpPOST", () => createProject({ orgName: "my org" }, created, getToken),
    PROJECTS, doc("p", { displayName: "P", description: "d" })],
  ["getProject", "httpGET", () => getProject(params, getToken), PROJECT],
  ["updateProject", "httpPUT", () => updateProject(params, spec, getToken), PROJECT,
    doc("p/1", { displayName: "Q", description: undefined })],
];

describe.each(calls)("%s", (_name, method, call, path, sentBody) => {
  it("sends the request and returns the body", async () => {
    http[method].mockResolvedValue(json({ metadata: { name: "p" } }));
    await expect(call()).resolves.toEqual({ metadata: { name: "p" } });
    expect(http[method].mock.calls[0][0]).toBe(path);
    if (sentBody !== undefined) expect(http[method].mock.calls[0][1]).toEqual(sentBody);
  });

  it("throws the server's error body on a non-OK response", async () => {
    http[method].mockResolvedValue(json({ code: "FORBIDDEN" }, 403));
    await expect(call()).rejects.toEqual({ code: "FORBIDDEN" });
  });
});

describe("a project route needs a project", () => {
  it("throws before sending when projName is missing", async () => {
    await expect(getProject({} as never)).rejects.toThrow("projName is required");
    await expect(updateProject({} as never, spec)).rejects.toThrow("projName is required");
    await expect(deleteProject({} as never)).rejects.toThrow("projName is required");
    expect(http.httpGET).not.toHaveBeenCalled();
  });
});

describe("deleteProject", () => {
  it("resolves to nothing on a 204", async () => {
    http.httpDELETE.mockResolvedValue(new Response(null, { status: 204 }));
    await expect(deleteProject(params, getToken)).resolves.toBeUndefined();
    expect(http.httpDELETE).toHaveBeenCalledWith(PROJECT, { token: "tok" });
  });

  it("throws the server's error body on a non-OK response", async () => {
    http.httpDELETE.mockResolvedValue(json({ code: "CONFLICT" }, 409));
    await expect(deleteProject(params)).rejects.toEqual({ code: "CONFLICT" });
  });
});
