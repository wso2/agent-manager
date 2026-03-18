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

import { useDeployAgent, useGetAgent, useGetAgentConfigurations, useListEnvironments } from "@agent-management-platform/api-client";
import { Rocket } from "@wso2/oxygen-ui-icons-react";
import { Box, Button, Checkbox, Collapse, FormControlLabel, Skeleton, Typography} from "@wso2/oxygen-ui";
import { EnvironmentVariable } from "./EnvironmentVariable";
import type { Environment, EnvironmentVariable as EnvVar } from "@agent-management-platform/types";
import { useEffect, useState } from "react";
import { TextInput, DrawerHeader, DrawerContent } from "@agent-management-platform/views";

interface DeploymentConfigProps {
    onClose: () => void;
    from?: string;
    to: string;
    orgName: string;
    projName: string;
    agentName: string;
    imageId: string;
}

export function DeploymentConfig({
    onClose,
    from,
    to,
    orgName,
    projName,
    agentName,
    imageId,
}: DeploymentConfigProps) {
    const [envVariables, setEnvVariables] = useState<Array<{
        key: string;
        value: string;
        isSensitive?: boolean;
        secretRef?: string;
        isSecretEdited?: boolean;
    }>>([]);
    const [enableAutoInstrumentation, setEnableAutoInstrumentation] = useState<boolean>(true);

    const { mutate: deployAgent, isPending } = useDeployAgent();
    const { data: agent, isLoading: isLoadingAgent } = useGetAgent({
        orgName,
        projName,
        agentName,
    });
    const { data: environments, isLoading: isLoadingEnvironments } = useListEnvironments({
        orgName,
    });
    const { data: configurations, isLoading: isLoadingConfigurations } = useGetAgentConfigurations({
        orgName,
        projName,
        agentName,
    }, {
        environment: to || '',
    });

    useEffect(() => {
        setEnvVariables(configurations?.configurations || []);
    }, [configurations]);

    useEffect(() => {
        if (agent?.configurations?.enableAutoInstrumentation !== undefined) {
            setEnableAutoInstrumentation(agent.configurations.enableAutoInstrumentation);
        }
    }, [agent?.configurations?.enableAutoInstrumentation]);

    const isPythonBuildpack = agent?.build?.type === 'buildpack' && 'buildpack' in agent.build && agent.build.buildpack.language === 'python';

    const handleDeploy = async () => {
        try {
            // Build env payload based on:
            // 1. Deleted items are not in envVariables array (already filtered out)
            // 2. If secret has secretRef and NOT edited: value = empty,
            //    secretRef = original ref (preserve)
            // 3. If secret is new (no secretRef) OR edited: value = new value, no secretRef
            const filteredEnvVars: EnvVar[] = envVariables
                .filter((envVar) => {
                    // Include if it has a key and either:
                    // - Has a value (plain env var or new/updated secret)
                    // - Is an existing secret that wasn't edited (has secretRef)
                    if (!envVar.key) return false;
                    if (envVar.value) return true;
                    if (envVar.isSensitive && envVar.secretRef && !envVar.isSecretEdited) {
                        return true;
                    }
                    return false;
                })
                .map((envVar) => {
                    if (envVar.isSensitive) {
                        // Check if this is an existing secret that should be preserved
                        const isExistingSecretPreserved =
                            envVar.secretRef && !envVar.isSecretEdited;

                        if (isExistingSecretPreserved) {
                            // Existing secret NOT changed - send empty value, keep secretRef
                            return {
                                key: envVar.key,
                                value: '',
                                isSensitive: true,
                                secretRef: envVar.secretRef,
                            };
                        } else {
                            // New secret OR existing secret with new value - send the value
                            return {
                                key: envVar.key,
                                value: envVar.value,
                                isSensitive: true,
                                // secretRef is intentionally omitted for new/updated secrets
                            };
                        }
                    }
                    // Plain env var
                    return {
                        key: envVar.key,
                        value: envVar.value,
                        isSensitive: false,
                    };
                });

            deployAgent({
                params: {
                    orgName,
                    projName,
                    agentName,
                },
                body: {
                    imageId: imageId,
                    env: filteredEnvVars.length > 0 ? filteredEnvVars : undefined,
                    ...(isPythonBuildpack && { enableAutoInstrumentation }),
                },
            }, {
                onSuccess: () => {
                    onClose();
                },
            });
        } catch {
            // Error handling is done by the mutation
        }
    };


    const toEnvironment = environments?.find((environment: Environment) => environment.name === to);

    const deployButtonText = from ? `Promote to ${toEnvironment?.displayName ?? to}` : `Deploy to ${toEnvironment?.displayName ?? to}`;
    const titleText = from ? `Promote to ${toEnvironment?.displayName ?? to}` : `Deploy to ${toEnvironment?.displayName ?? to}`;
    const descriptionText = from
        ? `Promote ${agent?.displayName || 'Agent'} to ${toEnvironment?.displayName ?? to} Environment. Configure environment variables and deploy immediately.`
        : `Deploy ${agent?.displayName || 'Agent'} to ${toEnvironment?.displayName ?? to} Environment. Configure environment variables and deploy immediately.`;

    return (
        <Box display="flex" flexDirection="column" height="100%">
            <DrawerHeader
                icon={<Rocket size={24} />}
                title={titleText}
                onClose={onClose}
            />
            <DrawerContent>
                <Typography variant="body2" color="text.secondary">
                    {descriptionText}
                </Typography>

            <Box display="flex" flexDirection="column" gap={3}>
                <Box display="flex" flexDirection="column" gap={2}>
                    <Typography variant="h6">
                        Deployment Details
                    </Typography>
                    <TextInput
                        label="Image ID"
                        value={imageId}
                        size="small"
                        disabled
                        fullWidth
                    />
                </Box>
                {isLoadingConfigurations || isLoadingEnvironments || isLoadingAgent ? (
                    <Box display="flex" flexDirection="column" gap={1} width="100%">
                        <Skeleton variant="rectangular" width="100%" height={305} />
                    </Box>
                ) : (
                    <EnvironmentVariable
                        envVariables={envVariables}
                        setEnvVariables={setEnvVariables}
                        isExistingData={true}
                    />
                )}
                <Collapse in={isPythonBuildpack && !isLoadingAgent}>
                    <Box display="flex" flexDirection="column" gap={1}>
                        <FormControlLabel
                            control={
                                <Checkbox
                                    checked={enableAutoInstrumentation}
                                    onChange={(e) => setEnableAutoInstrumentation(e.target.checked)}
                                    disabled={isPending}
                                />
                            }
                            label="Enable auto instrumentation"
                        />
                        <Typography variant="body2" color="text.secondary">
                            Automatically adds OTEL tracing instrumentation to
                            your agent for observability.
                        </Typography>
                    </Box>
                </Collapse>
            </Box>
            <Box display="flex" gap={1} justifyContent="flex-end" width="100%">
                <Button
                    variant="outlined"
                    color="primary"
                    onClick={onClose}
                    disabled={isPending}
                >
                    Cancel
                </Button>
                <Button
                    variant="contained"
                    color="primary"
                    onClick={handleDeploy}
                    startIcon={<Rocket size={16} />}
                    disabled={isPending}
                >
                    {isPending ? "Deploying..." : deployButtonText}
                </Button>
            </Box>
        </DrawerContent>
    </Box>
    );
}
