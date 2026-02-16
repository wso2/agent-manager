/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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
import { Card, CardContent, Stack, Typography, LinearProgress } from "@wso2/oxygen-ui";

export const ViewMonitorComponent: React.FC = () => (
  <PageLayout
    title="Monitor overview"
    description="Inspect monitor status, thresholds, and recent evaluation drifts."
    disableIcon   
  >
    <Stack spacing={3}>
      <Card variant="outlined">
        <CardContent>
          <Stack spacing={1.5}>
            <Typography variant="subtitle1">Monitor signal preview</Typography>
            <Typography variant="body2" color="text.secondary">
              Runtime metrics and evaluation history will render here once the APIs are available.
            </Typography>
            <LinearProgress variant="determinate" value={45} />
          </Stack>
        </CardContent>
      </Card>
    </Stack>
  </PageLayout>
);

export default ViewMonitorComponent;
