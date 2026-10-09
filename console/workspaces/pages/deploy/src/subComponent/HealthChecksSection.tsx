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

import { Box, Form, Stack, TextField, Typography } from "@wso2/oxygen-ui";
import type {
  HealthCheck,
  HealthCheckName,
  HealthCheckTimings,
  HealthChecks,
  ProbeTimings,
} from "@agent-management-platform/types";

// Bounds match the AgentProbeTimings schema in agent-manager-service.
const MAX_SECONDS = 3600;
const MAX_FAILURE_THRESHOLD = 1000;
/**
 * Longest the startup check may wait: initial delay + failures allowed x the longer of
 * interval and timeout (an attempt that times out takes its full timeout).
 */
const MAX_STARTUP_WINDOW_SECONDS = 3600;

const HEALTH_CHECK_NAMES: HealthCheckName[] = ["startup", "readiness", "liveness"];

const CHECK_TITLES: Record<HealthCheckName, string> = {
  startup: "Startup check",
  readiness: "Readiness check",
  liveness: "Liveness check",
};

type TimingField = keyof ProbeTimings;

const TIMING_FIELDS: {
  field: TimingField;
  label: string;
  min: number;
  max: number;
}[] = [
  {
    field: "initialDelaySeconds",
    label: "Initial delay (s)",
    min: 0,
    max: MAX_SECONDS,
  },
  {
    field: "periodSeconds",
    label: "Interval (s)",
    min: 1,
    max: MAX_SECONDS,
  },
  {
    field: "timeoutSeconds",
    label: "Timeout (s)",
    min: 1,
    max: MAX_SECONDS,
  },
  {
    field: "failureThreshold",
    label: "Failures allowed",
    min: 1,
    max: MAX_FAILURE_THRESHOLD,
  },
];

/** The wait times being edited, per check. A field is NaN while it is empty. */
export type TimingsForm = Record<HealthCheckName, Required<ProbeTimings>>;
export type TimingsErrors = Partial<Record<HealthCheckName, Partial<Record<TimingField, string>>>>;

/** Seeds the form from the health checks in effect. */
export function toTimingsForm(checks: HealthChecks): TimingsForm {
  const form = {} as TimingsForm;
  for (const name of HEALTH_CHECK_NAMES) {
    const check = checks[name];
    form[name] = {
      initialDelaySeconds: check?.initialDelaySeconds ?? NaN,
      periodSeconds: check?.periodSeconds ?? NaN,
      timeoutSeconds: check?.timeoutSeconds ?? NaN,
      failureThreshold: check?.failureThreshold ?? NaN,
    };
  }
  return form;
}

/**
 * The wait times the user changed, or undefined when none changed. Only these are
 * sent, so untouched ones keep following the agent's build configuration.
 */
export function changedTimings(
  form: TimingsForm,
  initial: TimingsForm,
): HealthCheckTimings | undefined {
  const changed: HealthCheckTimings = {};
  for (const name of HEALTH_CHECK_NAMES) {
    for (const { field } of TIMING_FIELDS) {
      // Object.is so that two empty (NaN) values count as unchanged.
      if (!Object.is(form[name][field], initial[name][field])) {
        changed[name] = { ...changed[name], [field]: form[name][field] };
      }
    }
  }
  return Object.keys(changed).length > 0 ? changed : undefined;
}

/** Checks the wait times of the checks that are on; the same rules the backend applies. */
export function validateTimings(form: TimingsForm, checks: HealthChecks): TimingsErrors {
  const errors: TimingsErrors = {};
  for (const name of HEALTH_CHECK_NAMES) {
    if (!checks[name]?.enabled) continue;
    const timings = form[name];
    const checkErrors: Partial<Record<TimingField, string>> = {};
    for (const { field, min, max } of TIMING_FIELDS) {
      const value = timings[field];
      if (!Number.isInteger(value)) checkErrors[field] = "Enter a whole number";
      else if (value < min || value > max) checkErrors[field] = `Must be between ${min} and ${max}`;
    }
    if (name === "startup" && Object.keys(checkErrors).length === 0) {
      const window =
        timings.initialDelaySeconds +
        timings.failureThreshold * Math.max(timings.periodSeconds, timings.timeoutSeconds);
      if (window > MAX_STARTUP_WINDOW_SECONDS) {
        checkErrors.failureThreshold =
          `Allows ${formatDuration(window)} to start; ` +
          `the maximum is ${formatDuration(MAX_STARTUP_WINDOW_SECONDS)}`;
      }
    }
    if (Object.keys(checkErrors).length > 0) errors[name] = checkErrors;
  }
  return errors;
}

/** Formats seconds for messages, e.g. 410 as "6 min 50s". */
function formatDuration(totalSeconds: number): string {
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (minutes === 0) return `${seconds}s`;
  return seconds === 0 ? `${minutes} min` : `${minutes} min ${seconds}s`;
}

/** What a check tests, e.g. "On · HTTP GET /health on port 8000". */
function describeCheck(check: HealthCheck | undefined): string {
  if (!check?.enabled) return "Off";
  const target = check.type === "http" ? `HTTP GET ${check.path ?? "/health"}` : "TCP";
  const port = check.port ? `port ${check.port}` : "the agent's port";
  return `On · ${target} on ${port}`;
}

interface HealthChecksSectionProps {
  /** The health checks in effect in the environment. */
  checks: HealthChecks;
  timings: TimingsForm;
  errors: TimingsErrors;
  disabled: boolean;
  onChange: (name: HealthCheckName, field: TimingField, value: number) => void;
}

/**
 * The environment's health checks: what each one tests (read-only, set in the
 * agent's build configuration) and its wait times (editable, this environment only).
 */
export function HealthChecksSection({
  checks,
  timings,
  errors,
  disabled,
  onChange,
}: HealthChecksSectionProps) {
  return (
    <Form.Section>
      <Form.Header>Health Checks</Form.Header>
      <Stack spacing={3}>
        <Typography variant="body2" color="text.secondary">
          What each check tests is set in the agent&apos;s build configuration. The wait
          times apply to this environment only.
        </Typography>
        {HEALTH_CHECK_NAMES.map((name) => {
          const check = checks[name];
          return (
            <Stack key={name} spacing={1.5}>
              <Stack direction="row" justifyContent="space-between" alignItems="baseline">
                <Typography variant="subtitle2">{CHECK_TITLES[name]}</Typography>
                <Typography variant="body2" color="text.secondary">
                  {describeCheck(check)}
                </Typography>
              </Stack>
              {check?.enabled && (
                <Box display="grid" gridTemplateColumns="1fr 1fr" gap={2}>
                  {TIMING_FIELDS.map(({ field, label, min }) => {
                    const id = `health-${name}-${field}`;
                    const value = timings[name][field];
                    const error = errors[name]?.[field];
                    return (
                      <Form.ElementWrapper key={field} label={label} name={id}>
                        <TextField
                          id={id}
                          type="number"
                          size="small"
                          fullWidth
                          disabled={disabled}
                          value={Number.isNaN(value) ? "" : value}
                          onChange={(e) =>
                            onChange(
                              name,
                              field,
                              e.target.value === "" ? NaN : Number(e.target.value),
                            )
                          }
                          error={!!error}
                          helperText={error}
                          slotProps={{ input: { inputProps: { min } } }}
                        />
                      </Form.ElementWrapper>
                    );
                  })}
                </Box>
              )}
            </Stack>
          );
        })}
      </Stack>
    </Form.Section>
  );
}
