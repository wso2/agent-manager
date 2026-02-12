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

import { CheckCircle, Circle } from "@wso2/oxygen-ui-icons-react";
import {
  Alert,
  Box,
  Collapse,
  Divider,
  Typography,
  Form,
  TextField,
} from "@wso2/oxygen-ui";
import { useCallback } from "react";
import type { CreateAgentFormValues } from "../form/schema";
import type { InputInterfaceType } from "@agent-management-platform/types";

interface InputInterfaceProps {
  formData: CreateAgentFormValues;
  setFormData: React.Dispatch<React.SetStateAction<CreateAgentFormValues>>;
  errors: Record<string, string | undefined>;
  setFieldError: (
    field: keyof CreateAgentFormValues,
    error: string | undefined
  ) => void;
  validateField: (
    field: keyof CreateAgentFormValues,
    value: unknown,
    fullData?: CreateAgentFormValues
  ) => string | undefined;
}

const inputInterfaces: Array<{
  label: string;
  description: string;
  default: boolean;
  value: InputInterfaceType;
}> = [
  {
    label: "Chat Agent",
    description: "Standard chat interface with /chat endpoint on port 8000",
    default: true,
    value: "DEFAULT",
  },
  {
    label: "Custom API Agent",
    description:
      "Custom HTTP API with user-specified OpenAPI specification and port configuration",
    default: false,
    value: "CUSTOM",
  },
];

export const InputInterface = ({
  formData,
  setFormData,
  errors,
  setFieldError,
  validateField,
}: InputInterfaceProps) => {
  const handleFieldChange = useCallback(
    (field: keyof CreateAgentFormValues, value: unknown) => {
      setFormData(prevData => {
        const newData = { ...prevData, [field]: value };
        // Validate with full data to catch refinements
        const error = validateField(field, value, newData as CreateAgentFormValues);
        setFieldError(field, error);
        return newData;
      });
    },
    [setFormData, validateField, setFieldError]
  );

  const handleSelect = useCallback(
    (value: InputInterfaceType) => {
      setFormData(prevData => {
        const newData = {
          ...prevData,
          interfaceType: value,
          ...(value === "DEFAULT" ? {
            openApiPath: "",
            port: "" as unknown as number,
            basePath: "/",
          } : {}),
        };
        
        // Validate interface type change with full data
        const error = validateField('interfaceType', value, newData as CreateAgentFormValues);
        setFieldError('interfaceType', error);
        
        // If switching to CUSTOM, validate the required fields
        if (value === 'CUSTOM') {
          // Validate port
          const portError = validateField('port', newData.port, newData as CreateAgentFormValues);
          setFieldError('port', portError);
          
          // Validate openApiPath
          const openApiError = validateField('openApiPath', newData.openApiPath, newData as CreateAgentFormValues);
          setFieldError('openApiPath', openApiError);
          
          // Validate basePath
          const basePathError = validateField('basePath', newData.basePath, newData as CreateAgentFormValues);
          setFieldError('basePath', basePathError);
        } else {
          // Clear validation errors when switching to DEFAULT
          setFieldError('port', undefined);
          setFieldError('openApiPath', undefined);
          setFieldError('basePath', undefined);
        }
        
        return newData;
      });
    },
    [setFormData, validateField, setFieldError]
  );

  const handlePortChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const next = e.target.value;
      if (/^\d*$/.test(next)) {
        handleFieldChange('port', next === "" ? ("" as unknown as number) : Number(next));
      }
    },
    [handleFieldChange]
  );

  return (
    <Form.Section>
      <Form.Subheader>Agent Type</Form.Subheader>
      <Typography variant="body2" color="text.secondary">
        How your agent receives requests
      </Typography>
      <Form.Stack spacing={2}>
        <Box display="flex" flexDirection="row" gap={1}>
          {inputInterfaces.map((inputInterface) => (
            <Form.CardButton
              key={inputInterface.value}
              onClick={() => handleSelect(inputInterface.value)}
              selected={formData.interfaceType === inputInterface.value}
              sx={{
                maxWidth: 500,
                flexGrow: 1,
              }}
            >
              <Form.CardContent sx={{ height: "100%" }}>
                <Box
                  display="flex"
                  flexDirection="row"
                  alignItems="center"
                  height="100%"
                  gap={1}
                >
                  <Box>
                    {formData.interfaceType === inputInterface.value ? (
                      <CheckCircle size={16} />
                    ) : (
                      <Circle size={16} />
                    )}
                  </Box>
                  <Divider orientation="vertical" flexItem />
                  <Box>
                    <Typography variant="h6">
                      {inputInterface.label}
                    </Typography>
                    <Typography variant="caption">
                      {inputInterface.description}
                    </Typography>
                  </Box>
                </Box>
              </Form.CardContent>
            </Form.CardButton>
          ))}
        </Box>
        <Collapse in={formData.interfaceType === "DEFAULT"}>
          <Alert severity="info">
            Uses the standard chat interface: <strong>POST /chat</strong> on
            port <strong>8000</strong>
            <br />
            Request:{" "}
            <code>{`{message: string, session_id: string, context: JSON}`}</code>
            <br />
            Response: <code>{`{response: string}`}</code>
          </Alert>
        </Collapse>
        <Collapse in={formData.interfaceType === "CUSTOM"}>
          <Form.Stack spacing={2}>
            <Form.Stack direction="row" spacing={2}>
              <Box display="flex" flexDirection="column" flexGrow={1}>
                <Form.ElementWrapper label="OpenAPI Spec Path" name="openApiPath">
                  <TextField
                    id="openApiPath"
                    placeholder="/openapi.yaml"
                    required
                    value={formData.openApiPath || ''}
                    onChange={(e) => handleFieldChange('openApiPath', e.target.value)}
                    error={!!errors.openApiPath}
                    helperText={
                      errors.openApiPath ||
                      "Path to OpenAPI schema file in your repository"
                    }
                    fullWidth
                  />
                </Form.ElementWrapper>
              </Box>
              <Box>
                <Form.ElementWrapper label="Port" name="port">
                  <TextField
                    id="port"
                    placeholder="8080"
                    required
                    value={formData.port ?? ''}
                    onChange={handlePortChange}
                    type="number"
                    error={!!errors.port}
                    helperText={
                      errors.port ||
                      (formData.port ? undefined : "Port is required")
                    }
                  />
                </Form.ElementWrapper>
              </Box>
            </Form.Stack>
            <Form.ElementWrapper label="Base Path" name="basePath">
              <TextField
                id="basePath"
                placeholder="/"
                required
                value={formData.basePath || ''}
                onChange={(e) => handleFieldChange('basePath', e.target.value)}
                error={!!errors.basePath}
                helperText={
                  errors.basePath ||
                  "API base path (e.g., / or /api/v1)"
                }
                fullWidth
              />
            </Form.ElementWrapper>
          </Form.Stack>
        </Collapse>
      </Form.Stack>
    </Form.Section>
  );
};
