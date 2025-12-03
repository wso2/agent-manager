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

import { TestComponent } from './Test.Component';
import { TestProject } from './Test.Project';
import { TestOrganization } from './Test.Organization';
import { Dashboard } from '@mui/icons-material';

export const metaData = {
  title: 'Test',
  description: 'A page component for Test',
  icon: Dashboard,
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
