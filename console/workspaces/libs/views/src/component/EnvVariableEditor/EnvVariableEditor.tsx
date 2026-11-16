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
  Checkbox,
  FormControl,
  FormControlLabel,
  FormLabel,
  IconButton,
  InputAdornment,
  Stack,
  Tooltip,
} from '@wso2/oxygen-ui';
import { Edit, Eye, EyeOff, Lock, Trash2 as DeleteOutline, X } from '@wso2/oxygen-ui-icons-react';
import { useState } from 'react';
import { TextInput } from '../FormElements';
import { MAX_FILE_SIZE, parseEnvFileContent } from '../EnvFileUpload';
import { INPUT_LIMITS } from "@agent-management-platform/types";

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
   * Called instead of onKeyChange/onValueChange when the pasted clipboard text
   * contains multiple "KEY=VALUE" lines (e.g. the full contents of a .env
   * file), so the caller can split it into multiple rows.
   */
  onBulkPaste?: (entries: { key: string; value: string }[]) => void;
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
   * When true, the key and value fields are locked by default (both unlock
   * together). Non-system rows can unlock them either via the explicit Edit
   * action, or by unchecking "Mark as Secret" (which clears isSensitive).
   */
  isExistingSecret?: boolean;
  /**
   * Whether this variable is injected by the platform rather than user-managed
   * (e.g. AMP-provided). When true, the key/value fields and the secret
   * checkbox are disabled, edit/delete are hidden, and a lock icon is shown
   * in their place.
   */
  isSystem?: boolean;
}

