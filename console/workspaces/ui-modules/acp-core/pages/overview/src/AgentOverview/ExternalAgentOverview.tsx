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

import { globalConfig } from '@agent-management-platform/types';
import { Box, Button, Skeleton } from "@wso2/oxygen-ui";
import { Settings } from "@wso2/oxygen-ui-icons-react";
import { useParams, useSearchParams } from "react-router-dom";
import {
  useGetAgent,
  useListGateways,
} from "@agent-management-platform/api-client";
import {
  DeploymentStatus,
  EnvironmentCard,
  usePipelineEnvironmentsState,
} from "@agent-management-platform/shared-component";
import { InstrumentationDrawer } from "./InstrumentationDrawer";
import { otelGatewayVhost } from "./otelGateway";
import { NoDataFound } from "@agent-management-platform/views";
import { EnvironmentSectionsContent } from "./EnvironmentSectionsContent";
import { EnvironmentSingleHeader } from "./EnvironmentSingleHeader";
import { EnvironmentTabsBar } from "./EnvironmentTabsBar";
import { UppercaseCaptionLabel } from "./SectionHeader";
import { ENV_SEARCH_PARAM, useSelectedEnvironmentParam } from "./useSelectedEnvironmentParam";

export const ExternalAgentOverview = () => {
  const { agentId, orgId, projectId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();

  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  // Show only the environments in the current project's deployment pipeline,
  // ordered by the promotion chain. isLoading covers environments + project + pipelines.
  const { environments: sortedEnvironmentList, isLoading: isEnvironmentsLoading } =
    usePipelineEnvironmentsState(orgId, projectId);
  const { selectedEnvironment, selectEnvironment } =
    useSelectedEnvironmentParam(sortedEnvironmentList);
  const selectedEnvironmentId = selectedEnvironment?.id ?? "";

  // OTEL endpoint for the Setup Agent panel. By default it is derived per
  // environment: the gateway mapped to the selected environment carries the
  // externally-reachable vhost, and the OTEL RestApi is published at
  // `<vhost>/otel`; the configured URL is the fallback until that lookup
  // resolves. A deployment that sets useConfiguredInstrumentationUrl takes the
  // configured URL as-is, so the gateway lookup is skipped entirely.
  // useListGateways has no `enabled` option, so orgName is withheld until the
  // lookup is actually needed to avoid firing a throwaway request.
  const derivesUrlFromGateway = !globalConfig.useConfiguredInstrumentationUrl;
  const { data: envGatewayList } = useListGateways(
    { orgName: derivesUrlFromGateway && selectedEnvironmentId ? (orgId ?? "") : "" },
    { environment: selectedEnvironmentId },
  );
  const envGatewayVhost = derivesUrlFromGateway
    ? otelGatewayVhost(envGatewayList?.gateways)
    : undefined;
  const agentInstrumentationUrl = envGatewayVhost
    ? `${envGatewayVhost.replace(/\/$/, "")}/otel`
    : (globalConfig.instrumentationUrl || "http://default-default.gateway.localhost:19080/otel");

  // Sets both the selected environment and setup=true in one functional
  // update — two separate setSearchParams calls in the same handler would
  // each independently compute `next` from `prev`, so the second call risks
  // clobbering the first's change instead of building on it.
  const handleSetupAgent = (environmentName: string) => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.set(ENV_SEARCH_PARAM, environmentName);
        next.set("setup", "true");
        return next;
      },
      { replace: true },
    );
  };

  const closeSetupDrawer = () => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.delete("setup");
        return next;
      },
      { replace: true },
    );
  };

  // Once loaded, a single environment doesn't need a "which environment" tab
  // strip or section label — only shown when there's more than one, or while
  // still loading (before we know how many there are).
  const showEnvironmentsHeader = isEnvironmentsLoading || sortedEnvironmentList.length !== 1;
  const hasMultipleEnvironments = sortedEnvironmentList.length > 1;

  return (
    <>
      <Box display="flex" flexDirection="column" gap={2}>
        {showEnvironmentsHeader && <UppercaseCaptionLabel>Environments</UppercaseCaptionLabel>}
        {isEnvironmentsLoading ? (
          <Box display="flex" flexDirection="column" gap={2}>
            <Skeleton variant="rounded" height={100} />
            <Skeleton variant="rounded" height={100} />
          </Box>
        ) : sortedEnvironmentList.length === 0 ? (
          <NoDataFound
            message="No environments found"
            subtitle="Environments will appear here once they are created"
          />
        ) : (
          selectedEnvironment &&
          orgId &&
          projectId &&
          agentId && (
            <EnvironmentCard
              key={selectedEnvironment.name}
              orgId={orgId}
              projectId={projectId}
              agentId={agentId}
              environment={selectedEnvironment}
              tabsHeader={
                hasMultipleEnvironments ? (
                  <EnvironmentTabsBar
                    environments={sortedEnvironmentList}
                    selectedName={selectedEnvironment.name}
                    onSelect={selectEnvironment}
                    dotColor={() => "success.main"}
                  />
                ) : (
                  <EnvironmentSingleHeader
                    environment={selectedEnvironment}
                    status={DeploymentStatus.ACTIVE}
                    dotColor="success.main"
                  />
                )
              }
              actions={
                <Button
                  variant="text"
                  size="small"
                  startIcon={<Settings size={16} />}
                  onClick={() => handleSetupAgent(selectedEnvironment.name)}
                >
                  Set up agent
                </Button>
              }
              bottomContent={
                <EnvironmentSectionsContent
                  orgId={orgId}
                  projectId={projectId}
                  agentId={agentId}
                  envId={selectedEnvironment.name}
                  external
                />
              }
            />
          )
        )}
      </Box>
      <InstrumentationDrawer
        open={searchParams.get("setup") === "true" && selectedEnvironmentId !== ""}
        onClose={closeSetupDrawer}
        agentId={agentId ?? ""}
        orgName={orgId ?? "default"}
        projName={projectId ?? "default"}
        agentName={agentId ?? ""}
        environment={selectedEnvironment?.name}
        instrumentationUrl={agentInstrumentationUrl}
        componentUid={agent?.uuid}
        environmentUid={selectedEnvironmentId}
        autoGenerate
      />
    </>
  );
};
