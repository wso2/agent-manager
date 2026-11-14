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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it } from "vitest";
import { buildKeyManagerConfig } from "./gatewayKeyManagerConfig";

describe("buildKeyManagerConfig", () => {
  it("renders the gateway key manager entry", () => {
    expect(
      buildKeyManagerConfig({
        name: "corp",
        issuer: "https://idp.example.com",
        jwksUri: "https://idp.example.com/oauth2/jwks",
        skipTlsVerify: true,
      }),
    ).toBe(
      [
        "[[policy_configurations.jwtauth_v1.keymanagers]]",
        'name = "corp"',
        'issuer = "https://idp.example.com"',
        "[policy_configurations.jwtauth_v1.keymanagers.jwks.remote]",
        'uri = "https://idp.example.com/oauth2/jwks"',
        "skipTlsVerify = true",
      ].join("\n"),
    );
  });

  it("escapes quotes and backslashes so a value cannot break the TOML", () => {
    const out = buildKeyManagerConfig({
      name: 'a"b\\c',
      issuer: "i",
      jwksUri: "j",
      skipTlsVerify: false,
    });
    expect(out).toContain('name = "a\\"b\\\\c"');
    expect(out).toContain("skipTlsVerify = false");
  });

  it("shows placeholders for fields not filled in yet", () => {
    const out = buildKeyManagerConfig({ name: "", issuer: "", jwksUri: "", skipTlsVerify: false });
    expect(out).toContain('name = "<identity-provider-name>"');
    expect(out).toContain('uri = "<jwks-uri>"');
  });
});
