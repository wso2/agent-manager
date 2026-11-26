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

import { Link, Navigate, generatePath, useParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@wso2/oxygen-ui";
import { AlertTriangle, Layers } from "@wso2/oxygen-ui-icons-react";
import { useRuntimeConfig } from "@agent-management-platform/api-client";
import {
  ErrorPages,
  usePipelineEnvironmentsState,
} from "@agent-management-platform/shared-component";
import { usePipelineFeatures } from "@agent-management-platform/feature-management";
import { LoadErrorNotice, PageLayout, StateNotice } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { LoadingFallback } from "../../components/LoadingFallback";
import { useNavEntries } from "../../Layouts/OxygenLayout/navigation/useNavEntries";
import {
  currentNavLevel,
  levelHomePath,
  resolveNavTarget,
  routeNeedsEnv,
  type NavEntry,
  type NavLevel,
  type NavParams,
} from "../../Layouts/OxygenLayout/navigation/navModel";
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
  // Events only exists once the runtime config says so; feature flags and
  // access are settled before the layout renders. Wait, so a refresh on an
  // Events gate isn't mistaken for an unknown item.
  const config = useRuntimeConfig();
  const params: NavParams = { orgId, projectId, agentId };
  const level = currentNavLevel(params);
  const entry = entries.find((candidate) => candidate.id === navKey);
  const target = entry && level ? resolveNavTarget(entry, level) : undefined;

  if (!level) {
    return <ErrorPages.NotFound />;
  }
  if (!entry && config.isLoading) {
    return <LoadingFallback />;
  }
  if (!entry && config.isError) {
    // Can't tell an unknown item from one the config would have enabled.
    return (
      <PageLayout title="Page unavailable" avatar={null}>
        <StateNotice
          icon={AlertTriangle}
          tone="error"
          title="Couldn't load the console configuration"
          description="This page depends on it. Try again to reload the configuration."
          actions={
            <Button variant="contained" onClick={() => config.refetch()}>
              Try again
            </Button>
          }
        />
      </PageLayout>
    );
  }
  if (!entry || !target) {
    // An item this user can't see here (another org's permissions, a stale
    // link): fall back to the level's overview rather than a dead end.
    return <Navigate replace to={levelHomePath(level, params)} />;
  }
  if (target.kind === "up") {
    return <UpLevelNotice entry={entry} from={level} to={target.level} params={params} />;
  }
  if (target.kind === "pick") {
    // Keyed so a choice made for one item or project doesn't carry over.
    return (
      <LevelPicker
        key={`${orgId}:${entry.id}:${projectId ?? ""}`}
        entry={entry}
        from={level}
        to={target.level}
        params={params}
      />
    );
  }
  return <OpenHere entry={entry} level={level} params={params} />;
}

/**
 * The page lives here: pick the environment and go. Whether the agent supports
 * the page is AgentGuard's job, for gates and direct URLs alike.
 */
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
  const {
    environments,
    isLoading: isLoadingEnvironments,
    isError: isEnvironmentsError,
  } = usePipelineEnvironmentsState(params.orgId, params.projectId);
  const queryClient = useQueryClient();
  const { canWrite: canEditPipelines } = usePipelineFeatures();

  if (needsEnv && isLoadingEnvironments) {
    return <LoadingFallback />;
  }
  if (needsEnv && isEnvironmentsError) {
    return (
      <PageLayout title={entry.label} avatar={null}>
        <LoadErrorNotice
          entityLabel="project's environments"
          // The state exposes no refetch; refetch the queries this page uses.
          onRetry={() => queryClient.refetchQueries({ type: "active" })}
        />
      </PageLayout>
    );
  }
  const envId = environments[0]?.name;
  if (needsEnv && !envId) {
    return (
      <PageLayout title={entry.label} avatar={null}>
        <StateNotice
          icon={Layers}
          title="No environment to open this in"
          description={`${entry.label} is shown per environment. Add an environment to this project's deployment pipeline first.`}
          actions={
            canEditPipelines && (
              <Button
                variant="contained"
                component={Link}
                to={generatePath(
                  absoluteRouteMap.children.org.children.deploymentPipelines.path,
                  { orgId: params.orgId },
                )}
              >
                Go to deployment pipelines
              </Button>
            )
          }
        />
      </PageLayout>
    );
  }
  return <Navigate replace to={generatePath(route.path, { ...params, envId })} />;
}
