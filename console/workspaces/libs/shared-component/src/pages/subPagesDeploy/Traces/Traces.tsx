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

import React, { useState } from 'react';
import { Box, Collapse, useTheme } from '@mui/material';
import { TopCards } from './subComponents/TopCards';
import { TracesTable } from './subComponents/TracesTable';
import { FadeIn } from '@agent-management-platform/views';
import { Route, Routes, useMatch } from 'react-router-dom';
import { TraceDetails } from './subComponents/TraceDetails';
import { TraceListTimeRange } from '@agent-management-platform/types';

export const Traces: React.FC = () => {
  const theme = useTheme();
  const [timeRange, setTimeRange] = useState<TraceListTimeRange>(TraceListTimeRange.ONE_DAY);
  const isTraceDetails = useMatch("/unknown");

  return (
    <FadeIn>
      <Box 
        sx={{ 
          pt: theme.spacing(1), 
          gap: theme.spacing(2), 
          display: 'flex', 
          flexDirection: 'column' 
        }}
      >
        <Collapse in={!isTraceDetails}>
          <TopCards timeRange={timeRange} />
        </Collapse>
        <Routes>
          <Route path="/" element={<TracesTable timeRange={timeRange} setTimeRange={setTimeRange} />} />
          <Route path="/:traceId" element={<TraceDetails />} />
        </Routes>
      </Box>
    </FadeIn>
  );
};

