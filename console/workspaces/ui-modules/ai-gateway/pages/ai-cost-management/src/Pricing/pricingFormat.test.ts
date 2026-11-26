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
import {
  formatPrice,
  formatUpdated,
  NO_VALUE,
} from "./pricingFormat";

describe("pricingFormat", () => {
  it("writes a price as the exact string with a dollar sign", () => {
    expect(formatPrice("2.50")).toBe("$2.50");
    expect(formatPrice("0.075")).toBe("$0.075");
    expect(formatPrice("0.00")).toBe("$0.00");
  });

  it("writes a dash where there is no price", () => {
    expect(formatPrice(null)).toBe(NO_VALUE);
    expect(formatPrice(undefined)).toBe(NO_VALUE);
    expect(formatPrice("")).toBe(NO_VALUE);
  });

  it("writes the update time, or nothing without one", () => {
    expect(formatUpdated("2026-11-25T08:00:00Z")).toMatch(/Nov 25, 2026/);
    expect(formatUpdated(null)).toBe("");
    expect(formatUpdated("not a date")).toBe("");
  });
});
