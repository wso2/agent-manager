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

import {
  createContext,
  useContext,
  useMemo,
  type ReactNode,
} from "react";
import { useLocation } from "react-router-dom";
import {
  ClipboardCheck as EvaluationOutline,
  Settings,
  Hammer,
  FlaskConical,
  Workflow,
  Logs,
  Rocket,
  Code,
  BrainCircuit,
  Server,
  ShieldCheck,
  Bot,
  Network,
  GitBranch,
  Lock,
  LayoutDashboard,
  Box,
  BookOpenText,
  Megaphone,
  Activity,
  Gauge,
  Layers,
  MonitorCheck,
  RadioTower,
} from "@wso2/oxygen-ui-icons-react";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  useTokenScopes,
  useEventsEnabled,
} from "@agent-management-platform/api-client";
import {
  useEnvironmentFeatures,
  useFeatureFlag,
  useFeatureManagement,
  useGatewayFeatures,
  usePipelineFeatures,
} from "@agent-management-platform/feature-management";
import { thunderInstancesMetadata } from "@agent-management-platform/env-thunders/metadata";
import { useExternalNavItems } from "@agent-management-platform/views";
import { MCPLogo } from "./MCPLogo";
import {
  PLATFORM_AGENTS_ONLY,
  SOURCE_AGENTS_ONLY,
  findActiveEntry,
  type NavEntry,
  type NavLevel,
  type NavRoute,
  type NavSectionId,
} from "./navModel";

export const NAV_SECTIONS: Array<{
  id: NavSectionId;
  title: string;
  icon: ReactNode;
  /** Its pages are all agent pages, so it's shown only at agent level. */
  agentOnly?: boolean;
}> = [
  { id: "lifecycle", title: "Agent lifecycle", icon: <GitBranch />, agentOnly: true },
  { id: "security", title: "Security", icon: <Lock />, agentOnly: true },
  { id: "observability", title: "Observability", icon: <Activity /> },
  { id: "evaluation", title: "Evaluation", icon: <EvaluationOutline /> },
  { id: "identities", title: "Agent identities", icon: <thunderInstancesMetadata.icon /> },
  // Last section, so it sits just above Gateways and Events (which follow the sections).
  { id: "infrastructure", title: "Infrastructure", icon: <Server /> },
];

const AGENT_ONLY_SECTIONS = new Set<NavSectionId | undefined>(
  NAV_SECTIONS.filter((section) => section.agentOnly).map((section) => section.id),
);

/**
 * A URL-safe gate key that keeps distinct module names distinct: letters,
 * digits and "-" stay, anything else (including "_") becomes "_" plus its
 * 4-digit hex code.
 */
const pluginNavKey = (name: string) =>
  name.replace(/[^a-zA-Z0-9-]/g, (c) => `_${c.charCodeAt(0).toString(16).padStart(4, "0")}`);

const org = absoluteRouteMap.children.org;
const projects = org.children.projects;
const agents = projects.children.agents;
const agentEnv = agents.children.environment.children;

const route = (path: string, activePaths?: string[]): NavRoute => ({ path, activePaths });
/** Includes the route only when `enabled`, so RBAC and flags decide where an entry exists. */
const when = (enabled: boolean, level: NavLevel, value: NavRoute) =>
  enabled ? { [level]: value } : {};

const PLUGIN_LEVELS: Record<"org" | "project" | "component", NavLevel> = {
  org: "org",
  project: "project",
  component: "agent",
};

