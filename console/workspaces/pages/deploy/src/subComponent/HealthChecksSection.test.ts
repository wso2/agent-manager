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
import { describeCheck, describeWaitTimes } from "./HealthChecksSection";

describe("describeCheck", () => {
  it("says Off for a check that is off or missing", () => {
    expect(describeCheck({ enabled: false, type: "http", path: "/health" })).toBe("Off");
    expect(describeCheck(undefined)).toBe("Off");
  });

  it("describes a TCP check on the agent's port", () => {
    expect(describeCheck({ enabled: true, type: "tcp" })).toBe("On · TCP on the agent's port");
  });

  it("describes an HTTP GET check on a set port", () => {
    expect(describeCheck({ enabled: true, type: "http", path: "/ready", port: 8000 })).toBe(
      "On · HTTP GET /ready on port 8000",
    );
  });
});

describe("describeWaitTimes", () => {
  it("lists every wait time", () => {
    expect(
      describeWaitTimes({
        enabled: true,
        initialDelaySeconds: 10,
        periodSeconds: 5,
        timeoutSeconds: 1,
        failureThreshold: 60,
      }),
    ).toBe("After 10s, every 5s, timeout 1s, 60 failures allowed");
  });

  it("leaves out a wait time the response does not have", () => {
    expect(describeWaitTimes({ enabled: true, periodSeconds: 5, failureThreshold: 6 })).toBe(
      "Every 5s, 6 failures allowed",
    );
  });
});
