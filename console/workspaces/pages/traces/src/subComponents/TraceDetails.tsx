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
  useGetAgent,
  useListEnvironments,
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
} from "@agent-management-platform/types";
import { Workflow } from "@wso2/oxygen-ui-icons-react";
import { useEffect, useMemo, useState } from "react";
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

  const {
    data: agentData,
    isPending: isAgentPending,
    isError: isAgentError,
  } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const {
    data: environmentsData,
    isPending: isEnvPending,
    isError: isEnvError,
  } = useListEnvironments({ orgName: orgId });

  const componentUid = agentData?.uuid;
  const matchedEnvironment = useMemo(
    () => environmentsData?.find((e) => e.name === envId),
    [environmentsData, envId],
  );
  const environmentUid = matchedEnvironment?.id;

  const prereqsPending = isAgentPending || isEnvPending;
  const prereqsError = isAgentError || isEnvError;
  const envListReady =
    !isEnvPending && !isEnvError && environmentsData !== undefined;
  const environmentUnresolved =
    envListReady &&
    !!envId &&
    (!matchedEnvironment || !String(matchedEnvironment.id ?? "").trim());
  const traceEnabled =
    !!componentUid && !!environmentUid && !!traceId && !environmentUnresolved;

  const {
    data: traceDetails,
    isPending: isTracePending,
    isError: isTraceError,
  } = useTrace(componentUid, environmentUid, traceId);

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

  const [selectedSpan, setSelectedSpan] = useState<Span | null>(null);
  useEffect(() => {
    setSelectedSpan(
      traceDetails?.spans?.find((span) => !span.parentSpanId) ??
        traceDetails?.spans?.[0] ??
        null,
    );
  }, [traceDetails]);

  if (prereqsPending) {
    return <TraceDetailsSkeleton />;
  }

  if (!componentUid || prereqsError) {
    return (
      <FadeIn>
        <NoDataFound
          message="Agent is not ready"
          iconElement={Workflow}
          disableBackground
          subtitle="Could not resolve the agent identifier for this trace."
        />
      </FadeIn>
    );
  }

  if (environmentUnresolved) {
    return (
      <FadeIn>
        <NoDataFound
          message="Unknown environment"
          iconElement={Workflow}
          disableBackground
          subtitle={`No environment matches "${envId}" or it has no UUID.`}
        />
      </FadeIn>
    );
  }

  if (traceEnabled && isTracePending) {
    return <TraceDetailsSkeleton />;
  }

  if (traceEnabled && isTraceError) {
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
              onOpenAttributesClick={setSelectedSpan}
              selectedSpan={selectedSpan}
              spans={traceDetails?.spans ?? []}
            />
          )}
        </Box>
        <Divider orientation="vertical" flexItem />
        <Box sx={{ width: "55%" }}>
          <SpanDetailsPanel
            span={selectedSpan ?? null}
            evaluatorScores={
              selectedSpan
                ? !selectedSpan.parentSpanId
                  ? [
                      ...traceEvalScores,
                      ...(spanScoresMap.get(selectedSpan.spanId) ?? []),
                    ]
                  : spanScoresMap.get(selectedSpan.spanId)
                : undefined
            }
          />
        </Box>
      </Stack>
    </FadeIn>
  );
}
