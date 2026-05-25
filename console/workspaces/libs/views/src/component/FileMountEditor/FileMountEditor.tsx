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
import {
  Trash2 as DeleteOutline,
  Edit as EditIcon,
  Upload as UploadIcon,
} from '@wso2/oxygen-ui-icons-react';
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
  onSecretEditCancel?: () => void;
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
  onSecretEditCancel,
}: FileMountEditorProps) {
  const initialIsNew = useRef(
    !keyValue && !mountPathValue && !contentValue,
  );
  const [isEditing, setIsEditing] = useState(
    initialIsNew.current,
  );
  const [isEditingSecret, setIsEditingSecret] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [snapshot, setSnapshot] = useState<{
    key: string; mountPath: string;
    content: string; isSensitive: boolean;
  } | null>(null);

  const handleEnableEditing = () => {
    setSnapshot({
      key: keyValue,
      mountPath: mountPathValue,
      content: contentValue,
      isSensitive,
    });
    setIsEditing(true);
    if (isExistingSecret && isSensitive) {
      setIsEditingSecret(true);
    }
  };

  const handleCancelEditing = () => {
    if (snapshot) {
      onKeyChange(snapshot.key);
      onMountPathChange(snapshot.mountPath);
      onContentChange(snapshot.content);
      if (onSensitiveChange
        && snapshot.isSensitive !== isSensitive) {
        onSensitiveChange(snapshot.isSensitive);
      }
      setSnapshot(null);
    }
    setIsEditing(false);
    if (isEditingSecret) {
      setIsEditingSecret(false);
      onSecretEditCancel?.();
    }
  };

  const handleUpdate = () => {
    setSnapshot(null);
    setIsEditing(false);
    setIsEditingSecret(false);
  };

  const MAX_FILE_SIZE = 1_000_000; // 1 MB — matches backend schema limit
  const [uploadError, setUploadError] = useState<string | null>(null);

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setUploadError(null);

    if (file.size > MAX_FILE_SIZE) {
      setUploadError(`File exceeds 1 MB limit (${(file.size / 1_000_000).toFixed(1)} MB)`);
      e.target.value = '';
      return;
    }

    const reader = new FileReader();
    reader.onload = () => {
      onContentChange(reader.result as string);
      if (!keyValue) {
        onKeyChange(file.name);
      }
    };
    reader.onerror = () => {
      setUploadError('Failed to read file');
    };
    reader.readAsText(file, 'utf-8');
    e.target.value = '';
  };

  // Collapsed (read-only) view
  if (!isEditing) {
    return (
      <Stack key={index} direction="row" gap={2} alignItems="center">
        <Box flex={1} minWidth={0}>
          <Box component="span" fontWeight={500}>File Name:</Box>{' '}{keyValue}
        </Box>
        <Box flex={1} minWidth={0}>
          <Box component="span" fontWeight={500}>Mount Path:</Box>{' '}{mountPathValue}
        </Box>
        <Box
          display="flex" alignItems="center" gap={1}
          minWidth={120} justifyContent="flex-end"
        >
          {(isSensitive && (isExistingSecret
            || (onSensitiveChange && !isExistingSecret))) && (
            <Chip label="Secret" size="small" color="warning" variant="outlined" />
          )}
          <IconButton size="small" color="primary" onClick={handleEnableEditing} title="Edit file mount">
            <EditIcon size={16} />
          </IconButton>
          <IconButton size="small" color="error" onClick={onRemove}>
            <DeleteOutline size={16} />
          </IconButton>
        </Box>
      </Stack>
    );
  }

  // Expanded (editing) view
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
          value={isEditingSecret ? (contentValue || '') : contentValue}
          onChange={(e) => onContentChange(e.target.value)}
          error={!!contentError}
          helperText={contentError}
          placeholder={
            isEditingSecret
              ? 'Enter new secret content...'
              : 'Paste or type file content here...'
          }
          type={
            isSensitive && !isEditingSecret && !isEditing
              ? 'password' : 'text'
          }
        />
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
          {uploadError && (
            <Alert severity="error" sx={{ mt: 1, py: 0.5 }}>{uploadError}</Alert>
          )}
        </Box>
      </Box>
      {isEditingSecret && (
        <Alert severity="warning" sx={{ py: 0.5 }}>
          Updating a Secret file removes the previous content permanently and cannot be restored.
        </Alert>
      )}
      {!initialIsNew.current && (
        <Stack direction="row" gap={1} justifyContent="flex-end">
          <Button variant="outlined" size="small" onClick={handleCancelEditing}>
            Cancel
          </Button>
          <Button variant="contained" size="small" color="primary" onClick={handleUpdate}>
            Update
          </Button>
        </Stack>
      )}
    </Stack>
  );
}
