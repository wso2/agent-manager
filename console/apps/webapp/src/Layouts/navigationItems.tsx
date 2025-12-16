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
import { metaData as overviewMetadata } from "@agent-management-platform/overview";
import { metaData as buildMetadata } from "@agent-management-platform/build";
import { metaData as testMetadata } from "@agent-management-platform/test";
import { metaData as tracesMetadata } from "@agent-management-platform/traces";
import { metaData as deploymentMetadata } from "@agent-management-platform/deploy";

export function useNavigationItems(): Array<
  NavigationSection | NavigationItem
> {
  const { orgId, projectId, agentId, envId } = useParams();
  const { data: agent, isLoading: isLoadingAgent } = useGetAgent({
    agentName: agentId,
    orgName: orgId,
    projName: projectId,
  });
  const { data: environments, isLoading: isLoadingEnvironments } =
    useListEnvironments({
      orgName: orgId,
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
            .children.environment.children.tryOut.wildPath,
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
