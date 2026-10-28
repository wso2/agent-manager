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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { afterEach, describe, expect, it } from "vitest";
import { getBasePath, stripBasePath, withBasePath } from "@agent-management-platform/types";

// The helpers read window.__RUNTIME_CONFIG__ (config.js) through the object
// setupTests.tsx creates, so each case sets basePath on that same object.
const config = (window as unknown as { __RUNTIME_CONFIG__: { basePath?: string } })
  .__RUNTIME_CONFIG__;
const setBase = (basePath: string | undefined) => {
  config.basePath = basePath;
};

afterEach(() => setBase(undefined));

describe("getBasePath", () => {
  it.each([undefined, "", "/", "//", "  /  "])("is empty at the origin root (%j)", (raw) => {
    setBase(raw);
    expect(getBasePath()).toBe("");
  });

  it.each(["/console", "console", "/console/", " /console// "])("normalises %j", (raw) => {
    setBase(raw);
    expect(getBasePath()).toBe("/console");
  });

  it("keeps nested prefixes", () => {
    setBase("/a/b/");
    expect(getBasePath()).toBe("/a/b");
  });
});

describe("withBasePath / stripBasePath", () => {
  it("are the identity at the origin root", () => {
    setBase("/");
    expect(withBasePath("/org/x")).toBe("/org/x");
    expect(stripBasePath("/org/x")).toBe("/org/x");
  });

  it("prefix router paths and strip browser paths", () => {
    setBase("/console");
    expect(withBasePath("/")).toBe("/console/");
    expect(withBasePath("/org/x?tab=a#h")).toBe("/console/org/x?tab=a#h");
    expect(withBasePath("org/x")).toBe("/console/org/x");
    expect(stripBasePath("/console")).toBe("/");
    expect(stripBasePath("/console/")).toBe("/");
    expect(stripBasePath("/console/org/x")).toBe("/org/x");
  });

  it("always prefix, even a route that starts with the base", () => {
    setBase("/login");
    expect(withBasePath("/login")).toBe("/login/login");
    expect(stripBasePath("/login/login")).toBe("/login");
  });

  it("leave paths outside the base alone when stripping", () => {
    setBase("/console");
    expect(stripBasePath("/consolex/org")).toBe("/consolex/org");
    expect(stripBasePath("/apim/")).toBe("/apim/");
  });
});
