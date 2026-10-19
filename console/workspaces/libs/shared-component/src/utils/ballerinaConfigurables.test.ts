/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import { describe, expect, it } from "vitest";
import {
  AGENTID_BALLERINA_CONFIGURABLE_ROWS,
  agentIdEnvVarRows,
  ballerinaConfigurableFor,
  ballerinaConfigurableSnippet,
  ballerinaConfigVarName,
} from "./ballerinaConfigurables";

describe("ballerinaConfigVarName", () => {
  // The mapping documented at ballerina.io: upper-cased, no separators added.
  it("upper-cases the configurable name without inserting separators", () => {
    expect(ballerinaConfigVarName("isAdmin")).toBe("BAL_CONFIG_VAR_ISADMIN");
    expect(ballerinaConfigVarName("myAgent1Url")).toBe("BAL_CONFIG_VAR_MYAGENT1URL");
  });
});

describe("agentIdEnvVarRows", () => {
  it("lists the plain AMP_AGENTID_* names when the setting is off", () => {
    expect(agentIdEnvVarRows(false).map((r) => r.name)).toEqual([
      "AMP_AGENTID_CLIENT_ID",
      "AMP_AGENTID_CLIENT_SECRET",
      "AMP_AGENTID_TOKEN_ENDPOINT",
      "AMP_AGENTID_SCOPES",
    ]);
    expect(agentIdEnvVarRows(undefined)).toHaveLength(4);
  });

  // Must match BalConfigVarAgentID* in agent-manager-service.
  it("lists only the configurables' BAL_CONFIG_VAR_ names when the setting is on", () => {
    expect(agentIdEnvVarRows(true).map((r) => r.name)).toEqual([
      "BAL_CONFIG_VAR_AMPAGENTIDCLIENTID",
      "BAL_CONFIG_VAR_AMPAGENTIDCLIENTSECRET",
      "BAL_CONFIG_VAR_AMPAGENTIDTOKENENDPOINT",
      "BAL_CONFIG_VAR_AMPAGENTIDSCOPES",
    ]);
  });

  it("names the AgentID configurables in camelCase", () => {
    expect(AGENTID_BALLERINA_CONFIGURABLE_ROWS.map((r) => r.name)).toEqual([
      "ampAgentidClientId",
      "ampAgentidClientSecret",
      "ampAgentidTokenEndpoint",
      "ampAgentidScopes",
    ]);
  });
});

describe("ballerinaConfigurableFor", () => {
  it("returns the camelCase name a var was generated from", () => {
    expect(ballerinaConfigurableFor("BAL_CONFIG_VAR_MYAGENT1URL", "myAgent1Url")).toBe("myAgent1Url");
  });

  // Verified against Ballerina 2201.13.6: BAL_CONFIG_VAR_INJECT1_APIKEY binds
  // `inject1_apikey`, but leaves `inject1Apikey` unused and the program exits.
  it("formats a renamed var's suffix as a lower-case identifier, keeping underscores", () => {
    expect(ballerinaConfigurableFor("BAL_CONFIG_VAR_INJECT1_APIKEY", "inject1ApiKey")).toBe(
      "inject1_apikey",
    );
    expect(ballerinaConfigurableFor("BAL_CONFIG_VAR_INJECT1APIKEY")).toBe("inject1apikey");
    expect(ballerinaConfigVarName(ballerinaConfigurableFor("BAL_CONFIG_VAR_OPENAI_URL")!)).toBe(
      "BAL_CONFIG_VAR_OPENAI_URL",
    );
  });

  it("is undefined for a name Ballerina does not bind", () => {
    expect(ballerinaConfigurableFor("OPENAI_URL")).toBeUndefined();
    expect(ballerinaConfigurableFor("BAL_CONFIG_VAR_")).toBeUndefined();
  });
});

describe("ballerinaConfigurableSnippet", () => {
  it("declares each configurable as a string defaulting to empty", () => {
    expect(ballerinaConfigurableSnippet(["myAgent1Url", "myAgent1ApiKey"])).toBe(
      'configurable string myAgent1Url = "";\nconfigurable string myAgent1ApiKey = "";',
    );
    expect(ballerinaConfigurableSnippet([])).toBe("");
  });
});
