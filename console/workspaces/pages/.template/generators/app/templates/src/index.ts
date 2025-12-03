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

import { <%= componentName %>Component } from './<%= componentName %>.Component';
import { <%= componentName %>Project } from './<%= componentName %>.Project';
import { <%= componentName %>Organization } from './<%= componentName %>.Organization';
import { Dashboard } from '@mui/icons-material';

export const metaData = {
  title: '<%= title %>',
  description: '<%= description %>',
  icon: Dashboard,
  path: '<%= routePath %>',
  component: <%= componentName %>Component,
  levels: {
    component: <%= componentName %>Component,
    project: <%= componentName %>Project,
    organization: <%= componentName %>Organization,
  },
};

export { 
  <%= componentName %>Component,
  <%= componentName %>Project,
  <%= componentName %>Organization,
};

export default <%= componentName %>Component;
