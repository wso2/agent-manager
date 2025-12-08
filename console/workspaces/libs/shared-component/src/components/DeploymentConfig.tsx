import { useDeployAgent, useGetAgent, useGetAgentConfigurations, useListEnvironments } from "@agent-management-platform/api-client";
import { Rocket } from "@wso2/oxygen-ui-icons-react";
import { Box, Button, Skeleton, Typography} from "@wso2/oxygen-ui";
import { FormProvider, useForm } from "react-hook-form";
import { EnvironmentVariable } from "./EnvironmentVariable";
import type { Environment, EnvironmentVariable as EnvVar } from "@agent-management-platform/types";
import { useEffect } from "react";
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

    const deployButtonText = from ? `Promote to ${toEnvironment?.displayName ?? to}` : `Deploy to ${toEnvironment?.displayName ?? to}`;
    const titleText = from ? `Promote to ${toEnvironment?.displayName ?? to}` : `Deploy to ${toEnvironment?.displayName ?? to}`;
    const descriptionText = from
        ? `Promote ${agent?.displayName || 'Agent'} to ${toEnvironment?.displayName ?? to} Environment. Configure environment variables and deploy immediately.`
        : `Deploy ${agent?.displayName || 'Agent'} to ${toEnvironment?.displayName ?? to} Environment. Configure environment variables and deploy immediately.`;

    return (
        <FormProvider {...methods}>
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
                        <EnvironmentVariable />
                    )}
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
        </FormProvider>
    );
}
