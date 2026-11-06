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

import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ProjectListResponse, ProjectResponse } from "@agent-management-platform/types";

vi.mock("@agent-management-platform/views", () => ({ useSnackBar: () => ({ pushSnackBar: vi.fn() }) }));
vi.mock("@agent-management-platform/auth", () => ({ useAuthHooks: () => ({}) }));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
const { listProjects } = vi.hoisted(() => ({ listProjects: vi.fn() }));
vi.mock("../apis", () => ({ listProjects: (...args: unknown[]) => listProjects(...args) }));

import { listAllProjects } from "./projects";

const projectsNamed = (from: number, count: number) =>
  Array.from({ length: count }, (_, i) => ({ name: `p${from + i}` }) as ProjectResponse);

function serve(total: number) {
  listProjects.mockImplementation(
    async (_params, query: { limit: number; offset: number }): Promise<ProjectListResponse> => ({
      projects: projectsNamed(
        query.offset,
        Math.max(0, Math.min(query.limit, total - query.offset)),
      ),
      total,
      limit: query.limit,
      offset: query.offset,
    }),
  );
}

describe("listAllProjects", () => {
  beforeEach(() => {
    listProjects.mockReset();
  });

  it("loads every page of a large org", async () => {
    serve(250);
    const all = await listAllProjects({ orgName: "acme" });
    expect(all.projects).toHaveLength(250);
    expect(all.projects[249].name).toBe("p249");
    expect(listProjects.mock.calls.map(([, q]) => q.offset)).toEqual([0, 100, 200]);
  });

  it("makes one request for a small org", async () => {
    serve(3);
    expect((await listAllProjects({ orgName: "acme" })).projects).toHaveLength(3);
    expect(listProjects).toHaveBeenCalledTimes(1);
  });

  it("stops when a page comes back empty", async () => {
    listProjects
      .mockResolvedValueOnce({ projects: projectsNamed(0, 100), total: 150, limit: 100, offset: 0 })
      .mockResolvedValueOnce({ projects: [], total: 100, limit: 100, offset: 100 });
    expect((await listAllProjects({ orgName: "acme" })).projects).toHaveLength(100);
    expect(listProjects).toHaveBeenCalledTimes(2);
  });
});
