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

import type { ReactNode } from "react";
import { Skeleton } from "@wso2/oxygen-ui";
import { Lock } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useParams } from "react-router-dom";
import { PageLayout, StateNotice } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useShadowAccess } from "@agent-management-platform/shadow-agent-discovery";
import { FindingDetail } from "./FindingDetail";
import { OnboardFinding } from "./OnboardFinding";

/** The registry's Unregistered tab, where an unregistered agent's Back goes on a direct visit. */
function unregisteredListPath(orgId: string): string {
  const registryPath = generatePath(absoluteRouteMap.children.org.children.agentRegistry.path, {
    orgId,
  });
  return `${registryPath}?registration=unregistered`;
}

/**
 * Shows the page only to callers who may read Shadow Agent Discovery's findings (organization
 * admins, by default); the service enforces the same.
 */
function ShadowAccessGate({ manage, children }: { manage?: boolean; children: ReactNode }) {
  const access = useShadowAccess();
  if (!access.resolved) {
    return (
      <PageLayout title="Agent" disableIcon>
        <Skeleton variant="rounded" height={240} />
      </PageLayout>
    );
  }
  if (!(manage ? access.canManage : access.canRead)) {
    return (
      <PageLayout title="Agent" disableIcon>
        <StateNotice
          icon={Lock}
          title="You don't have access to this agent"
          description="Unregistered agents are found by Shadow Agent Discovery, which is available to organization administrators."
        />
      </PageLayout>
    );
  }
  return children;
}

/** An unregistered agent's page: what its platform reports, the evidence, and onboarding. */
export function UnregisteredAgentPage() {
  const { orgId = "" } = useParams<{ orgId: string }>();
  const access = useShadowAccess();
  return (
    <ShadowAccessGate>
      <FindingDetail
        orgName={orgId}
        canManage={access.canManage}
        listPath={unregisteredListPath(orgId)}
      />
    </ShadowAccessGate>
  );
}

/** Onboarding an unregistered agent as an external agent. */
export function UnregisteredAgentOnboardPage() {
  const { orgId = "" } = useParams<{ orgId: string }>();
  return (
    <ShadowAccessGate manage>
      <OnboardFinding orgName={orgId} listPath={unregisteredListPath(orgId)} />
    </ShadowAccessGate>
  );
}
