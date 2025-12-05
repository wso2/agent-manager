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

import { Box, Card, CardContent, TextField, Typography, Alert } from "@mui/material";
import { CloudSync, Info } from "@mui/icons-material";
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

export const ConnectAgentForm = () => {
    const { register, formState: { errors }, watch, setValue } = useFormContext();
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
        <Box display="flex" flexDirection="column" gap={2} pt={2} flexGrow={1}>
            {/* Agent Details */}
            <Card variant="outlined">
                <CardContent>
                    <Box display="flex" flexDirection="row" alignItems="center" gap={1}>
                        <CloudSync fontSize="medium" color="disabled" />
                        <Typography variant="h5">
                            Agent Details
                        </Typography>
                    </Box>
                    <Typography variant="body2" color="text.secondary">
                        Provide basic information about your existing agent
                    </Typography>

                    <Box display="flex" flexDirection="column" gap={2} pt={2}>
                        <TextField
                            placeholder="e.g., Customer Support Agent"
                            label="Display Name"
                            fullWidth
                            error={!!errors.displayName}
                            helperText={(errors.displayName?.message as string) || "Human-readable name for your agent"}
                            {...register('displayName')}
                        />

                        <TextField
                            placeholder="e.g., customer-support-agent"
                            label="Name"
                            fullWidth
                            error={!!errors.name}
                            helperText={(errors.name?.message as string) || "Use lowercase letters, numbers, and hyphens only (used in URLs)"}
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
                            error={!!errors.description}
                            helperText={errors.description?.message as string}
                            {...register('description')}
                        />

                        <Alert severity="info" icon={<Info />} variant="outlined">
                            After creating the agent, you&apos;ll receive OpenTelemetry 
                            configuration details
                            to integrate with your existing deployment.
                        </Alert>
                    </Box>
                </CardContent>
            </Card>
        </Box>
    );
};
