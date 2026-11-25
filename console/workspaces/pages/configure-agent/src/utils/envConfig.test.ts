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

import { describe, expect, it, vi } from "vitest";

// envConfig re-exports from shared-component; mocking it avoids loading the whole component
// library (and its API client) for a pure-function test.
vi.mock("@agent-management-platform/shared-component", () => ({
  ENV_VAR_KEYS: ["url", "apikey"],
}));

import {
  ENV_VAR_KEYS,
  LLM_ENV_VAR_KEYS,
  generateEnvVarNames,
  generateLLMEnvVarNames,
} from "./envConfig";

describe("envConfig", () => {
  it("keeps the MCP key list to url and apikey", () => {
    expect(ENV_VAR_KEYS).toEqual(["url", "apikey"]);
    expect(generateEnvVarNames("my-mcp")).toEqual({
      url: "MY_MCP_URL",
      apikey: "MY_MCP_API_KEY",
    });
  });

  it("adds the provider key for LLM configurations only", () => {
    expect(LLM_ENV_VAR_KEYS).toEqual(["url", "apikey", "provider"]);
  });

  it("names the provider variable after the config like the other two", () => {
    expect(generateLLMEnvVarNames("openai")).toEqual({
      url: "OPENAI_URL",
      apikey: "OPENAI_API_KEY",
      provider: "OPENAI_PROVIDER",
    });
  });

  it("sanitises the prefix and falls back to bare names when it is empty", () => {
    expect(generateLLMEnvVarNames("1-my llm").provider).toBe("_1_MY_LLM_PROVIDER");
    expect(generateLLMEnvVarNames("")).toEqual({
      url: "URL",
      apikey: "API_KEY",
      provider: "PROVIDER",
    });
  });
});
