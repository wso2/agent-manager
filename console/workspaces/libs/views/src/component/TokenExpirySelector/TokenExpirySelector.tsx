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
  MenuItem,
  Popover,
  Select,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { isValid } from "date-fns";
import {
  customDateToExpiresIn,
  TOKEN_EXPIRY_PRESETS,
} from "./tokenExpiryPresets";

const CUSTOM_VALUE = "__custom__";
const isValidDate = (d: Date | null): d is Date => isValid(d);

const fmtDate = (d: Date) =>
  d.toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });

export interface TokenExpirySelectorProps {
  // Current Go duration value (e.g. "2160h"). Presets match on this;
  // anything else renders as custom.
  value: string;
  onChange: (expiresIn: string) => void;
  disabled?: boolean;
  size?: "small" | "medium";
}

export const TokenExpirySelector: React.FC<TokenExpirySelectorProps> = ({
  value,
  onChange,
  disabled,
  size = "small",
}) => {
  const anchorRef = useRef<HTMLDivElement>(null);
  const [popoverOpen, setPopoverOpen] = useState(false);
  const [draft, setDraft] = useState<Date | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Label shown for a custom value the user has picked this session.
  const [customLabel, setCustomLabel] = useState<string | null>(null);

  const isPreset = TOKEN_EXPIRY_PRESETS.some((p) => p.value === value);
  const selectValue = isPreset ? value : CUSTOM_VALUE;

  const now = new Date();
  const minDate = new Date(now.getTime() + 60 * 60 * 1000); // at least 1h out

  const openPopover = () => {
    setDraft(new Date(now.getTime() + 90 * 24 * 60 * 60 * 1000));
    setError(null);
    setPopoverOpen(true);
  };

  const handleApply = () => {
    if (!isValidDate(draft)) {
      setError("Pick a valid date");
      return;
    }
    try {
      const expiresIn = customDateToExpiresIn(draft);
      setCustomLabel(fmtDate(draft));
      onChange(expiresIn);
      setPopoverOpen(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Invalid expiry date");
    }
  };

  return (
    <Stack direction="row" spacing={0.5} alignItems="center">
      <Select
        ref={anchorRef}
        size={size}
        variant="outlined"
        value={selectValue}
        disabled={disabled}
        renderValue={(v) =>
          v === CUSTOM_VALUE
            ? (customLabel ?? "Custom date")
            : (TOKEN_EXPIRY_PRESETS.find((p) => p.value === v)?.label ?? v)
        }
        onChange={(e) => {
          if (e.target.value !== CUSTOM_VALUE) {
            onChange(e.target.value as string);
          }
        }}
        sx={{ minWidth: 130 }}
      >
        {TOKEN_EXPIRY_PRESETS.map((p) => (
          <MenuItem key={p.value} value={p.value}>
            {p.label}
          </MenuItem>
        ))}
        <Divider />
        <MenuItem value={CUSTOM_VALUE} onClick={openPopover}>
          Custom date...
        </MenuItem>
      </Select>

      <Popover
        open={popoverOpen}
        anchorEl={anchorRef.current}
        onClose={() => setPopoverOpen(false)}
        anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
        transformOrigin={{ vertical: "top", horizontal: "right" }}
      >
        <Stack spacing={2} sx={{ p: 2, width: 320 }}>
          <Typography variant="h6">Custom Expiry</Typography>
          <Divider />
          <DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
            <Form.ElementWrapper label="Expires at" name="expiresAt">
              <DatePickers.DateTimePicker
                value={draft}
                onChange={(v) => {
                  setDraft(v);
                  setError(null);
                }}
                minDateTime={minDate}
                slotProps={{ textField: { size: "small", fullWidth: true } }}
              />
            </Form.ElementWrapper>
          </DatePickers.LocalizationProvider>
          {error ? (
            <Typography variant="body2" color="error">
              {error}
            </Typography>
          ) : null}
          <Stack direction="row" spacing={1} justifyContent="flex-end">
            <Button size="small" variant="text" onClick={() => setPopoverOpen(false)}>
              Cancel
            </Button>
            <Button size="small" variant="contained" onClick={handleApply}>
              Apply
            </Button>
          </Stack>
        </Stack>
      </Popover>
    </Stack>
  );
};

export default TokenExpirySelector;
