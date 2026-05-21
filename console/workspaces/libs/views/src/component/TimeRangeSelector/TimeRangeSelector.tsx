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
    Button,
    Divider,
    IconButton,
    InputAdornment,
    MenuItem,
    Popover,
    Select,
    Stack,
    Typography,
} from "@wso2/oxygen-ui";
import { Clock, X } from "@wso2/oxygen-ui-icons-react";
import { TextInput } from "../FormElements/TextInput";

const CUSTOM_VALUE = "__custom__";

const isoToLocalInput = (iso: string): string => {
    const d = new Date(iso);
    return new Date(d.getTime() - d.getTimezoneOffset() * 60_000)
        .toISOString()
        .slice(0, 16);
};

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
    onCustomRangeClear: () => void;
}

export const TimeRangeSelector: React.FC<TimeRangeSelectorProps> = ({
    preset,
    customStart,
    customEnd,
    options,
    onPresetChange,
    onCustomRangeApply,
    onCustomRangeClear,
}) => {
    const anchorRef = useRef<HTMLElement>(null);
    const [popoverOpen, setPopoverOpen] = useState(false);
    const [draftStart, setDraftStart] = useState("");
    const [draftEnd, setDraftEnd] = useState("");

    const hasCustomRange = !!customStart && !!customEnd;
    const selectValue = hasCustomRange ? CUSTOM_VALUE : (preset ?? "");

    const openPopover = () => {
        const now = new Date();
        const hourAgo = new Date(now.getTime() - 60 * 60 * 1000);
        setDraftStart(isoToLocalInput(customStart ?? hourAgo.toISOString()));
        setDraftEnd(isoToLocalInput(customEnd ?? now.toISOString()));
        setPopoverOpen(true);
    };

    const handleApply = () => {
        onCustomRangeApply(new Date(draftStart).toISOString(), new Date(draftEnd).toISOString());
        setPopoverOpen(false);
    };

    const isApplyDisabled = !draftStart || !draftEnd || draftStart >= draftEnd;

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
                    if (e.target.value === CUSTOM_VALUE) {
                        openPopover();
                    } else {
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
                <MenuItem value={CUSTOM_VALUE}>Custom range...</MenuItem>
            </Select>

            {hasCustomRange && (
                <IconButton size="small" onClick={onCustomRangeClear} aria-label="Clear custom range">
                    <X size={14} />
                </IconButton>
            )}

            <Popover
                open={popoverOpen}
                anchorEl={anchorRef.current}
                onClose={() => setPopoverOpen(false)}
                anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
                transformOrigin={{ vertical: "top", horizontal: "right" }}
            >
                <Stack spacing={2} sx={{ p: 2, width: 300 }}>
                    <Typography variant="h6">
                        Custom Time Range
                    </Typography>
                    <Divider />
                    <TextInput
                        label="Start"
                        type="datetime-local"
                        size="small"
                        value={draftStart}
                        onChange={(e) => setDraftStart(e.target.value)}
                    />
                    <TextInput
                        label="End"
                        type="datetime-local"
                        size="small"
                        value={draftEnd}
                        onChange={(e) => setDraftEnd(e.target.value)}
                        inputProps={{ min: draftStart || undefined }}
                    />
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
