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

import { Button } from "@wso2/oxygen-ui";
import { Ban } from "@wso2/oxygen-ui-icons-react";
import { Link, generatePath } from "react-router-dom";
import { PageLayout, StateNotice } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import type { NavParams } from "../../../Layouts/OxygenLayout/navigation/navModel";

interface AgentUnsupportedNoticeProps {
  label: string;
  reason: string;
  params: NavParams;
}

/** Shown for an agent page that doesn't apply to this kind of agent. */
export function AgentUnsupportedNotice({
  label,
  reason,
  params,
}: AgentUnsupportedNoticeProps) {
  return (
    <PageLayout title={label} avatar={null}>
      <StateNotice
        icon={Ban}
        title={`${label} isn't available for this agent`}
        description={reason}
        actions={
          <Button
            variant="contained"
            component={Link}
            to={generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents.path,
              params,
            )}
          >
            Go to agent overview
          </Button>
        }
      />
    </PageLayout>
  );
}
