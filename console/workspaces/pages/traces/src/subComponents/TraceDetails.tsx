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

import { Box, Divider, Skeleton, Stack, Typography } from "@wso2/oxygen-ui";
import {
  useTrace,
  useTraceScores,
  useSpanDetail,
} from "@agent-management-platform/api-client";
import {
  FadeIn,
  NoDataFound,
  TraceExplorer,
} from "@agent-management-platform/views";
import { useParams } from "react-router-dom";
import {
  Span,
  EvaluatorScoreWithMonitor,
  TraceSpanSummary,
} from "@agent-management-platform/types";
import { Workflow } from "@wso2/oxygen-ui-icons-react";
import { useEffect, useMemo, useState } from "react";
import { SpanDetailsPanel } from "./SpanDetailsPanel";
import { SpanDetailsPanelSkeleton } from "./spanDetails/SpanDetailsPanelSkeleton";

function TraceDetailsSkeleton() {
  return (
    <Stack direction="row" height="calc(100vh - 64px)" gap={1}>
      <Skeleton variant="rounded" width="55%" height="100%" />
      <Divider orientation="vertical" flexItem />
      <Skeleton variant="rounded" width="45%" height="100%" />
    </Stack>
  );
}

/** Adapts a list-summary to the Span shape expected by TraceExplorer.
 *  Field names differ between the two backend endpoints:
 *  spanName → name, durationNs → durationInNanos. */
function traceSpanSummaryToSpan(s: TraceSpanSummary): Span {
  return {
    spanId: s.spanId,
    parentSpanId: s.parentSpanId?.trim() || undefined,
    name: s.spanName,
    startTime: s.startTime,
    endTime: s.endTime,
    durationInNanos: s.durationNs,
  };
}

interface TraceDetailsProps {
  traceId: string;
  organization: string;
  project: string;
  component: string;
  environment: string;
  startTime: string;
  endTime: string;
}
export function TraceDetails({
  traceId,
  organization,
  project,
  component,
  environment,
  startTime,
  endTime,
}: TraceDetailsProps) {
  const {
    orgId = "default",
    projectId = "default",
    agentId = "default",
  } = useParams();

  const {
    data: traceDetails,
    isPending: isTracePending,
    isError: isTraceError,
  } = useTrace(
    organization,
    project,
    component,
    environment,
    traceId,
    startTime,
    endTime,
  );

  const [selectedSpanId, setSelectedSpanId] = useState<string | null>(null);

  useEffect(() => {
    const spans = traceDetails?.spans;
    if (!spans?.length) {
      setSelectedSpanId(null);
      return;
    }
    const root = spans.find((s) => !s.parentSpanId?.trim()) ?? spans[0];
    setSelectedSpanId(root?.spanId ?? null);
  }, [traceDetails]);

  const {
    data: spanDetail,
    isPending: isSpanDetailPending,
    isError: isSpanDetailError,
  } = useSpanDetail(
    traceId,
    selectedSpanId,
    !!traceDetails?.spans?.length && !!selectedSpanId,
  );

  const panelSpan =
    spanDetail && spanDetail.spanId === selectedSpanId ? spanDetail : null;

  const spansForExplorer = useMemo(() => {
    if (!traceDetails?.spans?.length) return [];
    return traceDetails.spans.map((s) =>
      panelSpan?.spanId === s.spanId
        ? panelSpan
        : traceSpanSummaryToSpan(s),
    );
  }, [traceDetails?.spans, panelSpan]);

  const displaySelectedSpan = useMemo(() => {
    if (!selectedSpanId) return null;
    return spansForExplorer.find((s) => s.spanId === selectedSpanId) ?? null;
  }, [spansForExplorer, selectedSpanId]);

  const showSpanDetailSkeleton =
    !!selectedSpanId &&
    !isSpanDetailError &&
    (isSpanDetailPending || spanDetail?.spanId !== selectedSpanId);

  const { data: traceScoresData } = useTraceScores({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
    traceId,
  });

  const { traceEvalScores, spanScoresMap } = useMemo(() => {
    const traceEvals: EvaluatorScoreWithMonitor[] = [];
    const spanMap = new Map<string, EvaluatorScoreWithMonitor[]>();

    for (const monitor of traceScoresData?.monitors ?? []) {
      for (const ev of monitor.evaluators) {
        traceEvals.push({ ...ev, monitorName: monitor.monitorName });
      }
      for (const span of monitor.spans) {
        const existing = spanMap.get(span.spanId) ?? [];
        for (const ev of span.evaluators) {
          existing.push({ ...ev, monitorName: monitor.monitorName });
        }
        spanMap.set(span.spanId, existing);
      }
    }
    return { traceEvalScores: traceEvals, spanScoresMap: spanMap };
  }, [traceScoresData]);

  if (isTracePending) {
    return <TraceDetailsSkeleton />;
  }

  if (isTraceError) {
    return (
      <FadeIn>
        <Box sx={{ p: 2 }}>
          <Typography color="error">
            Failed to load trace details. Try again later.
          </Typography>
        </Box>
      </FadeIn>
    );
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
      <Stack direction="row" height="calc(100vh - 72px)">
        <Box sx={{ width: "45%" }} pr={1} overflow="auto">
          {traceId && (
            <TraceExplorer
              onOpenAttributesClick={(span) => setSelectedSpanId(span.spanId)}
              selectedSpan={displaySelectedSpan}
              spans={spansForExplorer}
            />
          )}
        </Box>
        <Divider orientation="vertical" flexItem />
        <Box sx={{ width: "55%" }}>
          {showSpanDetailSkeleton ? (
            <SpanDetailsPanelSkeleton />
          ) : isSpanDetailError ? (
            <Box sx={{ p: 2 }}>
              <Typography color="error">
                Failed to load span details. Try again later.
              </Typography>
            </Box>
          ) : (
            <SpanDetailsPanel
              span={panelSpan}
              evaluatorScores={
                panelSpan
                  ? !panelSpan.parentSpanId
                    ? [
                        ...traceEvalScores,
                        ...(spanScoresMap.get(panelSpan.spanId) ?? []),
                      ]
                    : spanScoresMap.get(panelSpan.spanId)
                  : undefined
              }
            />
          )}
        </Box>
      </Stack>
    </FadeIn>
  );
}
