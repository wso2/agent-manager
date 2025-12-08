import { BuildComponent } from './Build.Component';
import { BuildProject } from './Build.Project';
import { BuildOrganization } from './Build.Organization';
import { Wrench as BuildOutlined } from '@wso2/oxygen-ui-icons-react';

export const metaData = {
  title: 'Build',
  description: 'A page component for Build',
  icon: BuildOutlined,
  path: '/build',
  component: BuildComponent,
  levels: {
    component: BuildComponent,
    project: BuildProject,
    organization: BuildOrganization,
  },
};

export { 
  BuildComponent,
  BuildProject,
  BuildOrganization,
};

export default BuildComponent;
