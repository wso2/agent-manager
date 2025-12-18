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

import { Box, Skeleton } from "@wso2/oxygen-ui";
import { useTrace } from "@agent-management-platform/api-client";
import {
  FadeIn,
  NoDataFound,
  TraceExplorer,
  DrawerWrapper,
} from "@agent-management-platform/views";
import { useParams } from "react-router-dom";
import { Span } from "@agent-management-platform/types";
import { GitBranch } from "@wso2/oxygen-ui-icons-react";
import { useState, useCallback } from "react";
import { SpanDetailsPanel } from "./SpanDetailsPanel";

function TraceDetailsSkeleton() {
  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        gap: 0.5,
      }}
    >
      {[...Array(8)].map((_, index) => (
        <Skeleton
          key={index}
          variant="rectangular"
          width="100%"
          height={40}
          sx={{
            ml: (index % 3) * 2,
          }}
        />
      ))}
    </Box>
  );
}

export function TraceDetails() {
  const {
    orgId = "default",
    projectId = "default",
    agentId = "default",
    envId,
    traceId = "default",
  } = useParams();
  const { data: traceDetails, isLoading } = useTrace(
    orgId,
    projectId,
    agentId,
    envId ?? "",
    traceId
  );

  const [selectedSpan, setSelectedSpan] = useState<Span | null>(null);

  const handleCloseSpan = useCallback(() => setSelectedSpan(null), []);

  if (isLoading) {
    return <TraceDetailsSkeleton />;
  }

  const spans = traceDetails?.spans ?? [];

  if (spans.length === 0) {
    return (
      <FadeIn>
        <Box
          sx={{
            display: "flex",
            justifyContent: "center",
            alignItems: "center",
            height: "100%",
            padding: 10,
          }}
        >
          <NoDataFound
            message="No spans found"
            icon={<GitBranch size={16} />}
            subtitle="Try changing the time range"
          />
        </Box>
      </FadeIn>
    );
  }

  return (
    <FadeIn>
      <Box
        sx={{
          display: "flex",
          flexDirection: "column",
          gap: 2,
          height: "100%",
        }}
      >
        <Box
          sx={{
            display: "flex",
            flexDirection: "column",
            gap: 2,
          }}
        >
          {traceId && (
            <TraceExplorer
              onOpenAtributesClick={setSelectedSpan}
              spans={spans}
            />
          )}
        </Box>
        <DrawerWrapper open={!!selectedSpan} onClose={handleCloseSpan}>
          <SpanDetailsPanel span={selectedSpan} onClose={handleCloseSpan} />
        </DrawerWrapper>
      </Box>
    </FadeIn>
  );
}
