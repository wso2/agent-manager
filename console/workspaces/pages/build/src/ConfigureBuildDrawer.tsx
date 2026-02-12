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
  Box,
  Button,
  Card,
  CardContent,
  Typography,
  Collapse,
  Alert,
  Divider,
  useTheme,
} from "@wso2/oxygen-ui";
import { Settings, CheckCircle, Circle } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
  TextInput,
  useFormValidation,
} from "@agent-management-platform/views";
import { z } from "zod";
import { useUpdateAgentBuildParameters } from "@agent-management-platform/api-client";
import {
  AgentResponse,
  UpdateAgentBuildParametersRequest,
  InputInterfaceType,
} from "@agent-management-platform/types";
import { useEffect, useCallback, useMemo, useState } from "react";

interface ConfigureBuildDrawerProps {
  open: boolean;
  onClose: () => void;
  agent: AgentResponse;
  orgId: string;
  projectId: string;
}

interface ConfigureBuildFormValues {
  repositoryUrl: string;
  branch: string;
  appPath: string;
  runCommand: string;
  language: string;
  languageVersion?: string;
  interfaceType: InputInterfaceType;
  port?: number;
  basePath?: string;
  openApiPath?: string;
}

const configureBuildSchema = z.object({
  repositoryUrl: z
    .string()
    .trim()
    .url("Must be a valid URL")
    .min(1, "Repository URL is required"),
  branch: z.string().trim().min(1, "Branch is required"),
  appPath: z
    .string()
    .trim()
    .min(1, "App path is required")
    .refine((value) => value.startsWith("/"), {
      message: "App path must start with /",
    })
    .refine(
      (value) => {
        if (value === "/") return true;
        return !value.endsWith("/");
      },
      { message: "App path must be a valid path (use / for root directory)" },
    ),
  runCommand: z.string().trim().min(1, "Start Command is required"),
  language: z.string().trim().min(1, "Language is required"),
  languageVersion: z.string().trim().optional(),
  interfaceType: z.enum(["DEFAULT", "CUSTOM"]),
  port: z
    .union([z.number(), z.string(), z.undefined()])
    .transform((val) => {
      if (val === "" || val === null || val === undefined) return undefined;
      return typeof val === "string" ? Number(val) : val;
    })
    .optional(),
  basePath: z.string().trim().optional(),
  openApiPath: z.string().trim().optional(),
}).refine(
  (data) => {
    if (data.interfaceType === "CUSTOM" && !data.port) {
      return false;
    }
    return true;
  },
  { message: "Port is required when using custom interface", path: ["port"] }
).refine(
  (data) => {
    if (data.interfaceType === "CUSTOM" && data.port !== undefined) {
      if (isNaN(data.port)) return false;
      if (data.port < 1 || data.port > 65535) return false;
    }
    return true;
  },
  { message: "Port must be between 1 and 65535", path: ["port"] }
).refine(
  (data) => {
    if (data.interfaceType === "CUSTOM" && !data.basePath) {
      return false;
    }
    return true;
  },
  { message: "Base path is required when using custom interface", path: ["basePath"] }
).refine(
  (data) => {
    if (data.interfaceType === "CUSTOM" && !data.openApiPath) {
      return false;
    }
    return true;
  },
  { message: "OpenAPI spec path is required when using custom interface", path: ["openApiPath"] }
);

const inputInterfaces = [
  {
    label: "Chat Agent",
    description: "Standard chat interface with /chat endpoint on port 8000",
    value: "DEFAULT" as const,
  },
  {
    label: "Custom API Agent",
    description:
      "Custom HTTP API with user-specified OpenAPI specification and port configuration",
    value: "CUSTOM" as const,
  },
];

