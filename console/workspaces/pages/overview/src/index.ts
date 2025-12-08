import { OverviewComponent } from './Overview.Component';
import { OverviewProject } from './Overview.Project';
import { OverviewOrganization } from './Overview.Organization';
import { Home } from '@wso2/oxygen-ui-icons-react';

export const metaData = {
  title: 'Overview',
  description: 'A page component for Overview',
  icon: Home,
  path: '/overview',
  component: OverviewComponent,
  levels: {
    component: OverviewComponent,
    project: OverviewProject,
    organization: OverviewOrganization,
  },
};

export { 
  OverviewComponent,
  OverviewProject,
  OverviewOrganization,
};

export default OverviewComponent;
