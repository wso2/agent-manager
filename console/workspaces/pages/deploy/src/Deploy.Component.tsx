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

import { Box } from '@wso2/oxygen-ui';
import { BuildCard, DeployCard } from './subComponent';
import { useParams } from 'react-router-dom';
import { useListEnvironments } from '@agent-management-platform/api-client';

export const DeployComponent = () => {
  const { orgId } = useParams();

  const { data: environments } = useListEnvironments({
    orgName: orgId,
  });
  

  return (
    <Box display="flex" gap={4} pb={4} pt={4}>
      <BuildCard />
      {
        environments?.map((env) => (
          <DeployCard key={env.name} currentEnvironment={env} />
        ))
      }
    </Box>
  );
};

export default DeployComponent;
