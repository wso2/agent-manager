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
import { PageLayout } from "@agent-management-platform/views";
import {
  useDeleteAgentMCPConfig,
  useDeleteAgentModelConfig,
  useGetAgent,
  useGetAgentBuilds,
  useListAgentMCPConfigs,
  useListAgentModelConfigs,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type BuildResponse,
} from "@agent-management-platform/types";
import {
  AgentConfigTableSection,
  type AgentConfigTableLabels,
} from "./Configure/subComponents/AgentConfigTableSection";
import { AddMCPToolConfigPanel } from "./Configure/subComponents/AddMCPToolConfigPanel";
import { CONFIGURE_TAB_KEYS, CONFIGURE_TAB_PARAM } from "./configureTabs";

const configureRoutes =
  absoluteRouteMap.children.org.children.projects.children.agents.children
    .configure.children;

const llmLabels: AgentConfigTableLabels = {
  title: "LLM Configurations",
  searchPlaceholder: "Search LLM configurations...",
  addButtonLabel: "Add LLM Configuration",
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
  searchPlaceholder: "Search by name or description...",
  addButtonLabel: "Add Tool Configuration",
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

// A build only yields a runnable image in these two states; the API reports
// success under both names.
const isBuildComplete = (build: BuildResponse) =>
  build.status === "Completed" || build.status === "Succeeded";

const NO_COMPLETED_BUILD_REASON =
  "Complete a build for this agent before adding configurations.";

type TabPanelProps = {
  value: number;
  index: number;
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
  const requestedTabIndex = CONFIGURE_TAB_KEYS.indexOf(
    searchParams.get(
      CONFIGURE_TAB_PARAM,
    ) as (typeof CONFIGURE_TAB_KEYS)[number],
  );
  const tabIndex = requestedTabIndex === -1 ? 0 : requestedTabIndex;
  const handleTabChange = (_: React.SyntheticEvent, index: number) => {
    // Preserve any other query params already on the URL instead of
    // replacing the whole search string with just this one.
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.set(CONFIGURE_TAB_PARAM, CONFIGURE_TAB_KEYS[index]);
        return next;
      },
      { replace: true },
    );
  };

  const [isAddingMcp, setIsAddingMcp] = useState(false);
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
  // Adding a configuration is pointless until the agent has an image to run it
  // against, so the Add buttons stay closed until at least one build has
  // completed — an agent whose builds have all failed is blocked for the same
  // reason as one whose only build is still running. Later builds never
  // re-block: an earlier completed build has already produced an image.
  //
  // The gate only applies to agents that build their own image. An external
  // agent runs somewhere else entirely and a kind-sourced one takes its image
  // from the published kind, so neither has a build of its own to wait for —
  // gating them would strand their configuration behind a build they cannot
  // start.
  //
  // The gate fails open, deliberately. It is a hint, not an authorization check:
  // nothing server-side refuses a configuration for an agent with no build, and
  // a configuration added early is a deletable row, not damage. So while either
  // query is in flight — or if the agent lookup fails outright — the buttons
  // stay enabled rather than telling every visitor to "complete a build" for an
  // agent that is exempt, already built, or merely slow to load.
  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const buildsOwnImage =
    !!agent && agent.provisioning?.type !== "external" && !agent.kindName;

  // Exempt agents have no builds of their own, so the list is never fetched for
  // them — the gate is already open and the request would only be an empty
  // answer to a question that does not apply.
  const { data: buildsData, isLoading: isLoadingBuilds } = useGetAgentBuilds(
    {
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
    },
    undefined,
    { enabled: buildsOwnImage },
  );
  const builds = useMemo(() => buildsData?.builds ?? [], [buildsData]);
  const hasNoCompletedBuild =
    buildsOwnImage && !isLoadingBuilds && !builds.some(isBuildComplete);

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
          value={tabIndex}
          onChange={handleTabChange}
          variant="scrollable"
          allowScrollButtonsMobile
        >
          <Tab label={llmLabels.title} />
          <Tab label={mcpLabels.title} />
        </Tabs>
        <Divider />

        <TabPanel value={tabIndex} index={0}>
          <AgentConfigTableSection
            configs={llmConfigs}
            isLoading={isLoadingLLM}
            error={llmError}
            labels={llmLabels}
            addPath={llmAddPath}
            getViewPath={getLlmViewPath}
            isRemoving={isRemovingLLM}
            showTitle={false}
            addDisabled={hasNoCompletedBuild}
            addDisabledReason={NO_COMPLETED_BUILD_REASON}
            onRemove={(configId) =>
              deleteLLMConfig({
                ...deleteParams,
                configId,
              })
            }
          />
        </TabPanel>

        <TabPanel value={tabIndex} index={1}>
          <AgentConfigTableSection
            configs={mcpConfigs}
            isLoading={isLoadingMCP}
            error={mcpError}
            labels={mcpLabels}
            onAdd={() => setIsAddingMcp(true)}
            getViewPath={getMcpViewPath}
            isRemoving={isRemovingMCP}
            showTitle={false}
            addDisabled={hasNoCompletedBuild}
            addDisabledReason={NO_COMPLETED_BUILD_REASON}
            onRemove={(configId) =>
              deleteMCPConfig({
                ...deleteParams,
                configId,
              })
            }
          />
          {/* Single right-side drawer overlay; does not shrink the table. */}
          <AddMCPToolConfigPanel
            open={isAddingMcp}
            orgId={orgId}
            projectId={projectId}
            agentId={agentId}
            onClose={() => setIsAddingMcp(false)}
          />
        </TabPanel>
      </Card>
    </PageLayout>
  );
};

export default ConfigureComponent;
