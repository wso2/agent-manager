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

import {
  Box,
  Collapse,
  Form,
  MenuItem,
  Select,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import type {
  HealthCheckName,
  HealthCheckType,
  HealthChecks,
} from "@agent-management-platform/types";

// Bounds match the AgentHealthCheck schema in agent-manager-service.
const MAX_SECONDS = 3600;
const MAX_FAILURE_THRESHOLD = 1000;
/**
 * Longest the startup check may wait: initial delay + failures allowed x the longer of
 * interval and timeout (an attempt that times out takes its full timeout).
 */
const MAX_STARTUP_WINDOW_SECONDS = 3600;

const HEALTH_CHECK_NAMES: HealthCheckName[] = ["startup", "readiness", "liveness"];

/**
 * The health checks a new agent starts with in the create wizard. A copy of the
 * defaults in agent-api.yaml (parameters.probes); keep the two in step. The wizard
 * sends health checks only when the user changes them.
 */
export const DEFAULT_HEALTH_CHECKS: HealthChecks = {
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

const CHECK_TITLES: Record<HealthCheckName, string> = {
  startup: "Startup check",
  readiness: "Readiness check",
  liveness: "Liveness check",
};

/** One check as edited. Number fields are NaN while empty; an empty port means the agent's port. */
export interface HealthCheckFormValues {
  enabled: boolean;
  type: HealthCheckType;
  path: string;
  port: number;
  initialDelaySeconds: number;
  periodSeconds: number;
  timeoutSeconds: number;
  failureThreshold: number;
}

export type HealthChecksFormValues = Record<HealthCheckName, HealthCheckFormValues>;
type CheckErrors = Partial<Record<keyof HealthCheckFormValues, string>>;
export type HealthChecksFormErrors = Partial<Record<HealthCheckName, CheckErrors>>;

type TimingField = "initialDelaySeconds" | "periodSeconds" | "timeoutSeconds" | "failureThreshold";

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

/** Seeds the editor from an agent's health checks. */
export function toHealthChecksForm(checks: HealthChecks): HealthChecksFormValues {
  const form = {} as HealthChecksFormValues;
  for (const name of HEALTH_CHECK_NAMES) {
    const check = checks[name];
    form[name] = {
      enabled: check?.enabled ?? false,
      type: check?.type ?? "tcp",
      path: check?.path ?? "/health",
      port: check?.port ?? NaN,
      initialDelaySeconds: check?.initialDelaySeconds ?? NaN,
      periodSeconds: check?.periodSeconds ?? NaN,
      timeoutSeconds: check?.timeoutSeconds ?? NaN,
      failureThreshold: check?.failureThreshold ?? NaN,
    };
  }
  return form;
}

/**
 * Turns the editor's values into the request's healthChecks. Empty number fields
 * (NaN) are left out, so they use the agent's port or the platform default.
 */
export function toHealthChecksPayload(form: HealthChecksFormValues): HealthChecks {
  const checks: HealthChecks = {};
  for (const name of HEALTH_CHECK_NAMES) {
    const { enabled, type, path, port, ...timings } = form[name];
    checks[name] = { enabled, type, path: path.trim() };
    if (!Number.isNaN(port)) checks[name].port = port;
    for (const { field } of TIMING_FIELDS) {
      if (!Number.isNaN(timings[field])) checks[name][field] = timings[field];
    }
  }
  return checks;
}

/** Checks the values of the checks that are on; the same rules the backend applies. */
export function validateHealthChecksForm(form: HealthChecksFormValues): HealthChecksFormErrors {
  const errors: HealthChecksFormErrors = {};
  for (const name of HEALTH_CHECK_NAMES) {
    const check = form[name];
    if (!check.enabled) continue;
    const checkErrors: CheckErrors = {};
    if (!Number.isNaN(check.port) && !isWholeNumberBetween(check.port, 1, 65535)) {
      checkErrors.port = "Must be between 1 and 65535";
    }
    if (check.type === "http") {
      const path = check.path.trim();
      if (!path) checkErrors.path = "Path is required for an HTTP check";
      else if (!path.startsWith("/")) checkErrors.path = "Path must start with /";
    }
    for (const { field, min, max } of TIMING_FIELDS) {
      const value = check[field];
      if (!Number.isInteger(value)) checkErrors[field] = "Enter a whole number";
      else if (value < min || value > max) checkErrors[field] = `Must be between ${min} and ${max}`;
    }
    if (name === "startup" && TIMING_FIELDS.every(({ field }) => !checkErrors[field])) {
      const window =
        check.initialDelaySeconds +
        check.failureThreshold * Math.max(check.periodSeconds, check.timeoutSeconds);
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

/** Whether value is a whole number from min to max, inclusive. */
function isWholeNumberBetween(value: number, min: number, max: number): boolean {
  return Number.isInteger(value) && value >= min && value <= max;
}

/** Formats seconds for messages, e.g. 410 as "6 min 50s". */
function formatDuration(totalSeconds: number): string {
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (minutes === 0) return `${seconds}s`;
  return seconds === 0 ? `${minutes} min` : `${minutes} min ${seconds}s`;
}

/** Reads a number field: an empty field becomes NaN. */
function toNumber(raw: string): number {
  return raw === "" ? NaN : Number(raw);
}

/** Shows a number field's value: NaN shows as empty. */
function numberValue(value: number): number | string {
  return Number.isNaN(value) ? "" : value;
}

interface HealthChecksEditorProps {
  value: HealthChecksFormValues;
  onChange: (next: HealthChecksFormValues) => void;
  errors: HealthChecksFormErrors;
  disabled?: boolean;
}

/**
 * Edits an agent's health checks: whether each one runs, what it checks (TCP or an
 * HTTP path, on a port) and its baseline wait times.
 */
export function HealthChecksEditor({ value, onChange, errors, disabled }: HealthChecksEditorProps) {
  const update = <K extends keyof HealthCheckFormValues>(
    name: HealthCheckName,
    field: K,
    fieldValue: HealthCheckFormValues[K],
  ) => onChange({ ...value, [name]: { ...value[name], [field]: fieldValue } });

  return (
    <Stack spacing={3}>
      {HEALTH_CHECK_NAMES.map((name) => {
        const check = value[name];
        const checkErrors = errors[name] ?? {};
        const id = (field: string) => `health-check-${name}-${field}`;
        return (
          <Stack key={name} spacing={1.5}>
            <Stack direction="row" justifyContent="space-between" alignItems="center" gap={2}>
              <Typography variant="subtitle2">{CHECK_TITLES[name]}</Typography>
              <Switch
                size="small"
                checked={check.enabled}
                disabled={disabled}
                onChange={(_, checked) => update(name, "enabled", checked)}
                inputProps={{ "aria-label": `${CHECK_TITLES[name]} enabled` }}
              />
            </Stack>
            <Collapse in={check.enabled}>
              <Box display="grid" gridTemplateColumns="1fr 1fr" gap={2}>
                <Form.ElementWrapper label="Check type" name={id("type")}>
                  <Select
                    id={id("type")}
                    size="small"
                    fullWidth
                    disabled={disabled}
                    value={check.type}
                    onChange={(e) => update(name, "type", e.target.value as HealthCheckType)}
                  >
                    <MenuItem value="tcp">TCP: the port accepts connections</MenuItem>
                    <MenuItem value="http">HTTP GET: the path returns 2xx or 3xx</MenuItem>
                  </Select>
                </Form.ElementWrapper>
                <Form.ElementWrapper label="Port" name={id("port")}>
                  <TextField
                    id={id("port")}
                    type="number"
                    size="small"
                    fullWidth
                    disabled={disabled}
                    placeholder="Agent's port"
                    value={numberValue(check.port)}
                    onChange={(e) => update(name, "port", toNumber(e.target.value))}
                    error={!!checkErrors.port}
                    helperText={checkErrors.port}
                  />
                </Form.ElementWrapper>
                {check.type === "http" && (
                  <Box gridColumn="1 / -1">
                    <Form.ElementWrapper label="Path" name={id("path")}>
                      <TextField
                        id={id("path")}
                        size="small"
                        fullWidth
                        disabled={disabled}
                        placeholder="/health"
                        value={check.path}
                        onChange={(e) => update(name, "path", e.target.value)}
                        error={!!checkErrors.path}
                        helperText={checkErrors.path}
                      />
                    </Form.ElementWrapper>
                  </Box>
                )}
                {TIMING_FIELDS.map(({ field, label, min }) => (
                  <Form.ElementWrapper key={field} label={label} name={id(field)}>
                    <TextField
                      id={id(field)}
                      type="number"
                      size="small"
                      fullWidth
                      disabled={disabled}
                      value={numberValue(check[field])}
                      onChange={(e) => update(name, field, toNumber(e.target.value))}
                      error={!!checkErrors[field]}
                      helperText={checkErrors[field]}
                      slotProps={{ input: { inputProps: { min } } }}
                    />
                  </Form.ElementWrapper>
                ))}
              </Box>
            </Collapse>
          </Stack>
        );
      })}
    </Stack>
  );
}
