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

import { lazy } from 'react';
import { OverviewComponent } from './Overview.Component';
import { OverviewProject } from './Overview.Project';
import { OverviewOrganization } from './Overview.Organization';
import { Home, Settings } from '@wso2/oxygen-ui-icons-react';

const ConfigureComponent = lazy(() =>
  import('./Configure.Component').then((m) => ({ default: m.ConfigureComponent }))
);
const AddLLMProviderComponent = lazy(() =>
  import('./AddLLMProvider.Component').then((m) => ({
    default: m.AddLLMProviderComponent,
  }))
);

export const metaData = {
  title: 'Overview',
  description: 'A page component for Overview',
  icon: Home,
  path: '/overview',
  component: OverviewComponent,
  configure: {
    title: 'Configure',
    icon: Settings,
  },
  levels: {
    component: OverviewComponent,
    project: OverviewProject,
    organization: OverviewOrganization,
    configureComponent: ConfigureComponent,
    addLLMProviderComponent: AddLLMProviderComponent,
  },
};

export {
  OverviewComponent,
  OverviewProject,
  OverviewOrganization,
  ConfigureComponent,
  AddLLMProviderComponent,
};

export default OverviewComponent;
