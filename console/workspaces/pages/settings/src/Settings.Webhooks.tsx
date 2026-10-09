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

import { Box, Stack, Typography } from "@wso2/oxygen-ui";
import { Lock } from "@wso2/oxygen-ui-icons-react";
import { useParams } from "react-router-dom";
import { useWebhooksAccess } from "./settingsRoutes";
import { WebhooksSection } from "./webhooks/WebhooksSection";

/** Organization-level webhooks: events about the org's projects and shared resources. */
export const SettingsWebhooks: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const { canRead, canManage } = useWebhooksAccess();

  if (!canRead) {
    return (
      <Stack alignItems="center" justifyContent="center" spacing={2} sx={{ py: 10, px: 4 }}>
        <Box sx={{ color: "text.secondary", display: "flex" }}>
          <Lock size={48} />
        </Box>
        <Typography variant="h6">You don&apos;t have access to webhooks</Typography>
        <Typography variant="body2" color="text.secondary" textAlign="center" maxWidth={440}>
          Organization webhooks are available to organization administrators only.
        </Typography>
      </Stack>
    );
  }

  return (
    <WebhooksSection
      layout="embedded"
      title="Webhooks"
      description={
        "Receive organization events, such as projects, MCP servers and LLM providers " +
        "being created, changed or deleted. Project and agent events are set up on the " +
        "project and the agent."
      }
      target={{ scope: "org", orgName: orgId }}
      canManage={canManage}
    />
  );
};

export default SettingsWebhooks;
