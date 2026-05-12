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
import { FileMountEditor } from "@agent-management-platform/views";
import { CreateAgentFormValues } from "../form/schema";

interface FileMountProps {
  formData: CreateAgentFormValues;
  setFormData: React.Dispatch<React.SetStateAction<CreateAgentFormValues>>;
}

export const FileMount = ({
  formData,
  setFormData,
}: FileMountProps) => {
  const fileMounts = formData.files || [];
  const isOneEmpty = fileMounts.some((f) => !f?.key || !f?.mountPath);

  const handleAdd = () => {
    setFormData((prev) => ({
      ...prev,
      files: [...(prev.files || []), { key: '', mountPath: '', value: '', isSensitive: false }],
    }));
  };

  const handleRemove = (index: number) => {
    setFormData((prev) => ({
      ...prev,
      files: prev.files?.filter((_, i) => i !== index) || [],
    }));
  };

  const handleChange = (index: number, field: 'key' | 'mountPath' | 'value' | 'isSensitive', value: string | boolean) => {
    setFormData((prev) => ({
      ...prev,
      files: prev.files?.map((item, i) =>
        i === index ? { ...item, [field]: value } : item
      ) || [],
    }));
  };

  return (
    <Card variant="outlined">
      <CardContent>
        <Box display="flex" flexDirection="row" alignItems="center" gap={1}>
          <Typography variant="h5">
            File Mounts (Optional)
          </Typography>
        </Box>
        <Box display="flex" flexDirection="column" py={2} gap={2}>
          {fileMounts.map((item, index) => {
            const siblingKeys = new Set(
              fileMounts.flatMap((f, i) => (i !== index && f.key ? [f.key] : [])),
            );
            const keyError = item.key && siblingKeys.has(item.key)
              ? "Duplicate file name"
              : undefined;
            return (
              <FileMountEditor
                key={`file-${index}`}
                index={index}
                keyValue={item.key || ''}
                mountPathValue={item.mountPath || ''}
                contentValue={item.value || ''}
                isSensitive={item.isSensitive || false}
                onKeyChange={(value) => handleChange(index, 'key', value)}
                onMountPathChange={(value) => handleChange(index, 'mountPath', value)}
                onContentChange={(value) => handleChange(index, 'value', value)}
                onSensitiveChange={(value: boolean) => handleChange(index, 'isSensitive', value)}
                onRemove={() => handleRemove(index)}
                keyError={keyError}
              />
            );
          })}
        </Box>
        <Button
          startIcon={<Add fontSize="small" />}
          disabled={isOneEmpty}
          variant="outlined"
          color="primary"
          size="small"
          onClick={handleAdd}
        >
          Add File
        </Button>
      </CardContent>
    </Card>
  );
};
