import { useBuildAgent, useGetAgent } from "@agent-management-platform/api-client";
import { Wrench } from "@wso2/oxygen-ui-icons-react";
import { Box, Button, Typography } from "@wso2/oxygen-ui";
import { FormProvider, useForm } from "react-hook-form";
import { TextInput, DrawerHeader, DrawerContent } from "@agent-management-platform/views";

interface BuildPanelProps {
    onClose: () => void;
    orgName: string;
    projName: string;
    agentName: string;
}

interface BuildFormData {
    branch: string;
    commitId?: string;
}

export function BuildPanel({
    onClose,
    orgName,
    projName,
    agentName,
}: BuildPanelProps) {
    const { mutate: buildAgent, isPending } = useBuildAgent();
    const { data: agent, isLoading: isLoadingAgent } = useGetAgent({
        orgName,
        projName,
        agentName,
    });    const methods = useForm<BuildFormData>({
        defaultValues: {
            branch: "main",
            commitId: "",
        },
    });

    const handleBuild = async () => {
        try {
            const formData = methods.getValues();
            buildAgent({
                params: {
                    orgName,
                    projName,
                    agentName,
                },
                query: {
                    commitId: formData.commitId || "",
                },
            }, {
                onSuccess: () => {
                    onClose();
                },
                onError: (error) => {
                    console.error("Build trigger failed:", error);
                },
            });
        }
        catch (error) {
            console.error("Build trigger failed:", error);
        }
    };

    return (
        <FormProvider {...methods}>
            <Box display="flex" flexDirection="column" height="100%">
                <DrawerHeader
                    icon={<Wrench size={24} />}
                    title="Trigger Build"
                    onClose={onClose}
                />
                <DrawerContent>
                    <Typography variant="body2" color="text.secondary">
                        Build {agent?.displayName || agentName} from a specific branch and commit.
                    </Typography>

                <Box display="flex" flexDirection="column" gap={2}>
                    <TextInput
                        label="Branch"
                        placeholder="main"
                        size="small"
                        fullWidth
                        disabled
                        {...methods.register("branch", { required: false })}
                        helperText="Enter the branch name to build from"
                    />

                    <TextInput
                        label="Commit Hash (Optional)"
                        placeholder="e.g. 308c9a68c9ab6571e17004e259786ca0039d75e9"
                        size="small"
                        fullWidth
                        {...methods.register("commitId")}
                        helperText="Leave empty for latest commit"
                    />
                </Box>

                <Box display="flex" gap={1} justifyContent="flex-end" width="100%">
                    <Button
                        variant="outlined"
                        color="primary"
                        onClick={onClose}
                    >
                        Cancel
                    </Button>
                    <Button
                        variant="contained"
                        color="primary"
                        onClick={handleBuild}
                        startIcon={<Wrench size={16} />}
                        disabled={isPending || isLoadingAgent}
                    >
                        Trigger Build
                    </Button>
                </Box>
            </DrawerContent>
        </Box>
        </FormProvider>
    );
}

