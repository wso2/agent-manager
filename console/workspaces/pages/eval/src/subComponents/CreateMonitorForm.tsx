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
  AdapterDateFns,
  Collapse,
  DatePickers,
  Form,
  MenuItem,
  Select,
  Slider,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { History, Timer } from "@wso2/oxygen-ui-icons-react";
import { type MonitorType } from "@agent-management-platform/types";
import {
  MONITOR_DESCRIPTION_MAX_LENGTH,
  MONITOR_DISPLAY_NAME_MAX_LENGTH,
  type CreateMonitorFormValues,
} from "../form/schema";
import { getMonitorTypeFieldPatch } from "../utils/monitorFormUtils";

const SAMPLING_RATE_MARKS = [{ value: 100, label: "100%" }];

export interface EnvironmentOption {
  name: string;
  displayName?: string;
}

interface CreateMonitorFormProps {
  formData: CreateMonitorFormValues;
  errors: Partial<Record<keyof CreateMonitorFormValues, string | undefined>>;
  onFieldChange: (field: keyof CreateMonitorFormValues, value: unknown) => void;
  isTypeEditable?: boolean;
  environments?: EnvironmentOption[];
}

export function CreateMonitorForm({
  formData,
  errors,
  onFieldChange,
  isTypeEditable = true,
  environments = [],
}: CreateMonitorFormProps) {
  const handleTypeChange = (nextType: MonitorType) => {
    if (!isTypeEditable || formData.type === nextType) {
      return;
    }
    onFieldChange("type", nextType);
    const patch = getMonitorTypeFieldPatch(nextType);
    Object.entries(patch).forEach(([key, value]) => {
      onFieldChange(key as keyof CreateMonitorFormValues, value as unknown);
    });
  };

  return (
    <Form.Stack>
      <Form.Section>
        <Form.Header>Basic Details</Form.Header>
        <Form.ElementWrapper name="displayName" label="Monitor Title">
          <TextField
            slotProps={{ htmlInput: { maxLength: MONITOR_DISPLAY_NAME_MAX_LENGTH } }}
            id="displayName"
            placeholder="Enter monitor name"
            required
            fullWidth
            value={formData.displayName}
            onChange={(event) =>
              onFieldChange("displayName", event.target.value)
            }
            error={!!errors.displayName}
            helperText={
              errors.displayName ?? "Visible label shown in the monitors list"
            }
          />
        </Form.ElementWrapper>
        <Form.ElementWrapper name="description" label="Description">
          <TextField
            slotProps={{ htmlInput: { maxLength: MONITOR_DESCRIPTION_MAX_LENGTH } }}
            id="description"
            placeholder="Enter monitor description"
            fullWidth
            multiline
            minRows={3}
            value={formData.description ?? ""}
            onChange={(event) =>
              onFieldChange("description", event.target.value)
            }
            error={!!errors.description}
            helperText={errors.description}
          />
        </Form.ElementWrapper>
        {environments.length > 1 && (
          <Form.ElementWrapper name="environmentName" label="Environment">
            <Select
              id="environmentName"
              fullWidth
              value={formData.environmentName ?? ""}
              onChange={(event) =>
                onFieldChange("environmentName", event.target.value)
              }
              error={!!errors.environmentName}
            >
              {environments.map((env) => (
                <MenuItem key={env.name} value={env.name}>
                  {env.displayName ?? env.name}
                </MenuItem>
              ))}
            </Select>
            {errors.environmentName && (
              <Typography variant="caption" color="error">
                {errors.environmentName}
              </Typography>
            )}
          </Form.ElementWrapper>
        )}
      </Form.Section>
      <Form.Section>
        <Form.Header>Data Collection</Form.Header>
        <Form.Stack direction="row">
          <Form.CardButton
            onClick={
              isTypeEditable ? () => handleTypeChange("past") : undefined
            }
            selected={formData.type === "past"}
            disabled={!isTypeEditable}
          >
            <Form.CardHeader
              title={
                <Form.Stack
                  direction="row"
                  spacing={1}
                  justifyContent="center"
                  alignItems="center"
                >
                  <History size={24} />
                  <Form.Body>Past Traces</Form.Body>
                </Form.Stack>
              }
            />
          </Form.CardButton>
          <Form.CardButton
            onClick={
              isTypeEditable ? () => handleTypeChange("future") : undefined
            }
            selected={formData.type === "future"}
            disabled={!isTypeEditable}
          >
            <Form.CardHeader
              title={
                <Form.Stack
                  direction="row"
                  spacing={1}
                  justifyContent="center"
                  alignItems="center"
                >
                  <Timer size={24} />
                  <Form.Body>Future Traces</Form.Body>
                </Form.Stack>
              }
            />
          </Form.CardButton>
        </Form.Stack>
        {!isTypeEditable && (
          <Typography variant="caption" color="text.secondary">
            Monitor type is fixed for existing monitors.
          </Typography>
        )}
        <Collapse in={formData.type === "past"}>
          <Form.Stack
            direction="row"
            maxWidth={600}
            justifyContent="space-around"
            alignItems="flex-start"
          >
            <Form.ElementWrapper name="traceStart" label="Start Time">
              <DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
                <DatePickers.DateTimePicker
                  value={formData.traceStart ?? null}
                  onChange={(value) => onFieldChange("traceStart", value)}
                  minDateTime={new Date(Date.now() - 30 * 24 * 60 * 60 * 1000)}
                  maxDateTime={formData.traceEnd ?? new Date()}
                  closeOnSelect={true}
                  slotProps={{
                    textField: {
                      fullWidth: true,
                      error: !!errors.traceStart,
                      helperText: errors.traceStart,
                    },
                    actionBar: { actions: [] },
                  }}
                />
              </DatePickers.LocalizationProvider>
            </Form.ElementWrapper>
            <Form.ElementWrapper name="traceEnd" label="End Time">
              <DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
                <DatePickers.DateTimePicker
                  value={formData.traceEnd ?? null}
                  onChange={(value) => onFieldChange("traceEnd", value)}
                  minDateTime={formData.traceStart ?? undefined}
                  maxDateTime={new Date()}
                  closeOnSelect={true}
                  slotProps={{
                    textField: {
                      fullWidth: true,
                      error: !!errors.traceEnd,
                      helperText: errors.traceEnd,
                    },
                    actionBar: { actions: [] },
                  }}
                />
              </DatePickers.LocalizationProvider>
            </Form.ElementWrapper>
          </Form.Stack>
        </Collapse>
        <Form.Stack direction="column" spacing={2} maxWidth={600}>
          <Collapse in={formData.type === "future"}>
            <Form.ElementWrapper
              name="intervalMinutes"
              label="Run Every (minutes)"
            >
              <TextField
                id="intervalMinutes"
                type="number"
                placeholder="60"
                value={formData.intervalMinutes ?? ""}
                slotProps={{
                  input: {
                    inputProps: {
                      min: 5,
                      step: 1,
                      onWheel: (event) => event.currentTarget.blur(),
                    },
                  },
                }}
                onChange={(event) =>
                  onFieldChange("intervalMinutes", event.target.value)
                }
                error={!!errors.intervalMinutes}
                helperText={
                  errors.intervalMinutes ??
                  "How often the monitor should execute"
                }
                fullWidth
              />
            </Form.ElementWrapper>
          </Collapse>
        </Form.Stack>
        <Form.ElementWrapper name="samplingRate" label="Sampling Rate (%)">
          <Form.Stack spacing={0.5} maxWidth={320}>
            <Slider
              value={formData.samplingRate ?? 100}
              min={1}
              max={100}
              step={1}
              marks={SAMPLING_RATE_MARKS}
              valueLabelDisplay="auto"
              onChange={(_, sliderValue) =>
                onFieldChange(
                  "samplingRate",
                  Array.isArray(sliderValue) ? sliderValue[0] : sliderValue,
                )
              }
            />
            <Typography
              variant="caption"
              color={errors.samplingRate ? "error" : "text.secondary"}
            >
              {errors.samplingRate ??
                "Percentage of matching traces to evaluate"}
            </Typography>
          </Form.Stack>
        </Form.ElementWrapper>
      </Form.Section>
    </Form.Stack>
  );
}
