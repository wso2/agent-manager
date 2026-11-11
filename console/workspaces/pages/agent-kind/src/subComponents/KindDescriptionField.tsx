/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import { Form } from "@wso2/oxygen-ui";
import { MarkdownEditor } from "@agent-management-platform/shared-component";

export const KIND_DESCRIPTION_PLACEHOLDER = "Describe this Agent Kind. Markdown is supported.";

export interface KindDescriptionFieldProps {
  id: string;
  value: string;
  onChange: (value: string) => void;
}

/** The "Description" field shared by the Agent Kind create/edit forms. */
export function KindDescriptionField({ id, value, onChange }: KindDescriptionFieldProps) {
  return (
    <Form.ElementWrapper label="Description" name={id}>
      <MarkdownEditor
        id={id}
        placeholder={KIND_DESCRIPTION_PLACEHOLDER}
        value={value}
        onChange={onChange}
      />
    </Form.ElementWrapper>
  );
}
