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

import { useBuildAgent } from "@agent-management-platform/api-client";
import { Close, Construction } from "@mui/icons-material";
import { Box, Button, Divider, IconButton, TextField, Typography } from "@mui/material";
import { FormProvider, useForm } from "react-hook-form";

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
    const {mutate: buildAgent, isPending} = useBuildAgent();
    const methods = useForm<BuildFormData>({
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
                <Box width="100%" display="flex" flexDirection="column" gap={2}>
                    <Box display="flex" justifyContent="space-between" alignItems="center">
                        <Box display="flex" flexDirection="column" gap={1}>
                            <Typography variant="h4">
                                <Construction fontSize="inherit" />
                                &nbsp;
                                Trigger Build
                            </Typography>
                            <Typography variant="caption">
                                Build {agentName} from a specific branch and commit.
                                Leave commit ID empty to build from the latest commit.
                            </Typography>
                        </Box>
                        <IconButton color="error" size="small" onClick={onClose}>
                            <Close />
                        </IconButton>
                    </Box>
                    <Divider />

                    <Box display="flex" flexDirection="column" gap={2}>
                        <TextField
                            label="Branch"
                            placeholder="main"
                            fullWidth
                            disabled
                            {...methods.register("branch", { required: false })}
                            helperText="Enter the branch name to build from"
                        />

                        <TextField
                            label="Commit ID (Optional)"
                            placeholder="Leave empty for latest commit"
                            fullWidth
                            {...methods.register("commitId")}
                            helperText="Optionally specify a commit ID to build from"
                        />
                    </Box>

                    <Box display="flex" gap={1} justifyContent="flex-end" width="100%">
                        <Button
                            variant="outlined"
                            color="primary"
                            size="large"
                            onClick={onClose}
                        >
                            Cancel
                        </Button>
                        <Button
                            variant="contained"
                            color="primary"
                            size="large"
                            onClick={handleBuild}
                            startIcon={<Construction fontSize="small" />}
                            disabled={isPending}
                        >
                            Trigger Build
                        </Button>
                    </Box>
                </Box>
            </FormProvider>
    );
}

