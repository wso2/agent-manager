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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it } from "vitest";
import { validateEndpointUrl } from "./url";

describe("validateEndpointUrl", () => {
  it.each([
    ["https://api.example.com/v1", null],
    ["  http://localhost:8080  ", null],
    ["", "URL is required"],
    ["   ", "URL is required"],
    ["not a url", "Please enter a valid URL"],
    ["ftp://example.com", "URL must use http or https"],
  ])("%j -> %j", (value, expected) => {
    expect(validateEndpointUrl(value)).toBe(expected);
  });

  it("uses caller-supplied messages", () => {
    const options = { requiredMessage: "R", invalidMessage: "I", protocolMessage: "P" };
    expect(validateEndpointUrl("", options)).toBe("R");
    expect(validateEndpointUrl("::", options)).toBe("I");
    expect(validateEndpointUrl("ws://x", options)).toBe("P");
  });
});
