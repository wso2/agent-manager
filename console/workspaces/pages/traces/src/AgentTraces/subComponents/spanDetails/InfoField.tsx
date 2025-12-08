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

import { Box, Typography, useTheme } from "@wso2/oxygen-ui";
import { ReactNode } from "react";

interface InfoFieldProps {
    label: string;
    value: ReactNode;
    isMonospace?: boolean;
}

export function InfoField({ label, value, isMonospace = false }: InfoFieldProps) {
    const theme = useTheme();

    return (
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
                {label}
            </Typography>
            <Typography 
                variant="body2" 
                sx={{ 
                    fontFamily: isMonospace ? 'monospace' : 'inherit',
                    fontSize: isMonospace ? theme.typography.caption.fontSize : 'inherit',
                    color: theme.palette.text.primary 
                }}
            >
                {value}
            </Typography>
        </Box>
    );
}

