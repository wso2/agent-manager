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

import type { HealthCheck, HealthCheckType, HealthChecks } from '../api/agents';

// Plain logic shared by the health check fields in the console: the defaults shown,
// the suggested path, the field rules and what gets sent. Kept here, free of UI imports,
// so request builders and their tests can use it without loading the UI library.

/** The values a check uses for any field that is not set: today's fixed probes. */
export interface HealthCheckDefaults {
  enabled: boolean;
  type: HealthCheckType;
  initialDelaySeconds: number;
  periodSeconds: number;
  timeoutSeconds: number;
  failureThreshold: number;
}

// Must match the fallbacks in the agent-api ComponentType (agent-api.yaml).
export const STARTUP_CHECK_DEFAULTS: HealthCheckDefaults = {
  enabled: true, type: "TCP", initialDelaySeconds: 10, periodSeconds: 5, timeoutSeconds: 1, failureThreshold: 60,
};
export const READINESS_CHECK_DEFAULTS: HealthCheckDefaults = {
  enabled: true, type: "TCP", initialDelaySeconds: 0, periodSeconds: 5, timeoutSeconds: 1, failureThreshold: 6,
};
// There is no liveness check unless one is switched on; then the Kubernetes defaults apply.
export const LIVENESS_CHECK_DEFAULTS: HealthCheckDefaults = {
  enabled: false, type: "TCP", initialDelaySeconds: 0, periodSeconds: 10, timeoutSeconds: 1, failureThreshold: 3,
};

/**
 * The path suggested for an HTTP health check, by how the agent receives requests:
 *   - chat agent: /health (the chat contract has only POST /chat, so this is a suggestion);
 *   - custom API agent: /health under the agent's base path;
 *   - A2A agent: its agent card, which every A2A agent serves.
 */
export function suggestedHealthCheckPath(interfaceType: string, basePath?: string): string {
  if (interfaceType === "A2A") {
    return "/.well-known/agent-card.json";
  }
  if (interfaceType === "CUSTOM" && basePath && basePath !== "/") {
    return `${basePath.replace(/\/+$/, "")}/health`;
  }
  return "/health";
}

type HealthCheckFieldErrors = Partial<Record<keyof HealthCheck, string>>;

/** Which of the three checks a set of fields belongs to. */
export type HealthCheckName = "startup" | "readiness" | "liveness";

/**
 * The most a check's initial delay, interval or timeout may be, and the most the startup check
 * may take in total (initial delay + interval × failures allowed). Must match the backend's
 * maxHealthCheckSeconds.
 */
export const MAX_HEALTH_CHECK_SECONDS = 3600;

/**
 * Checks one health check against the rules the backend enforces, so the form can show
 * the problem next to the field before saving.
 */
