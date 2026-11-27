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

import { useState } from "react";
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
import { OverviewPanel } from "./subComponents/OverviewPanel";
import { RunScanButton } from "./subComponents/RunScanButton";
import { ScanFindings } from "./subComponents/ScanFindings";
import { ScansTab } from "./subComponents/ScansTab";
import { useShadowAccess } from "./useShadowAccess";
import { unmanagedAgentsPath } from "./shadowUi";

// The unmanaged agents it finds are listed in the agent registry, not here.
export type ShadowTab = "overview" | "scans" | "platforms";
const TABS: ShadowTab[] = ["overview", "platforms", "scans"];

const PAGE_TITLE = "Shadow Agent Discovery";
const PAGE_DESCRIPTION =
  "Find the agents your organization runs on AI platforms outside Agent Manager: the unmanaged agents.";

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
  const setTab = (t: ShadowTab) => setSearchParams({ tab: t }, { replace: true });
  const [tabActions, setTabActions] = useState<HTMLElement | null>(null);
  const openAgents = (provider?: string) =>
    navigate(unmanagedAgentsPath(orgName ?? "", provider));
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
            <Stack direction="row" alignItems="center" spacing={2} sx={{ pr: 2 }}>
              <Tabs
                value={tab}
                onChange={(_, v: ShadowTab) => setTab(v)}
                variant="scrollable"
                allowScrollButtonsMobile
                sx={{ flex: 1, minWidth: 0 }}
              >
                <Tab value="overview" label="Overview" />
                <Tab value="platforms" label="Sources" />
                <Tab value="scans" label="Scans" />
              </Tabs>
              {/* The open tab's actions, in line with the tabs (the Scans tab fills it). */}
              <Box ref={setTabActions} sx={{ flexShrink: 0 }} />
            </Stack>
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
            {tab === "scans" && (
              <ScansTab orgName={orgName} canManage={canManage} actionsSlot={tabActions} />
            )}
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
 * Shadow Agent Discovery (org level): overview, the unmanaged agents,
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
      <Route
        path="scans/:scanId"
        element={
          <ScanFindings orgName={orgId} listPath={listPath} canManage={access.canManage} />
        }
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
