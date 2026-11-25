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
import { Navigate, generatePath, useParams } from "react-router-dom";
import { Layers } from "@wso2/oxygen-ui-icons-react";
import { useGetAgent } from "@agent-management-platform/api-client";
import {
  ErrorPages,
  usePipelineEnvironmentsState,
} from "@agent-management-platform/shared-component";
import { PageLayout, StateNotice } from "@agent-management-platform/views";
import { LoadingFallback } from "../../components/LoadingFallback";
import { useNavEntries } from "../../Layouts/OxygenLayout/navigation/useNavEntries";
import {
  AGENT_SUPPORT,
  currentNavLevel,
  entryRoutePath,
  resolveNavTarget,
  routeNeedsEnv,
  type NavEntry,
  type NavLevel,
  type NavParams,
} from "../../Layouts/OxygenLayout/navigation/navModel";
import { AgentUnsupportedNotice } from "./subComponents/AgentUnsupportedNotice";
import { LevelPicker } from "./subComponents/LevelPicker";
import { UpLevelNotice } from "./subComponents/UpLevelNotice";

/**
 * `…/go/:navKey`: where a left-menu item lands when its page doesn't live at
 * the current level. Picks the project/agent, points upwards, or explains
 * that the page doesn't apply to this agent.
 */
export function LevelGate() {
  const { orgId, projectId, agentId, navKey } = useParams();
  const entries = useNavEntries();
  const params: NavParams = { orgId, projectId, agentId };
  const level = currentNavLevel(params);
  const entry = entries.find((candidate) => candidate.id === navKey);
  const target = entry && level ? resolveNavTarget(entry, level) : undefined;

  if (!level) {
    return <ErrorPages.NotFound />;
  }
  if (!entry || !target) {
    // An item this user can't see here (another org's permissions, a stale
    // link): fall back to the level's overview rather than a dead end.
    const overview = entries.find((candidate) => candidate.id === "overview");
    const home = overview && entryRoutePath(overview, level, params);
    return home ? <Navigate replace to={home} /> : <ErrorPages.NotFound />;
  }
  if (target.kind === "up") {
    return <UpLevelNotice entry={entry} from={level} to={target.level} params={params} />;
  }
  if (target.kind === "pick") {
    return <LevelPicker entry={entry} from={level} to={target.level} params={params} />;
  }
  return <OpenHere entry={entry} level={level} params={params} />;
}

/** The page lives here: check the agent supports it, pick the environment, go. */
function OpenHere({
  entry,
  level,
  params,
}: {
  entry: NavEntry;
  level: NavLevel;
  params: NavParams;
}) {
  const route = entry.routes[level]!;
  const needsEnv = routeNeedsEnv(route);
  const { data: agent, isLoading: isLoadingAgent } = useGetAgent({
    orgName: params.orgId,
    projName: params.projectId,
    agentName: params.agentId,
  });
  const { environments, isLoading: isLoadingEnvironments } =
    usePipelineEnvironmentsState(params.orgId, params.projectId);

  if ((level === "agent" && isLoadingAgent) || (needsEnv && isLoadingEnvironments)) {
    return <LoadingFallback />;
  }
  const reason = agent && level === "agent" ? entry.unsupportedReason?.(agent) : undefined;
  if (reason) {
    return <AgentUnsupportedNotice label={entry.label} reason={reason} params={params} />;
  }
  const envId = environments[0]?.name;
  if (needsEnv && !envId) {
    return (
      <PageLayout title={entry.label} avatar={null}>
        <StateNotice
          icon={Layers}
          title="No environment to open this in"
          description={`${entry.label} is shown per environment. Add an environment to this project's deployment pipeline first.`}
        />
      </PageLayout>
    );
  }
  return <Navigate replace to={generatePath(route.path, { ...params, envId })} />;
}

/**
 * Wraps an agent page so a direct URL to a page that doesn't apply to this
 * agent (Build on an external agent, …) explains why instead of misbehaving.
 */
export function AgentPageGuard({
  navKey,
  children,
}: {
  navKey: keyof typeof AGENT_SUPPORT;
  children: ReactNode;
}) {
  const { orgId, projectId, agentId } = useParams();
  const entries = useNavEntries();
  // AgentGuard above has already loaded the agent, so this reads the cache.
  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const reason = agent ? AGENT_SUPPORT[navKey]?.(agent) : undefined;
  if (reason) {
    const label = entries.find((entry) => entry.id === navKey)?.label ?? "This page";
    return (
      <AgentUnsupportedNotice
        label={label}
        reason={reason}
        params={{ orgId, projectId, agentId }}
      />
    );
  }
  return <>{children}</>;
}
