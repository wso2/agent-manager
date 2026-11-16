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

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import {
  LIVENESS_CHECK_DEFAULTS,
  READINESS_CHECK_DEFAULTS,
  STARTUP_CHECK_DEFAULTS,
  describeHealthChecksInEffect,
  healthCheckFieldErrors,
  healthChecksInEffect,
  healthChecksAreValid,
  healthChecksToSend,
  suggestedHealthCheckPath,
} from "@agent-management-platform/types";
import { HealthCheckFields } from "./HealthCheckFields";

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

  // A check saved as {} (through the API or amctl) is on. Liveness is off by default, so
  // dropping it would turn liveness off when the saved checks are replaced.
  it("keeps an empty liveness check, which means on", () => {
    expect(healthChecksToSend({ liveness: {}, readiness: { periodSeconds: 10 } })).toEqual({
      liveness: {},
      readiness: { periodSeconds: 10 },
    });
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

  // Without a path an HTTP check would quietly check "/", so a path is required.
  it("requires a path for an HTTP check", () => {
    expect(
      healthCheckFieldErrors({ type: "HTTP" }, "readiness", READINESS_CHECK_DEFAULTS).path,
    ).toBe("Path is required for HTTP GET");
    expect(healthCheckFieldErrors({ type: "TCP" }, "readiness", READINESS_CHECK_DEFAULTS)).toEqual({});
    // A check switched off never runs, so it needs no path.
    expect(
      healthCheckFieldErrors({ type: "HTTP", enabled: false }, "liveness", READINESS_CHECK_DEFAULTS),
    ).toEqual({});
  });

  // The backend rejects ".." in a path, so the form must too.
  it("rejects a path with ..", () => {
    const check = { type: "HTTP" as const, path: "/a/../health" };
    expect(healthCheckFieldErrors(check, "readiness", READINESS_CHECK_DEFAULTS).path).toEqual(
      expect.stringContaining('".."'),
    );
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
  // Readiness and liveness act after interval × failures allowed (5s × 720 = an hour here);
  // the initial delay does not count.
  it("caps readiness at an hour of interval × failures", () => {
    const readiness = (check: Parameters<typeof healthCheckFieldErrors>[0]) =>
      healthCheckFieldErrors(check, "readiness", READINESS_CHECK_DEFAULTS);
    expect(readiness({ initialDelaySeconds: 600, failureThreshold: 720 })).toEqual({});
    expect(readiness({ failureThreshold: 721 }).failureThreshold).toContain("3600");
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

describe("healthChecksInEffect", () => {
  // An environment with no health checks runs today's defaults: 2 of 3 on.
  it("fills in the defaults when nothing is set", () => {
    const checks = healthChecksInEffect(undefined);
    expect(checks.map((check) => [check.checkName, check.enabled, check.type])).toEqual([
      ["startup", true, "TCP"],
      ["readiness", true, "TCP"],
      ["liveness", false, "TCP"],
    ]);
    expect(checks[0]).toMatchObject({
      initialDelaySeconds: 10,
      periodSeconds: 5,
      failureThreshold: 60,
    });
    expect(describeHealthChecksInEffect(checks)).toBe(
      "Startup: on, TCP · Readiness: on, TCP · Liveness: off",
    );
  });

  // A liveness check that is set is on unless switched off, as the template treats it.
  it("shows the set checks, with an HTTP check without a path at /", () => {
    const checks = healthChecksInEffect({
      startup: { enabled: false },
      readiness: { type: "HTTP", periodSeconds: 3 },
      liveness: { type: "TCP" },
    });
    expect(describeHealthChecksInEffect(checks)).toBe(
      "Startup: off · Readiness: on, HTTP GET / · Liveness: on, TCP",
    );
    expect(checks[1]).toMatchObject({ periodSeconds: 3, timeoutSeconds: 1, failureThreshold: 6 });
  });
});

describe("HealthCheckFields", () => {
  // A lone "-" would otherwise be stored as NaN.
  it("ignores anything but digits in a number field", () => {
    const onChange = vi.fn();
    render(
      <HealthCheckFields
        checkName="readiness"
        title="Readiness"
        value={undefined}
        onChange={onChange}
        defaults={READINESS_CHECK_DEFAULTS}
      />,
    );
    fireEvent.change(screen.getByLabelText("Readiness interval (s)"), { target: { value: "-" } });
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("Readiness interval (s)"), { target: { value: "10" } });
    expect(onChange).toHaveBeenLastCalledWith({ periodSeconds: 10 });
  });

  // Clearing the last field of liveness (off by default) must not turn it off.
  it("keeps liveness on when its last field is cleared", () => {
    const onChange = vi.fn();
    render(
      <HealthCheckFields
        checkName="liveness"
        title="Liveness"
        value={{ port: 9000 }}
        onChange={onChange}
        defaults={LIVENESS_CHECK_DEFAULTS}
      />,
    );
    fireEvent.change(screen.getByLabelText("Liveness port"), { target: { value: "" } });
    expect(onChange).toHaveBeenLastCalledWith({});
  });

  // For readiness (on by default), unset already means on, so clearing leaves it unset.
  it("leaves readiness unset when its last field is cleared", () => {
    const onChange = vi.fn();
    render(
      <HealthCheckFields
        checkName="readiness"
        title="Readiness"
        value={{ port: 9000 }}
        onChange={onChange}
        defaults={READINESS_CHECK_DEFAULTS}
      />,
    );
    fireEvent.change(screen.getByLabelText("Readiness port"), { target: { value: "" } });
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });

  // A check that is off is still validated, so an error must stay in view.
  it("keeps a switched-off check open while it has an error", () => {
    render(
      <HealthCheckFields
        checkName="readiness"
        title="Readiness"
        value={{ enabled: false, periodSeconds: 0 }}
        onChange={() => undefined}
        defaults={READINESS_CHECK_DEFAULTS}
      />,
    );
    expect(screen.getByLabelText("Readiness interval (s)")).toBeVisible();
    expect(screen.getByText("Must be 1 or more")).toBeVisible();
  });
});
