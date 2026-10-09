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
import { changedTimings, toTimingsForm, validateTimings } from "./HealthChecksSection";

// What an environment runs: startup and readiness on, liveness off.
const inEffect: HealthChecks = {
  startup: {
    enabled: true, type: "tcp",
    initialDelaySeconds: 10, periodSeconds: 5, timeoutSeconds: 1, failureThreshold: 80,
  },
  readiness: {
    enabled: true, type: "tcp",
    initialDelaySeconds: 0, periodSeconds: 5, timeoutSeconds: 1, failureThreshold: 6,
  },
  liveness: {
    enabled: false, type: "tcp",
    initialDelaySeconds: 0, periodSeconds: 10, timeoutSeconds: 1, failureThreshold: 3,
  },
};

describe("changedTimings", () => {
  it("sends nothing when no wait time changed", () => {
    const form = toTimingsForm(inEffect);

    expect(changedTimings(form, toTimingsForm(inEffect))).toBeUndefined();
  });

  it("does not count a wait time that is missing on both sides as changed", () => {
    const withoutTimeout: HealthChecks = {
      ...inEffect,
      startup: { ...inEffect.startup, timeoutSeconds: undefined },
    };

    expect(
      changedTimings(toTimingsForm(withoutTimeout), toTimingsForm(withoutTimeout)),
    ).toBeUndefined();
  });

  it("sends only the wait times the user changed", () => {
    const initial = toTimingsForm(inEffect);
    const form = { ...initial, readiness: { ...initial.readiness, periodSeconds: 10 } };

    expect(changedTimings(form, initial)).toEqual({ readiness: { periodSeconds: 10 } });
  });
});

describe("validateTimings", () => {
  it("accepts the values in effect", () => {
    expect(validateTimings(toTimingsForm(inEffect), inEffect)).toEqual({});
  });

  it("rejects an out-of-range wait time", () => {
    const form = toTimingsForm(inEffect);
    form.readiness = { ...form.readiness, periodSeconds: 0 };

    expect(validateTimings(form, inEffect).readiness?.periodSeconds).toBe(
      "Must be between 1 and 3600",
    );
  });

  it("limits the startup window to an hour", () => {
    const form = toTimingsForm(inEffect);
    // 10s + 60s x 80 = 4810s.
    form.startup = { ...form.startup, periodSeconds: 60 };

    expect(validateTimings(form, inEffect).startup?.failureThreshold).toContain(
      "the maximum is 60 min",
    );
  });

  it("counts a timeout longer than the interval in the startup window", () => {
    const form = toTimingsForm(inEffect);
    // 10s + 80 x 100s = 8010s.
    form.startup = { ...form.startup, timeoutSeconds: 100 };

    expect(validateTimings(form, inEffect).startup?.failureThreshold).toContain(
      "the maximum is 60 min",
    );
  });

  it("does not check a check that is off", () => {
    const form = toTimingsForm(inEffect);
    form.liveness = { ...form.liveness, periodSeconds: 0 };

    expect(validateTimings(form, inEffect)).toEqual({});
  });
});
