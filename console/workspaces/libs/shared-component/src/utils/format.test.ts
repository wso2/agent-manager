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
import { capitalize, formatRelativeTime, normalizeVersion } from "./format";

describe("normalizeVersion", () => {
  it.each([
    [undefined, undefined],
    ["", ""],
    ["1.0.0", "v1.0.0"],
    ["v2", "v2"],
    ["V3", "V3"],
  ])("%j -> %j", (value, expected) => {
    expect(normalizeVersion(value)).toBe(expected);
  });
});

describe("capitalize", () => {
  it.each([
    ["", ""],
    ["active", "Active"],
    ["Already", "Already"],
  ])("%j -> %j", (value, expected) => {
    expect(capitalize(value)).toBe(expected);
  });
});

describe("formatRelativeTime", () => {
  it.each([undefined, "", "not a date"])("returns the fallback for %j", (value) => {
    expect(formatRelativeTime(value)).toBe("-");
    expect(formatRelativeTime(value, { fallback: "never" })).toBe("never");
  });

  it("formats recent dates in relative wording", () => {
    expect(formatRelativeTime(new Date())).toMatch(/just now/i);
    expect(formatRelativeTime(Date.now() - 2 * 60 * 60 * 1000)).toMatch(/2 hours ago/i);
  });
});
