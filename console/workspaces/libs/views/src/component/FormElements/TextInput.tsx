/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
  Box,
  FormControl,
  FormLabel,
  TextField,
  type TextFieldProps,
  IconButton,
  Tooltip,
  InputAdornment,
} from '@wso2/oxygen-ui';
import { Copy as ContentCopy, Eye, EyeOff } from '@wso2/oxygen-ui-icons-react';
import { useState } from 'react';

export interface TextInputProps extends Omit<TextFieldProps, 'variant'> {
  label?: string;
  labelAction?: React.ReactNode;
  copyable?: boolean;
  copyTooltipText?: string;
  showPasswordToggle?: boolean;
  /**
   * Hard cap on the number of characters the field accepts. Beyond blocking
   * further typing, it renders a live counter so the user sees the cap before
   * hitting it. Fields feed request bodies that an upstream WAF rejects when
   * they grow too large, so every free-text input should carry a limit — use
   * the shared `INPUT_LIMITS` values rather than an ad-hoc number.
   */
  maxLength?: number;
  /** Suppress the counter while keeping the cap (for dense or inline fields). */
  hideCharacterCount?: boolean;
}

export const TextInput = ({
  label,
  labelAction,
  copyable = false,
  copyTooltipText,
  value,
  slotProps,
  required,
  showPasswordToggle = false,
  type = 'text',
  maxLength,
  hideCharacterCount = false,
  helperText,
  error,
  ...props
}: TextInputProps) => {
  const [copied, setCopied] = useState(false);
  const [showPassword, setShowPassword] = useState(false);

  const handleCopy = async () => {
    if (typeof value === 'string' && value) {
      try {
        await navigator.clipboard.writeText(value);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      } catch {
        // Failed to copy - silently fail
      }
    }
  };

  const getCopyTooltipText = () => {
    if (copyTooltipText) {
      return copied ? 'Copied!' : copyTooltipText;
    }
    return copied ? 'Copied!' : 'Copy';
  };

  const isPasswordField = type === 'password' && showPasswordToggle;
  const displayType = isPasswordField && showPassword ? 'text' : type;

  const endAdornment = isPasswordField ? (
    <InputAdornment position="end">
      <Tooltip title={showPassword ? 'Hide password' : 'Show password'}>
        <IconButton
          onClick={() => setShowPassword(!showPassword)}
          edge="end"
          size="small"
          aria-label={showPassword ? 'Hide password' : 'Show password'}
        >
          {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
        </IconButton>
      </Tooltip>
    </InputAdornment>
  ) : copyable && typeof value === 'string' && value ? (
    <InputAdornment position="end">
      <Tooltip title={getCopyTooltipText()}>
        <IconButton
          onClick={handleCopy}
          edge="end"
          size="small"
        >
          <ContentCopy size={16} />
        </IconButton>
      </Tooltip>
    </InputAdornment>
  ) : undefined;

  const mergedSlotProps = {
    ...slotProps,
    input: {
      ...slotProps?.input,
      ...(endAdornment && { endAdornment }),
    },
    ...(maxLength != null && {
      htmlInput: {
        ...(slotProps?.htmlInput as object | undefined),
        maxLength,
      },
    }),
  };

  // The counter sits on the right of the helper row so an existing helper
  // message (or a validation error) keeps its place on the left.
  const characterCount = typeof value === 'string' ? value.length : 0;
  const showCounter = maxLength != null && !hideCharacterCount && !props.disabled;
  const resolvedHelperText = showCounter ? (
    <Box component="span" sx={{ display: 'flex', justifyContent: 'space-between', gap: 1 }}>
      <Box component="span">{helperText}</Box>
      <Box
        component="span"
        sx={{ flexShrink: 0, fontVariantNumeric: 'tabular-nums' }}
      >
        {characterCount}/{maxLength}
      </Box>
    </Box>
  ) : (
    helperText
  );

  return (
    <FormControl fullWidth>
      {label && (
        <FormLabel htmlFor={label} required={required}>
          <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
            {label}
            {labelAction}
          </Box>
        </FormLabel>
      )}
      <TextField
        id={label}
        sx={{
          minWidth: 100,
        }}
        variant="outlined"
        type={displayType}
        value={value}
        slotProps={mergedSlotProps}
        required={required}
        error={error}
        helperText={resolvedHelperText}
        {...props}
      />
    </FormControl>
  );
};
