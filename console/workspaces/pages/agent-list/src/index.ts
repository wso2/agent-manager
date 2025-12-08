import { AgentsListPage } from './AgentsListPage';
import { Users as PeopleOutlined } from "@wso2/oxygen-ui-icons-react";
import { absoluteRouteMap } from '@agent-management-platform/types';

export const metaData = {
  title: 'Agents',
  description: 'Agents List Page',
  icon: PeopleOutlined,
  path: absoluteRouteMap.children.org.children.projects.path,
  component: AgentsListPage,
}