export function healthCheckFieldErrors(
  check: HealthCheck | undefined,
  checkName: HealthCheckName,
  defaults: HealthCheckDefaults,
): HealthCheckFieldErrors {
  const errors: HealthCheckFieldErrors = {};
  if (!check) {
    return errors;
  }
  // A path is sent only for an HTTP check and must start with /.
  if (check.type === "HTTP" && check.path !== undefined && !/^\/[A-Za-z0-9._\-/]*$/.test(check.path)) {
    errors.path = "Path must start with / and contain only letters, numbers, ., _, - and /";
  }
  // The port, when set, must be a real port; when not set the agent's port is used.
  if (check.port !== undefined && (check.port < 1 || check.port > 65535)) {
    errors.port = "Port must be between 1 and 65535";
  }
  // The first check may run immediately, so the initial delay can be 0.
  if (check.initialDelaySeconds !== undefined && check.initialDelaySeconds < 0) {
    errors.initialDelaySeconds = "Must be 0 or more";
  }
  // The interval, timeout and failures allowed must each be at least 1.
  if (check.periodSeconds !== undefined && check.periodSeconds < 1) {
    errors.periodSeconds = "Must be 1 or more";
  }
  if (check.timeoutSeconds !== undefined && check.timeoutSeconds < 1) {
    errors.timeoutSeconds = "Must be 1 or more";
  }
  if (check.failureThreshold !== undefined && check.failureThreshold < 1) {
    errors.failureThreshold = "Must be 1 or more";
  }
  // The rules below combine fields, so a field that is not set counts as its default.
  const initialDelay = check.initialDelaySeconds ?? defaults.initialDelaySeconds;
  const period = check.periodSeconds ?? defaults.periodSeconds;
  const timeout = check.timeoutSeconds ?? defaults.timeoutSeconds;
  const failures = check.failureThreshold ?? defaults.failureThreshold;
  const overMax = `Must be ${MAX_HEALTH_CHECK_SECONDS} or less`;
  // The initial delay, interval and timeout are each capped.
  if (initialDelay > MAX_HEALTH_CHECK_SECONDS) {
    errors.initialDelaySeconds ??= overMax;
  }
  if (period > MAX_HEALTH_CHECK_SECONDS) {
    errors.periodSeconds ??= overMax;
  }
  if (timeout > MAX_HEALTH_CHECK_SECONDS) {
    errors.timeoutSeconds ??= overMax;
  }
  // A check still waiting when the next one is due makes no sense.
  if (timeout > period) {
    errors.timeoutSeconds ??= `Cannot be longer than the interval (${period}s)`;
  }
  // Startup may take at most an hour in total before the agent is restarted.
  const startupTotal = initialDelay + period * failures;
  if (checkName === "startup" && startupTotal > MAX_HEALTH_CHECK_SECONDS) {
    errors.failureThreshold ??=
      `Startup could take ${startupTotal}s (delay + interval × failures); ` +
      `the most is ${MAX_HEALTH_CHECK_SECONDS}s`;
  }
  return errors;
}

/** True when every check in the set passes healthCheckFieldErrors. */
export function healthChecksAreValid(healthChecks: HealthChecks | undefined): boolean {
  const checks: Array<[HealthCheck | undefined, HealthCheckName, HealthCheckDefaults]> = [
    [healthChecks?.startup, "startup", STARTUP_CHECK_DEFAULTS],
    [healthChecks?.readiness, "readiness", READINESS_CHECK_DEFAULTS],
    [healthChecks?.liveness, "liveness", LIVENESS_CHECK_DEFAULTS],
  ];
  return checks.every(
    ([check, checkName, defaults]) =>
      Object.keys(healthCheckFieldErrors(check, checkName, defaults)).length === 0,
  );
}

/**
 * Returns the health checks as they should be sent: fields that are not set removed, a
 * path removed from a check that is not HTTP, and a check left out when nothing in it
 * differs from its defaults. Returns {} when no check differs, which means "use the
 * defaults".
 */
export function healthChecksToSend(healthChecks: HealthChecks | undefined): HealthChecks {
  const toSend: HealthChecks = {};
  const tidy = (
    check: HealthCheck | undefined,
    defaults: HealthCheckDefaults,
  ): HealthCheck | undefined => {
    if (!check) {
      return undefined;
    }
    const tidied: HealthCheck = {};
    (Object.keys(check) as Array<keyof HealthCheck>).forEach((field) => {
      const value = check[field];
      if (value !== undefined && value !== "") {
        (tidied as Record<string, unknown>)[field] = value;
      }
    });
    if (tidied.type !== "HTTP") {
      delete tidied.path;
    }
    // Only the switch was touched and it is back at the default: nothing to send.
    const onlyEnabled = Object.keys(tidied).length === 1 && tidied.enabled !== undefined;
    if (Object.keys(tidied).length === 0 || (onlyEnabled && tidied.enabled === defaults.enabled)) {
      return undefined;
    }
    return tidied;
  };
  const startup = tidy(healthChecks?.startup, STARTUP_CHECK_DEFAULTS);
  const readiness = tidy(healthChecks?.readiness, READINESS_CHECK_DEFAULTS);
  const liveness = tidy(healthChecks?.liveness, LIVENESS_CHECK_DEFAULTS);
  if (startup) toSend.startup = startup;
  if (readiness) toSend.readiness = readiness;
  if (liveness) toSend.liveness = liveness;
  return toSend;
}
