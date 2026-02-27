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

import React from "react";
import { Box, Button } from "@wso2/oxygen-ui";
import { generatePath, Link, useParams } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import { relativeRouteMap } from "@agent-management-platform/types";
import { Plus } from "@wso2/oxygen-ui-icons-react";

export const LLMProvidersOrganization: React.FC = () => {
  const { orgId } = useParams();
  return (
    <PageLayout
      title="LLM Service Providers"
      disableIcon
      actions={
        <Button
          component={Link}
          to={generatePath(
            relativeRouteMap.children.org.children.llmProviders.children.add
              .path,
            { orgId: orgId },
          )}
          variant="contained"
          color="primary"
          startIcon={<Plus size={18} />}
        >
          Add Provider
        </Button>
      }
    >
      <Box>sss</Box>
    </PageLayout>
  );
};

export default LLMProvidersOrganization;
