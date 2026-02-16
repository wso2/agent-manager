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
import { PageLayout } from "@agent-management-platform/views";
import { Card, CardContent, Stack, Typography, Button } from "@wso2/oxygen-ui";

export const CreateMonitorComponent: React.FC = () => (
  <PageLayout
    title="Create Monitor"
    description="Define a new eval monitor by selecting targets, thresholds, and alerting rules."
    disableIcon
  >
    <Stack spacing={3}>
      <Card variant="outlined">
        <CardContent>
          <Stack spacing={2}>
            <Typography variant="h6">Monitor builder</Typography>
            <Typography variant="body2" color="text.secondary">
              This is a placeholder until the monitor creation workflow is wired to backend services.
            </Typography>
            <Button variant="contained" color="primary" disabled>
              Configure inputs
            </Button>
          </Stack>
        </CardContent>
      </Card>
    </Stack>
  </PageLayout>
);

export default CreateMonitorComponent;
