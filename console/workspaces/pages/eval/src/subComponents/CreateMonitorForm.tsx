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

import { AdapterDateFns, Collapse, DatePickers, Form, TextField, Typography } from "@wso2/oxygen-ui";
import { History, Timer } from "@wso2/oxygen-ui-icons-react";
import type { MonitorType } from "@agent-management-platform/types";
import type { CreateMonitorFormValues } from "../form/schema";
import { getMonitorTypeFieldPatch } from "../utils/monitorFormUtils";

interface CreateMonitorFormProps {
    formData: CreateMonitorFormValues;
    errors: Partial<Record<keyof CreateMonitorFormValues, string | undefined>>;
    onFieldChange: (field: keyof CreateMonitorFormValues, value: unknown) => void;
    isTypeEditable?: boolean;
}

export function CreateMonitorForm({ formData, errors, onFieldChange,
    isTypeEditable = true }: CreateMonitorFormProps) {
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
                    <Form.CardButton
                        onClick={isTypeEditable ? () => handleTypeChange("past") : undefined}
                        selected={formData.type === "past"}
                        disabled={!isTypeEditable}
                    >
                        <Form.CardHeader
                            title={(
                                <Form.Stack direction="row" spacing={1} justifyContent="center" alignItems="center">
                                    <History size={24} />
                                    <Form.Body>Past Traces</Form.Body>
                                </Form.Stack>
                            )}
                        />
                    </Form.CardButton>
                    <Form.CardButton
                        onClick={isTypeEditable ? () => handleTypeChange("future") : undefined}
                        selected={formData.type === "future"}
                        disabled={!isTypeEditable}
                    >
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
                {!isTypeEditable && (
                    <Typography variant="caption" color="text.secondary">
                        Monitor type is fixed for existing monitors.
                    </Typography>
                )}
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
                </Form.Stack>
            </Form.Section>
        </Form.Stack>
    );
}

