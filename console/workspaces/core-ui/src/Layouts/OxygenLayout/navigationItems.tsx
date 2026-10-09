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

import { useMemo } from "react";
import {
  ClipboardCheck as EvaluationOutline,
  Settings,
  Home,
  Hammer,
  FlaskConical,
  Workflow,
  Logs,
  Rocket,
  Code,
  BrainCircuit,
  Server,
  ShieldCheck,
  Boxes,
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
} from "@wso2/oxygen-ui-icons-react";
import {
  generatePath,
  matchPath,
  useLocation,
  useParams,
} from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  useGetAgent,
  useTokenScopes,
} from "@agent-management-platform/api-client";
import { usePipelineEnvironmentsState } from "@agent-management-platform/shared-component";
import { thunderInstancesMetadata } from "@agent-management-platform/env-thunders/metadata";
import { useExternalNavItems } from "@agent-management-platform/views";
import type { NavigationItem, NavigationSection } from "./LeftNavigation";

// MCP logo inlined here so mcp-proxies package stays in its own async chunk.
const MCP_LOGO_MASK =
  "data:image/png;base64,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";

const MCPLogo = ({
  size = 20,
  className,
}: {
  size?: number | string;
  className?: string;
}) => (
  <span
    aria-hidden="true"
    className={className}
    style={{
      backgroundColor: "currentColor",
      color: "inherit",
      display: "inline-block",
      height: size,
      maskImage: `url(${MCP_LOGO_MASK})`,
      maskPosition: "center",
      maskRepeat: "no-repeat",
      maskSize: "contain",
      WebkitMaskImage: `url(${MCP_LOGO_MASK})`,
      WebkitMaskPosition: "center",
      WebkitMaskRepeat: "no-repeat",
      WebkitMaskSize: "contain",
      verticalAlign: "middle",
      width: size,
    }}
  />
);

/**
 * TODO: Use nav bar instead of navigate to the items.
 */

export function useNavigationItems(): Array<
  NavigationSection | NavigationItem
