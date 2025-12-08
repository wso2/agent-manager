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
  FormControl,
  FormLabel,
  TextField,
  TextFieldProps,
  IconButton,
  Tooltip,
  InputAdornment,
} from '@wso2/oxygen-ui';
import { Copy as ContentCopy } from '@wso2/oxygen-ui-icons-react';
import { useState } from 'react';

export interface TextInputProps extends Omit<TextFieldProps, 'variant'> {
  label?: string;
  copyable?: boolean;
  copyTooltipText?: string;
}

export const TextInput = ({ 
  label, 
  copyable = false,
  copyTooltipText,
  value,
  slotProps,
  ...props 
}: TextInputProps) => {
  const [copied, setCopied] = useState(false);

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

  const endAdornment = copyable && typeof value === 'string' && value ? (
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
  };

  return (
    <FormControl fullWidth>
      {label && <FormLabel htmlFor={label}>{label}</FormLabel>}
      <TextField
        id={label}
        sx={{
          minWidth: 100,
        }}
        variant="outlined"
        value={value}
        slotProps={mergedSlotProps}
        {...props}
      />
    </FormControl>
  );
};
