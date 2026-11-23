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
import { getErrorMessage } from "./errorHelpers";

describe("getErrorMessage", () => {
  it.each([
    ["undefined", undefined, ""],
    ["null", null, ""],
    ["empty string", "", ""],
    ["a string", "boom", "boom"],
    ["an Error", new Error("bad thing"), "bad thing"],
    ["an object with message", { message: "from api" }, "from api"],
    ["an object with a non-string message", { message: 42 }, "An unknown error occurred."],
    ["a number", 7, "An unknown error occurred."],
  ])("%s", (_label, error, expected) => {
    expect(getErrorMessage(error)).toBe(expected);
  });
});
