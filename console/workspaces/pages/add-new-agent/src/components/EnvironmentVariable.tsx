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
import { EnvVariableEditor } from "@agent-management-platform/views";
import { CreateAgentFormValues } from "../form/schema";

interface EnvironmentVariableProps {
  formData: CreateAgentFormValues;
  setFormData: React.Dispatch<React.SetStateAction<CreateAgentFormValues>>;
}

export const EnvironmentVariable = ({
  formData,
  setFormData,
}: EnvironmentVariableProps) => {
  const envVariables = formData.env || [];
  const isOneEmpty = envVariables.some((e) => !e?.key || !e?.value);

  const handleAdd = () => {
    setFormData((prev) => ({
      ...prev,
      env: [...(prev.env || []), { key: '', value: '' }],
    }));
  };

  const handleRemove = (index: number) => {
    setFormData((prev) => ({
      ...prev,
      env: prev.env?.filter((_, i) => i !== index) || [],
    }));
  };

  const handleChange = (index: number, field: 'key' | 'value', value: string) => {
    setFormData((prev) => ({
      ...prev,
      env: prev.env?.map((item, i) =>
        i === index ? { ...item, [field]: value } : item
      ) || [],
    }));
  };

  const handleInitialEdit = (field: 'key' | 'value', value: string) => {
    setFormData((prev) => {
      const envList = prev.env || [];
      if (envList.length > 0) {
        return {
          ...prev,
          env: envList.map((item, i) =>
            i === 0 ? { ...item, [field]: value } : item
          ),
        };
      }

      return {
        ...prev,
        env: [
          {
            key: field === 'key' ? value : '',
            value: field === 'value' ? value : '',
          },
        ],
      };
    });
  };

  return (
    <Card variant="outlined">
      <CardContent>
        <Box display="flex" flexDirection="row" alignItems="center" gap={1}>
          <Typography variant="h5">
            Environment Variables (Optional)
          </Typography>
        </Box>
        <Box display="flex" flexDirection="column" py={2} gap={2}>
          {envVariables.length ? envVariables.map((item, index) => (
            <EnvVariableEditor
              key={`env-${index}`}
              index={index}
              keyValue={item.key || ''}
              valueValue={item.value || ''}
              onKeyChange={(value) => handleChange(index, 'key', value)}
              onValueChange={(value) => handleChange(index, 'value', value)}
              onRemove={() => handleRemove(index)}
            />
          )) :
            <EnvVariableEditor
              key={`env-0`}
              index={0}
              keyValue={envVariables?.[0]?.key || ''}
              valueValue={envVariables?.[0]?.value || ''}
              onKeyChange={(value) => handleInitialEdit('key', value)}
              onValueChange={(value) => handleInitialEdit('value', value)}
              onRemove={() => handleRemove(0)}
            />
          }
        </Box>
        <Button
          startIcon={<Add fontSize="small" />}
          disabled={isOneEmpty}
          variant="outlined"
          color="primary"
          onClick={handleAdd}
        >
          Add
        </Button>
      </CardContent>
    </Card>
  );
};
