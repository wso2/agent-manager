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
import { Box, Typography } from "@wso2/oxygen-ui";
import { KeyRound } from "@wso2/oxygen-ui-icons-react";
import { StateNotice } from "@agent-management-platform/views";

interface OAuthNoticeProps {
  /** The agent's chat URL, used to make the example command concrete. */
  endpoint?: string;
}

/** Shown when OAuth is the only security mode: the console can't sign requests for the user. */
export const OAuthNotice: React.FC<OAuthNoticeProps> = ({ endpoint }) => (
  <StateNotice
    icon={KeyRound}
    title="Testing is unavailable with OAuth"
    description="This agent only accepts OAuth tokens, which the console can't issue for you. Get a token from your identity provider and call the agent from your own client."
  >
    <Box
      component="pre"
      aria-label="Example request"
      sx={{
        m: 0,
        p: 2,
        width: "100%",
        maxWidth: 560,
        overflowX: "auto",
        borderRadius: 1,
        border: "1px solid",
        borderColor: "divider",
        bgcolor: "action.hover",
        typography: "caption",
        fontFamily: "monospace",
      }}
    >
      {`curl -X POST ${endpoint ? `'${endpoint}'` : "<agent-url>/chat"} \\
  -H "Authorization: Bearer <token>" \\
  -H "Content-Type: application/json" \\
  -d '{"message": "Hello", "session_id": "test", "context": {}}'`}
    </Box>
    <Typography variant="caption" color="text.secondary">
      To test from the console, also enable API key security in the deployment settings.
    </Typography>
  </StateNotice>
);
