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

import React, { useEffect, useRef, useState } from "react";
import { Link as RouterLink, generatePath, useLocation } from "react-router-dom";
import {
  alpha,
  Box,
  Button,
  ButtonBase,
  Card,
  Link,
  PageContent,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import {
  ArrowRight,
  BookOpenText,
  Brain,
  ChartLine,
  Cloud,
  Globe,
  Plus,
  Rocket,
  Server,
} from "@wso2/oxygen-ui-icons-react";
import { useListLLMProviders, useListMCPProxies } from "@agent-management-platform/api-client";
import { useFeatureFlag, useResourceAccess } from "@agent-management-platform/feature-management";
import { absoluteRouteMap, globalConfig } from "@agent-management-platform/types";
import { returnToState, useDocumentTitle } from "@agent-management-platform/views";
import { ResourceCard, type ResourceCardData } from "../../OrganizationOverview/subComponents/ResourceCard";
import { OverviewGreeting } from "../../OrganizationOverview/subComponents/OverviewGreeting";
import { mcpServerTotal } from "../../OrganizationOverview/resourceCounts";
import { CARD_GRID_SX } from "../../OrganizationOverview/cardGrid";
import { REDUCED } from "../../OrganizationOverview/motion";
import { AgentSphere } from "./AgentSphere";
import { AgentFlowScene, type AgentHosting } from "./AgentFlowScene";

const orgRoutes = absoluteRouteMap.children.org;
const projectRoutes = orgRoutes.children.projects;

const AGENT_TYPES: { id: AgentHosting; icon: React.ReactNode; label: string }[] = [
  {
    id: "internal",
    icon: <Cloud size={18} />,
    label: "Platform-hosted",
  },
  {
    id: "external",
    icon: <Globe size={18} />,
    label: "Externally-hosted",
  },
];
const ILLUSTRATION_SIZE = 300;

/**
 * One stacked illustration. The one leaving eases back and softens while it keeps
 * animating; the one arriving settles forward, then builds itself up on its canvas.
 */
const layerSx = (shown: boolean) =>
  ({
    opacity: shown ? 1 : 0,
    transform: shown ? "none" : "scale(0.96)",
    filter: shown ? "none" : "blur(3px)",
    transition: shown
      ? "opacity 360ms ease-out, transform 480ms cubic-bezier(0.2, 0.7, 0.2, 1), filter 360ms ease-out"
      : "opacity 260ms ease-in, transform 260ms ease-in, filter 260ms ease-in",
    pointerEvents: "none",
    [REDUCED]: { transition: "none", transform: "none", filter: "none" },
  }) as const;

// Paths on the docs site (globalConfig.docsUrl), grouped as the user's next steps.
const EXPLORE = [
  {
    icon: <BookOpenText size={22} />,
    title: "Get Started",
    links: [
      { label: "What is Agent Manager?", path: "/get-started/what-is-amp/" },
      { label: "Quick start", path: "/get-started/quick-start/" },
      { label: "Create your first agent", path: "/tutorials/create-your-first-agent/" },
    ],
  },
  {
    icon: <Rocket size={22} />,
    title: "Connect and Deploy",
    links: [
      { label: "Register an LLM service provider", path: "/guides/register-llm-service-provider/" },
      { label: "Register an MCP proxy", path: "/guides/register-mcp-proxy/" },
      { label: "Configure an agent's LLM", path: "/guides/configure-agent-llm-configuration/" },
    ],
  },
  {
    icon: <ChartLine size={22} />,
    title: "Observe and Evaluate",
    links: [
      { label: "Instrument your agent", path: "/guides/amp-instrumentation/" },
      { label: "Evaluation monitors", path: "/guides/evaluation-monitors/" },
      { label: "Guardrails and policies", path: "/concepts/guardrails-and-policies/" },
    ],
  },
];

/**
 * An org-level shortcut: manage what exists (or view it, without write access), or add
 * the first one. Only a successful, empty response offers "add the first one"; while the
 * count loads, or if it failed, the card links to the list page (which reports any error).
 */
function shortcut(card: {
  icon: React.ReactNode;
  title: string;
  description: string;
  count?: number;
  loading: boolean;
  /** The count could not be loaded. */
  failed: boolean;
  canWrite: boolean;
  /** Plural noun for the "Manage …" / "View …" labels, such as "LLM providers". */
  noun: string;
  addLabel: string;
  listPath: string;
  addPath: string;
  /** Router state for the add link, so the create page can return here. */
  addState: unknown;
}): ResourceCardData {
  const { canWrite, failed, noun, addLabel, listPath, addPath, addState, ...rest } = card;
  // A failed load says so in place of the description, naming the resource.
  const shown = failed ? { ...rest, description: `Couldn't load ${noun}. Open the list to retry.` } : rest;
  return card.count || card.loading || failed || !canWrite
    ? { ...shown, actionLabel: `${canWrite ? "Manage" : "View"} ${noun}`, to: listPath }
    : { ...shown, actionLabel: addLabel, to: addPath, state: addState };
}

interface ProjectLandingProps {
  orgId: string;
  projectId: string;
  projectName?: string;
  /** Shows "Create agent or AI service"; without it the card still explains what the project is for. */
  canCreateAgent: boolean;
  /** The project Settings action, in the same place as on the overview. */
  settingsAction?: React.ReactNode;
}

/** The first thing a project with no agents shows: where to start and what is set up org-wide. */
export function ProjectLanding({
  orgId,
  projectId,
  projectName,
  canCreateAgent,
  settingsAction,
}: ProjectLandingProps) {
  const location = useLocation();
  const [preview, setPreviewNow] = useState<AgentHosting | null>(null);
  // Showing a type is immediate; going back to the globe waits a moment, so brushing
  // past a label's edge doesn't flash the globe.
  const leaveTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const setPreview = (next: AgentHosting | null) => {
    clearTimeout(leaveTimer.current);
    if (next) setPreviewNow(next);
    else leaveTimer.current = setTimeout(() => setPreviewNow(null), 160);
  };
  useEffect(() => () => clearTimeout(leaveTimer.current), []);
  const agentIdEnabled = useFeatureFlag("enableAgentIdentity");
  // No PageLayout here, so set the tab title as it would.
  useDocumentTitle(projectName ?? "Project Overview");
  const params = { orgName: orgId };
  const { canWrite: canWriteLLM } = useResourceAccess("llm_provider");
  const { canWrite: canWriteMCP } = useResourceAccess("mcp_server");
  const {
    data: llmProviders,
    isLoading: isLoadingLLM,
    isError: isLLMError,
  } = useListLLMProviders(params);
  const {
    data: mcpProxies,
    isLoading: isLoadingMCP,
    isError: isMCPError,
  } = useListMCPProxies(params);
  const addState = returnToState(location);

  const shortcuts = [
    shortcut({
      icon: <Brain size={26} />,
      title: "LLM Service Providers",
      description: "Model providers your agents call, set up once for the organization.",
      count: llmProviders?.total,
      loading: isLoadingLLM,
      failed: isLLMError && !llmProviders,
      canWrite: canWriteLLM,
      noun: "LLM providers",
      addLabel: "Add an LLM provider",
      listPath: generatePath(orgRoutes.children.llmProviders.path, { orgId }),
      addPath: generatePath(orgRoutes.children.llmProviders.children.add.path, { orgId }),
      addState,
    }),
    shortcut({
      icon: <Server size={26} />,
      title: "MCP Servers",
      description: "Tool servers your agents can use, registered once and shared.",
      count: mcpServerTotal(mcpProxies),
      loading: isLoadingMCP,
      failed: isMCPError && !mcpProxies,
      canWrite: canWriteMCP,
      noun: "MCP servers",
      addLabel: "Register an MCP server",
      listPath: generatePath(orgRoutes.children.mcpProxies.path, { orgId }),
      addPath: generatePath(orgRoutes.children.mcpProxies.children.add.path, { orgId }),
      addState,
    }),
  ];

  const docsUrl = globalConfig.docsUrl?.replace(/\/+$/, "");

  return (
    <PageContent fullWidth>
      <Stack spacing={{ xs: 3, md: 4 }}>
        <OverviewGreeting
          subtitle={
            <>
              {projectName ? (
                <>
                  You&apos;re in the{" "}
                  <Box component="strong" sx={{ color: "text.primary" }}>
                    {projectName}
                  </Box>{" "}
                  project.
                </>
              ) : (
                "You're in a new project."
              )}{" "}
              It&apos;s empty for now, so start by adding an agent.
            </>
          }
          actions={settingsAction}
        />

        <Box
          sx={{
            display: "grid",
            gap: 2,
            gridTemplateColumns: { xs: "1fr", lg: "minmax(0, 2fr) minmax(0, 1fr)" },
          }}
        >
          <Card
            variant="outlined"
            sx={{
              position: "relative",
              overflow: "hidden",
              display: "grid",
              gridTemplateColumns: { xs: "1fr", md: "minmax(0, 3fr) minmax(0, 2fr)" },
              alignItems: "center",
              gap: 2,
              p: { xs: 3, md: 5 },
              borderColor: (theme) => alpha(theme.palette.primary.main, 0.35),
              background: (theme) =>
                `linear-gradient(135deg, ${alpha(theme.palette.primary.main, 0.08)} 0%, ${alpha(
                  theme.palette.primary.main,
                  0,
                )} 55%)`,
            }}
          >
            <Stack spacing={2.5} alignItems="flex-start">
              <Typography
                variant="overline"
                sx={{ color: "primary.main", fontWeight: 600, letterSpacing: "0.12em" }}
              >
                Start here
              </Typography>
              <Typography variant="h3" component="h2" sx={{ fontWeight: 600 }}>
                Build and run AI agents
              </Typography>
              <Typography variant="body1" color="text.secondary">
                Deploy an agent on the platform or register one you host elsewhere. Connect it to
                models and tools, then trace and evaluate every run.
              </Typography>
              {/* The types wrap as one group, so neither is left alone on a line. */}
              <Stack direction="row" alignItems="center" flexWrap="wrap" useFlexGap gap={1.5}>
                <Typography variant="body2" color="text.secondary">
                  Supports
                </Typography>
                {/* Pointing at (or focusing) a type shows how it runs; leaving shows the globe. */}
                <Stack
                  direction="row"
                  alignItems="center"
                  useFlexGap
                  gap={2}
                  onMouseLeave={() => setPreview(null)}
                  onBlur={(event) => {
                    // Moving focus between the two types is not leaving the group.
                    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
                      setPreview(null);
                    }
                  }}
                >
                  {AGENT_TYPES.map((type) => (
                    <ButtonBase
                      key={type.id}
                      disableRipple
                      aria-pressed={preview === type.id}
                      onMouseEnter={() => setPreview(type.id)}
                      onFocus={() => setPreview(type.id)}
                      onClick={() => setPreview(type.id)}
                      sx={{
                        gap: 0.75,
                        color: "primary.main",
                        whiteSpace: "nowrap",
                        borderRadius: 1,
                        cursor: "default",
                        "&.Mui-focusVisible": {
                          outline: (theme) => `2px solid ${theme.palette.primary.main}`,
                          outlineOffset: 2,
                        },
                      }}
                    >
                      {type.icon}
                      <Typography variant="body2" color="inherit">
                        {type.label}
                      </Typography>
                    </ButtonBase>
                  ))}
                </Stack>
              </Stack>
              {canCreateAgent && (
                <Button
                  variant="contained"
                  startIcon={<Plus size={18} />}
                  component={RouterLink}
                  to={generatePath(projectRoutes.children.newAgent.path, { orgId, projectId })}
                  state={returnToState(location)}
                  sx={{ mt: 1 }}
                >
                  Create agent or AI service
                </Button>
              )}
            </Stack>
            <Stack
              alignItems="center"
              spacing={1.5}
              sx={{ display: { xs: "none", md: "flex" } }}
            >
              {/* Stacked and crossfaded; only the one shown animates. */}
              <Box
                sx={{
                  position: "relative",
                  width: ILLUSTRATION_SIZE,
                  maxWidth: "100%",
                  aspectRatio: "1 / 1",
                  "& > *": { position: "absolute", inset: 0 },
                }}
              >
                <Box sx={layerSx(!preview)}>
                  <AgentSphere size={ILLUSTRATION_SIZE} active={!preview} />
                </Box>
                {AGENT_TYPES.map((type) => (
                  <Box key={type.id} sx={layerSx(preview === type.id)}>
                    <AgentFlowScene
                      hosting={type.id}
                      identity={agentIdEnabled}
                      size={ILLUSTRATION_SIZE}
                      active={preview === type.id}
                    />
                  </Box>
                ))}
              </Box>
            </Stack>
          </Card>

          {/* Equal rows that stretch to the hero card's height, so both columns end together. */}
          <Box sx={{ display: "grid", gap: 2, gridAutoRows: "1fr" }}>
            {shortcuts.map((card, index) => (
              <ResourceCard key={card.title} card={card} index={index} />
            ))}
          </Box>
        </Box>

        {docsUrl && (
          <Stack spacing={2}>
            <Typography variant="h5" component="h2" sx={{ fontWeight: 600 }}>
              Explore More
            </Typography>
            <Card
              variant="outlined"
              sx={{ ...CARD_GRID_SX, gap: 3, p: { xs: 2, md: 3 } }}
            >
              {EXPLORE.map((group) => (
                <Stack key={group.title} spacing={1.5}>
                  <Stack direction="row" spacing={1.25} alignItems="center" sx={{ color: "primary.main" }}>
                    {group.icon}
                    <Typography variant="h6" component="h3" color="text.primary" sx={{ fontWeight: 600 }}>
                      {group.title}
                    </Typography>
                  </Stack>
                  {group.links.map((link) => (
                    <Link
                      key={link.path}
                      href={`${docsUrl}${link.path}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      underline="hover"
                      color="text.secondary"
                      sx={{ display: "inline-flex", alignItems: "center", gap: 1 }}
                    >
                      <ArrowRight size={16} />
                      <Typography variant="body2" color="inherit">
                        {link.label}
                      </Typography>
                    </Link>
                  ))}
                </Stack>
              ))}
            </Card>
          </Stack>
        )}
      </Stack>
    </PageContent>
  );
}
