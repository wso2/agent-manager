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

import { globalConfig } from "@agent-management-platform/types";
import { Box, Button, Card, CardContent, Divider } from "@wso2/oxygen-ui";
import { Settings } from "@wso2/oxygen-ui-icons-react";
import { useParams, useSearchParams } from "react-router-dom";
import { useGetAgent, useListGateways } from "@agent-management-platform/api-client";
import { useFeatureFlag } from "@agent-management-platform/feature-management";
import { InstrumentationDrawer } from "./InstrumentationDrawer";
import { otelGatewayVhost } from "./otelGateway";
import { EnvAgentRolesGroupsSection } from "./EnvAgentRolesGroupsSection";
import { EnvAgentCardSummary } from "./EnvAgentCardSummary";
import { EnvConfigsSection } from "./EnvConfigsSection";
import { EnvMonitorsSection } from "./EnvMonitorsSection";
import { EnvObservabilitySection } from "./EnvObservabilitySection";

/** Agent ID, A2A card, configs, monitors and recent traces for an agent. */
export const AgentOverviewContent = () => {
  const { agentId = "", orgId = "", projectId = "" } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const agentIdEnabled = useFeatureFlag("enableAgentIdentity");

  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const external = agent?.provisioning?.type === "external";

  // OTEL endpoint for the Setup Agent panel: the gateway's externally-reachable
  // vhost carries the OTEL RestApi at `<vhost>/otel`; the configured URL is the
  // fallback. useConfiguredInstrumentationUrl skips the gateway lookup.
  const derivesUrlFromGateway = !globalConfig.useConfiguredInstrumentationUrl;
  const { data: gatewayList } = useListGateways({
    orgName: derivesUrlFromGateway ? orgId : "",
  });
  const gatewayVhost = derivesUrlFromGateway
    ? otelGatewayVhost(gatewayList?.gateways)
    : undefined;
  const agentInstrumentationUrl = gatewayVhost
    ? `${gatewayVhost.replace(/\/$/, "")}/otel`
    : (globalConfig.instrumentationUrl || "http://default-default.gateway.localhost:19080/otel");

  const setSetupOpen = (open: boolean) => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (open) next.set("setup", "true");
        else next.delete("setup");
        return next;
      },
      { replace: true },
    );
  };

  return (
    <>
      <Card variant="outlined">
        <CardContent>
          <Box display="flex" justifyContent="flex-end">
            <Button
              variant="text"
              size="small"
              startIcon={<Settings size={16} />}
              onClick={() => setSetupOpen(true)}
            >
              Set up agent
            </Button>
          </Box>
          <Divider sx={{ my: 1.5, mt: 0 }} />
          <EnvAgentCardSummary
            orgId={orgId}
            projectId={projectId}
            agentId={agentId}
            external={external}
          />
          {agentIdEnabled && (
            <Box sx={{ mb: 1.5 }}>
              <EnvAgentRolesGroupsSection orgId={orgId} projectId={projectId} agentId={agentId} />
            </Box>
          )}
          <EnvConfigsSection orgId={orgId} projectId={projectId} agentId={agentId} />
          <EnvMonitorsSection orgId={orgId} projectId={projectId} agentId={agentId} />
          <EnvObservabilitySection orgId={orgId} projectId={projectId} agentId={agentId} />
        </CardContent>
      </Card>
      <InstrumentationDrawer
        open={searchParams.get("setup") === "true"}
        onClose={() => setSetupOpen(false)}
        agentId={agentId}
        orgName={orgId || "default"}
        projName={projectId || "default"}
        agentName={agentId}
        instrumentationUrl={agentInstrumentationUrl}
        autoGenerate
      />
    </>
  );
};
