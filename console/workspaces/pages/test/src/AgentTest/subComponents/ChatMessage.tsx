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

import { useGetAgent } from "@agent-management-platform/api-client";
import { Box, Card, CardContent, Typography } from "@wso2/oxygen-ui";
import { useParams } from "react-router-dom";

interface ChatMessageProps {
  id: string;
  role: "user" | "assistant";
  content: string;
}

export function ChatMessage({ role, content }: ChatMessageProps) {
  const { orgId, projectId, agentId } = useParams();
  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  return (
    <Box
      display="flex"
      justifyContent={role === "user" ? "flex-end" : "flex-start"}
      width="100%"
      sx={{ mb: 0.5 }}
    >
      <Box
        display="flex"
        gap={1.5}
        maxWidth={500}
        flexDirection={role === "user" ? "row-reverse" : "row"}
        alignItems="flex-start"
      >
        <Card
          variant="outlined"
          sx={{
            borderBottomLeftRadius: role !== "user" ? 0 : 16,
            borderBottomRightRadius: role === "user" ? 0 : 16,
            "& .MuiCardContent-root": {
              minWidth: 300,
              backgroundColor:
                role === "user" ? "primary.main" : "background.paper",
            },
          }}
        >
          <CardContent>
            {role !== "user" && (
              <Typography variant="caption">{agent?.displayName}</Typography>
            )}
            {role === "user" && (
              <Typography variant="caption" color="primary.contrastText">
                You
              </Typography>
            )}
            <Typography
              variant="h6"
              color={role === "user" ? "primary.contrastText" : "text.primary"}
            >
              {content}
            </Typography>
          </CardContent>
        </Card>
      </Box>
    </Box>
  );
}
