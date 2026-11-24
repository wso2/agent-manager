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

import { describe, expect, it, vi } from "vitest";
import { httpDELETE, httpGET, httpPOST, httpPUT } from "../../shared/utils";
import { describeHttpCases, getToken, type HttpMocks } from "../../test-utils/http-cases";
import {
  createAgentAPIKey, issueTestAgentAPIKey, listAgentAPIKeys, revokeAgentAPIKey, rotateAgentAPIKey,
} from "./agent-api-keys";

vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../shared/utils")>()),
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));

const mocks = {
  GET: vi.mocked(httpGET),
  POST: vi.mocked(httpPOST),
  PUT: vi.mocked(httpPUT),
  DELETE: vi.mocked(httpDELETE),
} as unknown as HttpMocks;

const env = { orgName: "acme corp", projName: "p/1", agentName: "my bot", envId: "dev" };
const KEYS = "/m/ai-gateway/orgs/acme%20corp/projects/p%2F1/agents/my%20bot/environments/dev/api-keys";

describeHttpCases("agent API key API", mocks, [
  {
    name: "createAgentAPIKey",
    call: () => createAgentAPIKey(env, { name: "ci", expiresIn: "30d" } as never, getToken),
    method: "POST", url: KEYS, body: { name: "ci", expiresIn: "30d" },
  },
  { name: "listAgentAPIKeys", call: () => listAgentAPIKeys(env, getToken), method: "GET", url: KEYS },
  {
    name: "rotateAgentAPIKey",
    call: () => rotateAgentAPIKey({ ...env, keyName: "ci key" }, { expiresIn: "7d" } as never, getToken),
    method: "PUT", url: `${KEYS}/ci%20key`, body: { expiresIn: "7d" },
  },
  {
    name: "revokeAgentAPIKey",
    call: () => revokeAgentAPIKey({ ...env, keyName: "ci key" }, getToken),
    method: "DELETE", url: `${KEYS}/ci%20key`, returnsBody: false,
  },
  {
    name: "issueTestAgentAPIKey",
    call: () => issueTestAgentAPIKey(env, getToken),
    method: "POST", url: `${KEYS}/test`, body: {},
  },
]);

describe("agent API key required params", () => {
  it.each(["orgName", "projName", "agentName", "envId"] as const)(
    "refuses a missing %s before any request",
    async (missing) => {
      mocks.GET.mockReset();
      await expect(listAgentAPIKeys({ ...env, [missing]: "" }, getToken)).rejects.toThrow(missing);
      expect(mocks.GET).not.toHaveBeenCalled();
    },
  );

  it("refuses a missing key name", async () => {
    await expect(revokeAgentAPIKey({ ...env, keyName: "" }, getToken)).rejects.toThrow("keyName");
  });
});
