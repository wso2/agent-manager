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
import { useEffect } from "react";
import { FullPageLoader } from "@agent-management-platform/views";

export function Login() {
  // eslint-disable-next-line max-len
  const {
    isAuthenticated,
    login,
    userInfo,
    isLoadingUserInfo,
    isLoadingIsAuthenticated,
  } = useAuthHooks();

  const { state } = useLocation();
  const from = state?.from?.pathname;

  useEffect(() => {
    if (!isAuthenticated && !isLoadingIsAuthenticated) {
        login();    
    }
  }, [login, isAuthenticated, isLoadingIsAuthenticated]);

  if (isAuthenticated && !isLoadingUserInfo && !isLoadingIsAuthenticated) {
    return (
      <Navigate
        to={
          from
            ? from
            : generatePath(absoluteRouteMap.children.org.path, {
                orgId: userInfo?.orgHandle ?? "default",
              })
        }
      />
    );
  }
  if (isLoadingIsAuthenticated || isLoadingUserInfo) {
    return <FullPageLoader />;
  }
  return (
    <Box
      display="flex"
      flexDirection="column"
      alignItems="center"
      justifyContent="center"
      minHeight="100vh"
    >
      <Typography variant="h4" gutterBottom>
        Welcome to Agent Management Platform
      </Typography>
      <Button variant="contained" size="large" onClick={login} sx={{ mt: 2 }}>
        Login
      </Button>
    </Box>
  );
}
