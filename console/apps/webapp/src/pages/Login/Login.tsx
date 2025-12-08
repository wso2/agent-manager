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

import { useAuthHooks } from "@agent-management-platform/auth";
import { generatePath, Navigate, useLocation } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { Button, Box, Typography } from "@wso2/oxygen-ui";

export function Login() {
    const { isAuthenticated, login, userInfo } = useAuthHooks();
    const { state } = useLocation();
    const from = state?.from?.pathname;
    if (isAuthenticated) {
        return <Navigate to={from ? from : generatePath(absoluteRouteMap.children.org.path, { orgId: userInfo?.orgHandle ?? '' })} />;
    }

    return (
        <Box display="flex" flexDirection="column" alignItems="center" justifyContent="center" minHeight="100vh">
            <Typography variant="h4" gutterBottom>
                Welcome to Agent Management Platform
            </Typography>
            <Button 
                variant="contained" 
                size="large" 
                onClick={login}
                sx={{ mt: 2 }}
            >
                Login
            </Button>
        </Box>
    );
}
