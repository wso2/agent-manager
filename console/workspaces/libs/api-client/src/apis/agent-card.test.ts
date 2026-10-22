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

import { expect, it, vi } from "vitest";

vi.mock("../utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../utils")>()),
  httpGET: vi.fn(async () => ({ ok: true, json: async () => ({}) })),
}));
import { httpGET, SERVICE_BASE } from "../utils";
import { agentEnvironmentResourcePath } from "./agent-card";
import { getA2AProxy } from "./a2a-proxy";

const params = { orgName: "acme", projName: "p 1", agentName: "a/b", envId: "dev" };

it("encodes every segment and appends the resource", () => {
  expect(agentEnvironmentResourcePath(params, "a2a-connect"))
    .toBe(`${SERVICE_BASE}/orgs/acme/projects/p%201/agents/a%2Fb/environments/dev/a2a-connect`);
});
it("names the missing parameter", () => {
  expect(() => agentEnvironmentResourcePath({ ...params, envId: "" }, "agent-card"))
    .toThrow("Missing required parameter: envId");
});
it("keeps the A2A proxy URL", async () => {
  await getA2AProxy(params, async () => "t");
  expect(httpGET).toHaveBeenCalledWith(
    `${SERVICE_BASE}/orgs/acme/projects/p%201/agents/a%2Fb/environments/dev/a2a-proxy`,
    { token: "t" },
  );
});
