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

import React from "react";
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
import {
  ArrowRight,
  BookOpenText,
  Brain,
  Code,
  Plus,
  Server,
  Shield,
  Users,
} from "@wso2/oxygen-ui-icons-react";
import { useAuthHooks } from "@agent-management-platform/auth";
import {
  useListAgentKinds,
  useListEvaluators,
  useListLLMProviders,
  useListMCPProxies,
  useListProjects,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { returnToState } from "@agent-management-platform/views";
import { useFeatureFlag, useResourceAccess } from "@agent-management-platform/feature-management";
import { CARD_SLOTS, useRotatingSlots } from "./hooks/useRotatingSlots";
import { CARD_GRID_SX } from "./cardGrid";
import { ResourceCard, ResourceCount, type ResourceCardData } from "./subComponents/ResourceCard";
import { ProjectRow } from "./subComponents/ProjectRow";

const orgRoutes = absoluteRouteMap.children.org;
const PROJECTS_SHOWN = 3;

export const OrganizationOverview: React.FC = () => {
  const { orgId = "" } = useParams<{ orgId: string }>();
  const { userInfo } = useAuthHooks();
  const location = useLocation();
  const params = { orgName: orgId };
  const { canWrite: canWriteProject } = useResourceAccess("project");
  const { canWrite: canWriteLLM } = useResourceAccess("llm_provider");
  const { canWrite: canWriteMCP } = useResourceAccess("mcp_server");
  const agentIdEnabled = useFeatureFlag("enableAgentIdentity");

  const { data: kinds, isLoading: isLoadingKinds } = useListAgentKinds(params);
  const { data: llmProviders, isLoading: isLoadingLLM } = useListLLMProviders(params);
  const { data: mcpProxies, isLoading: isLoadingMCP } = useListMCPProxies(params);
  const { data: evaluators, isLoading: isLoadingEvaluators } = useListEvaluators(params);
  const { data: projects, isLoading: isLoadingProjects } = useListProjects(params);

  const projectList = projects?.projects ?? [];
  const identityRoutes = orgRoutes.children.thunderInstances.children;

  const resourceCards: ResourceCardData[] = [
    {
      icon: <BookOpenText size={26} />,
      title: "Agent Catalog",
      count: kinds?.total,
      loading: isLoadingKinds,
      description: "Reusable, versioned agent templates your teams can create agents from.",
      actionLabel: "Browse the catalog",
      to: generatePath(orgRoutes.children.catalog.path, { orgId }),
    },
    {
      icon: <Brain size={26} />,
      title: "LLM Service Providers",
      count: llmProviders?.total,
      loading: isLoadingLLM,
      alwaysShow: true,
      description: "Model providers your agents call, with guardrails and rate limits.",
      actionLabel: canWriteLLM ? "Add an LLM provider" : "View LLM providers",
      to: canWriteLLM
        ? generatePath(orgRoutes.children.llmProviders.children.add.path, { orgId })
        : generatePath(orgRoutes.children.llmProviders.path, { orgId }),
    },
    {
      icon: <Server size={26} />,
      title: "MCP Servers",
      // Backend sets pagination.count to the org-wide total (the spec text says page size).
      count: mcpProxies?.pagination?.count ?? mcpProxies?.count,
      loading: isLoadingMCP,
      alwaysShow: true,
      description: "Tool servers your agents can use, registered once and shared.",
      actionLabel: canWriteMCP ? "Register an MCP server" : "View MCP servers",
      to: canWriteMCP
        ? generatePath(orgRoutes.children.mcpProxies.children.add.path, { orgId })
        : generatePath(orgRoutes.children.mcpProxies.path, { orgId }),
    },
    {
      icon: <Code size={26} />,
      title: "Evaluators",
      count: evaluators?.total,
      loading: isLoadingEvaluators,
      description: "Checks that score your agents' traces, from built-in to your own.",
      actionLabel: "Browse evaluators",
      to: generatePath(orgRoutes.children.evaluators.path, { orgId }),
    },
    ...(agentIdEnabled
      ? [
          {
            icon: <Users size={26} />,
            title: "Agent Identity Groups",
            countless: true,
            description: "Groups that collect agent identities so access is granted together.",
            actionLabel: "Manage groups",
            to: generatePath(identityRoutes.groups.path, { orgId }),
          },
          {
            icon: <Shield size={26} />,
            title: "Agent Identity Roles",
            countless: true,
            description: "Roles that decide what each agent identity is allowed to do.",
            actionLabel: "Manage roles",
            to: generatePath(identityRoutes.roles.path, { orgId }),
          },
        ]
      : []),
  ];

  // Resources with nothing in them are left out. The rest fill three cards, and a card that
  // has more than one swaps between them in place. An organization with none of them yet
  // shows every resource, as the way to get started. Each card loads on its own and stays
  // until its count is known; a failed query counts as empty.
  const nonEmpty = resourceCards.filter(
    (card) => card.countless || card.alwaysShow || card.loading || (card.count ?? 0) > 0,
  );
  const shownCards = resourceCards.some((card) => (card.count ?? 0) > 0) ? nonEmpty : resourceCards;
  // Hold rotation until every count is in, so it starts once on the settled set.
  const countsLoading = resourceCards.some((card) => card.loading);
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
      Create Project
    </Button>
  );

  return (
    <PageContent fullWidth>
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
                count={isLoadingProjects ? undefined : (projects?.total ?? projectList.length)}
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
                <ProjectRow key={project.name} project={project} orgId={orgId} />
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
                    More Projects
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
