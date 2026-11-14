/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  Checkbox,
  FormControl,
  IconButton,
  InputAdornment,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Eye, EyeOff } from "@wso2/oxygen-ui-icons-react";
import { INPUT_LIMITS } from "@agent-management-platform/types";

export interface AuthHeaderRowProps {
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
  headerValue: string;
  onHeaderChange: (value: string) => void;
  valueValue: string;
  onValueChange: (value: string) => void;
  onValueFocus?: () => void;
  showValue: boolean;
  onToggleShowValue: () => void;
  error?: boolean;
  caption?: string | null;
  captionColor?: "error" | "text.secondary";
  monospaceValue?: boolean;
  iconButtonSize?: "small" | "medium";
}

// Postman-style auth header row: a checkbox that excludes the header from the
// fetch/save without losing the typed key/value, plus Key/Value fields.
export function AuthHeaderRow({
  enabled,
  onEnabledChange,
  headerValue,
  onHeaderChange,
  valueValue,
  onValueChange,
  onValueFocus,
  showValue,
  onToggleShowValue,
  error = false,
  caption,
  captionColor = "text.secondary",
  monospaceValue = false,
  iconButtonSize,
}: AuthHeaderRowProps) {
  return (
    <Stack spacing={1}>
      <Typography variant="subtitle2" fontWeight={600}>
        Headers
      </Typography>
      <Stack direction="row" spacing={1.5} alignItems="center" useFlexGap>
        <Checkbox
          size="small"
          checked={enabled}
          onChange={(event) => onEnabledChange(event.target.checked)}
          aria-label="Enable header"
        />
        <FormControl sx={{ flex: 1 }} error={error}>
          <TextField
            slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.KEY } }}
            fullWidth
            size="small"
            placeholder="Key"
            value={headerValue}
            onChange={(event) => onHeaderChange(event.target.value)}
            disabled={!enabled}
            error={error}
          />
        </FormControl>
        <FormControl sx={{ flex: 1 }} error={error}>
          <TextField
            fullWidth
            size="small"
            placeholder="Value"
            value={valueValue}
            onFocus={onValueFocus}
            onChange={(event) => onValueChange(event.target.value)}
            disabled={!enabled}
            error={error}
            type={showValue ? "text" : "password"}
            slotProps={{
              htmlInput: { maxLength: INPUT_LIMITS.VALUE },
              input: {
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      size={iconButtonSize}
                      aria-label={
                        showValue ? "Hide header value" : "Show header value"
                      }
                      onClick={onToggleShowValue}
                      edge="end"
                      disabled={!enabled}
                    >
                      {showValue ? <EyeOff size={18} /> : <Eye size={18} />}
                    </IconButton>
                  </InputAdornment>
                ),
              },
            }}
            sx={
              monospaceValue
                ? { "& .MuiInputBase-input": { fontFamily: "monospace" } }
                : undefined
            }
          />
        </FormControl>
      </Stack>
      {caption && (
        <Typography variant="caption" color={captionColor} sx={{ pl: 5 }}>
          {caption}
        </Typography>
      )}
    </Stack>
  );
}

export default AuthHeaderRow;
