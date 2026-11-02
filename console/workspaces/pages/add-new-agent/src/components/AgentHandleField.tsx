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

import { IconButton, InputAdornment } from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import { TextInput } from "@agent-management-platform/views";

/** Matches the agent name limit in the form schema. */
const AGENT_HANDLE_MAX_LENGTH = 50;

interface AgentHandleFieldProps {
  value: string;
  /** False while the handle is the auto-generated one; the user unlocks it with the edit icon. */
  editable: boolean;
  onEdit: () => void;
  onChange: (value: string) => void;
  error?: string;
}

/**
 * The agent's unique handle: generated from its name and read-only until the user chooses
 * to customise it, like the project ID on project creation.
 */
export function AgentHandleField({
  value,
  editable,
  onEdit,
  onChange,
  error,
}: AgentHandleFieldProps) {
  return (
    <TextInput
      label="Agent handle"
      id="name"
      maxLength={AGENT_HANDLE_MAX_LENGTH}
      hideCharacterCount={!editable || value.length < AGENT_HANDLE_MAX_LENGTH}
      placeholder="e.g., customer-support-agent"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      error={!!error}
      helperText={error || (editable ? "Unique and cannot be changed later" : undefined)}
      disabled={!editable && !value}
      slotProps={{
        htmlInput: { readOnly: !editable },
        input: {
          endAdornment: !editable && (
            <InputAdornment position="end">
              <IconButton size="small" aria-label="Edit agent handle" onClick={onEdit}>
                <Edit size={16} />
              </IconButton>
            </InputAdornment>
          ),
        },
      }}
    />
  );
}
