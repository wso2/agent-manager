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

import { ApiOutlined, AttachFile, CheckCircle, Circle, Settings } from "@mui/icons-material";
import { Box, Button, Card, CardContent, Collapse, Divider, TextField, Typography } from "@mui/material";
import { useCallback, useRef } from "react";
import { useFormContext, useWatch } from "react-hook-form";

const inputInterfaces = [
    {
        label: "Chat Agent",
        description: "Standard chat agent endpoint for quick deployment — POST /invocation on port 8000.",
        default: true,
        value: "DEFAULT",
        icon: <CheckCircle />,
    },
    {
        label: "Agent API",
        description: "Configure agent API endpoint with openAPI specification.",
        default: false,
        value: "CUSTOM",
        icon: <Settings />,
    },
];

export const InputInterface = () => {
    const { setValue, control, register, formState: { errors } } = useFormContext();
    const interfaceType = useWatch({ control, name: 'interfaceType' }) || 'DEFAULT';
    const port = useWatch({ control, name: 'port' }) as unknown as string;
    const openApiFileName = useWatch({ control, name: 'openApiFileName' }) as string;
    const fileInputRef = useRef<HTMLInputElement | null>(null);

    const handleSelect = useCallback((value: string) => {
        setValue('interfaceType', value, { shouldValidate: true });
        if (value === 'DEFAULT') {
            setValue('openApiFileName', '', { shouldValidate: true });
            setValue('openApiContent', '', { shouldValidate: true });
            setValue('port', '' as unknown as number, { shouldValidate: true });
            setValue('basePath', '/', { shouldValidate: true });
        }
    }, [setValue]);

    const handlePortChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
        const next = e.target.value;
        if (/^\d*$/.test(next)) {
            setValue('port', next === '' ? ('' as unknown as number) : Number(next), { shouldValidate: true });
        }
    }, [setValue]);

    const handleFilePick = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) return;

        // Validate file size (max 2MB)
        const MAX_FILE_SIZE = 2 * 1024 * 1024; // 2MB
        if (file.size > MAX_FILE_SIZE) {
            alert('File size exceeds 2MB. Please upload a smaller file.');
            e.target.value = ''; // Reset file input
            return;
        }

        // Validate file extension
        if (!file.name.match(/\.(yaml|yml)$/i)) {
            alert('Please upload a YAML file (.yaml or .yml)');
            e.target.value = '';
            return;
        }

        setValue('openApiFileName', file.name, { shouldValidate: true });
        const reader = new FileReader();
        reader.onload = () => {
            const text = typeof reader.result === 'string' ? reader.result : '';
            setValue('openApiContent', text, { shouldValidate: true });
        };
        reader.onerror = () => {
            alert('Failed to read file. Please try again.');
            setValue('openApiFileName', '');
        };
        reader.readAsText(file);
    }, [setValue]);

    return (
        <Card variant="outlined">
            <CardContent>
                <Box display="flex" flexDirection="row" alignItems="center" gap={1}>
                    <ApiOutlined fontSize="medium" color="disabled" />
                    <Typography variant="h5">
                        Agent Interface
                    </Typography>
                </Box>
                <Typography variant="body2" color="text.secondary">
                    How your agent receives requests
                </Typography>
                <Box display="flex" flexDirection="column" gap={2} pt={2}>
                    <Box display="flex" flexDirection="row" gap={2}>
                        {
                            inputInterfaces.map((inputInterface) => (
                                <Card
                                    key={inputInterface.value}
                                    variant="outlined"
                                    onClick={() => handleSelect(inputInterface.value)}
                                    sx={{
                                        cursor: 'pointer', flexGrow: 1,
                                        backgroundColor: interfaceType === inputInterface.value ? 'action.hover' : 'background.paper',
                                        '&:hover': {
                                            backgroundColor: 'action.hover',
                                        },
                                    }}

                                >
                                    <Box display="flex" p={2} flexDirection="column" gap={2}>
                                        <Box display="flex" flexDirection="row" alignItems="center" gap={2}>
                                            {interfaceType === inputInterface.value
                                                ? <CheckCircle color="success" /> : <Circle color="disabled" />}
                                            <Divider orientation="vertical" flexItem />
                                            <Box>
                                                <Typography variant="h6">{inputInterface.label}</Typography>
                                                <Typography variant="body2" color="text.secondary">{inputInterface.description}</Typography>
                                            </Box>
                                        </Box>
                                    </Box>
                                </Card>
                            ))
                        }
                    </Box>
                    <Collapse in={interfaceType === 'CUSTOM'}>
                        <Box display="flex" flexDirection="column" gap={2}>
                            <Box display="flex" flexDirection="row" gap={2}>
                                <Box display="flex" flexDirection="column" flexGrow={1}>
                                    <TextField
                                        label="OpenAPI Spec"
                                        placeholder="openapi.yaml"
                                        value={openApiFileName || ''}
                                        fullWidth
                                        InputProps={{ readOnly: true }}
                                        error={!!errors.openApiFileName || !!errors.openApiContent}
                                        helperText={(errors.openApiFileName?.message as string) || (errors.openApiContent?.message as string) || (openApiFileName ? "File loaded in browser" : "Upload your OpenAPI YAML file")}
                                    />
                                    <Box pt={1}>
                                        <Button variant="outlined" startIcon={<AttachFile />} onClick={() => fileInputRef.current?.click()}>Choose File</Button>
                                    </Box>
                                    <input
                                        ref={fileInputRef}
                                        type="file"
                                        accept=".yaml,.yml,text/yaml,application/x-yaml,application/yaml"
                                        style={{ display: 'none' }}
                                        onChange={handleFilePick}
                                    />
                                </Box>
                                <Box>
                                    <TextField
                                        label="Port"
                                        placeholder="8080"
                                        required
                                        value={port as unknown as string}
                                        onChange={handlePortChange}
                                        fullWidth
                                        type="number"
                                        error={!!errors.port}
                                        helperText={(errors.port?.message as string) || (port ? undefined : "Port is required")}
                                    />
                                </Box>
                            </Box>
                            <Box>
                                <TextField
                                    label="Base Path"
                                    placeholder="/"
                                    required
                                    fullWidth
                                    error={!!errors.basePath}
                                    helperText={(errors.basePath?.message as string) || "API base path (e.g., / or /api/v1)"}
                                    {...register('basePath')}
                                />
                            </Box>
                        </Box>
                    </Collapse>
                </Box>
            </CardContent>
        </Card>
    );
};
