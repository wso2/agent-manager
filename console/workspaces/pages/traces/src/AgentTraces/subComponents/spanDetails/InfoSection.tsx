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

import { Box, Card, CardContent, Typography, useTheme } from "@wso2/oxygen-ui";
import { ReactNode } from "react";

interface InfoSectionProps {
    title: string;
    children: ReactNode;
}

export function InfoSection({ title, children }: InfoSectionProps) {
    const theme = useTheme();

    return (
        <Box>
            <Typography 
                variant="subtitle2" 
                fontWeight="bold" 
                sx={{ 
                    color: theme.palette.text.secondary, 
                    mb: 1.5 
                }}
            >
                {title}
            </Typography>
            <Card variant="outlined">
                <CardContent>
                    <Box 
                        sx={{ 
                            display: 'flex', 
                            flexDirection: 'column', 
                            gap: 2 
                        }}
                    >
                        {children}
                    </Box>
                </CardContent>
            </Card>
        </Box>
    );
}

