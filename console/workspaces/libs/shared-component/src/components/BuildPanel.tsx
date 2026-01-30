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

import { useBuildAgent, useGetAgent, useListBranches, useListCommits } from "@agent-management-platform/api-client";
import { Wrench } from "@wso2/oxygen-ui-icons-react";
import {
    Box,
    Button,
    Typography,
    Select,
    MenuItem,
    SelectChangeEvent,
    CircularProgress,
    FormControl,
    InputLabel,
    FormHelperText,
    Chip,
} from "@wso2/oxygen-ui";
import { FormProvider, useForm } from "react-hook-form";
import { DrawerHeader, DrawerContent } from "@agent-management-platform/views";
import { useEffect, useMemo } from "react";

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

/**
 * Parses a GitHub repository URL to extract owner and repository name.
 * Supports formats:
 * - https://github.com/owner/repo
 * - https://github.com/owner/repo.git
 * - git@github.com:owner/repo.git
 */
function parseGitHubUrl(url: string): { owner: string; repo: string } | null {
    if (!url) return null;

    // Handle HTTPS URLs: https://github.com/owner/repo or https://github.com/owner/repo.git
    const httpsMatch = url.match(/github\.com\/([^/]+)\/([^/.]+)/);
    if (httpsMatch) {
        return { owner: httpsMatch[1], repo: httpsMatch[2] };
    }

    // Handle SSH URLs: git@github.com:owner/repo.git
    const sshMatch = url.match(/github\.com:([^/]+)\/([^/.]+)/);
    if (sshMatch) {
        return { owner: sshMatch[1], repo: sshMatch[2] };
    }

    return null;
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
    });

    const methods = useForm<BuildFormData>({
        defaultValues: {
            branch: "",
            commitId: "",
        },
    });

    const selectedBranch = methods.watch("branch");

    // Parse repository URL to get owner and repo name
    const repoInfo = useMemo(() => {
        const repoUrl = agent?.provisioning?.repository?.url;
        return repoUrl ? parseGitHubUrl(repoUrl) : null;
    }, [agent?.provisioning?.repository?.url]);

    // Fetch branches
    const {
        data: branchesData,
        isLoading: isLoadingBranches,
    } = useListBranches(
        {
            owner: repoInfo?.owner || "",
            repository: repoInfo?.repo || "",
        },
        { limit: 100 },
        !!repoInfo,
    );

    // Fetch commits for selected branch
    const {
        data: commitsData,
        isLoading: isLoadingCommits,
    } = useListCommits(
        {
            owner: repoInfo?.owner || "",
            repo: repoInfo?.repo || "",
            branch: selectedBranch || undefined,
        },
        { limit: 50 },
        !!repoInfo && !!selectedBranch,
    );

    // Set default branch when branches are loaded
    useEffect(() => {
        if (branchesData?.branches && !methods.getValues("branch")) {
            const defaultBranch = branchesData.branches.find(b => b.isDefault);
            if (defaultBranch) {
                methods.setValue("branch", defaultBranch.name);
            } else if (branchesData.branches.length > 0) {
                methods.setValue("branch", branchesData.branches[0].name);
            }
        }
    }, [branchesData?.branches, methods]);

    // Set first commit (latest) when commits are loaded or branch changes
    useEffect(() => {
        if (commitsData?.commits && commitsData.commits.length > 0) {
            methods.setValue("commitId", commitsData.commits[0].sha);
        } else {
            methods.setValue("commitId", "");
        }
    }, [commitsData?.commits, methods]);

    const handleBranchChange = (event: SelectChangeEvent<string>) => {
        methods.setValue("branch", event.target.value);
    };

    const handleCommitChange = (event: SelectChangeEvent<string>) => {
        methods.setValue("commitId", event.target.value);
    };

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
            });
        }
        catch {
            // Build trigger failed - error handling can be added here if needed
        }
    };

    const branches = branchesData?.branches || [];
    const commits = commitsData?.commits || [];

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
                    <FormControl fullWidth size="small">
                        <InputLabel id="branch-select-label" shrink>Branch</InputLabel>
                        <Select
                            notched
                            labelId="branch-select-label"
                            id="branch-select"
                            value={selectedBranch}
                            label="Branch"
                            onChange={handleBranchChange}
                            disabled={isLoadingBranches || !repoInfo}
                            endAdornment={
                                isLoadingBranches
                                    ? <CircularProgress size={20} sx={{ mr: 2 }} />
                                    : undefined
                            }
                            MenuProps={{
                                PaperProps: {
                                    style: {
                                        maxHeight: 300,
                                    },
                                },
                            }}
                        >
                            {branches.map((branch) => (
                                <MenuItem key={branch.name} value={branch.name}>
                                    {branch.name}
                                    {branch.isDefault && " (default)"}
                                </MenuItem>
                            ))}
                        </Select>
                        <FormHelperText>Select the branch to build from</FormHelperText>
                    </FormControl>

                    <FormControl fullWidth size="small">
                        <InputLabel id="commit-select-label" shrink>Commit</InputLabel>
                        <Select
                            notched
                            labelId="commit-select-label"
                            id="commit-select"
                            value={methods.watch("commitId") || ""}
                            label="Commit"
                            onChange={handleCommitChange}
                            disabled={isLoadingCommits || !selectedBranch}
                            endAdornment={
                                isLoadingCommits
                                    ? <CircularProgress size={20} sx={{ mr: 2 }} />
                                    : undefined
                            }
                            MenuProps={{
                                PaperProps: {
                                    style: {
                                        maxHeight: 300,
                                    },
                                },
                            }}
                        >
                            {commits.map((commit, index) => (
                                <MenuItem key={commit.sha} value={commit.sha}>
                                    <Box display="flex" flexDirection="column" width="100%">
                                        <Box display="flex" alignItems="center" gap={1}>
                                            <Typography variant="body2" noWrap sx={{ maxWidth: 350 }}>
                                                {commit.message?.split('\n')[0] || ""}
                                            </Typography>
                                            {index === 0 && (
                                                <Chip label="Latest" size="small" color="primary" />
                                            )}
                                        </Box>
                                        <Typography variant="caption" color="text.secondary">
                                            {commit.shortSha}
                                        </Typography>
                                    </Box>
                                </MenuItem>
                            ))}
                        </Select>
                        <FormHelperText>Select the commit to build</FormHelperText>
                    </FormControl>
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
                        disabled={isPending || isLoadingAgent || !selectedBranch}
                    >
                        Trigger Build
                    </Button>
                </Box>
            </DrawerContent>
        </Box>
        </FormProvider>
    );
}
