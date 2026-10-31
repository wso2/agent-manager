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
import { Button } from "@wso2/oxygen-ui";
import { Box } from "@wso2/oxygen-ui-icons-react";
import { PageLayout } from "@agent-management-platform/views";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";

// Placeholder: summary content for the organization lands here later.
export const OrganizationOverview: React.FC = () => {
  const { orgId = "" } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  return (
    <PageLayout
      title="Organization Overview"
      actions={
        <Button
          variant="outlined"
          size="small"
          startIcon={<Box size={16} />}
          onClick={() =>
            navigate(
              generatePath(
                absoluteRouteMap.children.org.children.projectListing.path,
                { orgId },
              ),
            )
          }
        >
          View Projects
        </Button>
      }
    >
      {null}
    </PageLayout>
  );
};

export default OrganizationOverview;
