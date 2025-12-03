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

import { Box, Divider, Skeleton, useTheme } from "@mui/material";

export function SpanDetailsPanelSkeleton() {
    const theme = useTheme();

    return (
        <Box
            sx={{
                width: theme.spacing(80),
                p: theme.spacing(2),
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
                gap: theme.spacing(2),
                bgcolor: theme.palette.background.paper
            }}
        >
            {/* Header */}
            <Box 
                sx={{ 
                    display: 'flex', 
                    justifyContent: 'space-between', 
                    alignItems: 'center' 
                }}
            >
                <Skeleton variant="text" width={theme.spacing(20)} height={theme.spacing(5)} />
                <Skeleton variant="circular" width={theme.spacing(4)} height={theme.spacing(4)} />
            </Box>
            <Divider />
            
            {/* Content */}
            <Box 
                sx={{ 
                    display: 'flex', 
                    flexDirection: 'column', 
                    gap: theme.spacing(2), 
                    overflow: 'auto', 
                    flex: 1 
                }}
            >
                {/* Basic Info Section */}
                <Box>
                    <Skeleton variant="text" width={theme.spacing(18)} height={theme.spacing(3)} />
                    <Skeleton 
                        variant="rectangular" 
                        width="100%" 
                        height={theme.spacing(30)} 
                        sx={{ mt: theme.spacing(1.5), borderRadius: 1 }} 
                    />
                </Box>

                <Divider />

                {/* Timing Section */}
                <Box>
                    <Skeleton variant="text" width={theme.spacing(12)} height={theme.spacing(3)} />
                    <Skeleton 
                        variant="rectangular" 
                        width="100%" 
                        height={theme.spacing(20)} 
                        sx={{ mt: theme.spacing(1.5), borderRadius: 1 }} 
                    />
                </Box>

                <Divider />

                {/* Status Section */}
                <Box>
                    <Skeleton variant="text" width={theme.spacing(10)} height={theme.spacing(3)} />
                    <Skeleton 
                        variant="rectangular" 
                        width="100%" 
                        height={theme.spacing(15)} 
                        sx={{ mt: theme.spacing(1.5), borderRadius: 1 }} 
                    />
                </Box>

                <Divider />

                {/* Attributes Section */}
                <Box>
                    <Skeleton variant="text" width={theme.spacing(14)} height={theme.spacing(3)} />
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: theme.spacing(2), mt: theme.spacing(1.5) }}>
                        {[...Array(3)].map((_, index) => (
                            <Box key={index}>
                                <Skeleton variant="text" width={theme.spacing(20)} height={theme.spacing(2.5)} />
                                <Skeleton 
                                    variant="rectangular" 
                                    width="100%" 
                                    height={theme.spacing(12)} 
                                    sx={{ mt: theme.spacing(0.75), borderRadius: 1 }} 
                                />
                            </Box>
                        ))}
                    </Box>
                </Box>
            </Box>
        </Box>
    );
}

