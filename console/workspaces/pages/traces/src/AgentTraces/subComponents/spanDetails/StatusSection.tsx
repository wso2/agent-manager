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

import { Box, Chip, Typography, useTheme } from "@wso2/oxygen-ui";
import { Span } from "@agent-management-platform/types";
import { InfoSection } from "./InfoSection";

interface StatusSectionProps {
    span: Span;
}

export function StatusSection({ span }: StatusSectionProps) {
    const theme = useTheme();

    return (
        <InfoSection title="Status">
            <Box>
                <Typography 
                    variant="caption" 
                    fontWeight="600" 
                    sx={{ 
                        color: theme.palette.text.secondary, 
                        display: 'block', 
                        mb: 0.5 
                    }}
                >
                    Kind
                </Typography>
                <Chip label={span.kind} size="small" />
            </Box>
            
            <Box>
                <Typography 
                    variant="caption" 
                    fontWeight="600" 
                    sx={{ 
                        color: theme.palette.text.secondary, 
                        display: 'block', 
                        mb: 0.5 
                    }}
                >
                    Status
                </Typography>
                <Chip 
                    label={span.status} 
                    size="small"
                    color={
                        span.status === 'OK' || span.status === 'UNSET' 
                            ? 'success' 
                            : 'error'
                    }
                />
            </Box>
        </InfoSection>
    );
}

