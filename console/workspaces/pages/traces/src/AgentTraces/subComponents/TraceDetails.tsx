import { Box, Divider, Skeleton } from "@wso2/oxygen-ui";
import { useTrace } from "@agent-management-platform/api-client";
import { FadeIn, NoDataFound, TraceExplorer, DrawerWrapper } from "@agent-management-platform/views";
import { useParams } from "react-router-dom";
import { Span } from "@agent-management-platform/types";
import { GitBranch } from "@wso2/oxygen-ui-icons-react";
import { useState, useCallback } from "react";
import { SpanDetailsPanel } from "./SpanDetailsPanel";

function TraceDetailsSkeleton() {
    return (
        <Box
            sx={{
                display: 'flex',
                flexDirection: 'column',
                gap: 2
            }}
        >
            <Box
                sx={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center'
                }}
            >
                <Skeleton variant="rectangular" width={150} height={45} />
                <Skeleton variant="text" width={150} height={40} />
            </Box>
            <Divider />
            <Box
                sx={{
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 1.5
                }}
            >
                <Skeleton variant="rectangular" width="100%" height={8} />
                {[...Array(8)].map((_, index) => (
                    <Skeleton
                        key={index}
                        variant="rectangular"
                        width="100%"
                        height={6}
                        sx={{
                            ml: (index % 3 * 2)
                        }}
                    />
                ))}
            </Box>
        </Box>
    );
}

export function TraceDetails() {    const { orgId = "default", projectId = "default", agentId = "default", envId, traceId = "default" } = useParams();
    const { data: traceDetails, isLoading } = useTrace(
        orgId,
        projectId,
        agentId,
        envId ?? '',
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
                        display: 'flex',
                        justifyContent: 'center',
                        alignItems: 'center',
                        height: '100%',
                        padding: 10
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
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 2,
                    height: '100%'
                }}
            >
                <Box
                    sx={{
                        display: 'flex',
                        flexDirection: 'column',
                        gap: 2
                    }}
                >
                    {traceId && (
                        <TraceExplorer onOpenAtributesClick={setSelectedSpan} spans={spans} />
                    )}
                </Box>
                <DrawerWrapper
                    open={!!selectedSpan}
                    onClose={handleCloseSpan}
                >
                    <SpanDetailsPanel span={selectedSpan} onClose={handleCloseSpan} />
                </DrawerWrapper>
            </Box>
        </FadeIn>
    );
}

