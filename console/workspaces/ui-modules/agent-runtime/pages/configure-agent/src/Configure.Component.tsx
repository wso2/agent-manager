/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import React, { useMemo, useState } from "react";
import { generatePath, useParams, useSearchParams } from "react-router-dom";
import { Box, Card, Divider, Tab, Tabs } from "@wso2/oxygen-ui";
import { isExternalAgent, PageLayout, servesA2A } from "@agent-management-platform/views";
import {
  useDeleteAgentMCPConfig,
  useDeleteAgentModelConfig,
  useGetAgent,
  useListAgentMCPConfigs,
  useListAgentModelConfigs,
  ConsoleAction,
  useTrack,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  AgentConfigTableSection,
  type AgentConfigTableLabels,
} from "./Configure/subComponents/AgentConfigTableSection";
import { AddMCPToolConfigPanel } from "./Configure/subComponents/AddMCPToolConfigPanel";
import { A2AConfigureTab } from "./Configure/subComponents/A2AConfigureTab";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import {
  CONFIGURE_TAB_KEYS,
  CONFIGURE_TAB_PARAM,
  type ConfigureTabKey,
} from "./configureTabs";

const configureRoutes =
  absoluteRouteMap.children.org.children.projects.children.agents.children
    .configure.children;

const llmLabels: AgentConfigTableLabels = {
  title: "LLM Configurations",
  searchPlaceholder: "Search LLM configurations",
  addButtonLabel: "Add LLM configuration",
  emptyTitle: "No LLM configurations added yet",
  emptyDescription:
    "Click Add LLM Configuration to connect a service provider.",
  errorTitle: "Failed to load LLM configurations",
  errorFallback: "Failed to load LLM configurations. Please try again.",
  searchEmptyTitle: "No LLM configurations match your search",
  searchEmptyDescription: "Try adjusting your search keywords.",
  removeTitle: "Remove LLM Configuration",
  removeTooltip: "Remove LLM configuration",
  removeConfirmation: () =>
    "This will remove the LLM configuration and its environment variable mappings from the agent. The catalog service itself will not be affected.",
  removeAriaLabel: (config) =>
    `Remove configuration ${config.name || config.uuid}`,
};

const mcpLabels: AgentConfigTableLabels = {
  title: "Tool Configurations",
  searchPlaceholder: "Search by name or description",
  addButtonLabel: "Add tool configuration",
  emptyTitle: "No tool configurations added yet",
  emptyDescription: "Add tool configurations that this agent can use.",
  errorTitle: "Failed to load tool configurations",
  errorFallback: "Failed to load tool configurations. Please try again.",
  searchEmptyTitle: "No tool configurations match your search criteria",
  searchEmptyDescription: "Try adjusting your search keywords.",
  removeTitle: "Remove Tool Configuration",
  removeTooltip: "Remove tool configuration",
  removeConfirmation: (config) =>
    `Are you sure you want to remove "${config.name}" from this agent?`,
  removeAriaLabel: (config) => `Remove ${config.name}`,
};

type TabPanelProps = {
  value: ConfigureTabKey;
  index: ConfigureTabKey;
  children: React.ReactNode;
};

function TabPanel({ value, index, children }: TabPanelProps) {
  return (
    <Box role="tabpanel" hidden={value !== index} sx={{ px: 3, py: 3 }}>
      {value === index ? children : null}
    </Box>
  );
}

