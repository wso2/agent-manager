/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import { afterEach, describe, expect, it } from "vitest";
import { isIdentityPromotionUiEnabled } from "@agent-management-platform/types";

// The helper reads window.__RUNTIME_CONFIG__ (config.js) through the object
// setupTests.tsx creates, so each case sets the flag on that same object.
const config = (window as unknown as { __RUNTIME_CONFIG__: { featureFlags?: unknown } })
  .__RUNTIME_CONFIG__;

afterEach(() => {
  delete config.featureFlags;
});

describe("isIdentityPromotionUiEnabled", () => {
  it("is false when featureFlags is missing", () => {
    delete config.featureFlags;
    expect(isIdentityPromotionUiEnabled()).toBe(false);
  });

  it("is false when the flag is missing", () => {
    config.featureFlags = { enableAgentIdentity: true };
    expect(isIdentityPromotionUiEnabled()).toBe(false);
  });

  it.each([
    ["false", false],
    ["an unsubstituted placeholder", "$FEATURE_FLAG_ENABLE_IDENTITY_PROMOTION_UI"],
    ["the string true", "true"],
    ["a truthy number", 1],
    ["null", null],
  ])("is false for %s", (_label, value) => {
    config.featureFlags = { enableIdentityPromotionUi: value };
    expect(isIdentityPromotionUiEnabled()).toBe(false);
  });

  it("is true only for a literal true", () => {
    config.featureFlags = { enableIdentityPromotionUi: true };
    expect(isIdentityPromotionUiEnabled()).toBe(true);
  });

  it("does not depend on the Agent ID flag", () => {
    config.featureFlags = { enableAgentIdentity: false, enableIdentityPromotionUi: true };
    expect(isIdentityPromotionUiEnabled()).toBe(true);
  });
});