export function EnvVariableEditor({
  index,
  keyValue,
  valueValue,
  onKeyChange,
  onValueChange,
  onRemove,
  onBulkPaste,
  keyLabel = 'Key',
  valueLabel = 'Value',
  isValueSecret = false,
  isSensitive = false,
  onSensitiveChange,
  keyError,
  valueError,
  keyDisabled = false,
  isExistingSecret = false,
  isSystem = false,
}: EnvVariableEditorProps) {
  // Existing secrets start locked: the stored value is never returned, so the
  // field is masked until the user explicitly clicks Edit. The key name is locked
  // alongside it — both unlock together, since renaming without invalidating the
  // stored secretRef is exactly what Edit is meant to allow.
  const [isEditing, setIsEditing] = useState(false);
  // Snapshot of the key/value when editing started, so Cancel can restore both even
  // after the user has typed changes. Kept as one object since the two always move
  // together (set on Edit, read on Cancel).
  const [beforeEdit, setBeforeEdit] = useState({ key: '', value: '' });
  // Toggles plaintext reveal of a secret value while the user is typing it.
  const [showValue, setShowValue] = useState(false);

  const isSecretField = isValueSecret || isSensitive;
  const isSecretLocked = isExistingSecret && isSensitive && !isEditing;
  // System-injected variables are always non-editable, regardless of secret state.
  const isValueLocked = isSecretLocked || isSystem;

  // Lets users paste a full "KEY=VALUE" line into the Key field and have it
  // split automatically, instead of forcing a manual copy into each field.
  const handleKeyPaste = (e: React.ClipboardEvent<HTMLInputElement>) => {
    if (keyDisabled || isValueLocked) return;
    const pasted = e.clipboardData.getData('text');

    // A paste this large is almost certainly the wrong clipboard contents (e.g.
    // an entire log file), not a real .env list — let the browser's default
    // paste behavior handle it instead of splitting it into countless rows.
    if (pasted.length > MAX_FILE_SIZE) return;

    // Pasting one or more "KEY=VALUE" lines (including the full contents of a
    // .env file) parses through the same helper as the upload path, so a
    // single entry with a leading/trailing comment line isn't corrupted by
    // falling through to the raw-text fallback below.
    if (onBulkPaste) {
      const entries = parseEnvFileContent(pasted);
      if (entries.length > 1) {
        e.preventDefault();
        onBulkPaste(entries);
        return;
      }
      if (entries.length === 1) {
        e.preventDefault();
        onBulkPaste([entries[0]]);
        return;
      }
    }

    const equalsIdx = pasted.indexOf('=');
    if (equalsIdx === -1) return;

    e.preventDefault();
    const pastedKey = pasted.slice(0, equalsIdx).trim();
    let pastedValue = pasted.slice(equalsIdx + 1).trim();
    if (
      pastedValue.length >= 2 &&
      ((pastedValue.startsWith('"') && pastedValue.endsWith('"')) ||
        (pastedValue.startsWith("'") && pastedValue.endsWith("'")))
    ) {
      pastedValue = pastedValue.slice(1, -1);
    }

    onKeyChange(pastedKey.replace(/\s/g, '_'));
    onValueChange(pastedValue);
  };

  const handleStartEdit = () => {
    setBeforeEdit({ key: keyValue, value: valueValue });
    setIsEditing(true);
  };

  const handleCancelEdit = () => {
    // Restore the previous key/value and re-lock the fields, discarding any edits.
    onKeyChange(beforeEdit.key);
    onValueChange(beforeEdit.value);
    setIsEditing(false);
    setShowValue(false);
  };

  // Reveal toggle for secret values; hidden while the field is locked since
  // there is nothing entered to reveal.
  const valueEndAdornment =
    isSecretField && !isValueLocked ? (
      <InputAdornment position="end">
        <Tooltip title={showValue ? 'Hide value' : 'Show value'}>
          <IconButton
            size="small"
            edge="end"
            aria-label={showValue ? 'Hide value' : 'Show value'}
            onClick={() => setShowValue((prev) => !prev)}
          >
            {showValue ? <EyeOff size={16} /> : <Eye size={16} />}
          </IconButton>
        </Tooltip>
      </InputAdornment>
    ) : undefined;

  // Extra breathing room below rows whose helper text would otherwise crowd
  // the next row; compact rows (no error) keep the default tight spacing.
  const hasError = !!keyError || !!valueError;

  return (
    <Stack key={index} direction="column" gap={1} mb={hasError ? 0.5 : 0}>
      <Stack direction="row" gap={2} alignItems="flex-start">
        <Box flex={1} minWidth={0}>
          <TextInput
            maxLength={INPUT_LIMITS.KEY}
            label={keyLabel}
            fullWidth
            size="small"
            value={keyValue}
            onChange={(e) => onKeyChange(e.target.value.replace(/\s/g, '_'))}
            onPaste={handleKeyPaste}
            error={!!keyError}
            helperText={keyError}
            disabled={keyDisabled || isValueLocked}
          />
        </Box>
        <Box flex={1} minWidth={0}>
          <TextInput
            maxLength={INPUT_LIMITS.VALUE}
            label={valueLabel}
            type={isSecretField && !showValue ? 'password' : 'text'}
            fullWidth
            size="small"
            value={valueValue}
            onChange={(e) => onValueChange(e.target.value)}
            error={!!valueError}
            helperText={valueError}
            disabled={isValueLocked}
            placeholder={isSecretLocked ? '••••••••' : undefined}
            slotProps={{ input: { endAdornment: valueEndAdornment } }}
          />
        </Box>
        {/* Always reserve this slot, same reasoning as the edit/delete icons below:
            a row with no onSensitiveChange (e.g. a kind-declared secret, which can't
            be un-marked) would otherwise be narrower than its siblings and throw off
            the Key/Value column alignment across rows. */}
        <Box
          mr={4}
          sx={{
            visibility: onSensitiveChange ? 'visible' : 'hidden',
            pointerEvents: onSensitiveChange ? 'auto' : 'none',
          }}
        >
          {/* The Key/Value labels are static FormLabels stacked above their
              TextInput, not MUI's floating label, so their input boxes start
              below a full label's height. An invisible FormLabel of the same
              height (rather than a guessed padding value) is what keeps this
              checkbox's top edge flush with the Key/Value inputs' top edge. */}
          <FormControl>
            <FormLabel sx={{ visibility: 'hidden' }}>{' '}</FormLabel>
            <FormControlLabel
              control={
                <Checkbox
                  size="small"
                  checked={isSensitive}
                  disabled={isSystem}
                  onChange={(e) => onSensitiveChange?.(e.target.checked)}
                />
              }
              label="Mark as Secret"
              sx={{ whiteSpace: 'nowrap', marginRight: 0 }}
            />
          </FormControl>
        </Box>
        <Box>
          {/* Same reasoning as the "Mark as Secret" slot above: an invisible
              FormLabel matches the Key/Value labels' height so the icons'
              top edge lines up with the inputs' top edge instead of the labels'. */}
          <FormControl>
            <FormLabel sx={{ visibility: 'hidden' }}>{' '}</FormLabel>
            <Box display="flex" alignItems="center">
              {/* Always reserve this slot so the delete/lock icons stay aligned across
                  rows. Existing secrets toggle between Edit (locked) and Cancel
                  (editing); other fields keep the slot hidden. Cancel stays visible
                  for the whole edit session even after typing clears the stored
                  secret flag upstream. System rows are excluded outright, even when
                  they happen to be an existing secret — they're never editable. */}
              <Tooltip title={isEditing ? 'Cancel edit' : 'Edit key/value'}>
                <IconButton
                  size="small"
                  aria-label={isEditing ? 'Cancel edit' : 'Edit key/value'}
                  onClick={isEditing ? handleCancelEdit : handleStartEdit}
                  sx={
                    isEditing
                      ? undefined
                      : {
                        visibility: isSecretLocked && !isSystem ? 'visible' : 'hidden',
                        pointerEvents: isSecretLocked && !isSystem ? 'auto' : 'none',
                      }
                  }
                >
                  {isEditing ? <X size={16} /> : <Edit size={16} />}
                </IconButton>
              </Tooltip>
              {isSystem ? (
                // Same slot/size as the delete button below, so system rows line up
                // pixel-for-pixel with editable rows instead of drifting left.
                <Tooltip title="System-injected variable — managed by the platform, not editable">
                  <IconButton
                    size="small"
                    disableRipple
                    tabIndex={-1}
                    aria-label="System-injected variable — managed by the platform, not editable"
                    sx={{ cursor: 'default' }}
                  >
                    <Lock size={16} />
                  </IconButton>
                </Tooltip>
              ) : (
                <IconButton size="small" color="error" onClick={onRemove}>
                  <DeleteOutline size={16} />
                </IconButton>
              )}
            </Box>
          </FormControl>
        </Box>
      </Stack>
    </Stack>
  );
}