/** Every menu entry this user can see, in menu order. Same list at every level. */
function useBuildNavEntries(): NavEntry[] {
  const agentIdEnabled = useFeatureFlag("enableAgentIdentity");
  const profileManagementEnabled = useFeatureFlag("enableProfileManagement");
  const { canView: canViewEnvironments } = useEnvironmentFeatures();
  const { canView: canViewGateways } = useGatewayFeatures();
  const { canView: canViewPipelines } = usePipelineFeatures();
  const { hasAccess } = useFeatureManagement();
  // Events isn't a feature-management resource; it follows the token's scopes.
  const { enforced, scopes } = useTokenScopes();
  // Events needs the service's event bus; /api/v1/config says whether it runs.
  const eventsEnabled = useEventsEnabled();
  const externalNavItems = useExternalNavItems();

  const canSeeResources =
    hasAccess("llm_provider_read") ||
    hasAccess("llm_provider_template_read") ||
    hasAccess("mcp_server_read") ||
    hasAccess("llm_proxy_read");
  const canSeeSettings =
    profileManagementEnabled &&
    (hasAccess("member_read") || hasAccess("role_read") || hasAccess("group_read"));
  // The org Events page needs the events scopes; projects and agents show
  // theirs to anyone who can see them.
  const canSeeOrgEvents =
    !enforced || scopes.has("amp:events:read") || scopes.has("amp:events:manage");
  const canSeeAgentIdentities = agentIdEnabled && hasAccess("agent_identity_read");
  const canSeeEvaluators = hasAccess("evaluator_read");
  const canSeeMcpServers = hasAccess("mcp_server_read");

  // Memoized on plain booleans, so callers (the top bar's breadcrumb model, the
  // menu) only rebuild when access, flags or plugins actually change.
  return useMemo(() => {
    const pluginEntries: NavEntry[] = externalNavItems.map((item) => {
      const levels = item.levels ?? { [item.level]: item.route };
      return {
        id: `ext-${pluginNavKey(item.moduleName)}`,
        label: item.title,
        icon: item.icon,
        routes: Object.fromEntries(
          Object.entries(levels).map(([level, path]) => [
            PLUGIN_LEVELS[level as keyof typeof PLUGIN_LEVELS],
            route(path as string),
          ]),
        ),
      };
    });

    const entries: NavEntry[] = [
      {
        id: "overview",
        label: "Overview",
        icon: <LayoutDashboard size={20} />,
        routes: {
          org: route(org.path, [org.path]),
          project: route(projects.path, [projects.path, projects.children.edit.path]),
          agent: route(agents.path, [agents.path, agents.children.edit.path]),
        },
      },
      {
        id: "projects",
        label: "Projects",
        icon: <Box size={20} />,
        ownLevelOnly: true,
        routes: {
          org: route(org.children.projectListing.path, [org.children.projectListing.path]),
        },
      },
      {
        id: "agents",
        label: "Agents",
        icon: <Bot size={20} />,
        ownLevelOnly: true,
        routes: {
          project: route(projects.children.agentListing.path, [
            projects.children.agentListing.path,
          ]),
        },
      },
      ...pluginEntries,
      {
        id: "configure",
        label: "Configure",
        icon: <Settings size={20} />,
        section: "lifecycle",
        routes: { agent: route(agents.children.configure.path) },
      },
      {
        id: "build",
        label: "Build",
        icon: <Hammer size={20} />,
        section: "lifecycle",
        routes: { agent: route(agents.children.build.path) },
        ...SOURCE_AGENTS_ONLY,
      },
      {
        id: "deploy",
        label: "Deploy",
        icon: <Rocket size={20} />,
        section: "lifecycle",
        routes: { agent: route(agents.children.deployment.path) },
        ...PLATFORM_AGENTS_ONLY,
      },
      {
        id: "try-it",
        label: "Try it",
        icon: <FlaskConical size={20} />,
        section: "lifecycle",
        routes: { agent: route(agentEnv.tryOut.path) },
        ...PLATFORM_AGENTS_ONLY,
      },
      {
        id: "publish",
        label: "Publish",
        icon: <Megaphone size={20} />,
        section: "lifecycle",
        routes: { agent: route(agents.children.publish.path) },
        ...SOURCE_AGENTS_ONLY,
      },
      {
        id: "agent-id",
        label: "Agent ID",
        icon: <thunderInstancesMetadata.icon size={20} />,
        section: "security",
        routes: when(agentIdEnabled, "agent", route(agents.children.agentId.path)),
      },
      {
        id: "credentials",
        label: "Credentials",
        icon: <ShieldCheck size={20} />,
        section: "security",
        routes: { agent: route(agentEnv.security.path) },
      },
      {
        id: "traces",
        label: "Traces",
        icon: <Workflow size={20} />,
        section: "observability",
        routes: { agent: route(agentEnv.observability.children.traces.path) },
      },
      {
        id: "runtime-logs",
        label: "Runtime logs",
        icon: <Logs size={20} />,
        section: "observability",
        routes: { agent: route(agentEnv.observability.children.logs.path) },
        ...PLATFORM_AGENTS_ONLY,
      },
      {
        id: "system-metrics",
        label: "System metrics",
        icon: <Gauge size={20} />,
        section: "observability",
        routes: { agent: route(agentEnv.observability.children.metrics.path) },
        ...PLATFORM_AGENTS_ONLY,
      },
      {
        id: "monitors",
        label: "Monitors",
        icon: <MonitorCheck size={20} />,
        section: "evaluation",
        routes: { agent: route(agentEnv.evaluation.children.monitor.path) },
      },
      {
        id: "evaluators",
        label: "Evaluators",
        icon: <Code size={20} />,
        section: "evaluation",
        routes: when(canSeeEvaluators, "org", route(org.children.evaluators.path)),
      },
      {
        id: "llm-providers",
        label: "LLM service providers",
        icon: <BrainCircuit size={20} />,
        routes: when(canSeeResources, "org", route(org.children.llmProviders.path)),
      },
      {
        id: "mcp-servers",
        label: "MCP servers",
        icon: <MCPLogo size={20} />,
        routes: {
          ...when(canSeeMcpServers, "org", route(org.children.mcpProxies.path)),
          ...when(
            canSeeMcpServers,
            "project",
            route(projects.children.mcpProxies.path),
          ),
        },
      },
      {
        id: "agent-catalog",
        label: "Agent catalog",
        icon: <BookOpenText size={20} />,
        dividerBefore: true,
        routes: { org: route(org.children.catalog.path) },
      },
      {
        id: "gateways",
        label: "Gateways",
        icon: <Network size={20} />,
        // Below the sections behind a divider, grouped with Events.
        afterSections: true,
        dividerBefore: true,
        routes: when(canViewGateways, "org", route(org.children.gateways.path)),
      },
      {
        id: "environments",
        label: "Environments",
        icon: <Layers size={20} />,
        section: "infrastructure",
        routes: when(canViewEnvironments, "org", route(org.children.environments.path)),
      },
      {
        id: "deployment-pipelines",
        label: "Deployment pipelines",
        icon: <Workflow size={20} />,
        section: "infrastructure",
        routes: when(canViewPipelines, "org", route(org.children.deploymentPipelines.path)),
      },
      ...(["groups", "roles"] as const).map((key): NavEntry => {
        const ChildIcon = thunderInstancesMetadata.children[key].icon;
        return {
          id: `agent-identity-${key}`,
          label: thunderInstancesMetadata.children[key].title,
          icon: <ChildIcon size={20} />,
          section: "identities",
          routes: when(
            canSeeAgentIdentities,
            "org",
            route(org.children.thunderInstances.children[key].path),
          ),
        };
      }),
      {
        id: "events",
        label: "Events",
        icon: <RadioTower size={20} />,
        afterSections: true,
        routes: eventsEnabled
          ? {
              ...when(canSeeOrgEvents, "org", route(org.children.events.path)),
              project: route(projects.children.events.path),
              agent: route(agents.children.events.path),
            }
          : {},
      },
      {
        id: "settings",
        label: "Settings",
        icon: <Settings size={20} />,
        pinBottom: true,
        routes: when(canSeeSettings, "org", route(org.children.settings.path)),
      },
    ];

    return entries
      .filter((entry) => Object.keys(entry.routes).length > 0)
      .map((entry) =>
        AGENT_ONLY_SECTIONS.has(entry.section) ? { ...entry, ownLevelOnly: true } : entry,
      );
  }, [
    agentIdEnabled,
    canSeeAgentIdentities,
    canSeeEvaluators,
    canSeeMcpServers,
    canSeeOrgEvents,
    canSeeResources,
    canSeeSettings,
    canViewEnvironments,
    canViewGateways,
    canViewPipelines,
    eventsEnabled,
    externalNavItems,
  ]);
}

interface NavEntriesValue {
  entries: NavEntry[];
  /** The entry the current URL belongs to, if any. */
  active?: NavEntry;
}

const NavEntriesContext = createContext<NavEntriesValue | null>(null);

/**
 * Builds the menu entries and the active one once for the whole layout, so the
 * menu, the top bar, the gate and AgentGuard share them instead of each
 * subscribing to every access hook and re-matching the URL.
 */
export function NavEntriesProvider({ children }: { children: ReactNode }) {
  const entries = useBuildNavEntries();
  const { pathname } = useLocation();
  const value = useMemo(
    () => ({ entries, active: findActiveEntry(entries, pathname) }),
    [entries, pathname],
  );
  return <NavEntriesContext.Provider value={value}>{children}</NavEntriesContext.Provider>;
}

function useNavEntriesContext(): NavEntriesValue {
  const value = useContext(NavEntriesContext);
  if (!value) throw new Error("Navigation entries are only available inside NavEntriesProvider");
  return value;
}

export const useNavEntries = () => useNavEntriesContext().entries;
export const useActiveNavEntry = () => useNavEntriesContext().active;
