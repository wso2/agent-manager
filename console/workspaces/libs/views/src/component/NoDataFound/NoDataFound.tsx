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

import { Box, Typography} from "@wso2/oxygen-ui";
import { SearchX as SearchOffOutlined } from "@wso2/oxygen-ui-icons-react";
import { FadeIn } from "../FadeIn/FadeIn";
import { ReactNode } from "react";

interface NoDataFoundProps {
    message?: string;
    action?: ReactNode;
    icon?: ReactNode;
    subtitle?: string;
}

export function NoDataFound({ 
    message = "No data found", 
    action,
    icon,
    subtitle
}: NoDataFoundProps) {    return (
        <FadeIn>
            <Box sx={{
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'center',
                alignItems: 'center',
                height: '100%',
                color: 'text.secondary',
                p: 2,
                gap: 1
            }}>
                {icon || (
                    <Box sx={{ fontSize: 100, mb: 2, opacity: 0.2, display: 'inline-flex' }}>
                        <SearchOffOutlined size={100} color="primary" />
                    </Box>
                )}
                <Typography variant="h6" align="center" color="textSecondary" sx={{ mb: subtitle ? 1 : 2 }}>
                    {message}
                </Typography>
                {subtitle && (
                    <Typography variant="body2" align="center" color="textSecondary" sx={{ mb: 2, opacity: 0.7 }}>
                        {subtitle}
                    </Typography>
                )}
                {action && (
                    <Box sx={{ mt: 2 }}>
                        {action}
                    </Box>
                )}
            </Box>
        </FadeIn>
    );
}
