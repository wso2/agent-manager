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
  CheckCircle,
  Circle,
  Settings,
} from "@wso2/oxygen-ui-icons-react";
import {
  Alert,
  Box,
  Card,
  CardContent,
  Collapse,
  Divider,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { useCallback } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { TextInput } from "@agent-management-platform/views";

const inputInterfaces = [
  {
    label: "Chat Agent",
    description: "Standard chat interface with /chat endpoint on port 8080",
    default: true,
    value: "DEFAULT",
    icon: <CheckCircle />,
  },
  {
    label: "Custom API Agent",
    description:
      "Custom HTTP API with user-specified OpenAPI specification and port configuration",
    default: false,
    value: "CUSTOM",
    icon: <Settings />,
  },
];

export const InputInterface = () => {
  const {
    setValue,
    control,
    register,
    formState: { errors },
  } = useFormContext();
  const interfaceType =
    useWatch({ control, name: "interfaceType" }) || "DEFAULT";
  const port = useWatch({ control, name: "port" }) as unknown as string;
  const theme = useTheme();
  const handleSelect = useCallback(
    (value: string) => {
      setValue("interfaceType", value, { shouldValidate: true });
      if (value === "DEFAULT") {
        setValue("openApiPath", "", { shouldValidate: true });
        setValue("port", "" as unknown as number, { shouldValidate: true });
        setValue("basePath", "/", { shouldValidate: true });
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
          { shouldValidate: true }
        );
      }
    },
    [setValue]
  );


  return (
    <Card variant="outlined">
      <CardContent sx={{ gap: 1, display: "flex", flexDirection: "column" }}>
        <Typography variant="h5">Agent Type</Typography>

        <Typography variant="body2" color="text.secondary">
          How your agent receives requests
        </Typography>
        <Box display="flex" flexDirection="column" gap={1}>
          <Box display="flex" flexDirection="row" gap={1}>
            {inputInterfaces.map((inputInterface) => (
              <Card
                key={inputInterface.value}
                variant="outlined"
                onClick={() => handleSelect(inputInterface.value)}
                sx={{
                  maxWidth: 500,
                  cursor: "pointer",
                  flexGrow: 1,
                  transition: theme.transitions.create([
                    "background-color",
                    "border-color",
                  ]),
                  "&.MuiCard-root": {
                    backgroundColor:
                      interfaceType === inputInterface.value
                        ? "background.default"
                        : "action.paper",
                    borderColor:
                      interfaceType === inputInterface.value
                        ? "primary.main"
                        : "divider",
                    "&:hover": {
                      backgroundColor: "background.default",
                      borderColor: "primary.main",
                    },
                  },
                }}
              >
                <CardContent sx={{ height: "100%" }}>
                  <Box
                    display="flex"
                    flexDirection="row"
                    alignItems="center"
                    height="100%"
                    gap={1}
                  >
                    <Box
               
                    >
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
                </CardContent>
              </Card>
            ))}
          </Box>
          <Collapse in={interfaceType === "DEFAULT"}>
            <Alert severity="info">
              Uses the standard chat interface: <strong>POST /chat</strong> on port <strong>8080</strong>
              <br />
              Request: <code>{`{message: string, session_id: string, context: JSON}`}</code>
              <br />
              Response: <code>{`{reply: string}`}</code>
            </Alert>
          </Collapse>
          <Collapse in={interfaceType === "CUSTOM"}>
            <Box display="flex" flexDirection="column" gap={1}>
              <Box display="flex" flexDirection="row" gap={1}>
                <Box display="flex" flexDirection="column" flexGrow={1}>
                  <TextInput
                    label="OpenAPI Spec Path"
                    placeholder="/openapi.yaml"
                    required
                    fullWidth
                    size="small"
                    error={!!errors.openApiPath}
                    helperText={
                      (errors.openApiPath?.message as string) ||
                      "Path to OpenAPI schema file in your repository"
                    }
                    {...register("openApiPath")}
                  />
                </Box>
                <Box>
                  <TextInput
                    label="Port"
                    placeholder="8080"
                    required
                    value={port}
                    onChange={handlePortChange}
                    size="small"
                    type="number"
                    error={!!errors.port}
                    helperText={
                      (errors.port?.message as string) ||
                      (port ? undefined : "Port is required")
                    }
                  />
                </Box>
              </Box>
              <Box>
                <TextInput
                  label="Base Path"
                  placeholder="/"
                  required
                  fullWidth
                  size="small"
                  error={!!errors.basePath}
                  helperText={
                    (errors.basePath?.message as string) ||
                    "API base path (e.g., / or /api/v1)"
                  }
                  {...register("basePath")}
                />
              </Box>
            </Box>
          </Collapse>
        </Box>
      </CardContent>
    </Card>
  );
};
