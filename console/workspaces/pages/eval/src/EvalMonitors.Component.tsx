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
import { PageLayout } from "@agent-management-platform/views";
import { Button } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useParams } from "react-router-dom";
import { absoluteRouteMap, DEFAULT_ENVIRONMENT } from "@agent-management-platform/types";
import { useListMonitors } from "@agent-management-platform/api-client";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import { MonitorTable } from "./subComponents/MonitorTable";

export const EvalMonitorsComponent: React.FC = () => {
  const { agentId, orgId, projectId } = useParams<{
    agentId: string;
    orgId: string;
    projectId: string;
  }>();
  const { canWrite } = useResourceAccess("monitor");

  // Same query as the table (deduped); the header CTA only shows once the list has items.
  const { data: monitorsList } = useListMonitors(
    { orgName: orgId, projName: projectId, agentName: agentId },
    { environmentName: DEFAULT_ENVIRONMENT },
  );
  const hasItems = (monitorsList?.monitors?.length ?? 0) > 0;

  // One definition, used both as the page action and as the empty state's CTA.
  const createButton = (
    <Button
      variant="contained"
      component={Link}
      to={generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents.children
          .evaluation.children.monitor.children.create.path,
        { orgId, projectId, agentId },
      )}
      startIcon={<Plus />}
      color="primary"
    >
      Add monitor
    </Button>
  );

  return (
    <PageLayout
      title="Eval Monitors"
      disableIcon
      actions={hasItems && canWrite ? createButton : undefined}
    >
      <MonitorTable createAction={canWrite ? createButton : undefined} />
    </PageLayout>
  );
};

export default EvalMonitorsComponent;
