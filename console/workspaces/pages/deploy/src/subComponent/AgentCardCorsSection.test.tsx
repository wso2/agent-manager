/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
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

import { describe, it, expect } from "vitest";
import { cardDraftFromConfig, cardDraftToPayload, isCardDraftInvalid } from "./AgentCardCorsSection";

describe("agent card CORS draft", () => {
  it("defaults to inherit when nothing is stored", () => {
    expect(cardDraftFromConfig(undefined).mode).toBe("inherit");
    expect(cardDraftToPayload(cardDraftFromConfig(undefined))).toEqual({ inherit: true });
  });

  it("seeds a custom override from the server", () => {
    const draft = cardDraftFromConfig({
      inherit: false, enabled: true, allowOrigin: ["https://a.example"],
      allowHeaders: ["Content-Type"], allowCredentials: true,
    });
    expect(draft).toMatchObject({
      mode: "custom", enabled: true, allowAll: false,
      origins: ["https://a.example"], allowCredentials: true,
    });
  });

  it("sends wildcard origins without credentials", () => {
    const draft = { ...cardDraftFromConfig(undefined), mode: "custom" as const, enabled: true, allowAll: true, allowCredentials: true };
    expect(cardDraftToPayload(draft)).toEqual({
      inherit: false, enabled: true, allowOrigin: ["*"], allowHeaders: ["Content-Type"], allowCredentials: false,
    });
  });

  it("sends a typed \"*\" origin as a wildcard without credentials", () => {
    const draft = {
      ...cardDraftFromConfig(undefined), mode: "custom" as const, enabled: true, allowAll: false,
      origins: ["https://a.example", "*"], allowCredentials: true,
    };
    expect(cardDraftToPayload(draft)).toEqual({
      inherit: false, enabled: true, allowOrigin: ["*"], allowHeaders: ["Content-Type"], allowCredentials: false,
    });
  });

  it("is invalid when custom, enabled and no origins are listed", () => {
    const draft = { ...cardDraftFromConfig(undefined), mode: "custom" as const, enabled: true, allowAll: false, origins: [] };
    expect(isCardDraftInvalid(draft)).toBe(true);
    expect(isCardDraftInvalid({ ...draft, enabled: false })).toBe(false);
    expect(isCardDraftInvalid({ ...draft, mode: "inherit" })).toBe(false);
  });
});
