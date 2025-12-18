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

import { Box, Skeleton} from "@wso2/oxygen-ui";
import { StatusCard } from '@agent-management-platform/views';
import { Gauge as Speed, Workflow } from '@wso2/oxygen-ui-icons-react';
import dayjs from 'dayjs';
import duration from 'dayjs/plugin/duration';
import relativeTime from 'dayjs/plugin/relativeTime';
import { useTraceList } from "@agent-management-platform/api-client";
import { useMemo } from "react";
import { useParams } from "react-router-dom";
import { TraceListResponse, TraceListTimeRange } from "@agent-management-platform/types";

dayjs.extend(duration);
dayjs.extend(relativeTime);

const getTimeRangeLabel = (timeRange: TraceListTimeRange): string => {
    const labels: Record<TraceListTimeRange, string> = {
        [TraceListTimeRange.TEN_MINUTES]: '10 Minutes',
        [TraceListTimeRange.THIRTY_MINUTES]: '30 Minutes',
        [TraceListTimeRange.ONE_HOUR]: '1 Hour',
        [TraceListTimeRange.THREE_HOURS]: '3 Hours',
        [TraceListTimeRange.SIX_HOURS]: '6 Hours',
        [TraceListTimeRange.TWELVE_HOURS]: '12 Hours',
        [TraceListTimeRange.ONE_DAY]: '1 Day',
        [TraceListTimeRange.THREE_DAYS]: '3 Days',
        [TraceListTimeRange.SEVEN_DAYS]: '7 Days',
    };
    return labels[timeRange] || 'Unknown';
};

function TopCardsSkeleton() {
    return (
        <Box sx={{
            display: 'grid',
            gap: 2,
            gridTemplateColumns: {
                xs: '1fr',
                md: '1fr 1fr',
                lg: '1fr 1fr 1fr'
            }
        }}>
            <Skeleton variant="rounded" height={120} />
            <Skeleton variant="rounded" height={120} />
        </Box>
    );
}

interface TracesTopCardsProps {
    timeRange: TraceListTimeRange;
}

export const TracesTopCards: React.FC<TracesTopCardsProps> = ({ timeRange }) => {    const { orgId = "default", projectId = "default", agentId = "default", envId = "default" } = useParams();
    const { data: traceData, isLoading } = useTraceList(
        orgId, projectId,
        agentId,
        envId,
        timeRange
    );

    const traceListResponse = traceData as unknown as TraceListResponse;

    const totalCount = traceListResponse?.totalCount ?? 0;
    const timeRangeLabel = getTimeRangeLabel(timeRange);

    const statistics = useMemo(() => {
        const traces = traceListResponse?.traces ?? [];
        const latestTrace = traces[0];
        const latestTraceTime = latestTrace?.startTime ?? '';

        const averageDuration = traces.length > 0
            ? traces.reduce((sum, trace) => {
                const start = new Date(trace.startTime).getTime();
                const end = new Date(trace.endTime).getTime();
                return sum + (end - start);
            }, 0) / traces.length
            : 0;

        const averageDurationSeconds = (averageDuration / 1000).toFixed(2);

        return {
            latestTraceTime,
            averageDuration,
            averageDurationSeconds,
        };
    }, [traceListResponse?.traces]);

    if (isLoading) {
        return <TopCardsSkeleton />;
    }

    const {
        latestTraceTime,
        averageDuration,
        averageDurationSeconds,
    } = statistics;

    return (
        <Box sx={{
            display: 'grid',
            gap: 4,
            gridTemplateColumns: {
                xs: '1fr',
                md: '1fr 1fr',
                lg: '1fr 1fr 1fr'
            }
        }}>
            <StatusCard
                title="Total Traces"
                value={totalCount.toString()}
                subtitle={latestTraceTime ? `Latest: ${dayjs(latestTraceTime).fromNow()}` : 'No traces'}
                icon={<Workflow />}
                iconVariant="info"
                tagVariant="info"
                minWidth="100%"
            />
            <StatusCard
                title="Average Duration"
                value={`${averageDurationSeconds}s`}
                subtitle={`in last ${timeRangeLabel.toLowerCase()}`}
                icon={<Speed />}
                iconVariant={averageDuration < 3000 ? "success" : "warning"}
                tagVariant={averageDuration < 3000 ? "success" : "warning"}
                minWidth="100%"
            />
        </Box>
    );
};

