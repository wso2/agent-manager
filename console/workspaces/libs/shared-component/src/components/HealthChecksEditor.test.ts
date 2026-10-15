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
import type { HealthChecks } from "@agent-management-platform/types";
import {
  livenessWarning,
  toHealthChecksForm,
  toHealthChecksPayload,
  validateHealthChecksForm,
} from "./HealthChecksEditor";

// The platform defaults (agent-api.yaml, parameters.probes).
const DEFAULT_HEALTH_CHECKS: HealthChecks = {
  startup: {
    enabled: true,
    type: "tcp",
    path: "/health",
    initialDelaySeconds: 10,
    periodSeconds: 5,
    timeoutSeconds: 1,
    failureThreshold: 60,
  },
  readiness: {
    enabled: true,
    type: "tcp",
    path: "/health",
    initialDelaySeconds: 0,
    periodSeconds: 5,
    timeoutSeconds: 1,
    failureThreshold: 6,
  },
  liveness: {
    enabled: false,
    type: "tcp",
    path: "/health",
    initialDelaySeconds: 0,
    periodSeconds: 10,
    timeoutSeconds: 1,
    failureThreshold: 3,
  },
};

describe("toHealthChecksForm", () => {
  it("seeds every check from the agent's health checks", () => {
    const form = toHealthChecksForm(DEFAULT_HEALTH_CHECKS);

    expect(form.startup).toEqual({
      enabled: true,
      type: "tcp",
      path: "/health",
      port: NaN,
      initialDelaySeconds: 10,
      periodSeconds: 5,
      timeoutSeconds: 1,
      failureThreshold: 60,
    });
    expect(form.liveness.enabled).toBe(false);
  });

  it("starts a missing check off, as TCP on /health", () => {
    const form = toHealthChecksForm({});

    expect(form.readiness.enabled).toBe(false);
    expect(form.readiness.type).toBe("tcp");
    expect(form.readiness.path).toBe("/health");
  });
});

describe("toHealthChecksPayload", () => {
  it("leaves the port out when it is empty, so the agent's port is used", () => {
    const payload = toHealthChecksPayload(toHealthChecksForm(DEFAULT_HEALTH_CHECKS));

    expect(payload.startup).not.toHaveProperty("port");
    expect(payload.startup?.failureThreshold).toBe(60);
  });

  it("leaves out empty wait times instead of sending null", () => {
    const form = toHealthChecksForm(DEFAULT_HEALTH_CHECKS);
    form.readiness = { ...form.readiness, timeoutSeconds: NaN };

    const payload = toHealthChecksPayload(form);

    expect(payload.readiness).not.toHaveProperty("timeoutSeconds");
    expect(payload.readiness?.periodSeconds).toBe(5);
    expect(JSON.stringify(payload)).not.toContain("null");
  });

  it("sends only that a check is off, not its unvalidated values", () => {
    const form = toHealthChecksForm(DEFAULT_HEALTH_CHECKS);
    // Out of range, but the check is off, so validation does not stop it.
    form.liveness = { ...form.liveness, enabled: false, periodSeconds: 0, path: "bad path" };

    expect(validateHealthChecksForm(form)).toEqual({});
    expect(toHealthChecksPayload(form).liveness).toEqual({ enabled: false });
  });

  it("sends a set port and a trimmed path", () => {
    const form = toHealthChecksForm(DEFAULT_HEALTH_CHECKS);
    form.readiness = { ...form.readiness, type: "http", path: " /health ", port: 8000 };

    const payload = toHealthChecksPayload(form);

    expect(payload.readiness).toMatchObject({ type: "http", path: "/health", port: 8000 });
  });
});

describe("validateHealthChecksForm", () => {
  it("accepts the defaults", () => {
    expect(validateHealthChecksForm(toHealthChecksForm(DEFAULT_HEALTH_CHECKS))).toEqual({});
  });

  it("requires a path starting with / for an HTTP check", () => {
    const form = toHealthChecksForm(DEFAULT_HEALTH_CHECKS);
    form.readiness = { ...form.readiness, type: "http", path: "health" };

    expect(validateHealthChecksForm(form).readiness?.path).toBe("Path must start with /");
  });

  it("rejects a path with a space or over 1024 characters", () => {
    const form = toHealthChecksForm(DEFAULT_HEALTH_CHECKS);
    form.readiness = { ...form.readiness, type: "http", path: "/he alth" };
    form.liveness = {
      ...form.liveness, enabled: true, type: "http", path: "/" + "a".repeat(1024),
    };

    const errors = validateHealthChecksForm(form);

    expect(errors.readiness?.path).toBe("Path must not contain spaces");
    expect(errors.liveness?.path).toBe("Path must be at most 1024 characters");
  });

  it("rejects a port out of range and an empty wait time", () => {
    const form = toHealthChecksForm(DEFAULT_HEALTH_CHECKS);
    form.startup = { ...form.startup, port: 70000, timeoutSeconds: NaN };

    const errors = validateHealthChecksForm(form).startup;

    expect(errors?.port).toBe("Must be between 1 and 65535");
    expect(errors?.timeoutSeconds).toBe("Enter a whole number");
  });

  it("limits the startup window to an hour", () => {
    const form = toHealthChecksForm(DEFAULT_HEALTH_CHECKS);
    // 10s + 10s x 400 = 4010s.
    form.startup = { ...form.startup, periodSeconds: 10, failureThreshold: 400 };

    expect(validateHealthChecksForm(form).startup?.failureThreshold).toBe(
      "Allows 66 min 50s to start; the maximum is 60 min",
    );
  });

  it("counts a timeout longer than the interval in the startup window", () => {
    const form = toHealthChecksForm(DEFAULT_HEALTH_CHECKS);
    // 10s + 60 x 100s = 6010s.
    form.startup = { ...form.startup, timeoutSeconds: 100 };

    expect(validateHealthChecksForm(form).startup?.failureThreshold).toBe(
      "Allows 100 min 10s to start; the maximum is 60 min",
    );
  });

  it("does not check a check that is off", () => {
    const form = toHealthChecksForm(DEFAULT_HEALTH_CHECKS);
    form.liveness = { ...form.liveness, enabled: false, periodSeconds: 0 };

    expect(validateHealthChecksForm(form)).toEqual({});
  });
});

describe("livenessWarning", () => {
  it("warns when liveness is on without the startup check", () => {
    const form = toHealthChecksForm(DEFAULT_HEALTH_CHECKS);
    form.startup = { ...form.startup, enabled: false };
    form.liveness = { ...form.liveness, enabled: true };

    expect(livenessWarning(form)).toContain("restarted");
  });

  it("does not warn with the startup check on, or liveness off", () => {
    const form = toHealthChecksForm(DEFAULT_HEALTH_CHECKS);
    form.liveness = { ...form.liveness, enabled: true };
    expect(livenessWarning(form)).toBeUndefined();

    form.startup = { ...form.startup, enabled: false };
    form.liveness = { ...form.liveness, enabled: false };
    expect(livenessWarning(form)).toBeUndefined();
  });
});
