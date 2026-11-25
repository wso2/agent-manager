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
import { chipsThatFit } from "./Governance";

// Chips are 4px apart; the "+N" chip is 30px wide in these cases.
describe("chipsThatFit", () => {
  it("shows every chip when they all fit, with no room kept for +N", () => {
    // 50 + 4 + 50 + 4 + 50 = 158
    expect(chipsThatFit([50, 50, 50], 30, 158)).toBe(3);
  });

  it("keeps room for +N when some chips do not fit", () => {
    // +N (30), then 4 + 50 = 84, 4 + 50 = 138; a third would need 192.
    expect(chipsThatFit([50, 50, 50, 50], 30, 150)).toBe(2);
  });

  it("always shows one chip, so a single long name is cut rather than hidden", () => {
    expect(chipsThatFit([400, 50], 30, 120)).toBe(1);
  });
});
