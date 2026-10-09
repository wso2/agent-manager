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

import { useParams } from "react-router-dom";
import { useTokenScopes } from "@agent-management-platform/api-client";
import { WebhooksSection } from "./webhooks/WebhooksSection";

/** Project-level webhooks: events about the project and its agents. */
export const ProjectWebhooks: React.FC = () => {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();
  const { enforced, scopes } = useTokenScopes();

  return (
    <WebhooksSection
      layout="page"
      title="Webhooks"
      description={
        "Receive project events, such as agents being created or deleted and the " +
        "project being updated. Agent events are set up on each agent."
      }
      target={{ scope: "project", orgName: orgId, projName: projectId }}
      canManage={!enforced || scopes.has("amp:project:update")}
    />
  );
};

export default ProjectWebhooks;
