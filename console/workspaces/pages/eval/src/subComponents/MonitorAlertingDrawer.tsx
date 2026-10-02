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
  Checkbox,
  CircularProgress,
  Form,
  FormControlLabel,
  IconButton,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { BellRing, Plus, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link } from "react-router-dom";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
} from "@agent-management-platform/views";
import {
  useGetMonitorAlertConfig,
  useUpdateMonitorAlertConfig,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type MonitorAlertOperator,
  type MonitorAlertThreshold,
  type MonitorEvaluator,
} from "@agent-management-platform/types";

/**
 * Aggregations offered for a threshold. The evaluation job reports only the
 * mean by default (amp-evaluation DEFAULT_AGGREGATIONS); a rule on an
 * aggregation that is never reported would silently never fire.
 */
const AGGREGATIONS = [{ value: "mean", label: "Mean" }];

interface ThresholdDraft {
  evaluator: string;
  aggregation: string;
  operator: MonitorAlertOperator;
  value: string;
}

interface Draft {
  enabled: boolean;
  alertOnRunFailure: boolean;
  thresholds: ThresholdDraft[];
  cooldownMinutes: string;
  consecutiveBreaches: string;
}

function toDraft(t: MonitorAlertThreshold): ThresholdDraft {
  return {
    evaluator: t.evaluator,
    aggregation: t.aggregation,
    operator: t.operator ?? "lt",
    value: String(t.value),
  };
}

function validate(draft: Draft): Record<string, string> {
  const errors: Record<string, string> = {};
  draft.thresholds.forEach((t, i) => {
    const v = Number(t.value);
    if (!t.evaluator) errors[`evaluator-${i}`] = "Select an evaluator";
    if (t.value.trim() === "" || Number.isNaN(v) || v < 0 || v > 1) {
      errors[`value-${i}`] = "Enter a value between 0 and 1";
    }
  });
  const seen = new Set<string>();
  draft.thresholds.forEach((t, i) => {
    const key = `${t.evaluator}|${t.aggregation}`;
    if (seen.has(key)) errors[`evaluator-${i}`] = "Duplicate rule";
    seen.add(key);
  });
  const cooldown = Number(draft.cooldownMinutes);
  if (!Number.isInteger(cooldown) || cooldown < 0 || cooldown > 10080) {
    errors.cooldownMinutes = "Enter 0 to 10080 minutes";
  }
  const consecutive = Number(draft.consecutiveBreaches);
  if (!Number.isInteger(consecutive) || consecutive < 1 || consecutive > 100) {
    errors.consecutiveBreaches = "Enter 1 to 100";
  }
  if (draft.enabled && !draft.alertOnRunFailure && draft.thresholds.length === 0) {
    errors.form = "Turn on run-failure alerts or add at least one score threshold.";
  }
  return errors;
}

export interface MonitorAlertingDrawerProps {
  open: boolean;
  onClose: () => void;
  orgName: string;
  projName: string;
  agentName: string;
  monitorName: string;
  evaluators: MonitorEvaluator[];
}

