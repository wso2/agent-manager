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

import { describe, expect, it } from "vitest";
import { subscriptionKeyCurlLine, subscriptionKeyHeaderOf } from "./subscriptionKey";

describe("subscription key header", () => {
  it("is the policy's header, or the default, only while subscription-validation is attached", () => {
    expect(subscriptionKeyHeaderOf(undefined)).toBeUndefined();
    expect(subscriptionKeyHeaderOf({ policies: [{ name: "mcp-acl-list" }] })).toBeUndefined();
    expect(subscriptionKeyHeaderOf({ policies: [{ name: "subscription-validation" }] })).toBe(
      "Subscription-Key",
    );
    expect(
      subscriptionKeyHeaderOf({
        policies: [{ name: "subscription-validation", params: { subscriptionKeyHeader: " X-Sub " } }],
      }),
    ).toBe("X-Sub");
    expect(
      subscriptionKeyHeaderOf({
        policies: [{ name: "subscription-validation", params: { subscriptionKeyHeader: "" } }],
      }),
    ).toBe("Subscription-Key");
  });

  it("goes into the example cURL with a placeholder key", () => {
    expect(subscriptionKeyCurlLine("Subscription-Key")).toBe(
      '  --header "Subscription-Key: <subscription-key>"',
    );
  });
});
