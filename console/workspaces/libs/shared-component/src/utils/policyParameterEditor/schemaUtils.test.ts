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
import { coerceValuesToSchemaTypes, initializeDefaultValues } from "./schemaUtils";
import type { ParameterSchema } from "./types";

// The request/response shape of the AWS Bedrock Guardrail policy, trimmed to
// the fields these tests touch.
const guardrailSchema: ParameterSchema = {
  type: "object",
  properties: {
    request: {
      type: "object",
      properties: {
        enabled: { type: "boolean", default: true },
        jsonPath: { type: "string", default: "$.messages[-1].content" },
        endpoint: { type: "string" },
      },
    },
    response: {
      type: "object",
      properties: {
        enabled: { type: "boolean", default: false },
        jsonPath: { type: "string", default: "$.choices[0].message.content" },
        streamingJsonPath: { type: "string" },
      },
    },
  },
};

describe("coerceValuesToSchemaTypes jsonPath", () => {
  it("sends an empty jsonPath, so the policy checks the whole payload", () => {
    const values = initializeDefaultValues(guardrailSchema);

    const result = coerceValuesToSchemaTypes(guardrailSchema, values);

    expect(result.request).toEqual({ enabled: true, jsonPath: "" });
    expect(result.response).toEqual({ enabled: false, jsonPath: "" });
  });

  it("sends a whitespace-only jsonPath as empty", () => {
    const result = coerceValuesToSchemaTypes(guardrailSchema, {
      request: { enabled: true, jsonPath: "   " },
    });

    expect(result.request).toEqual({ enabled: true, jsonPath: "" });
  });

  it("keeps a jsonPath the user typed", () => {
    const result = coerceValuesToSchemaTypes(guardrailSchema, {
      request: { enabled: true, jsonPath: " $.messages[-1].content[0].text " },
    });

    expect(result.request).toEqual({
      enabled: true,
      jsonPath: "$.messages[-1].content[0].text",
    });
  });

  it("still drops other empty optional fields", () => {
    const result = coerceValuesToSchemaTypes(guardrailSchema, {
      request: { enabled: true, jsonPath: "", endpoint: "" },
      response: { enabled: true, jsonPath: "", streamingJsonPath: "" },
    });

    expect(result.request).not.toHaveProperty("endpoint");
    expect(result.response).not.toHaveProperty("streamingJsonPath");
  });
});
