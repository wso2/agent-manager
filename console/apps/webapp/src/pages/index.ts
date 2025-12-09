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

export * from './Login';

// import the metaData from the pages
export { metaData as addNewAgentPageMetaData } from '@agent-management-platform/add-new-agent';
export { metaData as addNewProjectPageMetaData } from '@agent-management-platform/add-new-project';
// export { metaData as agentViewPageMetaData } from '@agent-management-platform/agent-view';

// 3Levels Page MetaData
export { metaData as overviewMetadata } from '@agent-management-platform/overview';
export { metaData as buildMetadata } from '@agent-management-platform/build';
export { metaData as deployMetadata } from '@agent-management-platform/deploy';
export { metaData as testMetadata } from '@agent-management-platform/test';
export { metaData as tracesMetadata } from '@agent-management-platform/traces';
export { metaData as deploymentMetadata } from '@agent-management-platform/deploy';


