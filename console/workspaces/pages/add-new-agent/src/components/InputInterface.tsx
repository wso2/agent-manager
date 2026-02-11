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
import { useCallback, useEffect } from "react";
import { useFormContext, useWatch, Controller } from "react-hook-form";

const inputInterfaces = [
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

export const InputInterface = () => {
  const {
    setValue,
    control,
    trigger,
    formState: { errors },
  } = useFormContext();
  const interfaceType =
    useWatch({ control, name: "interfaceType" }) || "DEFAULT";
  const port = useWatch({ control, name: "port" }) as unknown as string;

  // Re-trigger validation when interfaceType changes (for conditional validation)
  useEffect(() => {
    trigger(["port", "basePath", "openApiPath"]);
  }, [interfaceType, trigger]);

  const handleSelect = useCallback(
    (value: string) => {
      setValue("interfaceType", value, { shouldValidate: true, shouldTouch: true });
      if (value === "DEFAULT") {
        setValue("openApiPath", "", { shouldValidate: true, shouldTouch: true });
        setValue("port", "" as unknown as number, { shouldValidate: true, shouldTouch: true });
        setValue("basePath", "/", { shouldValidate: true, shouldTouch: true });
      }
    },
    [setValue]
  );

  const handlePortChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const next = e.target.value;
      if (/^\d*$/.test(next)) {
        setValue(
          "port",
          next === "" ? ("" as unknown as number) : Number(next),
          { shouldValidate: true, shouldTouch: true }
        );
      }
    },
    [setValue]
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
              selected={interfaceType === inputInterface.value}
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
                    {interfaceType === inputInterface.value ? (
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
        <Collapse in={interfaceType === "DEFAULT"}>
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
        <Collapse in={interfaceType === "CUSTOM"}>
          <Form.Stack spacing={2}>
            <Form.Stack direction="row" spacing={2}>
              <Box display="flex" flexDirection="column" flexGrow={1}>
                <Controller
                  name="openApiPath"
                  control={control}
                  render={({ field }) => (
                    <Form.ElementWrapper label="OpenAPI Spec Path" name="openApiPath">
                      <TextField
                        {...field}
                        id="openApiPath"
                        placeholder="/openapi.yaml"
                        required
                        error={!!errors.openApiPath}
                        helperText={
                          (errors.openApiPath?.message as string) ||
                          "Path to OpenAPI schema file in your repository"
                        }
                        fullWidth
                      />
                    </Form.ElementWrapper>
                  )}
                />
              </Box>
              <Box>
                <Form.ElementWrapper label="Port" name="port">
                  <TextField
                    id="port"
                    placeholder="8080"
                    required
                    value={port}
                    onChange={handlePortChange}
                    type="number"
                    error={!!errors.port}
                    helperText={
                      (errors.port?.message as string) ||
                      (port ? undefined : "Port is required")
                    }
                  />
                </Form.ElementWrapper>
              </Box>
            </Form.Stack>
            <Controller
              name="basePath"
              control={control}
              render={({ field }) => (
                <Form.ElementWrapper label="Base Path" name="basePath">
                  <TextField
                    {...field}
                    id="basePath"
                    placeholder="/"
                    required
                    error={!!errors.basePath}
                    helperText={
                      (errors.basePath?.message as string) ||
                      "API base path (e.g., / or /api/v1)"
                    }
                    fullWidth
                  />
                </Form.ElementWrapper>
              )}
            />
          </Form.Stack>
        </Collapse>
      </Form.Stack>
    </Form.Section>
  );
};
