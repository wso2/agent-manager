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

import React, { useRef, useState } from "react";
import {
    AdapterDateFns,
    Button,
    DatePickers,
    Divider,
    Form,
    InputAdornment,
    MenuItem,
    Popover,
    Select,
    Stack,
    Typography,
} from "@wso2/oxygen-ui";
import { Clock } from "@wso2/oxygen-ui-icons-react";
import { isValid } from "date-fns";

const CUSTOM_VALUE = "__custom__";
const isValidDate = (d: Date | null): d is Date => isValid(d);
const MAX_RANGE_MS = 14 * 24 * 60 * 60 * 1000;

const fmtLabel = (iso: string) =>
    new Date(iso).toLocaleString(undefined, {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
    });

export interface TimeRangeSelectorProps {
    preset?: string;
    customStart?: string;
    customEnd?: string;
    options: Array<{ value: string; label: string }>;
    onPresetChange: (value: string) => void;
    onCustomRangeApply: (startISO: string, endISO: string) => void;
}

export const TimeRangeSelector: React.FC<TimeRangeSelectorProps> = ({
    preset,
    customStart,
    customEnd,
    options,
    onPresetChange,
    onCustomRangeApply,
}) => {
    const anchorRef = useRef<HTMLElement>(null);
    const [popoverOpen, setPopoverOpen] = useState(false);
    const [draftStart, setDraftStart] = useState<Date | null>(null);
    const [draftEnd, setDraftEnd] = useState<Date | null>(null);

    const hasCustomRange = !!customStart && !!customEnd;
    const selectValue = hasCustomRange ? CUSTOM_VALUE : (preset ?? "");

    const openPopover = () => {
        const now = new Date();
        const hourAgo = new Date(now.getTime() - 60 * 60 * 1000);
        setDraftStart(customStart ? new Date(customStart) : hourAgo);
        setDraftEnd(customEnd ? new Date(customEnd) : now);
        setPopoverOpen(true);
    };

    const now = new Date();
    const minStart = new Date(now.getTime() - MAX_RANGE_MS);

    const isApplyDisabled =
        !isValidDate(draftStart) ||
        !isValidDate(draftEnd) ||
        draftStart < minStart ||
        draftEnd > now ||
        draftStart >= draftEnd ||
        draftEnd.getTime() - draftStart.getTime() > MAX_RANGE_MS;

    const handleApply = () => {
        if (isApplyDisabled) return;
        onCustomRangeApply(draftStart!.toISOString(), draftEnd!.toISOString());
        setPopoverOpen(false);
    };

    return (
        <Stack direction="row" spacing={0.5} alignItems="center">
            <Select
                ref={anchorRef}
                size="small"
                variant="outlined"
                value={selectValue}
                renderValue={(v) => {
                    if (v === CUSTOM_VALUE) {
                        return `${fmtLabel(customStart!)} – ${fmtLabel(customEnd!)}`;
                    }
                    return options.find((o) => o.value === v)?.label ?? v;
                }}
                onChange={(e) => {
                    if (e.target.value !== CUSTOM_VALUE) {
                        onPresetChange(e.target.value);
                    }
                }}
                startAdornment={
                    <InputAdornment position="start">
                        <Clock size={16} />
                    </InputAdornment>
                }
                sx={{ minWidth: 150 }}
            >
                {options.map((opt) => (
                    <MenuItem key={opt.value} value={opt.value}>
                        {opt.label}
                    </MenuItem>
                ))}
                <Divider />
                <MenuItem value={CUSTOM_VALUE} onClick={openPopover}>Custom Time Range</MenuItem>
            </Select>

            <Popover
                open={popoverOpen}
                anchorEl={anchorRef.current}
                onClose={() => setPopoverOpen(false)}
                anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
                transformOrigin={{ vertical: "top", horizontal: "right" }}
            >
                <Stack spacing={2} sx={{ p: 2, width: 320 }}>
                    <Typography variant="h6">Custom Time Range</Typography>
                    <Divider />
                    <DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
                        <Form.ElementWrapper label="Start" name="start">
                            <DatePickers.DateTimePicker
                                value={draftStart}
                                onChange={(v) => setDraftStart(v)}
                                minDateTime={minStart}
                                maxDateTime={now}
                                slotProps={{ textField: { size: "small", fullWidth: true } }}
                            />
                        </Form.ElementWrapper>
                        <Form.ElementWrapper label="End" name="end">
                            <DatePickers.DateTimePicker
                                value={draftEnd}
                                onChange={(v) => setDraftEnd(v)}
                                minDateTime={isValidDate(draftStart) ? draftStart : undefined}
                                maxDateTime={now}
                                slotProps={{ textField: { size: "small", fullWidth: true } }}
                            />
                        </Form.ElementWrapper>
                    </DatePickers.LocalizationProvider>
                    <Stack direction="row" spacing={1} justifyContent="flex-end">
                        <Button size="small" variant="text" onClick={() => setPopoverOpen(false)}>
                            Cancel
                        </Button>
                        <Button
                            size="small"
                            variant="contained"
                            onClick={handleApply}
                            disabled={isApplyDisabled}
                        >
                            Apply
                        </Button>
                    </Stack>
                </Stack>
            </Popover>
        </Stack>
    );
};

export default TimeRangeSelector;
