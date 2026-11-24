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
  getAgentConfigurations,
  getEnvironment,
  listAgentDeployments,
  listEnvironments,
  updateAgentDeploySettings,
} from "./deployments";

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

const agent = { orgName: "acme corp", projName: "p/1", agentName: "my bot" };
const AGENT = "/m/ai-gateway/orgs/acme%20corp/projects/p%2F1/agents/my%20bot";
const settings = { environmentName: "dev", enableAutoDeploy: true };

describeHttpCases("deployments API", mocks, [
  {
    name: "updateAgentDeploySettings",
    call: () => updateAgentDeploySettings(agent, settings as never, getToken),
    method: "PUT", url: `${AGENT}/deploy-settings`, body: settings, returnsBody: false,
  },
  {
    name: "listAgentDeployments",
    call: () => listAgentDeployments(agent, getToken),
    method: "GET", url: `${AGENT}/deployments`,
  },
  {
    name: "listAgentDeployments uses the default org and project",
    call: () => listAgentDeployments({ agentName: "a" } as never, getToken),
    method: "GET", url: "/m/ai-gateway/orgs/default/projects/default/agents/a/deployments",
  },
  {
    name: "getAgentConfigurations filters by environment",
    call: () => getAgentConfigurations(agent, { environment: "prod" }, getToken),
    method: "GET", url: `${AGENT}/configurations`, searchParams: { environment: "prod" },
  },
  {
    name: "listEnvironments reads from the core API",
    call: () => listEnvironments({ orgName: "acme corp" }, getToken),
    method: "GET", url: "/api/v1/orgs/acme%20corp/environments",
  },
  {
    name: "getEnvironment",
    call: () => getEnvironment({ orgName: "acme corp", envName: "dev 1" }, getToken),
    method: "GET", url: "/api/v1/orgs/acme%20corp/environments/dev%201",
  },
]);

describe("deployments API required params", () => {
  it.each([
    ["updateAgentDeploySettings", () => updateAgentDeploySettings({ ...agent, agentName: "" }, settings as never, getToken), "agentName is required"],
    ["listAgentDeployments", () => listAgentDeployments({ ...agent, agentName: "" }, getToken), "agentName is required"],
    ["getAgentConfigurations", () => getAgentConfigurations({ ...agent, agentName: "" }, { environment: "dev" }, getToken), "agentName is required"],
    ["getEnvironment", () => getEnvironment({ orgName: "o", envName: "" }, getToken), "envName is required"],
  ])("%s refuses before any request", async (_name, call, message) => {
    mocks.GET.mockReset();
    mocks.PUT.mockReset();
    await expect(call()).rejects.toThrow(message);
    expect(mocks.GET).not.toHaveBeenCalled();
    expect(mocks.PUT).not.toHaveBeenCalled();
  });
});
