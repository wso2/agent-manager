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

import { describe, it, expect } from "vitest";
import {
  READINESS_CHECK_DEFAULTS,
  STARTUP_CHECK_DEFAULTS,
  healthCheckFieldErrors,
  healthChecksAreValid,
  healthChecksToSend,
  suggestedHealthCheckPath,
} from "@agent-management-platform/types";

describe("healthChecksToSend", () => {
  // Nothing set, or everything left at its default, means "use the defaults".
  it("sends nothing when no check differs from its default", () => {
    expect(healthChecksToSend(undefined)).toEqual({});
    expect(healthChecksToSend({})).toEqual({});
    expect(
      healthChecksToSend({
        startup: { enabled: true },
        readiness: {},
        liveness: { enabled: false },
      }),
    ).toEqual({});
  });

  it("keeps the fields that are set and drops the empty ones", () => {
    expect(
      healthChecksToSend({ readiness: { type: "HTTP", path: "/health", periodSeconds: 3 } }),
    ).toEqual({ readiness: { type: "HTTP", path: "/health", periodSeconds: 3 } });
  });

  // A path left over from switching HTTP back to TCP must not be sent.
  it("drops the path of a check that is not HTTP", () => {
    expect(healthChecksToSend({ readiness: { type: "TCP", path: "/health" } })).toEqual({
      readiness: { type: "TCP" },
    });
  });

  // A switched-off check is sent, since off differs from the default for startup and readiness,
  // and liveness switched on is sent, since off is its default.
  it("sends a check whose switch differs from its default", () => {
    const switchedChecks = { startup: { enabled: false }, liveness: { enabled: true } };
    expect(healthChecksToSend(switchedChecks)).toEqual({
      startup: { enabled: false },
      liveness: { enabled: true },
    });
  });
});

describe("suggestedHealthCheckPath", () => {
  it("suggests a path suited to the agent type", () => {
    expect(suggestedHealthCheckPath("DEFAULT")).toBe("/health");
    expect(suggestedHealthCheckPath("CUSTOM", "/api/v1/")).toBe("/api/v1/health");
    expect(suggestedHealthCheckPath("CUSTOM", "/")).toBe("/health");
    expect(suggestedHealthCheckPath("A2A")).toBe("/.well-known/agent-card.json");
  });
});

describe("healthCheckFieldErrors", () => {
  it("reports each field that breaks the backend's rules", () => {
    expect(
      healthCheckFieldErrors({
        type: "HTTP",
        path: "health",
        port: 70000,
        initialDelaySeconds: -1,
        periodSeconds: 0,
        timeoutSeconds: 0,
        failureThreshold: 0,
      }, "readiness", READINESS_CHECK_DEFAULTS),
    ).toEqual({
      path: expect.any(String),
      port: expect.any(String),
      initialDelaySeconds: expect.any(String),
      periodSeconds: expect.any(String),
      timeoutSeconds: expect.any(String),
      failureThreshold: expect.any(String),
    });
    const goodCheck = { type: "HTTP" as const, path: "/health", port: 8000 };
    expect(healthCheckFieldErrors(goodCheck, "readiness", READINESS_CHECK_DEFAULTS)).toEqual({});
  });

  // Startup may take at most an hour: initial delay + interval × failures allowed, with
  // unset fields at their defaults (10s delay, 5s interval for startup).
  it("caps the total startup time at an hour", () => {
    const startup = (failureThreshold: number, initialDelaySeconds?: number) =>
      healthCheckFieldErrors(
        { failureThreshold, initialDelaySeconds },
        "startup",
        STARTUP_CHECK_DEFAULTS,
      );
    expect(startup(720, 0)).toEqual({});
    expect(startup(720).failureThreshold).toContain("3610s");
  });

  // Readiness and liveness run as long as the agent does, so they have no total.
  it("puts no total on readiness", () => {
    const manyFailures = { failureThreshold: 100000 };
    expect(healthCheckFieldErrors(manyFailures, "readiness", READINESS_CHECK_DEFAULTS)).toEqual({});
  });

  it("caps each delay, interval and timeout, and keeps the timeout within the interval", () => {
    const readiness = (check: Parameters<typeof healthCheckFieldErrors>[0]) =>
      healthCheckFieldErrors(check, "readiness", READINESS_CHECK_DEFAULTS);
    expect(readiness({ periodSeconds: 3601 }).periodSeconds).toBeDefined();
    expect(readiness({ initialDelaySeconds: 3601 }).initialDelaySeconds).toBeDefined();
    expect(readiness({ timeoutSeconds: 10 }).timeoutSeconds).toContain("interval");
    expect(readiness({ timeoutSeconds: 5, periodSeconds: 5 })).toEqual({});
  });

  it("finds any invalid check in a set", () => {
    expect(healthChecksAreValid({ startup: { failureThreshold: 720 } })).toBe(false);
    expect(healthChecksAreValid({ readiness: { type: "HTTP", path: "/health" } })).toBe(true);
  });
});
