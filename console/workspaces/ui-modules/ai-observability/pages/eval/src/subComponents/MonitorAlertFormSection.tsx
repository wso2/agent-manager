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

import { useParams } from "react-router-dom";
import { Alert, Collapse, Form, FormControlLabel, Stack, Switch, Typography } from "@wso2/oxygen-ui";
import type { MonitorEvaluator } from "@agent-management-platform/types";
import { MonitorAlertSettings, type AlertSettingsValue } from "./MonitorAlertSettings";

interface MonitorAlertFormSectionProps {
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
  settings: AlertSettingsValue;
  onSettingsChange: (settings: AlertSettingsValue) => void;
  /** The evaluators selected for the monitor. */
  evaluators: MonitorEvaluator[];
  showErrors: boolean;
  /** The saved settings could not be loaded, so they cannot be edited here. */
  unavailable?: boolean;
}

/** Shared alert settings within the monitor form. */
export function MonitorAlertFormSection({
  enabled,
  onEnabledChange,
  settings,
  onSettingsChange,
  evaluators,
  showErrors,
  unavailable = false,
}: MonitorAlertFormSectionProps) {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string; projectId: string; agentId: string;
  }>();
  if (unavailable) {
    return (
      <Form.Section>
        <Form.Header>Alerting</Form.Header>
        <Alert severity="error">
          Could not load this monitor&apos;s alert settings. Saving keeps them as they are;
          reload the page to change them.
        </Alert>
      </Form.Section>
    );
  }
  return (
    <Form.Section>
      <Form.Header>Alerting</Form.Header>
      <Stack spacing={2}>
        <Stack spacing={0.5}>
          <FormControlLabel
            control={
              <Switch checked={enabled} onChange={(e) => onEnabledChange(e.target.checked)} />
            }
            label="Enable monitor alerts"
          />
          <Typography variant="body2" color="text.secondary">
            Send alerts to organization destinations when runs fail, complete, or cross score
            thresholds. These settings apply to everyone managing this monitor.
          </Typography>
        </Stack>
        <Collapse in={enabled} unmountOnExit>
          <MonitorAlertSettings
            orgName={orgId}
            projName={projectId}
            agentName={agentId}
            value={settings}
            onChange={onSettingsChange}
            evaluators={evaluators}
            showErrors={showErrors}
          />
        </Collapse>
      </Stack>
    </Form.Section>
  );
}
