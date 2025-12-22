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

import { Box, Divider, Skeleton, Stack } from "@wso2/oxygen-ui";
import { useTrace } from "@agent-management-platform/api-client";
import {
  FadeIn,
  NoDataFound,
  TraceExplorer,
} from "@agent-management-platform/views";
import { useParams } from "react-router-dom";
import { Span } from "@agent-management-platform/types";
import { Workflow } from "@wso2/oxygen-ui-icons-react";
import { useEffect, useState } from "react";
import { SpanDetailsPanel } from "./SpanDetailsPanel";

function TraceDetailsSkeleton() {
  return (
    <Stack direction="row" height="calc(100vh - 64px)" gap={1}>
      <Skeleton variant="rounded" width="55%" height="100%" />
      <Divider orientation="vertical" flexItem />
      <Skeleton variant="rounded" width="45%" height="100%" />
    </Stack>
  );
}

interface TraceDetailsProps {
  traceId: string;
}
export function TraceDetails({ traceId }: TraceDetailsProps) {
  const {
    orgId = "default",
    projectId = "default",
    agentId = "default",
    envId = "default",
  } = useParams();
  const { data: traceDetails, isLoading } = useTrace(
    orgId,
    projectId,
    agentId,
    envId,
    traceId
  );

  const [selectedSpan, setSelectedSpan] = useState<Span | null>(null);
  useEffect(() => {
    setSelectedSpan(
      traceDetails?.spans?.find((span) => !span.parentSpanId) ??
        traceDetails?.spans?.[0] ??
        null
    );
  }, [traceDetails]);

  if (isLoading) {
    return <TraceDetailsSkeleton />;
  }

  if (traceDetails?.spans?.length == 0) {
    return (
      <FadeIn>
        <NoDataFound
          message="No spans found"
          iconElement={Workflow}
          disableBackground
          subtitle="Try changing the time range"
        />
      </FadeIn>
    );
  }

  return (
    <FadeIn>
      <Stack direction="row" height="calc(100vh - 64px)">
        <Box sx={{ width: "45%" }} pr={1} overflow="auto">
          {traceId && (
            <TraceExplorer
              onOpenAttributesClick={setSelectedSpan}
              selectedSpan={selectedSpan}
              spans={traceDetails?.spans ?? []}
            />
          )}
        </Box>
        <Divider orientation="vertical" flexItem />
        <Box sx={{ width: "55%" }}>
          <SpanDetailsPanel span={selectedSpan ?? null} />
        </Box>
      </Stack>
    </FadeIn>
  );
}
