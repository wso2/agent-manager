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

import { Box, Divider, ListingTable, Skeleton, Stack, Tab, Tabs } from "@wso2/oxygen-ui";
import { Lock } from "@wso2/oxygen-ui-icons-react";
import {
  generatePath,
  Route,
  Routes,
  useLocation,
  useNavigate,
  useParams,
  useResolvedPath,
  useSearchParams,
} from "react-router-dom";
import { PageLayout, returnToState, StateNotice } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { ConnectionEditor, editConnectionPath } from "./subComponents/ConnectionEditor";
import { ConnectionsTab } from "./subComponents/ConnectionsTab";
import { FindingDetail } from "./subComponents/FindingDetail";
import { FindingsTab } from "./subComponents/FindingsTab";
import { OverviewPanel } from "./subComponents/OverviewPanel";
import { RunScanButton } from "./subComponents/RunScanButton";
import { ScanFindings } from "./subComponents/ScanFindings";
import { ScansTab } from "./subComponents/ScansTab";
import { useShadowAccess } from "./useShadowAccess";

export type ShadowTab = "overview" | "agents" | "scans" | "platforms";
const TABS: ShadowTab[] = ["overview", "agents", "scans", "platforms"];

const PAGE_TITLE = "Shadow Agent Discovery";
const PAGE_DESCRIPTION =
  "Find the agents your organization runs on AI platforms outside Agent Manager: the unregistered agents.";

function ShadowOverview({
  orgName,
  canManage,
}: {
  orgName: string | undefined;
  canManage: boolean;
}) {
  const navigate = useNavigate();
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();
  const requested = searchParams.get("tab") as ShadowTab | null;
  const tab: ShadowTab = requested && TABS.includes(requested) ? requested : "overview";
  const platform = searchParams.get("platform") ?? "";
  const setTab = (t: ShadowTab, params: Record<string, string> = {}) =>
    setSearchParams({ tab: t, ...params }, { replace: true });
  const openAgents = (provider?: string) =>
    setTab("agents", provider ? { platform: provider } : {});
  const newConnection = () =>
    navigate(
      generatePath(
        absoluteRouteMap.children.org.children.shadowAgentDiscovery.children.newConnection.path,
        {
          orgId: orgName ?? "",
        },
      ),
      { state: returnToState(location) },
    );

  return (
    <PageLayout
      title={PAGE_TITLE}
      description={PAGE_DESCRIPTION}
      disableIcon
      actions={canManage ? <RunScanButton orgName={orgName} /> : undefined}
    >
      <ListingTable.Container>
        <Stack spacing={2.5}>
          <Box>
            <Tabs
              value={tab}
              onChange={(_, v: ShadowTab) => setTab(v)}
              variant="scrollable"
              allowScrollButtonsMobile
            >
              <Tab value="overview" label="Overview" />
              <Tab value="agents" label="Unregistered Agents" />
              <Tab value="scans" label="Scans" />
              <Tab value="platforms" label="Platforms" />
            </Tabs>
            <Divider />
          </Box>
          <Box sx={{ px: 2, pb: 2 }}>
            {tab === "overview" && (
              <OverviewPanel
                orgName={orgName}
                canManage={canManage}
                onConnect={newConnection}
                onOpenScans={() => setTab("scans")}
                onOpenPlatforms={() => setTab("platforms")}
                onOpenAgents={openAgents}
              />
            )}
            {tab === "agents" && (
              <FindingsTab
                orgName={orgName}
                canManage={canManage}
                provider={platform}
                onProviderChange={(p) => setTab("agents", p ? { platform: p } : {})}
              />
            )}
            {tab === "scans" && <ScansTab orgName={orgName} canManage={canManage} />}
            {tab === "platforms" && (
              <ConnectionsTab
                orgName={orgName}
                canManage={canManage}
                onAdd={newConnection}
                onEdit={(c) =>
                  navigate(editConnectionPath(orgName ?? "", c.id), {
                    state: returnToState(location),
                  })
                }
              />
            )}
          </Box>
        </Stack>
      </ListingTable.Container>
    </PageLayout>
  );
}

/**
 * Shadow Agent Discovery (org level): overview, the unregistered agents,
 * scans (each with the agents it found) and platforms, with the scan,
 * agent and platform form pages below it. Mount it on a "/*" route.
 */
export function ShadowAgentDiscovery() {
  const { orgId } = useParams<{ orgId: string }>();
  const listPath = useResolvedPath(".").pathname;
  const access = useShadowAccess();

  if (!access.resolved) {
    return (
      <PageLayout title={PAGE_TITLE} disableIcon>
        <Skeleton variant="rounded" height={240} />
      </PageLayout>
    );
  }
  if (!access.canRead) {
    return (
      <PageLayout title={PAGE_TITLE} disableIcon>
        <StateNotice
          icon={Lock}
          title="You don't have access to Shadow Agent Discovery"
          description="It holds platform credentials and shows the agents they reach, so it is available to organization administrators."
        />
      </PageLayout>
    );
  }
  return (
    <Routes>
      <Route index element={<ShadowOverview orgName={orgId} canManage={access.canManage} />} />
      <Route path="scans/:scanId" element={<ScanFindings orgName={orgId} listPath={listPath} />} />
      <Route
        path="agents/:findingId"
        element={<FindingDetail orgName={orgId} canManage={access.canManage} listPath={listPath} />}
      />
      {access.canManage && (
        <>
          <Route
            path="platforms/new"
            element={<ConnectionEditor orgName={orgId} listPath={listPath} />}
          />
          <Route
            path="platforms/:connectionId/edit"
            element={<ConnectionEditor orgName={orgId} listPath={listPath} />}
          />
        </>
      )}
    </Routes>
  );
}

export default ShadowAgentDiscovery;
