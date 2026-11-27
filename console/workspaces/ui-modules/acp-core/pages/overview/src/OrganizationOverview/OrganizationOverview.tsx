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

import React, { useCallback, useState } from "react";
import { Link as RouterLink, generatePath, useLocation, useParams } from "react-router-dom";
import {
  Box,
  Button,
  Card,
  Divider,
  PageContent,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { ArrowRight, Plus } from "@wso2/oxygen-ui-icons-react";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useListProjects } from "@agent-management-platform/api-client";
import { absoluteRouteMap, type ResourceSummaryState } from "@agent-management-platform/types";
import { returnToState, usePageFooter, useResourceSummaries } from "@agent-management-platform/views";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import { CARD_SLOTS, useRotatingSlots } from "./hooks/useRotatingSlots";
import { CARD_GRID_SX } from "./cardGrid";
import { ResourceCard, ResourceCount, type ResourceCardData } from "./subComponents/ResourceCard";
import { ProjectRow } from "./subComponents/ProjectRow";
import { ResourceSummaryBoundary, ResourceSummaryProbe } from "./ResourceSummaryProbe";

const orgRoutes = absoluteRouteMap.children.org;
const PROJECTS_SHOWN = 3;

export const OrganizationOverview: React.FC = () => {
  const { orgId = "" } = useParams<{ orgId: string }>();
  const footer = usePageFooter();
  const { userInfo } = useAuthHooks();
  const location = useLocation();
  const params = { orgName: orgId };
  const { canWrite: canWriteProject } = useResourceAccess("project");
  // Resource cards come from the modules that own the resources (resource-summary
  // contributions); each one's count comes from its own hook, run by a probe below.
  const summaries = useResourceSummaries();
  const [summaryStates, setSummaryStates] = useState<Record<string, ResourceSummaryState>>({});
  const reportSummary = useCallback((key: string, state: ResourceSummaryState) => {
    setSummaryStates((prev) => ({ ...prev, [key]: state }));
  }, []);
  const [failedSummaries, setFailedSummaries] = useState<ReadonlySet<string>>(new Set());
  const failSummary = useCallback((key: string) => {
    setFailedSummaries((prev) => new Set(prev).add(key));
  }, []);

  const { data: projects, isLoading: isLoadingProjects } = useListProjects(params);

  const projectList = projects?.list ?? [];
  const projectTotal = projects?.pagination?.total ?? projectList.length;

  // A card joins once its hook has reported; until then it holds rotation like
  // a loading count.
  const reportedSummaries = summaries.filter(
    (summary) => summaryStates[summary.key] && !failedSummaries.has(summary.key),
  );
  const summariesLoading = summaries.some(
    (summary) => !summaryStates[summary.key] && !failedSummaries.has(summary.key),
  );
  const resourceCards: ResourceCardData[] = reportedSummaries.map((summary) => {
    const state = summaryStates[summary.key];
    const Icon = summary.icon;
    return {
      icon: <Icon size={26} />,
      title: summary.title,
      description: summary.description,
      alwaysShow: summary.alwaysShow,
      count: state.count,
      countless: state.countless,
      loading: state.isLoading,
      actionLabel: state.actionLabel,
      to: state.to,
    };
  });

  // Resources with nothing in them are left out. The rest fill three cards, and a card that
  // has more than one swaps between them in place. An organization with none of them yet
  // shows every resource, as the way to get started. Each card loads on its own and stays
  // until its count is known; a failed query counts as empty.
  const nonEmpty = resourceCards.filter(
    (card) => card.countless || card.alwaysShow || card.loading || (card.count ?? 0) > 0,
  );
  const shownCards = resourceCards.some((card) => (card.count ?? 0) > 0) ? nonEmpty : resourceCards;
  // Hold rotation until every count is in, so it starts once on the settled set.
  const countsLoading = summariesLoading || resourceCards.some((card) => card.loading);
  const { order, leavingSlot, setPaused } = useRotatingSlots(
    countsLoading ? Math.min(CARD_SLOTS, shownCards.length) : shownCards.length,
  );

  const createProjectPath = generatePath(orgRoutes.children.newProject.path, { orgId });
  const createProjectButton = canWriteProject && (
    <Button
      variant="outlined"
      size="small"
      startIcon={<Plus size={16} />}
      component={RouterLink}
      to={createProjectPath}
      state={returnToState(location)}
    >
      Create project
    </Button>
  );

  return (
    <PageContent fullWidth footer={footer}>
      {summaries.map((summary) => (
        <ResourceSummaryBoundary key={summary.key} summaryKey={summary.key} onFail={failSummary}>
          <ResourceSummaryProbe
            summaryKey={summary.key}
            useSummary={summary.useSummary}
            onReport={reportSummary}
          />
        </ResourceSummaryBoundary>
      ))}
      <Stack spacing={{ xs: 3, md: 4 }}>
        <Stack spacing={1}>
          <Typography
            variant="h1"
            component="h1"
            sx={{ fontWeight: 600 }}
          >
            Hello
            {userInfo?.givenName && (
              <>
                {", "}
                <Box component="span" sx={{ color: "primary.main" }}>
                  {userInfo.givenName}
                </Box>
              </>
            )}
          </Typography>
          <Typography variant="body1" color="text.secondary">
            Deploy AI agents, connect them to models and tools, then observe them on one
            platform.
          </Typography>
        </Stack>

        <Box sx={CARD_GRID_SX}>
          {order.map((cardIndex, index) => (
            <ResourceCard
              key={index}
              card={shownCards[cardIndex]}
              index={index}
              leaving={leavingSlot === index}
              onPause={setPaused}
            />
          ))}
        </Box>

        <Card variant="outlined" sx={{ display: "flex", flexDirection: "column", minHeight: { xs: 0, md: 360 } }}>
          <Stack
            direction="row"
            alignItems="center"
            justifyContent="space-between"
            sx={{ px: { xs: 2, md: 2.5 }, py: 2, gap: 1 }}
          >
            <Stack direction="row" spacing={1.5} alignItems="center">
              <Typography variant="h5" component="h2" sx={{ fontWeight: 600 }}>
                Projects
              </Typography>
              <Divider orientation="vertical" flexItem />
              <ResourceCount
                count={isLoadingProjects ? undefined : projectTotal}
              />
            </Stack>
            {projectList.length > 0 && createProjectButton}
          </Stack>
          <Divider />
          {isLoadingProjects ? (
            <Stack spacing={1} sx={{ p: { xs: 2, md: 2.5 } }}>
              {Array.from({ length: PROJECTS_SHOWN }).map((_, index) => (
                <Skeleton key={index} variant="rounded" height={48} />
              ))}
            </Stack>
          ) : projectList.length === 0 ? (
            <Stack alignItems="center" justifyContent="center" spacing={2} sx={{ py: 6, px: 2, flex: 1 }}>
              <Typography variant="body2" color="text.secondary">
                {canWriteProject
                  ? "Create a project to group your agents and their configuration."
                  : "No projects yet."}
              </Typography>
              {createProjectButton}
            </Stack>
          ) : (
            <>
              {projectList.slice(0, PROJECTS_SHOWN).map((project) => (
                <ProjectRow key={project.metadata.name} project={project} orgId={orgId} />
              ))}
              {/* Pinned to the card's bottom edge however few projects there are. */}
              <Box sx={{ mt: "auto" }}>
                <Divider />
                <Stack
                  component={RouterLink}
                  to={generatePath(orgRoutes.children.projectListing.path, { orgId })}
                  direction="row"
                  spacing={1}
                  alignItems="center"
                  justifyContent="flex-end"
                  sx={{ px: 2.5, py: 1.5, color: "primary.main", textDecoration: "none" }}
                >
                  <Typography variant="subtitle2" color="inherit">
                    More projects
                  </Typography>
                  <ArrowRight size={16} />
                </Stack>
              </Box>
            </>
          )}
        </Card>
      </Stack>
    </PageContent>
  );
};

export default OrganizationOverview;
