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

import { PeopleAltOutlined, ViewAgendaOutlined } from '@mui/icons-material';
import { generatePath, matchPath, useLocation, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import type { NavigationItem, NavigationSection } from '@agent-management-platform/views';
// import { useGetAgent } from '@agent-management-platform/api-client';

export function useNavigationItems(): Array<NavigationSection | NavigationItem> {
  const { orgId, projectId } = useParams();
  const { pathname } = useLocation();
  if (orgId && projectId) {
    return [
      {
        label: 'Agents',
        type: 'item',
        icon: <PeopleAltOutlined fontSize='small' />,
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.path,
          { orgId, projectId }),
        isActive: !!matchPath(absoluteRouteMap.children.org.children.projects.path, pathname) ||
          !!matchPath(absoluteRouteMap.
            children.org.children.projects.children.agents.wildPath, pathname),
      },
    ]
  }
  if (orgId) {
    return [
      {
        label: 'Projects',
        type: 'item',
        icon: <ViewAgendaOutlined fontSize='small' />,
        href: generatePath(
          absoluteRouteMap.children.org.path,
          { orgId }),
        isActive: !!matchPath(absoluteRouteMap.children.org.path, pathname),
      },
    ]
  }
  return []
}
