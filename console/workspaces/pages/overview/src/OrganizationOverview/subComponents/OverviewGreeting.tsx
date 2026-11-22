/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import React from "react";
import { Box, Stack, Typography } from "@wso2/oxygen-ui";
import { useAuthHooks } from "@agent-management-platform/auth";

export interface OverviewGreetingProps {
  /** The line under the greeting. */
  subtitle: React.ReactNode;
  /** Shown at the right of the greeting, such as a Settings button. */
  actions?: React.ReactNode;
}

/** "Hello, {first name}" with a subtitle: the heading of the overview pages. */
export function OverviewGreeting({ subtitle, actions }: OverviewGreetingProps) {
  const { userInfo } = useAuthHooks();
  return (
    <Stack
      direction={{ xs: "column", sm: "row" }}
      justifyContent="space-between"
      alignItems="flex-start"
      gap={2}
    >
      <Stack spacing={1}>
        <Typography variant="h1" component="h1" sx={{ fontWeight: 600 }}>
          Hello
          {userInfo?.givenName && (
            <>
              {", "}
              <Box component="span" sx={{ color: "primary.main" }}>
                {userInfo.givenName}
              </Box>
            </>
          )}
        </Typography>
        <Typography variant="body1" color="text.secondary">
          {subtitle}
        </Typography>
      </Stack>
      {actions}
    </Stack>
  );
}