> {
  const { orgId, projectId, agentId, envId } = useParams();
  const { data: agent, isLoading: isLoadingAgent } = useGetAgent({
    agentName: agentId,
    orgName: orgId,
    projName: projectId,
  });
  const { environments, isLoading: isLoadingEnvironments } =
    usePipelineEnvironmentsState(orgId, projectId);

  const externalNavItems = useExternalNavItems();
  const { enforced, resolved: scopesResolved, scopes } = useTokenScopes();

  const navVisibility = useMemo(() => {
    const showAll = {
      resources: true,
      evaluation: true,
      infrastructure: true,
      identityUsers: true,
      identityRoles: true,
      identityGroups: true,
      agentIdentities: true,
    };
    if (!enforced) return showAll;
    // An unresolved token is handled below by rendering no navigation at all,
    // not here: this map only ever describes a scope set that has been read. A
    // read that found nothing is one of those — every section then resolves to
    // false on its own, which is the right answer and needs no special case.
    const s = scopes;
    return {
      resources:
        s.has("amp:llm-provider:read") ||
        s.has("amp:llm-provider-template:read") ||
        s.has("amp:mcp-server:read") ||
        s.has("amp:llm-proxy:read"),
      evaluation: s.has("amp:evaluator:read"),
      infrastructure:
        s.has("amp:gateway:read") ||
        s.has("amp:deployment-pipeline:read") ||
        s.has("amp:environment:read"),
      identityUsers:
        s.has("amp:org:invite-member") || s.has("amp:org:remove-member"),
      identityRoles:
        s.has("amp:role:read") ||
        s.has("amp:role:create") ||
        s.has("amp:role:update") ||
        s.has("amp:role:delete"),
      identityGroups:
        s.has("amp:group:read") ||
        s.has("amp:group:create") ||
        s.has("amp:group:update") ||
        s.has("amp:group:delete"),
      // The Agent Identities section lists the roles and groups an AGENT can
      // hold, not the organization's own. identityRoles/identityGroups above are
      // the org scopes and gate the Settings tabs; these pages read the
      // agent-identity API and belong to its scope.
      agentIdentities: s.has("amp:agent-identity:read"),
    };
  }, [enforced, scopes]);

  const defaultEnv =
    envId ?? (environments.length > 0 ? environments[0]?.name : "");
  const { pathname } = useLocation();

  const llmProvidersOrgRoute = (
    absoluteRouteMap.children.org.children as unknown as Record<
      string,
      { path: string; wildPath: string }
    >
  ).llmProviders;
  const mcpProxiesOrgRoute = (
    absoluteRouteMap.children.org.children as unknown as Record<
      string,
      { path: string; wildPath: string }
    >
  ).mcpProxies;
  const agentsChildren = absoluteRouteMap.children.org.children.projects
    .children.agents.children as Record<
    string,
    { path: string; wildPath: string }
  >;
  const gatewaysOrgRoute = (
    absoluteRouteMap.children.org.children as unknown as Record<
      string,
      { path: string; wildPath: string }
    >
  ).gateways;
  const settingsOrgRoute = (
    absoluteRouteMap.children.org.children as unknown as Record<
      string,
      { path: string; wildPath: string }
    >
  ).settings;
  const deploymentPipelinesOrgRoute = (
    absoluteRouteMap.children.org.children as unknown as Record<
      string,
      { path: string; wildPath: string }
    >
  ).deploymentPipelines;
  const environmentsOrgRoute = (
    absoluteRouteMap.children.org.children as unknown as Record<
      string,
      { path: string; wildPath: string }
    >
  ).environments;
  const evaluatorsOrgRoute = absoluteRouteMap.children.org.children.evaluators;

  // An unread scope set is not an empty one. Rendering the navigation before the
  // access token has been decoded would show the sections a caller with no
  // permissions sees, then rearrange the sidebar under them once the scopes
  // arrive — so it waits, the same way it waits for the agent and the
  // environment list.
  if (
    isLoadingAgent ||
    (isLoadingEnvironments && agentId) ||
    (enforced && !scopesResolved)
  ) {
    return [];
  }

  if (
    agent?.provisioning.type === "external" &&
    agentId &&
    projectId &&
    orgId
  ) {
    return [
      {
        label: "Overview",
        type: "item",
        icon: <LayoutDashboard size={20} />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          pathname,
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          { orgId, projectId, agentId },
        ),
      },
      ...externalNavItems
        .filter((item) => item.level === "component")
        .map((item) => ({
          label: item.title,
          type: "item" as const,
          icon: item.icon,
          isActive: !!matchPath(item.route, pathname),
          href: generatePath(item.route, { orgId, projectId, agentId }),
        })),
      {
        title: "Agent Lifecycle",
        type: "section",
        icon: <GitBranch />,
        collapsible: true,
        items: [
          {
            label: "Configure",
            type: "item",
            icon: <Settings size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.configure.wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.configure.path,
              { orgId, projectId, agentId },
            ),
          },
        ],
      },
      {
        title: "Security",
        type: "section",
        icon: <Lock />,
        collapsible: true,
        items: [
          {
            label: "Agent ID",
            type: "item",
            icon: <thunderInstancesMetadata.icon size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.agentId.wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.agentId.path,
              { orgId, projectId, agentId },
            ),
          },
        ],
      },
      {
        title: "Observability",
        type: "section",
        icon: <Activity />,
        collapsible: true,
        items: [
          {
            label: "Traces",
            type: "item",
            icon: <Workflow size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
        ],
      },
      {
        title: "Evaluation",
        type: "section",
        icon: <EvaluationOutline />,
        collapsible: true,
        items: [
          {
            label: "Monitors",
            type: "item",
            icon: <MonitorCheck size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.evaluation.children.monitor
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.evaluation.children.monitor.path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
        ],
      },
    ];
  }

  if (orgId && projectId && agentId && defaultEnv && agent?.kindName) {
    return [
      {
        label: "Overview",
        type: "item",
        icon: <LayoutDashboard size={20} />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          pathname,
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          { orgId, projectId, agentId },
        ),
      },
      {
        title: "Agent Lifecycle",
        type: "section",
        icon: <GitBranch />,
        collapsible: true,
        items: [
          {
            label: "Configure",
            type: "item",
            icon: <Settings size={20} />,
            isActive: !!matchPath(
              agentsChildren.configure?.wildPath ?? "",
              pathname,
            ),
            href: generatePath(agentsChildren.configure?.path ?? "", {
              orgId,
              projectId,
              agentId,
            }),
          },
          {
            label: "Deploy",
            type: "item",
            icon: <Rocket size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.deployment.wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.deployment.path,
              { orgId, projectId, agentId },
            ),
          },
          {
            label: "Try It",
            type: "item",
            icon: <FlaskConical size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.tryOut.wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.tryOut.path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
        ],
      },
      {
        title: "Security",
        type: "section",
        icon: <Lock />,
        collapsible: true,
        items: [
          {
            label: "Agent ID",
            type: "item" as const,
            icon: <thunderInstancesMetadata.icon size={20} />,
            isActive: !!matchPath(
              agentsChildren.agentId?.wildPath ?? "",
              pathname,
            ),
            href: generatePath(agentsChildren.agentId?.path ?? "", {
              orgId,
              projectId,
              agentId,
            }),
          },
          ...(agent?.agentType?.type === "agent-api"
            ? [
                {
                  label: "Credentials",
                  type: "item" as const,
                  icon: <ShieldCheck size={20} />,
                  isActive: !!matchPath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.security.wildPath,
                    pathname,
                  ),
                  href: generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.security.path,
                    { orgId, projectId, agentId, envId: defaultEnv },
                  ),
                },
              ]
            : []),
        ],
      },
      {
        title: "Observability",
        type: "section",
        icon: <Activity />,
        collapsible: true,
        items: [
          {
            label: "Traces",
            type: "item",
            icon: <Workflow size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
          {
            label: "Runtime Logs",
            type: "item",
            icon: <Logs size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.logs
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.logs.path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
          {
            label: "System Metrics",
            type: "item",
            icon: <Gauge size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.metrics
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.metrics
                .path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
        ],
      },
      {
        title: "Evaluation",
        type: "section",
        icon: <EvaluationOutline />,
        collapsible: true,
        items: [
          {
            label: "Monitors",
            type: "item",
            icon: <MonitorCheck size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.evaluation.children.monitor
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.evaluation.children.monitor.path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
        ],
      },
      ...externalNavItems
        .filter((item) => item.level === "component")
        .map((item) => ({
          label: item.title,
          type: "item" as const,
          icon: item.icon,
          isActive: !!matchPath(item.route, pathname),
          href: generatePath(item.route, { orgId, projectId, agentId }),
        })),
    ];
  }
  if (orgId && projectId && agentId && defaultEnv && !agent?.kindName) {
    return [
      {
        label: "Overview",
        type: "item",
        icon: <LayoutDashboard size={20} />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          pathname,
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          { orgId, projectId, agentId },
        ),
      },
      {
        title: "Agent Lifecycle",
        type: "section",
        icon: <GitBranch />,
        collapsible: true,
        items: [
          {
            label: "Configure",
            type: "item",
            icon: <Settings size={20} />,
            isActive: !!matchPath(
              agentsChildren.configure?.wildPath ?? "",
              pathname,
            ),
            href: generatePath(agentsChildren.configure?.path ?? "", {
              orgId,
              projectId,
              agentId,
            }),
          },
          {
            label: "Build",
            type: "item",
            icon: <Hammer size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.build.wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.build.path,
              { orgId, projectId, agentId },
            ),
          },
          {
            label: "Deploy",
            type: "item",
            icon: <Rocket size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.deployment.wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.deployment.path,
              { orgId, projectId, agentId },
            ),
          },
          {
            label: "Try It",
            type: "item",
            icon: <FlaskConical size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.tryOut.wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.tryOut.path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
          {
            label: "Publish",
            type: "item",
            icon: <Megaphone size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.publish.wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.publish.path,
              { orgId, projectId, agentId },
            ),
          },
        ],
      },
      {
        title: "Security",
        type: "section",
        icon: <Lock />,
        collapsible: true,
        items: [
          {
            label: "Agent ID",
            type: "item" as const,
            icon: <thunderInstancesMetadata.icon size={20} />,
            isActive: !!matchPath(
              agentsChildren.agentId?.wildPath ?? "",
              pathname,
            ),
            href: generatePath(agentsChildren.agentId?.path ?? "", {
              orgId,
              projectId,
              agentId,
            }),
          },
          ...(agent?.agentType?.type === "agent-api"
            ? [
                {
                  label: "Credentials",
                  type: "item" as const,
                  icon: <ShieldCheck size={20} />,
                  isActive: !!matchPath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.security.wildPath,
                    pathname,
                  ),
                  href: generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.security.path,
                    { orgId, projectId, agentId, envId: defaultEnv },
                  ),
                },
              ]
            : []),
        ],
      },
      {
        title: "Observability",
        type: "section",
        icon: <Activity />,
        collapsible: true,
        items: [
          {
            label: "Traces",
            type: "item",
            icon: <Workflow size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
          {
            label: "Runtime Logs",
            type: "item",
            icon: <Logs size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.logs
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.logs.path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
          {
            label: "System Metrics",
            type: "item",
            icon: <Gauge size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.metrics
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.metrics
                .path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
        ],
      },
      {
        title: "Evaluation",
        type: "section",
        icon: <EvaluationOutline />,
        collapsible: true,
        items: [
          {
            label: "Monitors",
            type: "item",
            icon: <MonitorCheck size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.evaluation.children.monitor
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.evaluation.children.monitor.path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
        ],
      },
      ...externalNavItems
        .filter((item) => item.level === "component")
        .map((item) => ({
          label: item.title,
          type: "item" as const,
          icon: item.icon,
          isActive: !!matchPath(item.route, pathname),
          href: generatePath(item.route, { orgId, projectId, agentId }),
        })),
    ];
  }
  if (orgId && projectId) {
    return [
      {
        label: "Agents",
        type: "item",
        icon: <Home size={20} />,
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.path,
          { orgId, projectId },
        ),
        isActive:
          !!matchPath(
            absoluteRouteMap.children.org.children.projects.path,
            pathname,
          ) ||
          !!matchPath(
            absoluteRouteMap.children.org.children.projects.children.agents
              .wildPath,
            pathname,
          ),
      },
    ];
  }
  if (orgId) {
    return [
      {
        label: "Projects",
        type: "item",
        icon: <Box size={20} />,
        href: generatePath(absoluteRouteMap.children.org.path, { orgId }),
        isActive: !!matchPath(absoluteRouteMap.children.org.path, pathname),
      },
      {
        label: "Agent Catalog",
        type: "item",
        icon: <BookOpenText size={20} />,
        href: generatePath(
          absoluteRouteMap.children.org.children.catalog.path,
          { orgId },
        ),
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.catalog.wildPath,
          pathname,
        ),
      },
      ...(navVisibility.identityUsers ||
      navVisibility.identityRoles ||
      navVisibility.identityGroups
        ? [
            {
              label: "Settings",
              type: "item" as const,
              icon: <Settings size={20} />,
              href: generatePath(settingsOrgRoute.path, { orgId }),
              isActive: !!matchPath(settingsOrgRoute.wildPath, pathname),
              pinBottom: true,
            },
          ]
        : []),
      ...(navVisibility.resources
        ? [
            {
              type: "section" as const,
              title: "Resources",
              icon: <Boxes size={20} />,
              collapsible: true,
              items: [
                {
                  label: "LLM Service Providers",
                  type: "item" as const,
                  icon: <BrainCircuit size={20} />,
                  href: generatePath(llmProvidersOrgRoute.path, { orgId }),
                  isActive: !!matchPath(
                    llmProvidersOrgRoute.wildPath,
                    pathname,
                  ),
                },
                {
                  label: "MCP Servers",
                  type: "item" as const,
                  icon: <MCPLogo size={20} />,
                  href: generatePath(mcpProxiesOrgRoute.path, { orgId }),
                  isActive: !!matchPath(mcpProxiesOrgRoute.wildPath, pathname),
                },
              ],
            },
          ]
        : []),
      ...(navVisibility.evaluation
        ? [
            {
              title: "Evaluation",
              type: "section" as const,
              icon: <EvaluationOutline />,
              collapsible: true,
              items: [
                {
                  label: "Evaluators",
                  type: "item" as const,
                  icon: <Code size={20} />,
                  isActive: !!matchPath(evaluatorsOrgRoute.wildPath, pathname),
                  href: generatePath(evaluatorsOrgRoute.path, { orgId }),
                },
              ],
            },
          ]
        : []),
      ...(navVisibility.infrastructure
        ? [
            {
              title: "Infrastructure",
              type: "section" as const,
              icon: <Server />,
              collapsible: true,
              items: [
                {
                  label: "Gateways",
                  type: "item" as const,
                  icon: <Network size={20} />,
                  href: generatePath(gatewaysOrgRoute.path, { orgId }),
                  isActive: !!matchPath(gatewaysOrgRoute.wildPath, pathname),
                },
                {
                  label: "Environments",
                  type: "item" as const,
                  icon: <Layers size={20} />,
                  href: generatePath(environmentsOrgRoute.path, { orgId }),
                  isActive: !!matchPath(
                    environmentsOrgRoute.wildPath,
                    pathname,
                  ),
                },
                {
                  label: "Deployment Pipelines",
                  type: "item" as const,
                  icon: <Workflow size={20} />,
                  href: generatePath(deploymentPipelinesOrgRoute.path, {
                    orgId,
                  }),
                  isActive: !!matchPath(
                    deploymentPipelinesOrgRoute.wildPath,
                    pathname,
                  ),
                },
              ],
            },
          ]
        : []),
      ...(navVisibility.agentIdentities
        ? [
            {
              title: "Agent Identities",
              type: "section" as const,
              icon: <thunderInstancesMetadata.icon />,
              collapsible: true,
              // Both children read the same agent-identity API, so one scope
              // governs both and there is nothing left to filter per item.
              items: (["groups", "roles"] as const).map((key) => {
                const ChildIcon = thunderInstancesMetadata.children[key].icon;
                const thunderInstancesChildRoute =
                  absoluteRouteMap.children.org.children.thunderInstances
                    .children[key];
                return {
                  label: thunderInstancesMetadata.children[key].title,
                  type: "item" as const,
                  icon: <ChildIcon size={20} />,
                  href: generatePath(thunderInstancesChildRoute.path, {
                    orgId,
                  }),
                  isActive: !!matchPath(
                    thunderInstancesChildRoute.wildPath,
                    pathname,
                  ),
                };
              }),
            },
          ]
        : []),
      ...externalNavItems
        .filter((item) => item.level === "org")
        .map((item) => ({
          label: item.title,
          type: "item" as const,
          icon: item.icon,
          isActive: !!matchPath(item.route, pathname),
          href: generatePath(item.route, { orgId }),
        })),
    ];
  }
  return [];
}
