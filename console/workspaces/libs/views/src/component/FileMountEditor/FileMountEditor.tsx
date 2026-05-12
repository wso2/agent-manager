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

import { useRef, useState } from 'react';
import { Alert, Box, Button, Checkbox, Chip, FormControlLabel, IconButton, Stack } from '@wso2/oxygen-ui';
import { Trash2 as DeleteOutline, Edit as EditIcon, X as CancelIcon, Upload as UploadIcon } from '@wso2/oxygen-ui-icons-react';
import { TextInput } from '../FormElements';

export interface FileMountEditorProps {
  index: number;
  keyValue: string;
  mountPathValue: string;
  contentValue: string;
  onKeyChange: (value: string) => void;
  onMountPathChange: (value: string) => void;
  onContentChange: (value: string) => void;
  onRemove: () => void;
  isSensitive?: boolean;
  onSensitiveChange?: (value: boolean) => void;
  keyError?: string;
  mountPathError?: string;
  contentError?: string;
  isExistingSecret?: boolean;
}

export function FileMountEditor({
  index,
  keyValue,
  mountPathValue,
  contentValue,
  onKeyChange,
  onMountPathChange,
  onContentChange,
  onRemove,
  isSensitive = false,
  onSensitiveChange,
  keyError,
  mountPathError,
  contentError,
  isExistingSecret = false,
}: FileMountEditorProps) {
  const [isEditingSecret, setIsEditingSecret] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const isSecretLocked = isExistingSecret && isSensitive && !isEditingSecret;

  const handleEnableEditing = () => {
    setIsEditingSecret(true);
    onContentChange('');
  };

  const handleCancelEditing = () => {
    setIsEditingSecret(false);
    onContentChange('');
  };

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      onContentChange(reader.result as string);
      if (!keyValue) {
        onKeyChange(file.name);
      }
    };
    reader.readAsText(file);
    // Reset so the same file can be re-uploaded
    e.target.value = '';
  };

  return (
    <Stack key={index} direction="column" gap={1}>
      <Stack direction="row" gap={2} alignItems="end">
        <Box flex={1} minWidth={0}>
          <TextInput
            label="File Name"
            fullWidth
            size="small"
            value={keyValue}
            onChange={(e) => onKeyChange(e.target.value)}
            error={!!keyError}
            helperText={keyError}
          />
        </Box>
        <Box flex={1} minWidth={0}>
          <TextInput
            label="Mount Path"
            fullWidth
            size="small"
            value={mountPathValue}
            onChange={(e) => onMountPathChange(e.target.value)}
            error={!!mountPathError}
            helperText={mountPathError}
            placeholder="/etc/config"
          />
        </Box>
        {isExistingSecret && isSensitive && (
          <Box display="flex" alignItems="center" gap={1} pb={1}>
            <Chip label="Secret" size="small" color="warning" variant="outlined" />
            {!isEditingSecret ? (
              <IconButton size="small" color="primary" onClick={handleEnableEditing} title="Edit secret content">
                <EditIcon size={16} />
              </IconButton>
            ) : (
              <IconButton size="small" color="default" onClick={handleCancelEditing} title="Cancel editing">
                <CancelIcon size={16} />
              </IconButton>
            )}
          </Box>
        )}
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
      <Box>
        <TextInput
          label="File Content"
          fullWidth
          size="small"
          multiline
          minRows={3}
          maxRows={10}
          value={contentValue}
          onChange={(e) => onContentChange(e.target.value)}
          error={!!contentError}
          helperText={contentError}
          disabled={isSecretLocked}
          placeholder={isSecretLocked ? '••••••••' : 'Paste or type file content here...'}
          type={isSensitive && !isEditingSecret ? 'password' : 'text'}
        />
        {!isSecretLocked && (
          <Box mt={1}>
            <input
              type="file"
              ref={fileInputRef}
              onChange={handleFileUpload}
              style={{ display: 'none' }}
            />
            <Button
              variant="text"
              size="small"
              startIcon={<UploadIcon size={14} />}
              onClick={() => fileInputRef.current?.click()}
            >
              Upload file
            </Button>
          </Box>
        )}
      </Box>
      {isEditingSecret && (
        <Alert severity="warning" sx={{ py: 0.5 }}>
          Updating a Secret file removes the previous content permanently and cannot be restored.
        </Alert>
      )}
    </Stack>
  );
}
