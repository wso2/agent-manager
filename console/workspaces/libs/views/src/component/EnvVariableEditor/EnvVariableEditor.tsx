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

import { useState } from 'react';
import { Alert, Box, Checkbox, Chip, FormControlLabel, IconButton, Stack } from '@wso2/oxygen-ui';
import { Trash2 as DeleteOutline, Edit as EditIcon, X as CancelIcon } from '@wso2/oxygen-ui-icons-react';
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
   * Whether this env variable is marked as sensitive/secret
   */
  isSensitive?: boolean;
  /**
   * Callback when isSensitive checkbox changes
   */
  onSensitiveChange?: (value: boolean) => void;
  /**
   * Error message for the key field
   */
  keyError?: string;
  /**
   * Error message for the value field
   */
  valueError?: string;
  /**
   * Whether the key field is disabled (e.g. when keys are pre-filled from provider)
   */
  keyDisabled?: boolean;
  /**
   * Whether this is an existing secret (already saved, not newly created)
   * When true, the value field will be locked by default and require explicit edit action
   */
  isExistingSecret?: boolean;
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
  isSensitive = false,
  onSensitiveChange,
  keyError,
  valueError,
  keyDisabled = false,
  isExistingSecret = false,
}: EnvVariableEditorProps) {
  const [isEditingSecret, setIsEditingSecret] = useState(false);

  // For existing secrets, value is locked unless user explicitly enables editing
  const isSecretLocked = isExistingSecret && isSensitive && !isEditingSecret;

  const handleEnableEditing = () => {
    setIsEditingSecret(true);
    // Clear the value when enabling editing since the existing value is masked
    onValueChange('');
  };

  const handleCancelEditing = () => {
    setIsEditingSecret(false);
    // Clear the value since we're canceling the edit
    onValueChange('');
  };

  return (
    <Stack key={index} direction="column" gap={1}>
      <Stack direction="row" gap={2} alignItems="end">
        <Box flex={1} minWidth={0}>
          <TextInput
            label={keyLabel}
            fullWidth
            size="small"
            value={keyValue}
            onChange={(e) => onKeyChange(e.target.value.replace(/\s/g, '_'))}
            error={!!keyError}
            helperText={keyError}
            disabled={keyDisabled}
          />
        </Box>
        <Box flex={1} minWidth={0}>
          <TextInput
            label={valueLabel}
            type={isValueSecret || isSensitive ? 'password' : 'text'}
            fullWidth
            size="small"
            value={valueValue}
            onChange={(e) => onValueChange(e.target.value)}
            error={!!valueError}
            helperText={valueError}
            disabled={isSecretLocked}
            placeholder={isSecretLocked ? '••••••••' : undefined}
          />
        </Box>
        {/* Show Secret chip and edit/cancel button for existing secrets */}
        {isExistingSecret && isSensitive && (
          <Box display="flex" alignItems="center" gap={1} pb={1}>
            <Chip
              label="Secret"
              size="small"
              color="warning"
              variant="outlined"
            />
            {!isEditingSecret ? (
              <IconButton
                size="small"
                color="primary"
                onClick={handleEnableEditing}
                title="Edit secret value"
              >
                <EditIcon size={16} />
              </IconButton>
            ) : (
              <IconButton
                size="small"
                color="default"
                onClick={handleCancelEditing}
                title="Cancel editing"
              >
                <CancelIcon size={16} />
              </IconButton>
            )}
          </Box>
        )}
        {/* Show checkbox for new env variables (not existing secrets) */}
        {onSensitiveChange && !isExistingSecret && (
          <Box mr={4}>
            <FormControlLabel
              control={
                <Checkbox
                  size="small"
                  checked={isSensitive}
                  onChange={(e) => onSensitiveChange(e.target.checked)}
                />
              }
              label="Mark as Secret"
              sx={{ whiteSpace: 'nowrap', marginRight: 0 }}
            />
          </Box>
        )}
        <Box pb={1}>
          <IconButton size="small" color="error" onClick={onRemove}>
            <DeleteOutline size={16} />
          </IconButton>
        </Box>
      </Stack>
      {/* Warning message when editing an existing secret */}
      {isEditingSecret && (
        <Alert severity="warning" sx={{ py: 0.5 }}>
          Updating a Secret value removes the previous value permanently and cannot be restored.
        </Alert>
      )}
    </Stack>
  );
}