export function ConfigureBuildDrawer({
  open,
  onClose,
  agent,
  orgId,
  projectId,
}: ConfigureBuildDrawerProps) {
  const theme = useTheme();
  const isCustomInterface =
    !!agent.inputInterface?.schema?.path ||
    !!agent.inputInterface?.port ||
    !!agent.inputInterface?.basePath ||
    agent.agentType?.subType === "custom-api";
  const resolvedInterfaceType: InputInterfaceType =
    agent.agentType?.subType === "custom-api"
      ? "CUSTOM"
      : agent.agentType?.subType === "chat-api"
        ? "DEFAULT"
        : isCustomInterface
          ? "CUSTOM"
          : "DEFAULT";
  const repo = agent.provisioning?.repository;
  const buildpackConfig = agent.build?.type === 'buildpack' ? agent.build.buildpack : undefined;
  const inputInterface = agent.inputInterface;
  const buildDefaults = useMemo(
    () => ({
      repositoryUrl: repo?.url || "",
      branch: repo?.branch || "",
      appPath: repo?.appPath ?? "",
      runCommand: buildpackConfig?.runCommand ?? "python main.py",
      language:
        buildpackConfig?.language && buildpackConfig.language !== ""
          ? buildpackConfig?.language
          : "python",
      languageVersion: buildpackConfig?.languageVersion ?? "3.11",
      interfaceType: resolvedInterfaceType,
      port: inputInterface?.port,
      basePath: inputInterface?.basePath ?? "",
      openApiPath: inputInterface?.schema?.path ?? "",
    }),
    [
      repo?.url,
      repo?.branch,
      repo?.appPath,
      buildpackConfig?.runCommand,
      buildpackConfig?.language,
      buildpackConfig?.languageVersion,
      inputInterface?.port,
      inputInterface?.basePath,
      inputInterface?.schema?.path,
      resolvedInterfaceType,
    ],
  );
  
  const [formData, setFormData] = useState<ConfigureBuildFormValues>(buildDefaults);
  const { errors, validateField, validateForm, clearErrors, setFieldError } =
    useFormValidation<ConfigureBuildFormValues>(configureBuildSchema);

  const { mutate: updateBuildParameters, isPending } = useUpdateAgentBuildParameters();

  // Reset form when drawer opens or agent changes
  useEffect(() => {
    if (open) {
      setFormData(buildDefaults);
      clearErrors();
    }
  }, [open, buildDefaults, clearErrors]);

  const handleFieldChange = useCallback(
    (
      field: keyof ConfigureBuildFormValues,
      value: string | number | InputInterfaceType | undefined
    ) => {
    setFormData(prevData => {
      const newData = { ...prevData, [field]: value };
      const error = validateField(field, value);
      setFieldError(field, error);
      return newData;
    });
  }, [validateField, setFieldError]);

  const handleSelectInterface = useCallback(
    (value: InputInterfaceType) => {
      setFormData(prevData => {
        const newData = {
          ...prevData,
          interfaceType: value,
          ...(value === "DEFAULT" ? {
            openApiPath: "",
            port: undefined,
            basePath: "/",
          } : {}),
        };
        // Validate the interfaceType field
        const error = validateField('interfaceType', value);
        setFieldError('interfaceType', error);
        return newData;
      });
    },
    [validateField, setFieldError],
  );

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!validateForm(formData)) {
      return;
    }

    const nextAgentType = agent.agentType
      ? {
          ...agent.agentType,
          subType: formData.interfaceType === "CUSTOM" ? "custom-api" : "chat-api",
        }
      : {
          type: "agent-api",
          subType: formData.interfaceType === "CUSTOM" ? "custom-api" : "chat-api",
        };

    const buildParametersPayload: UpdateAgentBuildParametersRequest = {
      provisioning: {
        type: agent.provisioning.type,
        repository: {
          url: formData.repositoryUrl,
          branch: formData.branch,
          appPath: formData.appPath,
        },
      },
      agentType: nextAgentType,
      build: {
        type: "buildpack",
        buildpack: {
          language: formData.language || "python",
          languageVersion: formData.languageVersion || "",
          runCommand: formData.runCommand,
        },
      },
      inputInterface: {
        type: "HTTP",
        ...(formData.interfaceType === "CUSTOM"
          ? {
              port: Number(formData.port),
              basePath: formData.basePath || "/",
              schema: {
                path: formData.openApiPath || "",
              },
            }
          : {}),
      },
    };

    updateBuildParameters(
      {
        params: {
          orgName: orgId,
          projName: projectId,
          agentName: agent.name,
        },
        body: buildParametersPayload,
      },
      {
        onSuccess: () => {
          clearErrors();
          onClose();
        },
      },
    );
  };

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<Settings size={24} />}
        title="Configure Build"
        onClose={onClose}
      />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Box display="flex" flexDirection="column" gap={2} flexGrow={1}>
            <Card variant="outlined">
              <CardContent
                sx={{ gap: 1, display: "flex", flexDirection: "column" }}
              >
                <Typography variant="h5">Repository Details</Typography>
                <Box display="flex" flexDirection="column" gap={1}>
                  <TextInput
                    placeholder="https://github.com/username/repo"
                    label="GitHub Repository"
                    fullWidth
                    size="small"
                    value={formData.repositoryUrl}
                    onChange={(e) => handleFieldChange('repositoryUrl', e.target.value)}
                    error={!!errors.repositoryUrl}
                    helperText={errors.repositoryUrl}
                    disabled={isPending}
                  />
                  <Box display="flex" flexDirection="row" gap={1}>
                    <TextInput
                      placeholder="main"
                      label="Branch"
                      fullWidth
                      size="small"
                      value={formData.branch}
                      onChange={(e) => handleFieldChange('branch', e.target.value)}
                      error={!!errors.branch}
                      helperText={errors.branch}
                      disabled={isPending}
                    />
                    <TextInput
                      placeholder="my-agent"
                      label="Project Path"
                      fullWidth
                      size="small"
                      value={formData.appPath}
                      onChange={(e) => handleFieldChange('appPath', e.target.value)}
                      error={!!errors.appPath}
                      helperText={errors.appPath}
                      disabled={isPending}
                    />
                  </Box>
                </Box>
              </CardContent>
            </Card>

            <Card variant="outlined">
              <CardContent
                sx={{ gap: 1, display: "flex", flexDirection: "column" }}
              >
                <Typography variant="h5">Build Details</Typography>
                <Box display="flex" flexDirection="column" gap={1}>
                  <Box display="flex" flexDirection="row" gap={1}>
                    <TextInput
                      placeholder="python"
                      disabled
                      label="Language"
                      fullWidth
                      size="small"
                      value={formData.language}
                      onChange={(e) => handleFieldChange('language', e.target.value)}
                      error={!!errors.language}
                      helperText={errors.language || "e.g., python, nodejs, go"}
                    />
                    <TextInput
                      placeholder="3.11"
                      label="Language Version"
                      fullWidth
                      size="small"
                      value={formData.languageVersion}
                      onChange={(e) => handleFieldChange('languageVersion', e.target.value)}
                      error={!!errors.languageVersion}
                      helperText={errors.languageVersion || "e.g., 3.11, 20, 1.21"}
                      disabled={isPending}
                    />
                  </Box>
                  <TextInput
                    placeholder="python main.py"
                    label="Start Command"
                    fullWidth
                    size="small"
                    value={formData.runCommand}
                    onChange={(e) => handleFieldChange('runCommand', e.target.value)}
                    error={!!errors.runCommand}
                    helperText={
                      errors.runCommand ||
                      "Dependencies auto-install from package.json, requirements.txt, or pyproject.toml"
                    }
                    disabled={isPending}
                  />
                </Box>
              </CardContent>
            </Card>

              <Card variant="outlined">
                <CardContent
                  sx={{ gap: 1, display: "flex", flexDirection: "column" }}
                >
                  <Typography variant="h5">Agent Interface</Typography>
                  <Typography variant="body2" color="text.secondary">
                    How your agent receives requests
                  </Typography>
                  <Box display="flex" flexDirection="column" gap={1}>
                    <Box display="flex" flexDirection="row" gap={1}>
                      {inputInterfaces.map((interfaceOption) => (
                        <Card
                          key={interfaceOption.value}
                          variant="outlined"
                          onClick={() =>
                            handleSelectInterface(interfaceOption.value)
                          }
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
                              formData.interfaceType === interfaceOption.value
                                ? "background.default"
                                : "action.paper",
                            borderColor:
                              formData.interfaceType === interfaceOption.value
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
                            <Box>
                              {formData.interfaceType === interfaceOption.value ? (
                                <CheckCircle size={16} />
                              ) : (
                                <Circle size={16} />
                              )}
                            </Box>
                            <Divider orientation="vertical" flexItem />
                            <Box>
                              <Typography variant="h6">
                                {interfaceOption.label}
                              </Typography>
                              <Typography variant="caption">
                                {interfaceOption.description}
                              </Typography>
                            </Box>
                          </Box>
                        </CardContent>
                      </Card>
                    ))}
                  </Box>
                  <Collapse in={formData.interfaceType === "DEFAULT"}>
                    <Alert severity="info">
                      Uses the standard chat interface:{" "}
                      <strong>POST /chat</strong> on port{" "}
                      <strong>8000</strong>
                      <br />
                      Request:{" "}
                      <code>{`{message: string, session_id: string, context: JSON}`}</code>
                      <br />
                      Response: <code>{`{response: string}`}</code>
                    </Alert>
                  </Collapse>
                  <Collapse in={formData.interfaceType === "CUSTOM"}>
                    <Box display="flex" flexDirection="column" gap={1}>
                      <Box display="flex" flexDirection="row" gap={1}>
                        <Box
                          display="flex"
                          flexDirection="column"
                          flexGrow={1}
                        >
                          <TextInput
                            label="OpenAPI Spec Path"
                            placeholder="/openapi.yaml"
                            required={formData.interfaceType === "CUSTOM"}
                            fullWidth
                            size="small"
                            value={formData.openApiPath || ""}
                            onChange={(e) => handleFieldChange('openApiPath', e.target.value)}
                            error={!!errors.openApiPath}
                            helperText={
                              errors.openApiPath ||
                              "Path to OpenAPI schema file in your repository"
                            }
                            disabled={isPending}
                          />
                        </Box>
                        <Box>
                          <TextInput
                            label="Port"
                            placeholder="8080"
                            required={formData.interfaceType === "CUSTOM"}
                            value={formData.port ?? ""}
                            onChange={(e) => {
                              const next = e.target.value;
                              if (/^\d*$/.test(next)) {
                                handleFieldChange('port', next === "" ? undefined : Number(next));
                              }
                            }}
                            size="small"
                            type="number"
                            error={!!errors.port}
                            helperText={
                              errors.port ||
                              (formData.port ? undefined : "Port is required")
                            }
                            disabled={isPending}
                          />
                        </Box>
                      </Box>
                      <Box>
                        <TextInput
                          label="Base Path"
                          placeholder="/"
                          required={formData.interfaceType === "CUSTOM"}
                          fullWidth
                          size="small"
                          value={formData.basePath || ""}
                          onChange={(e) => handleFieldChange('basePath', e.target.value)}
                          error={!!errors.basePath}
                          helperText={
                            errors.basePath ||
                            "API base path (e.g., / or /api/v1)"
                          }
                          disabled={isPending}
                        />
                      </Box>
                    </Box>
                  </Collapse>
                </Box>
              </CardContent>
            </Card>

            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button
                variant="outlined"
                color="inherit"
                onClick={onClose}
                disabled={isPending}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                variant="contained"
                color="primary"
                disabled={isPending}
              >
                {isPending ? "Updating..." : "Update Build Configuration"}
              </Button>
            </Box>
          </Box>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}
