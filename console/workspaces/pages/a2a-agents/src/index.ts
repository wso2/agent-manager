/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Network } from '@wso2/oxygen-ui-icons-react';
import type { PageMetadata } from '@agent-management-platform/types';
import { A2AAgentsOrganization } from './A2AAgents.Organization';
import { A2AAgentPage } from './A2AAgentPage';

export const metaData: PageMetadata = {
  title: 'A2A Agents',
  description: 'Organization-wide catalogue of A2A agents',
  icon: Network,
  path: '/a2a-agents',
  component: A2AAgentsOrganization,
  levels: {
    organization: A2AAgentsOrganization,
  },
};

export { A2AAgentsOrganization, A2AAgentPage };

export default A2AAgentsOrganization;
