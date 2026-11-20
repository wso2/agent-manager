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
import { fromExpiryTime, toExpiryTime } from "./expiryTime";

describe("expiry time", () => {
  it("sends the end of the picked day in local time", () => {
    const sent = new Date(toExpiryTime(new Date(2026, 11, 31)));
    expect([sent.getFullYear(), sent.getMonth(), sent.getDate()]).toEqual([2026, 11, 31]);
    expect([sent.getHours(), sent.getMinutes(), sent.getSeconds()]).toEqual([23, 59, 59]);
    expect(sent.getMilliseconds()).toBe(999);
  });

  it("ignores the time of day the picker returns", () => {
    expect(toExpiryTime(new Date(2026, 11, 31, 8, 30))).toBe(toExpiryTime(new Date(2026, 11, 31)));
  });

  it("loads a stored expiry back as the same local day", () => {
    const day = fromExpiryTime(toExpiryTime(new Date(2026, 11, 31)));
    expect(day).toEqual(new Date(2026, 11, 31));
  });
});
