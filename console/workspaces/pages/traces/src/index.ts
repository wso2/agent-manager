import { TracesComponent } from './Traces.Component';
import { TracesProject } from './Traces.Project';
import { TracesOrganization } from './Traces.Organization';
import { Workflow } from '@wso2/oxygen-ui-icons-react';

export const metaData = {
  title: 'Traces',
  description: 'A page component for Traces',
  icon: Workflow,
  path: '/traces',
  component: TracesComponent,
  levels: {
    component: TracesComponent,
    project: TracesProject,
    organization: TracesOrganization,
  },
};

export { 
  TracesComponent,
  TracesProject,
  TracesOrganization,
};

export default TracesComponent;
