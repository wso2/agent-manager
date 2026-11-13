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

import { afterEach, describe, expect, it } from "vitest";
import { getScriptRef } from "./releaseRef";

// globalConfig reads window.__RUNTIME_CONFIG__ (config.js) through the object
// setupTests.tsx creates, so each case sets ampVersion on that same object.
const config = (window as unknown as { __RUNTIME_CONFIG__: Record<string, string | undefined> })
  .__RUNTIME_CONFIG__;

afterEach(() => {
  delete config.ampVersion;
});

describe("getScriptRef", () => {
  it("pins a release to its amp/v tag", () => {
    config.ampVersion = "v1.2.0";
    expect(getScriptRef()).toBe("amp/v1.2.0");
    config.ampVersion = "1.2.0";
    expect(getScriptRef()).toBe("amp/v1.2.0");
  });

  it("follows main when no release version is set", () => {
    expect(getScriptRef()).toBe("main");
    config.ampVersion = "0.0.0-dev";
    expect(getScriptRef()).toBe("main");
  });
});
