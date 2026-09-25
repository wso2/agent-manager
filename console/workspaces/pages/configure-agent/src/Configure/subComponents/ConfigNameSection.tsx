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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Form, Typography } from "@wso2/oxygen-ui";
import { TextInput } from "@agent-management-platform/views";
import { INPUT_LIMITS } from "@agent-management-platform/types";

interface ConfigNameSectionProps {
  value: string;
  /** Receives the new name. The caller is responsible for marking it edited. */
  onChange: (value: string) => void;
  /** Explains where the auto-generated name comes from and that it's editable. */
  description: string;
  placeholder?: string;
}

/**
 * The "Configuration Name" section shared by the LLM and MCP agent-config
 * create forms. The name is auto-populated by the caller and stays editable.
 */
export function ConfigNameSection({
  value,
  onChange,
  description,
  placeholder,
}: ConfigNameSectionProps) {
  return (
    <Form.Section>
      <Form.Subheader>Configuration Name</Form.Subheader>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        {description}
      </Typography>
      <TextInput
        maxLength={INPUT_LIMITS.NAME}
        label="Name"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        fullWidth
      />
    </Form.Section>
  );
}
