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

import { Box, IconButton, Stack } from '@wso2/oxygen-ui';
import { Trash2 as DeleteOutline } from '@wso2/oxygen-ui-icons-react';
import { TextInput } from '../FormElements';

export interface EnvVariableEditorProps {
  /**
   * Index of the environment variable in the array
   */
  index: number;
  /**
   * Current value of the key field
   */
  keyValue: string;
  /**
   * Current value of the value field
   */
  valueValue: string;
  /**
   * Callback when key field changes
   */
  onKeyChange: (value: string) => void;
  /**
   * Callback when value field changes
   */
  onValueChange: (value: string) => void;
  /**
   * Callback to remove this environment variable
   */
  onRemove: () => void;
  /**
   * Label for the key field (default: "Key")
   */
  keyLabel?: string;
  /**
   * Label for the value field (default: "Value")
   */
  valueLabel?: string;
  /**
   * Whether the value field should be a password type (default: false)
   */
  isValueSecret?: boolean;
  /**
   * Error message for the key field
   */
  keyError?: string;
  /**
   * Error message for the value field
   */
  valueError?: string;
}

export function EnvVariableEditor({
  index,
  keyValue,
  valueValue,
  onKeyChange,
  onValueChange,
  onRemove,
  keyLabel = 'Key',
  valueLabel = 'Value',
  isValueSecret = false,
  keyError,
  valueError,
}: EnvVariableEditorProps) {
  return (
    <Stack key={index} direction="row" gap={2} alignItems="end">
      <Box flexGrow={1}>
        <TextInput
          label={keyLabel}
          fullWidth
          size="small"
          value={keyValue}
          onChange={(e) => onKeyChange(e.target.value)}
          error={!!keyError}
          helperText={keyError}
        />
      </Box>
      <Box flexGrow={1}>
        <TextInput
          label={valueLabel}
          type={isValueSecret ? 'password' : 'text'}
          fullWidth
          size="small"
          value={valueValue}
          onChange={(e) => onValueChange(e.target.value)}
          error={!!valueError}
          helperText={valueError}
        />
      </Box>
      <Box pb={1}>
        <IconButton size="small" color="error" onClick={onRemove}>
          <DeleteOutline size={16} />
        </IconButton>
      </Box>
    </Stack>
  );
}
