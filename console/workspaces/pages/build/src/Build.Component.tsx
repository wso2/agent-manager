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

import React, { useCallback } from "react";
import { AgentBuild } from "./AgentBuild/AgentBuild";
import {
  FadeIn,
  DrawerWrapper,
  PageLayout,
} from "@agent-management-platform/views";
import { Button, Box } from "@wso2/oxygen-ui";
import { Wrench as BuildOutlined, Settings } from "@wso2/oxygen-ui-icons-react";
import { useParams, useSearchParams } from "react-router-dom";
import { BuildPanel } from "@agent-management-platform/shared-component";
import { useGetAgent } from "@agent-management-platform/api-client";
import { ConfigureBuildDrawer } from "./ConfigureBuildDrawer";

export const BuildComponent: React.FC = () => {
  const [searchParams, setSearchParams] = useSearchParams();

  const { orgId, projectId, agentId } = useParams();

  const isBuildPanelOpen = searchParams.get("buildPanel") === "open";
  const isConfigureBuildOpen = searchParams.get("configureBuild") === "open";

  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  const closeBuildPanel = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.delete("buildPanel");
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  const handleBuild = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.set("buildPanel", "open");
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  const closeConfigure = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.delete("configureBuild");
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  const handleConfigureBuild = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.set("configureBuild", "open");
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  return (
    <FadeIn>
      <PageLayout
        title="Build"
        disableIcon
        actions={
          <Box display="flex" gap={1}>
            {agent?.provisioning.type === 'internal' && (
              <Button
                onClick={handleConfigureBuild}
                variant="outlined"
                color="primary"
                startIcon={<Settings size={16} />}
                size="small"
              >
                Configure Build
              </Button>
            )}
            <Button
              onClick={handleBuild}
              variant="contained"
              color="primary"
              startIcon={<BuildOutlined size={16} />}
              size="small"
            >
              Trigger a Build
            </Button>
          </Box>
        }
      >
        <AgentBuild />
        <DrawerWrapper open={isBuildPanelOpen} onClose={closeBuildPanel}>
          <BuildPanel
            onClose={closeBuildPanel}
            orgName={orgId || ""}
            projName={projectId || ""}
            agentName={agentId || ""}
          />
        </DrawerWrapper>
        {agent && agent.provisioning.type === 'internal' && (
          <ConfigureBuildDrawer
            open={isConfigureBuildOpen}
            onClose={closeConfigure}
            agent={agent}
            orgId={orgId || 'default'}
            projectId={projectId || 'default'}
          />
        )}
      </PageLayout>
    </FadeIn>
  );
};

export default BuildComponent;
