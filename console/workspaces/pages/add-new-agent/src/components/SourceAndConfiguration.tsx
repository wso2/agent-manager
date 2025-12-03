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

import { AppRegistration, Info } from "@mui/icons-material";
import { Alert, Box, Card, CardContent, TextField, Typography, useTheme } from "@mui/material";
import { useFormContext } from "react-hook-form";
import { useEffect, useRef } from "react";

// Generate a random 6-character string
const generateRandomString = (length: number = 6): string => {
    const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
    let result = '';
    for (let i = 0; i < length; i++) {
        result += chars.charAt(Math.floor(Math.random() * chars.length));
    }
    return result;
};

// Convert display name to URL-friendly format
const sanitizeNameForUrl = (displayName: string): string => {
    return displayName
        .toLowerCase()
        .trim()
        .replace(/[^a-z0-9\s-]/g, '') // Remove special characters
        .replace(/\s+/g, '-') // Replace spaces with hyphens
        .replace(/-+/g, '-') // Replace multiple hyphens with single hyphen
        .replace(/^-|-$/g, ''); // Remove leading/trailing hyphens
};

export const SourceAndConfiguration = () => {
    const { register, formState: { errors }, watch, setValue } = useFormContext();
    const theme = useTheme();
    const isNameManuallyEdited = useRef(false);
    const randomSuffix = useRef<string>('');
    const displayName = watch('displayName');

    // Generate random suffix once
    if (!randomSuffix.current) {
        randomSuffix.current = generateRandomString(6);
    }

    // Auto-generate name from display name
    useEffect(() => {
        if (displayName && !isNameManuallyEdited.current) {
            const sanitizedName = sanitizeNameForUrl(displayName);
            if (sanitizedName) {
                const generatedName = `${sanitizedName}-${randomSuffix.current}`;
                setValue('name', generatedName, { 
                    shouldValidate: true, 
                    shouldDirty: true,
                    shouldTouch: false 
                });
            }
        } else if (!displayName && !isNameManuallyEdited.current) {
            // Clear the name field if display name is empty
            setValue('name', '', { 
                shouldValidate: true, 
                shouldDirty: true,
                shouldTouch: false 
            });
        }
    }, [displayName, setValue]);

    return (
        <Card variant="outlined">
            <CardContent>
                <Box display="flex" flexDirection="row" alignItems="center" gap={1}>
                    <AppRegistration fontSize="medium" color="disabled" />
                    <Typography variant="h5">
                        Source & Configuration
                    </Typography>
                </Box>
                <Typography variant="body2" color="text.secondary">
                    Connect your GitHub repository and configure basics
                </Typography>

                <Box display="flex" flexDirection="column" gap={2} pt={2}>
                    <TextField
                        placeholder="e.g., Customer Support Agent"
                        label="Display Name"
                        fullWidth
                        error={!!errors.displayName}
                        helperText={errors.displayName?.message as string || "Human-readable name for your agent"}
                        {...register('displayName')}
                    />

                    <TextField
                        placeholder="e.g., customer-support-agent"
                        label="Agent Name"
                        fullWidth
                        error={!!errors.name}
                        helperText={errors.name?.message as string || "Use lowercase letters, numbers, and hyphens only (used in URLs)"}
                        {...register('name', {
                            onChange: () => {
                                isNameManuallyEdited.current = true;
                            }
                        })}
                    />

                    <TextField
                        placeholder="Short description of what this agent does"
                        label="Description (optional)"
                        fullWidth
                        multiline
                        minRows={2}
                        maxRows={6}
                        sx={{
                            "& .MuiInputBase-root": {
                                padding: theme.spacing(0),
                            },
                        }}
                        error={!!errors.description}
                        helperText={errors.description?.message as string}
                        {...register('description')}
                    />

                    <TextField
                        placeholder="https://github.com/username/repo"
                        label="GitHub Repository"
                        fullWidth
                        error={!!errors.repositoryUrl}
                        helperText={errors.repositoryUrl?.message as string}
                        {...register('repositoryUrl')}
                    />

                    <Box display="flex" flexDirection="row" gap={2}>
                        <TextField
                            placeholder="main"
                            label="Branch"
                            fullWidth
                            error={!!errors.branch}
                            helperText={errors.branch?.message as string}
                            {...register('branch')}
                        />
                        <TextField
                            placeholder="/ (root directory)"
                            label="Project Path"
                            fullWidth
                            error={!!errors.appPath}
                            helperText={errors.appPath?.message as string}
                            {...register('appPath')}
                        />
                    </Box>

                    <Box display="flex" flexDirection="row" gap={2}>
                        <TextField
                            placeholder="python"
                            disabled
                            label="Language"
                            fullWidth
                            error={!!errors.language}
                            helperText={errors.language?.message as string || "e.g., python, nodejs, go"}
                            {...register('language')}
                        />
                        <TextField
                            placeholder="3.11"
                            label="Language Version"
                            fullWidth
                            error={!!errors.languageVersion}
                            helperText={errors.languageVersion?.message as string || "e.g., 3.11, 20, 1.21"}
                            {...register('languageVersion')}
                        />
                    </Box>

                    <TextField
                        placeholder="python main.py"
                        label="Start Command"
                        fullWidth
                        error={!!errors.runCommand}
                        helperText={errors.runCommand?.message as string || "Dependencies auto-install from package.json, requirements.txt, or pyproject.toml"}
                        {...register('runCommand')}
                    />

                    <Alert severity="warning" icon={<Info />} variant="outlined">
                        {`All deployments go to Development first. After testing, promote to production from the agent details page.`}
                    </Alert>
                </Box>
            </CardContent>
        </Card>
    );
};


