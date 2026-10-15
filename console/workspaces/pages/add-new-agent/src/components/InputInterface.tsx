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
  MenuItem,
  Select,
  TextField,
} from "@wso2/oxygen-ui";
import { useCallback } from "react";
import type { CreateAgentFormValues } from "../form/schema";
import { type InputInterfaceType, INPUT_LIMITS } from "@agent-management-platform/types";
import { readinessOnTypeChange, readinessOptionFor } from "../utils/readinessCheck";

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
  {
    label: "A2A Agent",
    description:
      "Speaks the A2A protocol. The agent serves its own agent card; the gateway exposes both A2A transports.",
    default: false,
    value: "A2A",
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
      // First update the form data
      let newData: CreateAgentFormValues | null = null;
      setFormData(prevData => {
        newData = { ...prevData, [field]: value } as CreateAgentFormValues;
        return newData;
      });
      // Then validate with the full updated data and set error (side effect outside updater)
      if (newData) {
        const error = validateField(field, value, newData);
        setFieldError(field, error);
      }
    },
    [setFormData, validateField, setFieldError]
  );

  const handleSelect = useCallback(
    (value: InputInterfaceType) => {
      // Compute new data outside the updater
      setFormData(prevData => {
        const newData = {
          ...prevData,
          interfaceType: value,
          ...(value === "DEFAULT" ? {
            openApiPath: "",
            port: "" as unknown as number,
            basePath: "/",
          } : {}),
          ...readinessOnTypeChange(prevData, value),
        };
        return newData;
      });

      // Perform all validations outside the updater
      setFormData(currentData => {
        const error = validateField('interfaceType', value, currentData);
        setFieldError('interfaceType', error);
        setFieldError(
          'readinessPath',
          validateField('readinessPath', currentData.readinessPath, currentData),
        );
        
        switch (value) {
          case 'CUSTOM': {
            const portError = validateField('port', currentData.port, currentData);
            setFieldError('port', portError);

            const openApiError = validateField('openApiPath', currentData.openApiPath, currentData);
            setFieldError('openApiPath', openApiError);

            const basePathError = validateField('basePath', currentData.basePath, currentData);
            setFieldError('basePath', basePathError);
            break;
          }
          case 'A2A': {
            // An A2A agent declares a port like a custom API, but serves its own
            // agent card at a well-known path — so it has no OpenAPI document and
            // no base path to validate.
            const portError = validateField('port', currentData.port, currentData);
            setFieldError('port', portError);

            setFieldError('openApiPath', undefined);
            setFieldError('basePath', undefined);
            break;
          }
          default:
            // DEFAULT: the platform fixes the port and paths.
            setFieldError('port', undefined);
            setFieldError('openApiPath', undefined);
            setFieldError('basePath', undefined);
        }
        
        return currentData;
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

  const handleReadinessTypeChange = useCallback(
    (readinessType: "tcp" | "http") => {
      setFormData(prevData => ({
        ...prevData,
        readinessType,
        readinessPath:
          prevData.readinessPath || readinessOptionFor(prevData.interfaceType)?.httpPath || "",
      }));
      if (readinessType === "tcp") setFieldError('readinessPath', undefined);
    },
    [setFormData, setFieldError]
  );

  const readinessOption = readinessOptionFor(formData.interfaceType);

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
                    slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.PATH } }}
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
                slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.PATH } }}
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
        <Collapse in={formData.interfaceType === "A2A"}>
          <Form.Stack spacing={2}>
            <Alert severity="info">
              The gateway exposes both A2A transports — JSON-RPC at{" "}
              <strong>/rpc</strong> and HTTP+JSON at <strong>/rest</strong> — and
              serves the agent&apos;s own card at the well-known path.
            </Alert>
            <Box>
              <Form.ElementWrapper label="Port" name="port">
                <TextField
                  id="port"
                  placeholder="9099"
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
        </Collapse>
        {readinessOption && (
          <Form.Stack spacing={1}>
            <Form.Stack direction="row" spacing={2}>
              <Box minWidth={280}>
                <Form.ElementWrapper label="Readiness Check" name="readinessType">
                  <Select
                    id="readinessType"
                    fullWidth
                    value={formData.readinessType ?? "tcp"}
                    onChange={(e) => handleReadinessTypeChange(e.target.value as "tcp" | "http")}
                  >
                    <MenuItem value="tcp">TCP: the port accepts connections</MenuItem>
                    <MenuItem value="http">HTTP GET: the path returns 2xx or 3xx</MenuItem>
                  </Select>
                </Form.ElementWrapper>
              </Box>
              {formData.readinessType === "http" && (
                <Box display="flex" flexDirection="column" flexGrow={1}>
                  <Form.ElementWrapper label="Path" name="readinessPath">
                    <TextField
                      slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.PATH } }}
                      id="readinessPath"
                      placeholder={readinessOption.httpPath}
                      required
                      value={formData.readinessPath || ''}
                      onChange={(e) => handleFieldChange('readinessPath', e.target.value)}
                      error={!!errors.readinessPath}
                      helperText={errors.readinessPath}
                      fullWidth
                    />
                  </Form.ElementWrapper>
                </Box>
              )}
            </Form.Stack>
            <Typography variant="caption" color="text.secondary">
              When the agent is ready to take requests. Checked on the agent&apos;s port. The
              wait times and the startup and liveness checks can be changed in the agent&apos;s
              build configuration.
            </Typography>
          </Form.Stack>
        )}
      </Form.Stack>
    </Form.Section>
  );
};
