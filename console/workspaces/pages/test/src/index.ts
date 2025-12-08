import { TestComponent } from './Test.Component';
import { TestProject } from './Test.Project';
import { TestOrganization } from './Test.Organization';
import { FlaskConical as ScienceOutlined } from '@wso2/oxygen-ui-icons-react';

export const metaData = {
  title: 'Try your agent',
  description: 'A page component for Test',
  icon: ScienceOutlined,
  path: '/test',
  component: TestComponent,
  levels: {
    component: TestComponent,
    project: TestProject,
    organization: TestOrganization,
  },
};

export { 
  TestComponent,
  TestProject,
  TestOrganization,
};

export default TestComponent;
