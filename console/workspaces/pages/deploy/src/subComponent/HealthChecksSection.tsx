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

import { Form, Stack, Typography } from "@wso2/oxygen-ui";
import type {
  HealthCheck,
  HealthCheckName,
  HealthChecks,
} from "@agent-management-platform/types";

const HEALTH_CHECK_NAMES: HealthCheckName[] = ["startup", "readiness", "liveness"];

const CHECK_TITLES: Record<HealthCheckName, string> = {
  startup: "Startup check",
  readiness: "Readiness check",
  liveness: "Liveness check",
};

/** What a check tests, e.g. "On · HTTP GET /health on port 8000". */
export function describeCheck(check: HealthCheck | undefined): string {
  if (!check?.enabled) return "Off";
  const target = check.type === "http" ? `HTTP GET ${check.path ?? "/health"}` : "TCP";
  const port = check.port ? `port ${check.port}` : "the agent's port";
  return `On · ${target} on ${port}`;
}

/**
 * How long a check waits, e.g. "After 10s, every 5s, timeout 1s, 60 failures allowed".
 * A wait time the response leaves out is left out here too.
 */
export function describeWaitTimes(check: HealthCheck): string {
  const parts = [
    check.initialDelaySeconds !== undefined && `After ${check.initialDelaySeconds}s`,
    check.periodSeconds !== undefined && `every ${check.periodSeconds}s`,
    check.timeoutSeconds !== undefined && `timeout ${check.timeoutSeconds}s`,
    check.failureThreshold !== undefined && `${check.failureThreshold} failures allowed`,
  ].filter((part): part is string => !!part);
  const text = parts.join(", ");
  return text.charAt(0).toUpperCase() + text.slice(1);
}

interface HealthChecksSectionProps {
  /** The health checks of the release the environment runs. */
  checks: HealthChecks;
}

/**
 * The environment's health checks, read-only. They are set in the agent's build
 * configuration and reach an environment on its next deploy.
 */
export function HealthChecksSection({ checks }: HealthChecksSectionProps) {
  return (
    <Form.Section>
      <Form.Header>Health Checks</Form.Header>
      <Stack spacing={2}>
        <Typography variant="body2" color="text.secondary">
          Set in the agent&apos;s build configuration. A change applies to this environment
          on its next deploy.
        </Typography>
        {HEALTH_CHECK_NAMES.map((name) => {
          const check = checks[name];
          return (
            <Stack key={name} spacing={0.5}>
              <Stack direction="row" justifyContent="space-between" alignItems="baseline" gap={2}>
                <Typography variant="subtitle2">{CHECK_TITLES[name]}</Typography>
                <Typography variant="body2" color="text.secondary">
                  {describeCheck(check)}
                </Typography>
              </Stack>
              {check?.enabled && (
                <Typography variant="caption" color="text.secondary">
                  {describeWaitTimes(check)}
                </Typography>
              )}
            </Stack>
          );
        })}
      </Stack>
    </Form.Section>
  );
}
