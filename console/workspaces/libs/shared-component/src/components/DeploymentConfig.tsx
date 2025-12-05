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

import { useDeployAgent, useGetAgentConfigurations, useListEnvironments } from "@agent-management-platform/api-client";
import { Close, Rocket } from "@mui/icons-material";
import { Box, Button, Divider, IconButton, Skeleton, TextField, Typography } from "@mui/material";
import { FormProvider, useForm } from "react-hook-form";
import { EnvironmentVariable } from "./EnvironmentVariable";
import type { Environment, EnvironmentVariable as EnvVar } from "@agent-management-platform/types";
import { useEffect } from "react";

interface DeploymentConfigProps {
    onClose: () => void;
    from?: string;
    to: string;
    orgName: string;
    projName: string;
    agentName: string;
    imageId: string;
}

interface DeploymentFormData {
    env: Array<{ key: string; value: string }>
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
    const { mutate: deployAgent, isPending } = useDeployAgent();
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

    const methods = useForm<DeploymentFormData>({
        defaultValues: {
            env: configurations?.configurations || [],
        },
    });

    useEffect(() => {
        methods.reset({
            env: configurations?.configurations || [],
        });
    }, [configurations, methods]);

    const handleDeploy = async () => {
        try {
            const formData = methods.getValues();

            const envVariables: EnvVar[] = formData.env
                .filter((envVar: { key: string; value: string }) => envVar.key && envVar.value)
                .map((envVar: { key: string; value: string }) => ({
                    key: envVar.key,
                    value: envVar.value,
                }));
            deployAgent({
                params: {
                    orgName,
                    projName,
                    agentName,
                },
                body: {
                    imageId: imageId,
                    env: envVariables.length > 0 ? envVariables : undefined,
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

    const deployButtonText = from ? `Promote to ${toEnvironment?.displayName || to}` : `Deploy to ${toEnvironment?.displayName || to}`;
    const titleText = from ? `Promote to ${to}` : `Deploy to ${to}`;
    const descriptionText = from
        ? `Promote ${agentName || 'Agent'} to ${to} environment. Configure environment variables and deploy immediately.`
        : `Deploy ${agentName || 'Agent'} to ${to} environment. Configure environment variables and deploy immediately.`;

    return (
        <FormProvider {...methods}>
            <Box width="100%" display="flex" flexDirection="column" gap={2}>
                <Box display="flex" justifyContent="space-between" alignItems="center">
                    <Box display="flex" flexDirection="column" gap={1}>
                        <Typography variant="h4">
                            <Rocket fontSize="inherit" />
                            &nbsp;
                            {titleText}
                        </Typography>
                        <Typography variant="caption">
                            {descriptionText}
                        </Typography>
                    </Box>
                    <IconButton color="error" size="small" onClick={onClose}>
                        <Close />
                    </IconButton>
                </Box>
                <Divider />
                <Typography variant="h5">
                    Deployment Details
                </Typography>
                <Box display="flex" flexDirection="column" gap={1}>
                    <TextField
                        label="Image ID"
                        value={imageId}
                        disabled
                        fullWidth
                    />
                </Box>
                {isLoadingConfigurations || isLoadingEnvironments ? (
                    <Box display="flex" flexDirection="column" gap={1} width="100%">
                        <Skeleton variant="rectangular" width="100%" height={305} />
                    </Box>
                ) : (
                    <EnvironmentVariable />
                )}
                <Box display="flex" gap={1} justifyContent="flex-end" width="100%">
                    <Button
                        variant="outlined"
                        color="primary"
                        size="large"
                        onClick={onClose}
                        disabled={isPending}
                    >
                        Cancel
                    </Button>
                    <Button
                        variant="contained"
                        color="primary"
                        size="large"
                        onClick={handleDeploy}
                        startIcon={<Rocket fontSize="small" />}
                        disabled={isPending}
                    >
                        {isPending ? "Deploying..." : deployButtonText}
                    </Button>
                </Box>
            </Box>
        </FormProvider>
    );
}
