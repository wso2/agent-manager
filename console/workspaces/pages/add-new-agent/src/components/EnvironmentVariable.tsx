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

import { Box, Button, Card, CardContent, Typography } from "@wso2/oxygen-ui";
import { Plus as Add } from "@wso2/oxygen-ui-icons-react";
import { EnvFileUploadButton, EnvVariableEditor } from "@agent-management-platform/views";
import { CreateAgentFormValues } from "../form/schema";

interface EnvironmentVariableProps {
  formData: CreateAgentFormValues;
  setFormData: React.Dispatch<React.SetStateAction<CreateAgentFormValues>>;
  llmReservedNames?: Set<string>;
  /** Keys that cannot be edited or removed (pre-defined by the Agent Kind schema) */
  lockedKeys?: Set<string>;
  /**
   * Keys declared as isSecret by the Agent Kind schema. Their value is never
   * pre-filled with the kind's real default (the backend doesn't return it), so
   * the field renders locked/masked until the user explicitly edits it, and the
   * "Mark as Secret" toggle is hidden since it isn't a user choice for these keys.
   */
  kindSecretKeys?: Set<string>;
  /**
   * Changing this value (e.g. the selected Agent Kind version) forces every row's
   * editor to remount instead of reusing its previous instance. Without it, a row
   * that was mid-edit (or had its value revealed) for one set of rows would silently
   * carry that open/revealed state into the next set once the underlying schema
   * changes, since editors are otherwise matched up by position, not identity.
   */
  resetKey?: string;
  /** Hide the Add button (e.g. in catalog flow where env vars are fully pre-defined) */
  hideAdd?: boolean;
  /**
   * Highlight rows that have a key but no value. Gated on a submit attempt
   * (mirrors the schema's own refine) rather than shown live while a row is
   * still mid-edit.
   */
  showMissingValueError?: boolean;
}

export const EnvironmentVariable = ({
  formData,
  setFormData,
  llmReservedNames = new Set(),
  lockedKeys = new Set(),
  kindSecretKeys = new Set(),
  resetKey = "",
  hideAdd = false,
  showMissingValueError = false,
}: EnvironmentVariableProps) => {
  const envVariables = formData.env || [];
  const isOneEmpty = envVariables.some((e) => !e?.key || !e?.value);

  const handleAdd = () => {
    setFormData((prev) => ({
      ...prev,
      env: [...(prev.env || []), { key: '', value: '', isSensitive: false }],
    }));
  };

  const handleRemove = (index: number) => {
    setFormData((prev) => ({
      ...prev,
      env: prev.env?.filter((_, i) => i !== index) || [],
    }));
  };

  const handleChange = (index: number, field: 'key' | 'value' | 'isSensitive', value: string | boolean) => {
    setFormData((prev) => ({
      ...prev,
      env: prev.env?.map((item, i) =>
        i === index ? { ...item, [field]: value } : item
      ) || [],
    }));
  };

  const handleEnvFileParsed = (entries: { key: string; value: string }[]) => {
    setFormData((prev) => {
      const nextEnv = [...(prev.env || [])].filter((e) => e.key || e.value);
      for (const rawEntry of entries) {
        const key = rawEntry.key.replace(/\s/g, '_');
        const value = rawEntry.value;
        if (lockedKeys.has(key) || kindSecretKeys.has(key)) continue;
        const existingIndex = nextEnv.findIndex((e) => e.key === key);
        if (existingIndex !== -1) {
          nextEnv[existingIndex] = { ...nextEnv[existingIndex], key, value };
        } else {
          nextEnv.push({ key, value, isSensitive: false });
        }
      }
      return { ...prev, env: nextEnv };
    });
  };

  return (
    <Card variant="outlined">
      <CardContent>
        <Typography variant="h5">
          {hideAdd ? "Environment Variables" : "Environment Variables (Optional)"}
        </Typography>
        <Box display="flex" flexDirection="column" py={2} gap={2}>
          {envVariables.map((item, index) => {
            const siblingKeys = new Set(
              envVariables.flatMap((e, i) => (i !== index && e.key ? [e.key] : [])),
            );
            const isLocked = !!item.key && lockedKeys.has(item.key);
            const isKindSecret = !!item.key && kindSecretKeys.has(item.key);
            const keyError = item.key && llmReservedNames.has(item.key)
              ? "Already used as an LLM provider variable"
              : item.key && siblingKeys.has(item.key)
                ? "Duplicate key"
                : undefined;
            // A kind-declared secret with a default (isKindSecret) is intentionally
            // left blank to inherit that default server-side — not a missing value.
            const valueError =
              showMissingValueError && item.key && !item.value?.trim() && !isKindSecret
                ? "Value is required"
                : undefined;
            return (
              <EnvVariableEditor
                key={`env-${resetKey}-${index}`}
                index={index}
                keyValue={item.key || ''}
                valueValue={item.value || ''}
                isSensitive={item.isSensitive || false}
                isExistingSecret={isKindSecret}
                valueLabel={isKindSecret ? "Value (uses kind default unless edited)" : undefined}
                onKeyChange={(value) => handleChange(index, 'key', value)}
                onValueChange={(value) => handleChange(index, 'value', value)}
                onBulkPaste={isLocked ? undefined : handleEnvFileParsed}
                onSensitiveChange={isKindSecret ? undefined : (value: boolean) => handleChange(index, 'isSensitive', value)}
                onRemove={isLocked ? () => {} : () => handleRemove(index)}
                keyDisabled={isLocked}
                keyError={keyError}
                valueError={valueError}
              />
            );
          })}
        </Box>
        {!hideAdd && (
          <Box display="flex" flexDirection="row" alignItems="center" gap={1.5} flexWrap="wrap">
            <Button
              startIcon={<Add fontSize="small" />}
              disabled={isOneEmpty}
              variant="outlined"
              color="primary"
              size="small"
              onClick={handleAdd}
            >
              Add
            </Button>
            <Box display="flex" flexDirection="column" alignItems="flex-start">
              <EnvFileUploadButton onParsed={handleEnvFileParsed} />
            </Box>
            <Typography variant="caption" color="text.secondary">
              or paste .env text into a Key field above
            </Typography>
          </Box>
        )}
      </CardContent>
    </Card>
  );
};
