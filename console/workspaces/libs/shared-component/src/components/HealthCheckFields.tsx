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

import {
  Box,
  Collapse,
  FormControl,
  FormControlLabel,
  FormLabel,
  MenuItem,
  Select,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import {
  INPUT_LIMITS,
  healthCheckFieldErrors,
  type HealthCheck,
  type HealthCheckDefaults,
  type HealthCheckName,
  type HealthCheckType,
} from "@agent-management-platform/types";

export interface HealthCheckFieldsProps {
  /** Which check these fields are for; decides the rules that apply. */
  checkName: HealthCheckName;
  /** Shown as the check's heading, e.g. "Readiness". */
  title: string;
  /** What this check is for, shown under the heading. */
  description?: string;
  /** The check as saved or edited; undefined when it is not set. */
  value: HealthCheck | undefined;
  onChange: (value: HealthCheck | undefined) => void;
  /** Today's values for this check, shown as placeholders for fields that are not set. */
  defaults: HealthCheckDefaults;
  /** Suggested path for an HTTP check, shown as the path's placeholder. */
  suggestedPath?: string;
  /** The agent's port, shown as the port's placeholder. */
  agentPort?: number;
  /** Show the on/off switch. The create page always keeps readiness on. */
  showEnabledSwitch?: boolean;
  /** Show initial delay, interval, timeout and failures allowed. */
  showWaitTimes?: boolean;
  disabled?: boolean;
}

/** The fields for one health check (startup, readiness or liveness). */
export function HealthCheckFields({
  checkName,
  title,
  description,
  value,
  onChange,
  defaults,
  suggestedPath = "/health",
  agentPort,
  showEnabledSwitch = true,
  showWaitTimes = true,
  disabled = false,
}: HealthCheckFieldsProps) {
  const errors = healthCheckFieldErrors(value, checkName, defaults);
  const enabled = value?.enabled ?? (value ? true : defaults.enabled);
  const checkType = value?.type ?? defaults.type;

  // Sets one field; an empty value removes it, so it falls back to its default.
  const setField = <K extends keyof HealthCheck>(
    field: K,
    fieldValue: HealthCheck[K] | undefined,
  ) => {
    const next: HealthCheck = { ...(value ?? {}) };
    if (fieldValue === undefined) {
      delete next[field];
    } else {
      next[field] = fieldValue;
    }
    onChange(Object.keys(next).length === 0 ? undefined : next);
  };
  // Switching to HTTP fills in the suggested path, since an HTTP check with no path would
  // check "/"; switching back to TCP drops the path.
  const setType = (nextType: HealthCheckType) => {
    const next: HealthCheck = { ...(value ?? {}), type: nextType };
    if (nextType === "HTTP" && !next.path) {
      next.path = suggestedPath;
    }
    if (nextType === "TCP") {
      delete next.path;
    }
    onChange(next);
  };
  // Number fields: empty means "use the default"; anything else must be digits only, so a
  // lone "-" can never be stored as NaN.
  const setNumberField = (field: keyof HealthCheck, text: string) => {
    if (!/^\d*$/.test(text)) {
      return;
    }
    setField(field, text === "" ? undefined : Number(text));
  };
  const numberText = (fieldValue: number | undefined) => (fieldValue === undefined ? "" : String(fieldValue));

  return (
    <Box sx={{ border: 1, borderColor: "divider", borderRadius: 1, p: 2 }}>
      <Stack direction="row" alignItems="center" justifyContent="space-between">
        <Box>
          <Typography variant="subtitle2" fontWeight={600}>
            {title}
          </Typography>
          {description && (
            <Typography variant="caption" color="text.secondary">
              {description}
            </Typography>
          )}
        </Box>
        {showEnabledSwitch && (
          <FormControlLabel
            control={
              <Switch
                checked={enabled}
                slotProps={{ input: { "aria-label": `${title} on` } }}
                onChange={(_, checked) => setField("enabled", checked)}
                disabled={disabled}
              />
            }
            label={enabled ? "On" : "Off"}
          />
        )}
      </Stack>
      {/* A check that is off is still validated (and saved with its values), so its fields
          stay open while any of them has an error. */}
      <Collapse in={enabled || Object.keys(errors).length > 0}>
        <Stack spacing={2} sx={{ mt: 2 }}>
          <Stack direction={{ xs: "column", md: "row" }} spacing={2} useFlexGap>
            <FormControl sx={{ minWidth: 140 }}>
              <FormLabel>Type</FormLabel>
              <Select
                size="small"
                value={checkType}
                inputProps={{ "aria-label": `${title} type` }}
                onChange={(e) => setType(e.target.value as HealthCheckType)}
                disabled={disabled}
              >
                <MenuItem value="TCP">TCP</MenuItem>
                <MenuItem value="HTTP">HTTP GET</MenuItem>
              </Select>
            </FormControl>
            {checkType === "HTTP" && (
              <FormControl sx={{ flex: 1 }} error={Boolean(errors.path)}>
                <FormLabel>Path</FormLabel>
                <TextField
                  size="small"
                  slotProps={{
                    htmlInput: { maxLength: INPUT_LIMITS.PATH, "aria-label": `${title} path` },
                  }}
                  value={value?.path ?? ""}
                  placeholder={suggestedPath}
                  onChange={(e) => setField("path", e.target.value === "" ? undefined : e.target.value)}
                  error={Boolean(errors.path)}
                  helperText={errors.path}
                  disabled={disabled}
                />
              </FormControl>
            )}
            <FormControl sx={{ maxWidth: 160 }} error={Boolean(errors.port)}>
              <FormLabel>Port</FormLabel>
              <TextField
                size="small"
                slotProps={{ htmlInput: { "aria-label": `${title} port` } }}
                value={numberText(value?.port)}
                placeholder={agentPort ? String(agentPort) : "Agent's port"}
                onChange={(e) => setNumberField("port", e.target.value)}
                error={Boolean(errors.port)}
                helperText={errors.port}
                disabled={disabled}
              />
            </FormControl>
          </Stack>
          {showWaitTimes && (
            <Stack direction={{ xs: "column", md: "row" }} spacing={2} useFlexGap>
              {(
                [
                  ["initialDelaySeconds", "Initial delay (s)"],
                  ["periodSeconds", "Interval (s)"],
                  ["timeoutSeconds", "Timeout (s)"],
                  ["failureThreshold", "Failures allowed"],
                ] as const
              ).map(([field, label]) => (
                <FormControl key={field} sx={{ flex: 1 }} error={Boolean(errors[field])}>
                  <FormLabel>{label}</FormLabel>
                  <TextField
                    size="small"
                    slotProps={{ htmlInput: { "aria-label": `${title} ${label.toLowerCase()}` } }}
                    value={numberText(value?.[field])}
                    placeholder={String(defaults[field])}
                    onChange={(e) => setNumberField(field, e.target.value)}
                    error={Boolean(errors[field])}
                    helperText={errors[field]}
                    disabled={disabled}
                  />
                </FormControl>
              ))}
            </Stack>
          )}
        </Stack>
      </Collapse>
    </Box>
  );
}
