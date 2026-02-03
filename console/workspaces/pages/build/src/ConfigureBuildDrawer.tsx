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
} from "@agent-management-platform/views";
import { useForm, FormProvider, useWatch, Controller } from "react-hook-form";
import { yupResolver } from "@hookform/resolvers/yup";
import * as yup from "yup";
import { useUpdateAgent } from "@agent-management-platform/api-client";
import {
  AgentResponse,
  UpdateAgentRequest,
  InputInterfaceType,
} from "@agent-management-platform/types";
import { useEffect, useCallback, useMemo } from "react";

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

const configureBuildSchema = yup.object({
  repositoryUrl: yup
    .string()
    .trim()
    .url("Must be a valid URL")
    .required("Repository URL is required"),
  branch: yup.string().trim().required("Branch is required"),
  appPath: yup
    .string()
    .trim()
    .required("App path is required")
    .test("starts-with-slash", "App path must start with /", (value) => {
      if (!value) return false;
      return value.startsWith("/");
    })
    .test(
      "valid-path",
      "App path must be a valid path (use / for root directory)",
      (value) => {
        if (!value) return false;
        if (value === "/") return true;
        return !value.endsWith("/");
      },
    ),
  runCommand: yup.string().trim().required("Start Command is required"),
  language: yup.string().trim().required("Language is required"),
  languageVersion: yup.string().trim(),
  interfaceType: yup
    .mixed<InputInterfaceType>()
    .oneOf(["DEFAULT", "CUSTOM"])
    .required(),
  port: yup
    .number()
    .transform((value, original) =>
      original === "" || original === null ? undefined : value,
    )
    .when("interfaceType", {
      is: "CUSTOM",
      then: (schema) => schema.required("Port is required").min(1).max(65535),
      otherwise: (schema) => schema.notRequired(),
    }),
  basePath: yup
    .string()
    .trim()
    .when("interfaceType", {
      is: "CUSTOM",
      then: (schema) => schema.required("Base path is required"),
      otherwise: (schema) => schema.notRequired(),
    }),
  openApiPath: yup
    .string()
    .trim()
    .when("interfaceType", {
      is: "CUSTOM",
      then: (schema) => schema.required("OpenAPI spec path is required"),
      otherwise: (schema) => schema.notRequired(),
    }),
});

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
  const runtimeConfigs = agent.runtimeConfigs;
  const inputInterface = agent.inputInterface;
  const buildDefaults = useMemo(
    () => ({
      repositoryUrl: repo?.url || "",
      branch: repo?.branch || "",
      appPath: repo?.appPath ?? "",
      runCommand: runtimeConfigs?.runCommand ?? "python main.py",
      language:
        runtimeConfigs?.language && runtimeConfigs.language !== ""
          ? runtimeConfigs?.language
          : "python",
      languageVersion: runtimeConfigs?.languageVersion ?? "3.11",
      interfaceType: resolvedInterfaceType,
      port: inputInterface?.port,
      basePath: inputInterface?.basePath ?? "",
      openApiPath: inputInterface?.schema?.path ?? "",
    }),
    [
      repo?.url,
      repo?.branch,
      repo?.appPath,
      runtimeConfigs?.runCommand,
      runtimeConfigs?.language,
      runtimeConfigs?.languageVersion,
      inputInterface?.port,
      inputInterface?.basePath,
      inputInterface?.schema?.path,
      resolvedInterfaceType,
    ],
  );
  const methods = useForm<ConfigureBuildFormValues>({
    resolver: yupResolver(configureBuildSchema),
    defaultValues: buildDefaults,
  });

  const { mutate: updateAgent, isPending } = useUpdateAgent();
  const interfaceType =
    useWatch({ control: methods.control, name: "interfaceType" }) || "DEFAULT";
  const port = useWatch({
    control: methods.control,
    name: "port",
  }) as unknown as string;

  // Reset form when agent changes
  useEffect(() => {
    if (open) {
      methods.reset(buildDefaults);
    }
  }, [open, agent, methods, buildDefaults]);

  const handleSelectInterface = useCallback(
    (value: InputInterfaceType) => {
      methods.setValue("interfaceType", value, { shouldValidate: true });
      if (value === "DEFAULT") {
        methods.setValue("openApiPath", "", { shouldValidate: true });
        methods.setValue("port", "" as unknown as number, {
          shouldValidate: true,
        });
        methods.setValue("basePath", "/", { shouldValidate: true });
      }
    },
    [methods],
  );

  const handleSubmit = (data: ConfigureBuildFormValues) => {
    const nextAgentType = agent.agentType
      ? {
          ...agent.agentType,
          subType: data.interfaceType === "CUSTOM" ? "custom-api" : "chat-api",
        }
      : {
          type: "agent-api",
          subType: data.interfaceType === "CUSTOM" ? "custom-api" : "chat-api",
        };
    const payload: UpdateAgentRequest = {
      name: agent.name,
      displayName: agent.displayName,
      description: agent.description,
      provisioning: {
        type: agent.provisioning.type,
        repository: {
          url: data.repositoryUrl,
          branch: data.branch,
          appPath: data.appPath,
        },
      },
      agentType: nextAgentType,
      runtimeConfigs: {
        language: data.language || "python",
        languageVersion: data.languageVersion || "",
        runCommand: data.runCommand,
        env: agent.runtimeConfigs?.env || [],
      },
      inputInterface: {
        type: "HTTP",
        ...(data.interfaceType === "CUSTOM"
          ? {
              port: Number(data.port),
              basePath: data.basePath || "/",
              schema: {
                path: data.openApiPath || "",
              },
            }
          : {}),
      },
    };

    updateAgent(
      {
        params: {
          orgName: orgId,
          projName: projectId,
          agentName: agent.name,
        },
        body: payload,
      },
      {
        onSuccess: () => {
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
        <FormProvider {...methods}>
          <form onSubmit={methods.handleSubmit(handleSubmit)}>
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
                      error={!!methods.formState.errors.repositoryUrl}
                      helperText={
                        methods.formState.errors.repositoryUrl
                          ?.message as string
                      }
                      {...methods.register("repositoryUrl")}
                    />
                    <Box display="flex" flexDirection="row" gap={1}>
                      <TextInput
                        placeholder="main"
                        label="Branch"
                        fullWidth
                        size="small"
                        error={!!methods.formState.errors.branch}
                        helperText={
                          methods.formState.errors.branch?.message as string
                        }
                        {...methods.register("branch")}
                      />
                      <TextInput
                        placeholder="my-agent"
                        label="Project Path"
                        fullWidth
                        size="small"
                        error={!!methods.formState.errors.appPath}
                        helperText={
                          methods.formState.errors.appPath?.message as string
                        }
                        {...methods.register("appPath")}
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
                        error={!!methods.formState.errors.language}
                        helperText={
                          (methods.formState.errors.language
                            ?.message as string) || "e.g., python, nodejs, go"
                        }
                        {...methods.register("language")}
                      />
                      <TextInput
                        placeholder="3.11"
                        label="Language Version"
                        fullWidth
                        size="small"
                        error={!!methods.formState.errors.languageVersion}
                        helperText={
                          (methods.formState.errors.languageVersion
                            ?.message as string) || "e.g., 3.11, 20, 1.21"
                        }
                        {...methods.register("languageVersion")}
                      />
                    </Box>
                    <TextInput
                      placeholder="python main.py"
                      label="Start Command"
                      fullWidth
                      size="small"
                      error={!!methods.formState.errors.runCommand}
                      helperText={
                        (methods.formState.errors.runCommand
                          ?.message as string) ||
                        "Dependencies auto-install from package.json, requirements.txt, or pyproject.toml"
                      }
                      {...methods.register("runCommand")}
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
                                interfaceType === interfaceOption.value
                                  ? "background.default"
                                  : "action.paper",
                              borderColor:
                                interfaceType === interfaceOption.value
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
                                {interfaceType === interfaceOption.value ? (
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
                    <Collapse in={interfaceType === "DEFAULT"}>
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
                    <Collapse in={interfaceType === "CUSTOM"}>
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
                              required={interfaceType === "CUSTOM"}
                              fullWidth
                              size="small"
                              error={!!methods.formState.errors.openApiPath}
                              helperText={
                                (methods.formState.errors.openApiPath
                                  ?.message as string) ||
                                "Path to OpenAPI schema file in your repository"
                              }
                              {...methods.register("openApiPath")}
                            />
                          </Box>
                          <Box>
                            <Controller
                              name="port"
                              control={methods.control}
                              render={({ field }) => (
                                <TextInput
                                  label="Port"
                                  placeholder="8080"
                                  required={interfaceType === "CUSTOM"}
                                  value={field.value ?? ""}
                                  onChange={(e) => {
                                    const next = e.target.value;
                                    if (/^\d*$/.test(next)) {
                                      field.onChange(
                                        next === "" ? undefined : Number(next),
                                      );
                                    }
                                  }}
                                  size="small"
                                  type="number"
                                  error={!!methods.formState.errors.port}
                                  helperText={
                                    (methods.formState.errors.port
                                      ?.message as string) ||
                                    (port ? undefined : "Port is required")
                                  }
                                />
                              )}
                            />
                          </Box>
                        </Box>
                        <Box>
                          <TextInput
                            label="Base Path"
                            placeholder="/"
                            required={interfaceType === "CUSTOM"}
                            fullWidth
                            size="small"
                            error={!!methods.formState.errors.basePath}
                            helperText={
                              (methods.formState.errors.basePath
                                ?.message as string) ||
                              "API base path (e.g., / or /api/v1)"
                            }
                            {...methods.register("basePath")}
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
        </FormProvider>
      </DrawerContent>
    </DrawerWrapper>
  );
}
