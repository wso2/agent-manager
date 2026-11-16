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
import {
  getExternalProjectManageUrl,
  isProjectManagedExternally,
} from "@agent-management-platform/types";

// The helpers read window.__RUNTIME_CONFIG__ (config.js) through the object
// setupTests.tsx creates, so each case sets the URL on that same object.
const config = (
  window as unknown as { __RUNTIME_CONFIG__: { externalProjectManagementUrl?: string } }
).__RUNTIME_CONFIG__;
const setUrl = (url: string | undefined) => {
  config.externalProjectManagementUrl = url;
};

afterEach(() => setUrl(undefined));

describe("external project management unset or unusable", () => {
  it.each([
    undefined,
    "",
    "   ",
    // envsubst leaves the placeholder empty, never unexpanded, but a bare host
    // or relative path would be followed as a path inside this console.
    "console.example.com",
    "console.example.com/",
    "/projects",
    "ftp://console.example.com",
    "https://",
    "https:///path",
  ])("manages projects in this console (%j)", (raw) => {
    setUrl(raw);
    expect(isProjectManagedExternally()).toBe(false);
    expect(getExternalProjectManageUrl("my-project")).toBeUndefined();
  });
});

describe("external project management set", () => {
  it.each([
    ["https://console.example.com", "https://console.example.com"],
    ["http://localhost:3001", "http://localhost:3001"],
    ["HTTPS://Console.Example.com", "HTTPS://Console.Example.com"],
    ["https://console.example.com/", "https://console.example.com"],
    ["https://console.example.com///", "https://console.example.com"],
    ["  https://console.example.com/  ", "https://console.example.com"],
    ["https://example.com/cloud/", "https://example.com/cloud"],
  ])("builds the settings URL from %j", (raw, host) => {
    setUrl(raw);
    expect(isProjectManagedExternally()).toBe(true);
    expect(getExternalProjectManageUrl("my-project")).toBe(
      `${host}/projects/my-project/settings`,
    );
  });

  it("encodes the project handle", () => {
    setUrl("https://console.example.com");
    expect(getExternalProjectManageUrl("a b/c?d#e")).toBe(
      "https://console.example.com/projects/a%20b%2Fc%3Fd%23e/settings",
    );
  });
});
