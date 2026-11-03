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

import React, { useEffect, useState } from "react";
import { Link as RouterLink, generatePath, useParams } from "react-router-dom";
import {
  alpha,
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
  Clock,
  Code,
  Package,
  Plus,
  Server,
  Shield,
} from "@wso2/oxygen-ui-icons-react";
import { useAuthHooks } from "@agent-management-platform/auth";
import {
  useListAgentKinds,
  useListEvaluators,
  useListLLMProviders,
  useListMCPProxies,
  useListProjects,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  isAgentIdentityEnabled,
  type ProjectResponse,
} from "@agent-management-platform/types";
import { formatRelativeTime } from "@agent-management-platform/shared-component";
import { IconTile } from "@agent-management-platform/views";

const orgRoutes = absoluteRouteMap.children.org;
const PROJECTS_SHOWN = 3;
const CARD_STAGGER_MS = 70;
const CARD_SLOTS = 3;
const SWAP_INTERVAL_MS = 5500;
const SWAP_STAGGER_MS = 1800;
const SWAP_OUT_MS = 260;
const COUNT_UP_MS = 700;
const REDUCED = "@media (prefers-reduced-motion: reduce)";

const prefersReducedMotion = () =>
  typeof window !== "undefined" &&
  window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;

/** Eases a number up from 0 once it is known, so counts settle in rather than pop in. */
function useCountUp(target?: number): number | undefined {
  const [value, setValue] = useState<number | undefined>(undefined);

  useEffect(() => {
    if (target === undefined) {
      setValue(undefined);
      return undefined;
    }
    if (prefersReducedMotion() || target === 0) {
      setValue(target);
      return undefined;
    }
    let frame = 0;
    const start = performance.now();
    const tick = (now: number) => {
      const progress = Math.min((now - start) / COUNT_UP_MS, 1);
      setValue(Math.round(target * (1 - (1 - progress) ** 3)));
      if (progress < 1) frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [target]);

  return value;
}
const CARD_GRID_SX = {
  display: "grid",
  gap: 2,
  gridTemplateColumns: { xs: "1fr", md: "repeat(3, minmax(0, 1fr))" },
} as const;

/** The number beside a section title, set off by a divider so it reads as a count. */
function ResourceCount({ count }: { count?: number }) {
  const shown = useCountUp(count);
  if (shown === undefined) return <Skeleton variant="text" width={20} />;
  return (
    <Typography variant="body1" color="text.secondary" sx={{ minWidth: "1ch" }}>
      {shown}
    </Typography>
  );
}

interface ResourceCardData {
  icon: React.ReactNode;
  title: string;
  count?: number;
  description: string;
  actionLabel: string;
  to: string;
  /** For resources with no single org-wide total (agent identity is per environment). */
  countless?: boolean;
}

interface ResourceCardProps {
  /** What this card shows over time; with more than one it swaps between them in place. */
  cards: ResourceCardData[];
  /** Position in the grid, which sets when the card's entrance and its swaps start. */
  index: number;
}

/**
 * A platform resource at a glance: what it is, how many exist, and where to add more. When
 * given several resources it keeps its frame and swaps the content inside it: the current
 * content lifts away and the next one rises in.
 */
function ResourceCard({ cards, index }: ResourceCardProps) {
  const [active, setActive] = useState(0);
  const [leaving, setLeaving] = useState(false);
  const [paused, setPaused] = useState(false);
  const current = cards[active % cards.length];

  useEffect(() => {
    if (cards.length < 2 || paused || prefersReducedMotion()) return undefined;
    // Each card starts its cycle a beat after the one before it, so they never swap together.
    const wait = SWAP_INTERVAL_MS + (active === 0 ? index * SWAP_STAGGER_MS : 0);
    let swap: ReturnType<typeof setTimeout>;
    const lift = setTimeout(() => {
      setLeaving(true);
      swap = setTimeout(() => {
        setActive((value) => (value + 1) % cards.length);
        setLeaving(false);
      }, SWAP_OUT_MS);
    }, wait);
    return () => {
      clearTimeout(lift);
      clearTimeout(swap);
      // A pause mid-swap must not leave the content lifted away.
      setLeaving(false);
    };
  }, [cards.length, paused, active, index]);

  const swapSx = {
    animation: leaving
      ? `overviewSwapOut ${SWAP_OUT_MS}ms ease-in both`
      : `overviewSwapIn 420ms cubic-bezier(0.2, 0.7, 0.2, 1) both`,
    "@keyframes overviewSwapOut": {
      to: { opacity: 0, transform: "translateY(-10px)" },
    },
    "@keyframes overviewSwapIn": {
      from: { opacity: 0, transform: "translateY(12px)" },
      to: { opacity: 1, transform: "none" },
    },
    [REDUCED]: { animation: "none" },
  } as const;

  return (
    <Card
      variant="outlined"
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
      sx={{
        display: "flex",
        flexDirection: "column",
        minHeight: 170,
        // The cards arrive in a short cascade; nothing else on the page moves.
        animation: `overviewCardIn 480ms cubic-bezier(0.2, 0.7, 0.2, 1) ${index * CARD_STAGGER_MS}ms both`,
        "@keyframes overviewCardIn": {
          from: { opacity: 0, transform: "translateY(14px)" },
          to: { opacity: 1, transform: "none" },
        },
        [REDUCED]: { animation: "none" },
      }}
    >
      <Stack direction="row" spacing={2} sx={{ p: 2.5, flex: 1, ...swapSx }}>
        <IconTile selected size={52}>
          {current.icon}
        </IconTile>
        <Box sx={{ minWidth: 0 }}>
          <Stack direction="row" spacing={1.5} alignItems="center" sx={{ mb: 0.5 }}>
            <Typography variant="h6" component="h2" sx={{ fontWeight: 600 }}>
              {current.title}
            </Typography>
            {!current.countless && (
              <>
                <Divider orientation="vertical" flexItem />
                <ResourceCount key={current.title} count={current.count} />
              </>
            )}
          </Stack>
          <Typography variant="body2" color="text.secondary">
            {current.description}
          </Typography>
        </Box>
      </Stack>
      <Divider />
      <Stack
        component={RouterLink}
        to={current.to}
        direction="row"
        justifyContent="space-between"
        alignItems="center"
        sx={{
          px: 2.5,
          py: 1.5,
          color: "primary.main",
          textDecoration: "none",
          "&:hover": { bgcolor: (theme) => alpha(theme.palette.primary.main, 0.06) },
          ...swapSx,
        }}
      >
        <Typography variant="subtitle2" color="inherit">
          {current.actionLabel}
        </Typography>
        <ArrowRight size={16} />
      </Stack>
    </Card>
  );
}

function ProjectRow({ project, orgId }: { project: ProjectResponse; orgId: string }) {
  return (
    <Stack
      component={RouterLink}
      to={generatePath(orgRoutes.children.projects.path, { orgId, projectId: project.name })}
      direction="row"
      alignItems="center"
      spacing={2}
      sx={{
        px: 2.5,
        py: 1.5,
        color: "text.primary",
        textDecoration: "none",
        "&:hover": { bgcolor: "action.hover" },
      }}
    >
      <IconTile selected size={40}>
        <Package size={20} />
      </IconTile>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography variant="subtitle2" noWrap>
          {project.displayName}
        </Typography>
        {project.description && (
          <Typography variant="caption" color="text.secondary" noWrap sx={{ display: "block" }}>
            {project.description}
          </Typography>
        )}
      </Box>
      <Stack direction="row" spacing={0.75} alignItems="center" sx={{ color: "text.secondary" }}>
        <Clock size={14} />
        <Typography variant="body2" noWrap>
          {formatRelativeTime(project.createdAt)}
        </Typography>
      </Stack>
    </Stack>
  );
}

export const OrganizationOverview: React.FC = () => {
  const { orgId = "" } = useParams<{ orgId: string }>();
  const { userInfo } = useAuthHooks();
  const params = { orgName: orgId };

  const { data: kinds } = useListAgentKinds(params);
  const { data: llmProviders } = useListLLMProviders(params);
  const { data: mcpProxies } = useListMCPProxies(params);
  const { data: evaluators } = useListEvaluators(params);
  const { data: projects, isLoading: isLoadingProjects } = useListProjects(params);

  const projectList = projects?.projects ?? [];
  const identityRoutes = orgRoutes.children.thunderInstances.children;

  const resourceCards: ResourceCardData[] = [
    {
      icon: <BookOpenText size={26} />,
      title: "Agent Catalog",
      count: kinds?.total,
      description: "Reusable, versioned agent templates your teams can create agents from.",
      actionLabel: "Browse the catalog",
      to: generatePath(orgRoutes.children.catalog.path, { orgId }),
    },
    {
      icon: <Brain size={26} />,
      title: "LLM Service Providers",
      count: llmProviders?.total,
      description: "Model providers your agents call, with guardrails and rate limits.",
      actionLabel: "Add an LLM provider",
      to: generatePath(orgRoutes.children.llmProviders.children.add.path, { orgId }),
    },
    {
      icon: <Server size={26} />,
      title: "MCP Servers",
      count: mcpProxies?.count,
      description: "Tool servers your agents can use, registered once and shared.",
      actionLabel: "Register an MCP server",
      to: generatePath(orgRoutes.children.mcpProxies.children.add.path, { orgId }),
    },
    {
      icon: <Code size={26} />,
      title: "Evaluators",
      count: evaluators?.total,
      description: "Checks that score your agents' traces, from built-in to your own.",
      actionLabel: "Browse evaluators",
      to: generatePath(orgRoutes.children.evaluators.path, { orgId }),
    },
    ...(isAgentIdentityEnabled()
      ? [
          {
            icon: <Shield size={26} />,
            title: "Agent Identity",
            countless: true,
            description: "Groups and roles that decide what each agent identity can access.",
            actionLabel: "Manage groups and roles",
            to: generatePath(identityRoutes.groups.path, { orgId }),
          },
        ]
      : []),
  ];

  // Resources with nothing in them are left out. The rest fill three cards, and a card that
  // has more than one swaps between them in place. An organization with none of them yet
  // shows every resource, as the way to get started.
  const isLoadingCounts = resourceCards.some((card) => !card.countless && card.count === undefined);
  const nonEmpty = resourceCards.filter((card) => card.countless || (card.count ?? 0) > 0);
  const shownCards = nonEmpty.some((card) => !card.countless) ? nonEmpty : resourceCards;
  const slots = Array.from({ length: Math.min(CARD_SLOTS, shownCards.length) }, (_, slot) =>
    shownCards.filter((_card, position) => position % CARD_SLOTS === slot),
  );

  const createProjectPath = generatePath(orgRoutes.children.newProject.path, { orgId });
  const createProjectButton = (
    <Button
      variant="outlined"
      size="small"
      startIcon={<Plus size={16} />}
      component={RouterLink}
      to={createProjectPath}
    >
      Create Project
    </Button>
  );

  return (
    <PageContent fullWidth>
      <Stack spacing={4}>
        <Stack spacing={1}>
          <Typography
            variant="h4"
            component="h1"
            sx={{ fontWeight: 600, fontSize: { xs: "1.75rem", md: "2.25rem" } }}
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
          {isLoadingCounts
            ? Array.from({ length: CARD_SLOTS }).map((_, index) => (
                <Skeleton key={index} variant="rounded" height={170} />
              ))
            : slots.map((cards, index) => (
                <ResourceCard key={cards[0].title} cards={cards} index={index} />
              ))}
        </Box>

        <Card variant="outlined">
          <Stack
            direction="row"
            alignItems="center"
            justifyContent="space-between"
            sx={{ px: 2.5, py: 2 }}
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
            <Stack spacing={1} sx={{ p: 2.5 }}>
              {Array.from({ length: PROJECTS_SHOWN }).map((_, index) => (
                <Skeleton key={index} variant="rounded" height={48} />
              ))}
            </Stack>
          ) : projectList.length === 0 ? (
            <Stack alignItems="center" spacing={2} sx={{ py: 6, px: 2 }}>
              <Typography variant="body2" color="text.secondary">
                Create a project to group your agents and their configuration.
              </Typography>
              {createProjectButton}
            </Stack>
          ) : (
            <>
              {projectList.slice(0, PROJECTS_SHOWN).map((project) => (
                <ProjectRow key={project.name} project={project} orgId={orgId} />
              ))}
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
            </>
          )}
        </Card>
      </Stack>
    </PageContent>
  );
};

export default OrganizationOverview;
