import { DeployComponent } from './Deploy.Component';
import { DeployProject } from './Deploy.Project';
import { DeployOrganization } from './Deploy.Organization';
import { Rocket as RocketLaunchOutlined } from '@wso2/oxygen-ui-icons-react';

export const metaData = {
  title: 'Deployment',
  description: 'A page component for Deploy',
  icon: RocketLaunchOutlined,
  path: '/deploy',
  component: DeployComponent,
  levels: {
    component: DeployComponent,
    project: DeployProject,
    organization: DeployOrganization,
  },
};

export { 
  DeployComponent,
  DeployProject,
  DeployOrganization,
};

export default DeployComponent;
