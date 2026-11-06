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

import { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Divider,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Bell, BellOff } from "@wso2/oxygen-ui-icons-react";
import { DrawerContent, DrawerHeader, DrawerWrapper } from "@agent-management-platform/views";
import {
  useDeleteMonitorAlert,
  useMonitorAlert,
  useEventsEnabled,
  useTokenScopes,
  useSaveMonitorAlert,
} from "@agent-management-platform/api-client";
import type { MonitorEvaluator } from "@agent-management-platform/types";
import {
  MonitorAlertSettings,
  defaultAlertSettings,
  hasAlertSettingsErrors,
  thresholdKey,
  toAlertRequest,
  validateAlertSettings,
  alertSettingsFromResponse,
  type AlertSettingsValue,
} from "./MonitorAlertSettings";

interface MonitorAlertButtonProps {
  orgName: string | undefined;
  projName: string | undefined;
  agentName: string | undefined;
  monitorName: string | undefined;
  monitorDisplayName: string | undefined;
  evaluators: MonitorEvaluator[];
}

/**
 * Manages shared monitor alerts delivered to organization destinations.
 */
export function MonitorAlertButton({
  orgName,
  projName,
  agentName,
  monitorName,
  monitorDisplayName,
  evaluators,
}: MonitorAlertButtonProps) {
  const eventsEnabled = useEventsEnabled();
  const { enforced, resolved, scopes } = useTokenScopes();
  const canManage = !enforced || (resolved && scopes.has("amp:monitor:update"));
  // Reading the settings needs only monitor read; changing them, update.
  const canView = canManage || (resolved && scopes.has("amp:monitor:read"));
  const params = useMemo(
    () => ({ orgName, projName, agentName, monitorName }),
    [orgName, projName, agentName, monitorName],
  );
  const { data: alert, isLoading, error } = useMonitorAlert(params, eventsEnabled);
  const save = useSaveMonitorAlert(params);
  const disableAlerts = useDeleteMonitorAlert(params);

  const [open, setOpen] = useState(false);
  const [settings, setSettings] = useState<AlertSettingsValue>(defaultAlertSettings);
  const [submitted, setSubmitted] = useState(false);

  // Load the saved settings each time the drawer opens.
  useEffect(() => {
    if (!open || !alert) return;
    setSettings(alertSettingsFromResponse(alert));
    setSubmitted(false);
  }, [open, alert]);

  const breached = useMemo(
    () => new Set((alert?.breached ?? []).map(thresholdKey)),
    [alert?.breached],
  );

  if (!eventsEnabled) return null;

  const enabled = alert?.enabled === true;

  const handleSave = () => {
    setSubmitted(true);
    if (!canManage || hasAlertSettingsErrors(validateAlertSettings(settings, evaluators))) return;
    save.mutate(toAlertRequest(settings), { onSuccess: () => setOpen(false) });
  };

  const handleDisableAlerts = () => {
    if (canManage) disableAlerts.mutate(undefined, { onSuccess: () => setOpen(false) });
  };

  return (
    <>
      <Tooltip
        title={
          !canManage
            ? "View this monitor's alert settings"
            : enabled
              ? "Manage this monitor's alert settings"
              : "Send monitor alerts to organization destinations"
        }
      >
        <span>
          <Button
            size="small"
            variant="outlined"
            startIcon={enabled ? <Bell size={16} /> : <BellOff size={16} />}
            onClick={() => setOpen(true)}
            disabled={isLoading || !canView}
          >
            Alerting
          </Button>
        </span>
      </Tooltip>
      <DrawerWrapper open={open} onClose={() => setOpen(false)} minWidth="min(720px, 100vw)" maxWidth="min(720px, 100vw)">
        <DrawerHeader
          icon={<Bell size={24} />}
          title={`Alerting for ${monitorDisplayName ?? monitorName ?? "monitor"}`}
          onClose={() => setOpen(false)}
        />
        <DrawerContent>
          <Stack spacing={3}>
            <Typography variant="body2" color="text.secondary">
              Send alerts about this monitor&apos;s runs to selected organization destinations.
              These settings apply to the monitor and are shared with your team.
            </Typography>
            {!!error && <Alert severity="error">Could not load alert settings. Close this panel and try again.</Alert>}
            {!canManage && (
              <Alert severity="info">
                {enabled
                  ? "You can view these settings. Changing them requires permission to update the monitor."
                  : "This monitor has no alert settings. Setting them up requires permission to update the monitor."}
              </Alert>
            )}
            <Divider />
            {(canManage || enabled) && (
              <MonitorAlertSettings
                orgName={orgName}
                projName={projName}
                agentName={agentName}
                value={settings}
                onChange={setSettings}
                evaluators={evaluators}
                breached={breached}
                showErrors={submitted}
                readOnly={!canManage}
              />
            )}
            {!canManage ? (
              <Box>
                <Button variant="outlined" onClick={() => setOpen(false)}>
                  Close
                </Button>
              </Box>
            ) : (
              <Stack direction="row" spacing={1} justifyContent="flex-start" alignItems="center">
                <Button variant="outlined" onClick={() => setOpen(false)}>
                  Cancel
                </Button>
                <Button
                  variant="contained"
                  onClick={handleSave}
                  disabled={save.isPending || disableAlerts.isPending || !canManage || !!error}
                  startIcon={save.isPending ? <CircularProgress size={14} /> : undefined}
                >
                  {enabled ? "Save" : "Enable alerting"}
                </Button>
                {enabled && (
                  <Box sx={{ ml: "auto" }}>
                    <Button
                      color="error"
                      onClick={handleDisableAlerts}
                      disabled={disableAlerts.isPending || save.isPending || !canManage || !!error}
                    >
                      Disable alerting
                    </Button>
                  </Box>
                )}
              </Stack>
            )}
          </Stack>
        </DrawerContent>
      </DrawerWrapper>
    </>
  );
}