export const ConfigureComponent: React.FC = () => {
  const [searchParams, setSearchParams] = useSearchParams();
  const { track } = useTrack();
  const handleTabChange = (_: React.SyntheticEvent, key: ConfigureTabKey) => {
    track(ConsoleAction.TabSwitch, {
      page: "configure-agent",
      tab: key,
    });
    // Preserve any other query params already on the URL instead of
    // replacing the whole search string with just this one.
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.set(CONFIGURE_TAB_PARAM, key);
        return next;
      },
      { replace: true },
    );
  };

  const [isAddingMcp, setIsAddingMcp] = useState(false);
  const { canWrite } = useResourceAccess("agent");
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  const {
    data: llmData,
    isLoading: isLoadingLLM,
    error: llmError,
  } = useListAgentModelConfigs(
    { orgName: orgId, projName: projectId, agentName: agentId },
    { limit: 1000, offset: 0 },
  );
  const {
    data: mcpData,
    isLoading: isLoadingMCP,
    error: mcpError,
  } = useListAgentMCPConfigs(
    { orgName: orgId, projName: projectId, agentName: agentId },
    { limit: 1000, offset: 0 },
  );
  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const isA2AAgent = servesA2A(agent);
  const isExternal = isExternalAgent(agent);
  const visibleTabKeys = CONFIGURE_TAB_KEYS.filter(
    (key) => key !== "a2a" || isA2AAgent,
  );
  const requestedTab = searchParams.get(CONFIGURE_TAB_PARAM) as ConfigureTabKey;
  const tabKey = visibleTabKeys.includes(requestedTab)
    ? requestedTab
    : visibleTabKeys[0];
  const { mutate: deleteLLMConfig, isPending: isRemovingLLM } =
    useDeleteAgentModelConfig();
  const { mutate: deleteMCPConfig, isPending: isRemovingMCP } =
    useDeleteAgentMCPConfig();

  const llmConfigs = useMemo(() => llmData?.configs ?? [], [llmData]);
  const mcpConfigs = useMemo(() => mcpData?.configs ?? [], [mcpData]);

  const hasParams = Boolean(orgId && projectId && agentId);
  const deleteParams = {
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  };

  const llmAddPath = hasParams
    ? generatePath(configureRoutes.llmProviders.children.add.path, {
        orgId,
        projectId,
        agentId,
      })
    : "#";
  const getLlmViewPath = (configId: string) =>
    hasParams
      ? generatePath(configureRoutes.llmProviders.children.view.path, {
          orgId,
          projectId,
          agentId,
          configId: encodeURIComponent(configId),
        })
      : "#";
  const getMcpViewPath = (configId: string) =>
    hasParams
      ? generatePath(configureRoutes.mcpProxies.children.view.path, {
          orgId,
          projectId,
          agentId,
          proxyId: encodeURIComponent(configId),
        })
      : "#";

  return (
    <PageLayout title="Configure Agent" disableIcon>
      <Card variant="outlined">
        <Tabs
          value={tabKey}
          onChange={handleTabChange}
          variant="scrollable"
          allowScrollButtonsMobile
        >
          <Tab value="llm" label={llmLabels.title} />
          <Tab value="tools" label={mcpLabels.title} />
          {isA2AAgent && <Tab value="a2a" label="A2A" />}
        </Tabs>
        <Divider />

        <TabPanel value={tabKey} index="llm">
          <AgentConfigTableSection
            configs={llmConfigs}
            isLoading={isLoadingLLM}
            error={llmError}
            labels={llmLabels}
            addPath={llmAddPath}
            getViewPath={getLlmViewPath}
            isRemoving={isRemovingLLM}
            showTitle={false}
            onRemove={(configId) =>
              deleteLLMConfig({
                ...deleteParams,
                configId,
              })
            }
          />
        </TabPanel>

        <TabPanel value={tabKey} index="tools">
          <AgentConfigTableSection
            configs={mcpConfigs}
            isLoading={isLoadingMCP}
            error={mcpError}
            labels={mcpLabels}
            onAdd={() => setIsAddingMcp(true)}
            getViewPath={getMcpViewPath}
            isRemoving={isRemovingMCP}
            showTitle={false}
            onRemove={(configId) =>
              deleteMCPConfig({
                ...deleteParams,
                configId,
              })
            }
          />
          {/* Single right-side drawer overlay; does not shrink the table. */}
          {canWrite && (
            <AddMCPToolConfigPanel
              open={isAddingMcp}
              orgId={orgId}
              projectId={projectId}
              agentId={agentId}
              onClose={() => setIsAddingMcp(false)}
            />
          )}
        </TabPanel>

        {isA2AAgent && orgId && projectId && agentId && (
          <TabPanel value={tabKey} index="a2a">
            <A2AConfigureTab
              orgId={orgId}
              projectId={projectId}
              agentId={agentId}
              external={isExternal}
            />
          </TabPanel>
        )}
      </Card>
    </PageLayout>
  );
};

export default ConfigureComponent;
