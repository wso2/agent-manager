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
  Alert,
  Box,
  Typography,
  Form,
  Grid,
  Stack,
} from "@wso2/oxygen-ui";
import { useCallback } from "react";
import { SelectionDetails } from "./SelectionDetails";
import type { CreateAgentFormValues } from "../form/schema";
import { type InputInterfaceType, INPUT_LIMITS } from "@agent-management-platform/types";
import { AgentTypeIcon, SELECTION_GRID_SX, SelectionCard, TextInput } from "@agent-management-platform/views";

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
  /** One line shown on the card. */
  description: string;
  /** The full explanation, shown as the description's tooltip. */
  fullDescription: string;
  value: InputInterfaceType;
}> = [
  {
    label: "Chat agent",
    description: "/chat endpoint, port 8000",
    fullDescription: "Standard chat interface with /chat endpoint on port 8000",
    value: "DEFAULT",
  },
  {
    label: "Custom API agent",
    description: "Your OpenAPI spec and port",
    fullDescription:
      "Custom HTTP API with user-specified OpenAPI specification and port configuration",
    value: "CUSTOM",
  },
  {
    label: "A2A agent",
    description: "Serves its own agent card",
    fullDescription:
      "Speaks the A2A protocol. The agent serves its own agent card; the gateway exposes both A2A transports.",
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
        };
        return newData;
      });

      // Perform all validations outside the updater
      setFormData(currentData => {
        const error = validateField('interfaceType', value, currentData);
        setFieldError('interfaceType', error);
        
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

  return (
    <Stack spacing={2}>
      <Typography variant="subtitle1">Agent Type</Typography>
      <Typography variant="body2" color="text.secondary">
        How your agent receives requests
      </Typography>
      <Form.Stack spacing={2}>
        <Box role="radiogroup" aria-label="Agent type" sx={SELECTION_GRID_SX}>
          {inputInterfaces.map(({ label, description, fullDescription, value }) => (
            <SelectionCard
              key={value}
              icon={<AgentTypeIcon type={value} size={18} />}
              title={label}
              description={description}
              fullDescription={fullDescription}
              selected={formData.interfaceType === value}
              onSelect={() => handleSelect(value)}
            />
          ))}
        </Box>
        {formData.interfaceType === "DEFAULT" && (
          <Alert severity="info">
            Uses the standard chat interface: <strong>POST /chat</strong> on
            port <strong>8000</strong>
            <br />
            Request:{" "}
            <code>{`{message: string, session_id: string, context: JSON}`}</code>
            <br />
            Response: <code>{`{response: string}`}</code>
          </Alert>
        )}
        <SelectionDetails open={formData.interfaceType === "CUSTOM"} title="Custom API configuration">
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, md: 6, xl: 4 }}>
              <TextInput
                label="OpenAPI Spec Path"
                id="openApiPath"
                maxLength={INPUT_LIMITS.PATH}
                hideCharacterCount={(formData.openApiPath?.length ?? 0) < INPUT_LIMITS.PATH}
                placeholder="/openapi.yaml"
                value={formData.openApiPath || ''}
                onChange={(e) => handleFieldChange('openApiPath', e.target.value)}
                error={!!errors.openApiPath}
                helperText={
                  errors.openApiPath ||
                  "Path to OpenAPI schema file in your repository"
                }
              />
            </Grid>
            <Grid size={{ xs: 12, md: 6, xl: 4 }}>
              <TextInput
                label="Port"
                id="port"
                placeholder="8080"
                value={formData.port ?? ''}
                onChange={handlePortChange}
                type="number"
                error={!!errors.port}
                helperText={
                  errors.port ||
                  (formData.port ? undefined : "Port is required")
                }
              />
            </Grid>
            <Grid size={{ xl: 4 }} sx={{ display: { xs: 'none', xl: 'block' } }} />
            <Grid size={{ xs: 12, md: 6, xl: 4 }}>
              <TextInput
                label="Base Path"
                id="basePath"
                maxLength={INPUT_LIMITS.PATH}
                hideCharacterCount={(formData.basePath?.length ?? 0) < INPUT_LIMITS.PATH}
                placeholder="/"
                value={formData.basePath || ''}
                onChange={(e) => handleFieldChange('basePath', e.target.value)}
                error={!!errors.basePath}
                helperText={
                  errors.basePath ||
                  "API base path (e.g., / or /api/v1)"
                }
              />
            </Grid>
          </Grid>
        </SelectionDetails>
        <SelectionDetails open={formData.interfaceType === "A2A"} title="A2A configuration">
          <Form.Stack spacing={2}>
            <Alert severity="info">
              The gateway exposes both A2A transports — JSON-RPC at{" "}
              <strong>/rpc</strong> and HTTP+JSON at <strong>/rest</strong> — and
              serves the agent&apos;s own card at the well-known path.
            </Alert>
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, md: 6, xl: 4 }}>
                <TextInput
                  label="Port"
                  id="port"
                  placeholder="9099"
                  value={formData.port ?? ''}
                  onChange={handlePortChange}
                  type="number"
                  error={!!errors.port}
                  helperText={
                    errors.port ||
                    (formData.port ? undefined : "Port is required")
                  }
                />
              </Grid>
            </Grid>
          </Form.Stack>
        </SelectionDetails>
      </Form.Stack>
    </Stack>
  );
};
