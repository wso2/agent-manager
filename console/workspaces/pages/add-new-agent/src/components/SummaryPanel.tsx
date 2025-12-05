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

import { Box, Button, Container, Typography } from "@mui/material";
import { Close, RocketOutlined } from "@mui/icons-material";
import { memo, useMemo } from "react";
import { Control, FieldErrors, useWatch } from "react-hook-form";

interface SummaryPanelProps {
    control: Control<any>;
    errors: FieldErrors<any>;
    isValid: boolean;
    isPending: boolean;
    onCancel: () => void;
    onSubmit: () => void;
}

export const SummaryPanel = memo((
    { control, errors, isValid, isPending, onCancel, onSubmit }: SummaryPanelProps
) => {
    const agentName = useWatch({ control, name: 'agentName' }) || 'Untitled Agent';
    const repositoryUrl = useWatch({ control, name: 'repositoryUrl' });
    const branch = useWatch({ control, name: 'branch' }) || 'main';
    const interfaceType = useWatch({ control, name: 'interfaceType' }) || 'DEFAULT';

    const repoDisplay = useMemo(() => {
        if (!repositoryUrl) return 'Repository not set';
        try {
            const url = new URL(repositoryUrl);
            const parts = url.pathname.split('/').filter(Boolean);
            return parts.length >= 2 ? `${parts[parts.length - 2]}/${parts[parts.length - 1].replace('.git', '')}` : repositoryUrl;
        } catch {
            return repositoryUrl;
        }
    }, [repositoryUrl]);

    const interfaceDisplay = interfaceType === 'DEFAULT' ? 'Default Interface' : 'Custom API';

    const errorMessages = useMemo(() => {
        const msgs: string[] = [];
        if (errors.agentName?.message) msgs.push('Agent Name');
        if (errors.repositoryUrl?.message) msgs.push('Repository URL');
        if (errors.branch?.message) msgs.push('Branch');
        if (errors.runCommand?.message) msgs.push('Start Command');
        if (errors.port?.message) msgs.push('Port');
        if (errors.openApiFileName?.message || errors.openApiContent?.message) msgs.push('OpenAPI Spec');
        return msgs;
    }, [errors]);

    return (
        <Container maxWidth="lg" sx={{ display: 'flex', justifyContent: 'space-between', gap: 1 }}>
            <Box display="flex" flexDirection="column" gap={0.5}>
                <Typography variant="h6">{agentName} → Development</Typography>
                <Typography variant="caption" color="text.secondary">
                    {repoDisplay} • {branch} • {interfaceDisplay}
                </Typography>
                {errorMessages.length > 0 && (
                    <Typography variant="body2" color="error">
                        Please fill required fields: {errorMessages.join(', ')}
                    </Typography>
                )}
            </Box>
            <Box display="flex" flexDirection="row" gap={1} alignItems="center">
                <Button variant="outlined" color="primary" size='medium' onClick={onCancel} startIcon={<Close fontSize="small" />}>
                    Cancel
                </Button>
                <Button 
                    variant="contained" 
                    color="primary" 
                    size='large' 
                    startIcon={<RocketOutlined fontSize="small" />} 
                    onClick={onSubmit}
                    disabled={isPending || !isValid}
                >
                    Deploy to Development
                </Button>
            </Box>
        </Container>
    );
});

