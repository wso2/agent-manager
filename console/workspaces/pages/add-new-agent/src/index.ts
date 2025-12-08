import { AddNewAgent } from './AddNewAgent';
import { UserPlus as PersonAddOutlined } from '@wso2/oxygen-ui-icons-react';
import { absoluteRouteMap } from '@agent-management-platform/types';

export const metaData = {
  title: 'Add New Agent',
  description: 'A page component for Add New Agent',
  icon: PersonAddOutlined,
  path: absoluteRouteMap.children.org.children.projects.children.newAgent.path,
  component: AddNewAgent,
};

export { AddNewAgent };
export default AddNewAgent;
