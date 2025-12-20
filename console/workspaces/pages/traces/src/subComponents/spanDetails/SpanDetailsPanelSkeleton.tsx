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

import { Box, Divider, Skeleton, useTheme } from "@wso2/oxygen-ui";

export function SpanDetailsPanelSkeleton() {
    const theme = useTheme();

    return (
        <Box
            sx={{
                width: 80,
                p: 2,
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
                gap: 2,
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
                <Skeleton variant="text" width={20} height={5} />
                <Skeleton variant="circular" width={4} height={4} />
            </Box>
            <Divider />
            
            {/* Content */}
            <Box 
                sx={{ 
                    display: 'flex', 
                    flexDirection: 'column', 
                    gap: 2, 
                    overflow: 'auto', 
                    flex: 1 
                }}
            >
                {/* Basic Info Section */}
                <Box>
                    <Skeleton variant="text" width={18} height={3} />
                    <Skeleton 
                        variant="rectangular" 
                        width="100%" 
                        height={30} 
                        sx={{ mt: 1.5, borderRadius: 1 }} 
                    />
                </Box>

                <Divider />

                {/* Timing Section */}
                <Box>
                    <Skeleton variant="text" width={12} height={3} />
                    <Skeleton 
                        variant="rectangular" 
                        width="100%" 
                        height={20} 
                        sx={{ mt: 1.5, borderRadius: 1 }} 
                    />
                </Box>

                <Divider />

                {/* Status Section */}
                <Box>
                    <Skeleton variant="text" width={10} height={3} />
                    <Skeleton 
                        variant="rectangular" 
                        width="100%" 
                        height={15} 
                        sx={{ mt: 1.5, borderRadius: 1 }} 
                    />
                </Box>

                <Divider />

                {/* Attributes Section */}
                <Box>
                    <Skeleton variant="text" width={14} height={3} />
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, mt: 1.5 }}>
                        {[...Array(3)].map((_, index) => (
                            <Box key={index}>
                                <Skeleton variant="text" width={20} height={2.5} />
                                <Skeleton 
                                    variant="rectangular" 
                                    width="100%" 
                                    height={12} 
                                    sx={{ mt: 0.75, borderRadius: 1 }} 
                                />
                            </Box>
                        ))}
                    </Box>
                </Box>
            </Box>
        </Box>
    );
}

