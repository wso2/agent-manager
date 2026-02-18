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

import { AdapterDateFns, Collapse, DatePickers, Form, Slider, Stack, TextField, Typography } from "@wso2/oxygen-ui";
import { History, Timer } from "@wso2/oxygen-ui-icons-react";
import type { MonitorType } from "@agent-management-platform/types";
import type { CreateMonitorFormValues } from "../form/schema";

interface CreateMonitorFormProps {
    formData: CreateMonitorFormValues;
    errors: Partial<Record<keyof CreateMonitorFormValues, string | undefined>>;
    onFieldChange: (field: keyof CreateMonitorFormValues, value: unknown) => void;
}

export function CreateMonitorForm({ formData, errors, onFieldChange }: CreateMonitorFormProps) {
    const handleTypeChange = (nextType: MonitorType) => {
        if (formData.type === nextType) {
            return;
        }
        onFieldChange("type", nextType);
        if (nextType === "future") {
            onFieldChange("traceStart", null);
            onFieldChange("traceEnd", null);
            onFieldChange("intervalMinutes", 10);
        }
        if (nextType === "past") {
            onFieldChange("intervalMinutes", undefined);
            onFieldChange("traceStart", new Date(Date.now() - 24 * 60 * 60 * 1000));
            onFieldChange("traceEnd", new Date());
        }
    };

    return (
        <Form.Stack>
            <Form.Section>
                <Form.Header>
                    Basic Details
                </Form.Header>
                <Form.ElementWrapper name="displayName" label="Monitor Title">
                    <TextField
                        id="displayName"
                        placeholder="Enter monitor name"
                        required
                        fullWidth
                        value={formData.displayName}
                        onChange={(event) => onFieldChange("displayName", event.target.value)}
                        error={!!errors.displayName}
                        helperText={errors.displayName ?? "Visible label shown in the monitors list"}
                    />
                </Form.ElementWrapper>
                <Form.ElementWrapper name="description" label="Description">
                    <TextField
                        id="description"
                        placeholder="Enter monitor description"
                        fullWidth
                        multiline
                        minRows={3}
                        value={formData.description ?? ""}
                        onChange={(event) => onFieldChange("description", event.target.value)}
                        error={!!errors.description}
                        helperText={errors.description}
                    />
                </Form.ElementWrapper>
            </Form.Section>
            <Form.Section>
                <Form.Header>
                    Data Collection
                </Form.Header>
                <Form.Stack direction="row">
                    <Form.CardButton onClick={() => handleTypeChange("past")} selected={formData.type === "past"}>
                        <Form.CardHeader
                            title={(
                                <Form.Stack direction="row" spacing={1} justifyContent="center" alignItems="center">
                                    <History size={24} />
                                    <Form.Body>Past Traces</Form.Body>
                                </Form.Stack>
                            )}
                        />
                    </Form.CardButton>
                    <Form.CardButton onClick={() => handleTypeChange("future")} selected={formData.type === "future"}>
                        <Form.CardHeader
                            title={(
                                <Form.Stack direction="row" spacing={1} justifyContent="center" alignItems="center">
                                    <Timer size={24} />
                                    <Form.Body>Future Traces</Form.Body>
                                </Form.Stack>
                            )}
                        />
                    </Form.CardButton>
                </Form.Stack>
                <Collapse in={formData.type === "past"}>
                    <Form.Stack direction="row" maxWidth={600} justifyContent="space-around" alignItems="flex-start">
                        <Form.ElementWrapper name="traceStart" label="Start Time">
                            <DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
                                <DatePickers.DateTimePicker
                                    value={formData.traceStart ?? null}
                                    onChange={(value) => onFieldChange("traceStart", value)}
                                    slotProps={{
                                        textField: {
                                            fullWidth: true,
                                            error: !!errors.traceStart,
                                            helperText: errors.traceStart,
                                        },
                                    }}
                                />
                            </DatePickers.LocalizationProvider>
                        </Form.ElementWrapper>
                        <Form.ElementWrapper name="traceEnd" label="End Time">
                            <DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
                                <DatePickers.DateTimePicker
                                    value={formData.traceEnd ?? null}
                                    onChange={(value) => onFieldChange("traceEnd", value)}
                                    slotProps={{
                                        textField: {
                                            fullWidth: true,
                                            error: !!errors.traceEnd,
                                            helperText: errors.traceEnd,
                                        },
                                    }}
                                />
                            </DatePickers.LocalizationProvider>
                        </Form.ElementWrapper>
                    </Form.Stack>
                </Collapse>
                <Form.Stack direction="column" spacing={2} maxWidth={600}>
                    <Collapse in={formData.type === "future"}>
                        <Form.ElementWrapper name="intervalMinutes" label="Run Every (minutes)">
                            <TextField
                                id="intervalMinutes"
                                type="number"
                                placeholder="60"
                                value={formData.intervalMinutes ?? ""}
                                onChange={(event) => onFieldChange("intervalMinutes", event.target.value)}
                                error={!!errors.intervalMinutes}
                                helperText={errors.intervalMinutes ?? "How often the monitor should execute"}
                                fullWidth
                            />
                        </Form.ElementWrapper>
                    </Collapse>
                    <Form.ElementWrapper name="samplingRate" label={`Sampling (${formData.samplingRate ?? 0}%)`}>
                        <Stack spacing={1} sx={{ px: 1 }}>
                            <Slider
                                aria-label="Sampling rate"
                                min={0}
                                max={100}
                                step={1}
                                value={formData.samplingRate ?? 0}
                                valueLabelDisplay="auto"
                                onChange={(_, value) => {
                                    const nextValue = Array.isArray(value) ? value[0] : value;
                                    onFieldChange("samplingRate", nextValue);
                                }}
                            />
                            <Typography variant="caption" color={errors.samplingRate ? "error" : "text.secondary"}>
                                {errors.samplingRate ?? "Percentage of traces evaluated"}
                            </Typography>
                        </Stack>
                    </Form.ElementWrapper>
                </Form.Stack>
            </Form.Section>
        </Form.Stack>
    );
}

