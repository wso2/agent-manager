import { BarChart3 as AutoGraphOutlined, Binoculars } from "@wso2/oxygen-ui-icons-react";
import {
  generatePath,
  matchPath,
  useLocation,
  useParams,
} from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import type {
  NavigationItem,
  NavigationSection,
} from "@agent-management-platform/views";
import {
  useGetAgent,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import {
  overviewMetadata,
  buildMetadata,
  testMetadata,
  tracesMetadata,
  deploymentMetadata,
} from "../pages";
// import { useGetAgent } from '@agent-management-platform/api-client';

export function useNavigationItems(): Array<
  NavigationSection | NavigationItem
> {
  const { orgId, projectId, agentId, envId } = useParams();
  const { data: agent, isLoading: isLoadingAgent } = useGetAgent({
    agentName: agentId ?? "",
    orgName: orgId ?? "",
    projName: projectId ?? "",
  });
  const { data: environments, isLoading: isLoadingEnvironments } =
    useListEnvironments({
      orgName: orgId ?? "",
    });
  const defaultEnv = envId ?? environments?.[0]?.name;
  const { pathname } = useLocation();

  if (isLoadingAgent || isLoadingEnvironments) {
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
        label: overviewMetadata.title,
        type: "item",
        icon: <overviewMetadata.icon  />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          pathname
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          { orgId, projectId, agentId }
        ),
      },
      {
        title: "Observability",
        type: "section",
        icon: <AutoGraphOutlined />,
        items: [
          {
            label: tracesMetadata.title,
            type: "item",
            icon: <tracesMetadata.icon  />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.observe.children.traces.wildPath,
              pathname
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.observe.children.traces.path,
              { orgId, projectId, agentId }
            ),
          },
        ],
      },
    ];
  }

  if (orgId && projectId && agentId && defaultEnv) {
    return [
      {
        label: overviewMetadata.title,
        type: "item",
        icon: <overviewMetadata.icon  />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          pathname
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          { orgId, projectId, agentId }
        ),
      },
      {
        label: buildMetadata.title,
        type: "item",
        icon: <buildMetadata.icon  />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.build.wildPath,
          pathname
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.build.path,
          { orgId, projectId, agentId }
        ),
      },
      {
        label: deploymentMetadata.title,
        type: "item",
        icon: <deploymentMetadata.icon  />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.deployment.wildPath,
          pathname
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.deployment.path,
          { orgId, projectId, agentId }
        ),
      },
      {
        label: testMetadata.title,
        type: "item",
        icon: <testMetadata.icon  />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.environment.children.tryOut.path,
          pathname
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.environment.children.tryOut.path,
          { orgId, projectId, agentId, envId: defaultEnv }
        ),
      },
      {
        title: "Observability",
        type: "section",
        icon: <Binoculars  />,
        items: [
          {
            label: tracesMetadata.title,
            type: "item",
            icon: <tracesMetadata.icon  />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .wildPath,
              pathname
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .path,
              { orgId, projectId, agentId, envId: defaultEnv }
            ),
          },
        ],
      },
    ];
  }
  if (orgId && projectId) {
    return [
      {
        label: "Agents",
        type: "item",
        icon: <overviewMetadata.icon  />,
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.path,
          { orgId, projectId }
        ),
        isActive:
          !!matchPath(
            absoluteRouteMap.children.org.children.projects.path,
            pathname
          ) ||
          !!matchPath(
            absoluteRouteMap.children.org.children.projects.children.agents
              .wildPath,
            pathname
          ),
      },
    ];
  }
  if (orgId) {
    return [
      {
        label: "Projects",
        type: "item",
        icon: <overviewMetadata.icon  />,
        href: generatePath(absoluteRouteMap.children.org.path, { orgId }),
        isActive: !!matchPath(absoluteRouteMap.children.org.path, pathname),
      },
    ];
  }
  return [];
}
