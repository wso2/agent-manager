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

import { useBuildAgent, useGetAgent } from "@agent-management-platform/api-client";
import { Wrench } from "@wso2/oxygen-ui-icons-react";
import { Box, Button, Typography } from "@wso2/oxygen-ui";
import { FormProvider, useForm } from "react-hook-form";
import { TextInput, DrawerHeader, DrawerContent } from "@agent-management-platform/views";
import { useNotification } from "../providers";

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
    const { notify } = useNotification();
    const { data: agent, isLoading: isLoadingAgent } = useGetAgent({
        orgName,
        projName,
        agentName,
    });

    const methods = useForm<BuildFormData>({
        defaultValues: {
            branch: "main",
            commitId: "",
        },
    });

    const handleBuild = () => {
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
                notify('success', 'Build triggered successfully');
                onClose();
            },
            onError: (error) => {
                const message = error instanceof Error ? error.message : 'Build trigger failed';
                notify('error', message);
            },
        });
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

