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

import { Box, Typography } from "@wso2/oxygen-ui";
import { CodeBlock } from "@agent-management-platform/shared-component";

interface SetupStepProps {
    stepNumber: number;
    title: string;
    description?: string;
    code: string;
    language?: string;
    fieldId?: string;
}

export const SetupStep = ({ 
    stepNumber, 
    title, 
    description,
    code, 
    language = "bash",
    fieldId 
}: SetupStepProps) => {
    return (
        <Box display="flex" gap={1} flexDirection="column">
            <Box display="flex" alignItems="center" gap={1}>
                <Box
                    sx={{
                        gap: 2,
                        width: 20,
                        height: 20,
                        borderRadius: '50%',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        bgcolor: (theme) =>
                           theme.palette.primary.main,
                        color: 'primary.contrastText',
                        fontWeight: 600,
                    }}
                >
                    <Typography variant="body2" fontWeight={600}>
                        {stepNumber}
                    </Typography>
                </Box>
                <Typography variant="body1">
                    {title}
                </Typography>
            </Box>
            <Box>
                <CodeBlock
                    code={code}
                    language={language}
                    fieldId={fieldId}
                />
            </Box>
            {description && (
                <Typography variant="body2" color="textSecondary">
                    {description}
                </Typography>
            )}
        </Box>
    );
};