export function MonitorAlertingDrawer({
  open,
  onClose,
  orgName,
  projName,
  agentName,
  monitorName,
  evaluators,
}: MonitorAlertingDrawerProps) {
  const params = useMemo(
    () => ({ orgName, projName, agentName, monitorName }),
    [orgName, projName, agentName, monitorName],
  );
  const { data, isLoading } = useGetMonitorAlertConfig(params);
  const { mutate: save, isPending } = useUpdateMonitorAlertConfig();

  const [draft, setDraft] = useState<Draft | null>(null);
  const [submitted, setSubmitted] = useState(false);

  // Build the draft once per opening. A background refetch (window focus, an
  // invalidation elsewhere) must not overwrite edits the user has not saved,
  // so later data only fills a draft that does not exist yet.
  useEffect(() => {
    if (!open) {
      setDraft(null);
      return;
    }
    if (!data) return;
    setDraft(
      (current) =>
        current ?? {
          enabled: data.enabled,
          alertOnRunFailure: data.alertOnRunFailure,
          thresholds: data.thresholds.map(toDraft),
          cooldownMinutes: String(data.cooldownMinutes),
          consecutiveBreaches: String(data.consecutiveBreaches),
        },
    );
  }, [open, data]);

  useEffect(() => {
    if (open) setSubmitted(false);
  }, [open]);

  const errors = draft ? validate(draft) : {};
  const off = !draft?.enabled;
  const shownErrors = submitted ? errors : {};

  const update = (patch: Partial<Draft>) =>
    setDraft((d) => (d ? { ...d, ...patch } : d));
  const updateThreshold = (index: number, patch: Partial<ThresholdDraft>) =>
    setDraft((d) =>
      d
        ? {
            ...d,
            thresholds: d.thresholds.map((t, i) => (i === index ? { ...t, ...patch } : t)),
          }
        : d,
    );

  const handleAddThreshold = () =>
    setDraft((d) =>
      d
        ? {
            ...d,
            thresholds: [
              ...d.thresholds,
              {
                evaluator: evaluators[0]?.displayName ?? "",
                aggregation: "mean",
                operator: "lt",
                value: "0.7",
              },
            ],
          }
        : d,
    );

  const handleSave = () => {
    if (!draft) return;
    setSubmitted(true);
    if (Object.keys(errors).length > 0) return;
    save(
      {
        params,
        body: {
          enabled: draft.enabled,
          alertOnRunFailure: draft.alertOnRunFailure,
          thresholds: draft.thresholds.map((t) => ({
            evaluator: t.evaluator,
            aggregation: t.aggregation,
            operator: t.operator,
            value: Number(t.value),
          })),
          cooldownMinutes: Number(draft.cooldownMinutes),
          consecutiveBreaches: Number(draft.consecutiveBreaches),
        },
      },
      { onSuccess: onClose },
    );
  };

  const settingsHref = generatePath(
    absoluteRouteMap.children.org.children.settings.children.alerting.path,
    { orgId: orgName },
  );

  return (
    <DrawerWrapper open={open} onClose={onClose} maxWidth={560}>
      <DrawerHeader icon={<BellRing size={24} />} title="Monitor Alerting" onClose={onClose} />
      <DrawerContent>
        {isLoading || !draft ? (
          <Stack spacing={2}>
            <Skeleton variant="rounded" height={48} />
            <Skeleton variant="rounded" height={160} />
          </Stack>
        ) : (
          <Form.Stack>
            <Typography variant="body2" color="text.secondary">
              Send an alert to your organization&apos;s alert endpoint when this
              monitor&apos;s run fails or a score falls below a threshold.
              Successful runs are never alerted.
            </Typography>
            {!data?.orgEndpointConfigured && (
              <Alert severity="warning">
                Your organization has no enabled alert endpoint, so no alerts
                will be sent.{" "}
                <Link to={settingsHref}>Configure it in Settings</Link>.
              </Alert>
            )}
            <FormControlLabel
              control={
                <Switch
                  checked={draft.enabled}
                  onChange={(_, enabled) => update({ enabled })}
                />
              }
              label={draft.enabled ? "Alerting is on for this monitor" : "Alerting is off for this monitor"}
            />
            <Box
              aria-disabled={!draft.enabled}
              sx={{ opacity: draft.enabled ? 1 : 0.5, transition: "opacity 150ms" }}
            >
              <Form.Stack spacing={3}>
                <FormControlLabel
                  control={
                    <Checkbox
                      disabled={off}
                      checked={draft.alertOnRunFailure}
                      onChange={(_, alertOnRunFailure) => update({ alertOnRunFailure })}
                    />
                  }
                  label="Alert when an evaluation run fails"
                />

                <Form.Section>
                  <Form.Header>Score thresholds</Form.Header>
                  <Typography variant="caption" color="text.secondary">
                    Alert when an evaluator&apos;s aggregated score for a run is
                    below the value. Scores range from 0 to 1.
                  </Typography>
                  <Stack spacing={1.5} sx={{ mt: 1 }}>
                    {draft.thresholds.map((t, i) => (
                      <Box key={i} display="flex" gap={1} alignItems="flex-start">
                        <Box flex={2} minWidth={0}>
                          <Select
                            disabled={off}
                            fullWidth
                            size="small"
                            value={t.evaluator}
                            onChange={(e) =>
                              updateThreshold(i, { evaluator: String(e.target.value) })
                            }
                            error={!!shownErrors[`evaluator-${i}`]}
                          >
                            {evaluators.map((e) => (
                              <MenuItem key={e.identifier} value={e.displayName}>
                                {e.displayName}
                              </MenuItem>
                            ))}
                          </Select>
                          {shownErrors[`evaluator-${i}`] && (
                            <Typography variant="caption" color="error">
                              {shownErrors[`evaluator-${i}`]}
                            </Typography>
                          )}
                        </Box>
                        <Select
                          disabled={off}
                          size="small"
                          sx={{ flex: 1 }}
                          value={t.aggregation}
                          onChange={(e) =>
                            updateThreshold(i, { aggregation: String(e.target.value) })
                          }
                        >
                          {AGGREGATIONS.map((a) => (
                            <MenuItem key={a.value} value={a.value}>
                              {a.label}
                            </MenuItem>
                          ))}
                        </Select>
                        <Select
                          disabled={off}
                          size="small"
                          sx={{ width: 72 }}
                          value={t.operator}
                          onChange={(e) =>
                            updateThreshold(i, { operator: e.target.value as MonitorAlertOperator })
                          }
                        >
                          <MenuItem value="lt">&lt;</MenuItem>
                          <MenuItem value="lte">&le;</MenuItem>
                        </Select>
                        <TextField
                          disabled={off}
                          size="small"
                          sx={{ width: 96 }}
                          type="number"
                          slotProps={{ htmlInput: { min: 0, max: 1, step: 0.05 } }}
                          value={t.value}
                          onChange={(e) => updateThreshold(i, { value: e.target.value })}
                          error={!!shownErrors[`value-${i}`]}
                          helperText={shownErrors[`value-${i}`]}
                        />
                        <IconButton
                          size="small"
                          disabled={off}
                          aria-label="Remove threshold"
                          onClick={() =>
                            update({ thresholds: draft.thresholds.filter((_, j) => j !== i) })
                          }
                        >
                          <Trash2 size={16} />
                        </IconButton>
                      </Box>
                    ))}
                    <Box>
                      <Button
                        size="small"
                        variant="text"
                        startIcon={<Plus size={16} />}
                        onClick={handleAddThreshold}
                        disabled={off || evaluators.length === 0}
                      >
                        Add threshold
                      </Button>
                    </Box>
                  </Stack>
                </Form.Section>

                <Form.Section>
                  <Form.Header>Noise control</Form.Header>
                  <Box display="flex" gap={2}>
                    <TextField
                      disabled={off}
                      label="Cooldown (minutes)"
                      size="small"
                      type="number"
                      value={draft.cooldownMinutes}
                      onChange={(e) => update({ cooldownMinutes: e.target.value })}
                      error={!!shownErrors.cooldownMinutes}
                      helperText={shownErrors.cooldownMinutes ?? "Minimum time between alerts"}
                    />
                    <TextField
                      disabled={off}
                      label="Consecutive breaches"
                      size="small"
                      type="number"
                      value={draft.consecutiveBreaches}
                      onChange={(e) => update({ consecutiveBreaches: e.target.value })}
                      error={!!shownErrors.consecutiveBreaches}
                      helperText={
                        shownErrors.consecutiveBreaches ?? "Breached runs in a row before alerting"
                      }
                    />
                  </Box>
                </Form.Section>
              </Form.Stack>
            </Box>

            {shownErrors.form && <Alert severity="error">{shownErrors.form}</Alert>}

            <Stack direction="row" spacing={1} justifyContent="flex-end">
              <Button variant="outlined" onClick={onClose} disabled={isPending}>
                Cancel
              </Button>
              <Button
                variant="contained"
                onClick={handleSave}
                disabled={isPending}
                startIcon={isPending ? <CircularProgress size={16} /> : undefined}
              >
                Save
              </Button>
            </Stack>
          </Form.Stack>
        )}
      </DrawerContent>
    </DrawerWrapper>
  );
}

export default MonitorAlertingDrawer;
