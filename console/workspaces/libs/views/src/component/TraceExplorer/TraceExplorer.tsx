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

import { Span } from "@agent-management-platform/types";
import { alpha, Box, Button, ButtonBase, Chip, Collapse, Divider, Tooltip, Typography, useTheme } from "@mui/material";
import { useCallback, useMemo, useState } from "react";
import { ViewArrayOutlined, AccessTimeOutlined, ModelTrainingOutlined, TypeSpecimenOutlined, BarChartOutlined, TokenOutlined, TokenRounded, CircleOutlined, ExpandCircleDownOutlined } from "@mui/icons-material";

interface TraceExplorerProps {
    spans: Span[];
    onOpenAtributesClick: (span: Span) => void;
}

interface RenderSpan {
    span: Span;
    children: RenderSpan[];
    key: string;
    parentKey: string | null;
    childrenKeys: string[] | null;
}

function formatDuration(durationInNanos: number) {
    if (durationInNanos > (1000 * 1000 * 1000)) {
        return `${(durationInNanos / (1000 * 1000 * 1000)).toFixed(2)}s`;
    }
    if (durationInNanos > (1000 * 1000)) {
        return `${(durationInNanos / (1000 * 1000)).toFixed(2)}ms`;
    }
    return `${(durationInNanos / 1000).toFixed(2)}μs`;
}
const populateRenderSpans = (spans: Span[]): {
    
    renderSpanMap: Map<string, RenderSpan>,
    rootSpans: string[]
} => {
    // Sort spans by start time (earliest first)
    const sortedSpans = [...spans].sort((a, b) => {
        const timeA = new Date(a.startTime).getTime();
        const timeB = new Date(b.startTime).getTime();
        return timeA - timeB;
    });

    // First pass: Build a map of spanId -> array of child spanIds
    const childrenMap = new Map<string, string[]>();
    const rootSpans: string[] = [];
    
    sortedSpans.forEach((span) => {
        if (span.parentSpanId) {
            const children = childrenMap.get(span.parentSpanId) || [];
            children.push(span.spanId);
            childrenMap.set(span.parentSpanId, children);
        } else {
            rootSpans.push(span.spanId);
        }
    });

    // Second pass: Create RenderSpan objects and store them in a Map keyed by spanId
    const renderSpanMap = new Map<string, RenderSpan>();

    sortedSpans.forEach((span) => {
        const childrenKeys = childrenMap.get(span.spanId) || null;
        renderSpanMap.set(span.spanId, {
            span,
            children: [],
            key: span.spanId,
            parentKey: span.parentSpanId || null,
            childrenKeys: childrenKeys,
        });
    });

    return { renderSpanMap, rootSpans };
};

export function TraceExplorer(props: TraceExplorerProps) {
    const { spans, onOpenAtributesClick } = props;
    const theme = useTheme();

    const renderSpan = useCallback((
        key: string,
        renderSpanMap: Map<string, RenderSpan>,
        expandedSpans: Record<string, boolean>,
        toggleExpanded: (key: string) => void,
        isLastChild?: boolean,
        isRoot?: boolean,
    ) => {
        const span = renderSpanMap.get(key);
        if (!span) {
            return null;
        }
        const expanded = expandedSpans[key];
        const hasChildren = span.childrenKeys && span.childrenKeys.length > 0;
        return (
            <Box
                key={key}
                display="flex"
                position="relative"
                flexDirection="column"
                flexGrow={1}
            >
                {/* Connecting lines - only show for non-root nodes */}
                {!isRoot && (
                    <>
                        {/* Horizontal line */}
                        <Box
                            position="absolute"
                            sx={{
                                width: 31,
                                height: 44,
                                borderLeft: isLastChild ? `2px solid ${alpha(theme.palette.secondary.main, 0.8)}` : 'none',
                                borderBottom: `2px solid ${alpha(theme.palette.secondary.main, 0.8)}`,
                                left: -31,
                                top: -12,
                                borderBottomLeftRadius: isLastChild ? '8px' : 0,
                            }}
                        />
                        {/* Vertical line continuing down (only if not last child) */}
                        {!isLastChild && (
                            <Box
                                position="absolute"
                                sx={{
                                    width: 2,
                                    height: '100%',
                                    background: alpha(theme.palette.secondary.main, 0.8),
                                    left: -31,
                                    top: -12,
                                }}
                            />
                        )}
                    </>
                )}

                <ButtonBase onClick={() => toggleExpanded(key)}
                    sx={{
                        // height: 'fit-content',
                        width: '100%',
                        mb: 1,
                        // gap: 1,
                        justifyContent: 'space-between',
                        textAlign: 'left',
                        flexGrow: 1,
                        display: 'flex',
                        borderRadius: theme.spacing(1),
                        border: `1px solid ${theme.palette.divider}`,
                        transition: 'all 0.2s ease-in-out',
                        // backgroundColor: alpha(theme.palette.background.paper, 1),
                        '&:hover': {
                            // border: `1px solid ${alpha(color, 0.5)}`,
                            backgroundColor: theme.palette.background.paper,
                        },
                    }}>
                    <Box display="flex" height="100%" flexDirection="row" gap={1}>
                        <Box
                            sx={{
                                borderRadius: theme.spacing(0.5, 0, 0, 0.5),
                                height: '100%',
                                display: 'flex',
                                justifyContent: 'center',
                                alignItems: 'center',
                                pl: 1,
                                gap: 1,
                                borderLeft: `2px solid ${theme.palette.secondary.main}`,
                            }} >
                            {(hasChildren) ?
                                <>
                                    {
                                        expanded ? 
                                        <ExpandCircleDownOutlined fontSize="small" color={"secondary"} sx={{ transform: 'rotate(180deg)' }} />
                                            : <ExpandCircleDownOutlined fontSize="small" color={"secondary"} />
                                    }
                                </> : <CircleOutlined fontSize="small" color="disabled" />
                            }
                        </Box>
                        <Divider orientation="vertical" flexItem />
                        <Box
                            display="flex"
                            flexDirection="column"
                            justifyContent="center"
                            gap={1}
                            py={0.5}
                        >
                            <Typography variant="body1" color="text.primary">
                                {span.span.name} &nbsp;
                                <Chip
                                    icon={<AccessTimeOutlined fontSize="inherit" />}
                                    label={formatDuration(span.span.durationInNanos)}
                                    size="small"
                                    variant="outlined"
                                />
                            </Typography>
                            <Typography component="span" variant="caption" color="text.secondary">
                                ID:&nbsp;
                                {span.span.spanId}
                            </Typography>
                        </Box>
                    </Box>
                    <Box p={1} display="flex" gap={1} alignItems="flex-end" justifyContent="right">
                        {/* <TimeLine
                            key={`${span.span.spanId}-timeline`}
                            startTime={span.span.startTime}
                            endTime={span.span.endTime}
                            parentStartTime={parentSpan?.span.startTime}
                            parentEndTime={parentSpan?.span.endTime}
                        /> */}
                        <Box display="flex" flexDirection="row" gap={1}>
                            {
                                !!span.span?.attributes["gen_ai.request.model"] && (
                                    <Tooltip title={"GenAI Model"}>
                                        <Chip icon={<ModelTrainingOutlined fontSize="inherit" />} label={span.span?.attributes["gen_ai.request.model"] as string} color="default" size="small" variant="outlined" />
                                    </Tooltip>
                                )
                            }
                            {
                                !!span.span?.attributes["traceloop.association.properties.ls_model_type"] && (
                                    <Tooltip title={"Language Service Model Type"}>
                                        <Chip icon={<TypeSpecimenOutlined fontSize="inherit" />} label={span.span?.attributes["traceloop.association.properties.ls_model_type"] as string} color="default" size="small" variant="outlined" />
                                    </Tooltip>
                                )
                            }
                            {
                                !!span.span?.attributes["traceloop.span.kind"] && (
                                    <Tooltip title={"Span Kind"}>
                                        <Chip icon={<BarChartOutlined fontSize="inherit" />} label={span.span?.attributes["traceloop.span.kind"] as string} color="default" size="small" variant="outlined" />
                                    </Tooltip>
                                )
                            }
                            {
                                !!span.span?.attributes["gen_ai.usage.completion_tokens"] && (
                                    <Tooltip title={"Completion Tokens"}>
                                        <Chip icon={<TokenOutlined fontSize="inherit" />} label={span.span?.attributes["gen_ai.usage.completion_tokens"] as string} color="default" size="small" variant="outlined" />
                                    </Tooltip>
                                )
                            }
                            {
                                !!span.span?.attributes["gen_ai.usage.prompt_tokens"] && (
                                    <Tooltip title={"Prompt Tokens"}>
                                        <Chip icon={<TokenRounded fontSize="inherit" />} label={span.span?.attributes["gen_ai.usage.prompt_tokens"] as string} color="default" size="small" variant="outlined" />
                                    </Tooltip>
                                )
                            }
                        </Box>
                        <Button
                            onClick={(e) => {
                                onOpenAtributesClick(span.span);
                                e.stopPropagation();
                                e.preventDefault();
                            }}
                            startIcon={<ViewArrayOutlined />}
                            variant="text"
                            size="small"
                            color="secondary"
                        >
                            View Attributes
                        </Button>
                    </Box>
                </ButtonBase>
                {hasChildren && (
                    <Collapse in={expanded} unmountOnExit>
                        <Box display="flex" flexDirection="column" pl={4} position="relative">
                            {span.childrenKeys?.map((childKey, index) => (
                                <Box key={childKey} display="flex" position="relative">
                                    {renderSpan(
                                        childKey,
                                        renderSpanMap,
                                        expandedSpans,
                                        toggleExpanded,
                                        index === (span.childrenKeys?.length || 0) - 1,
                                        false
                                    )}
                                </Box>
                            ))}
                        </Box>
                    </Collapse>
                )}
            </Box>
        )
    }, [onOpenAtributesClick, theme]);

    const [expandedSpans, setExpandedSpans] = useState<Record<string, boolean>>(() => {
        return spans.reduce((acc, span) => {
            acc[span.spanId] = !span.parentSpanId;
            return acc;
        }, {} as Record<string, boolean>);
    });


    const renderSpans = useMemo(() => populateRenderSpans(spans), [spans]);

    const renderedSpans = useMemo(() => {
        const toggleExpanded = (key: string) => {
            setExpandedSpans((prev) => ({
                ...prev,
                [key]: !prev[key],
            }));
        };
        return renderSpans.rootSpans.map((rootSpan, index) => (
            <Box key={rootSpan} mb={2} display="flex" flexGrow={1}>
                {renderSpan(
                    rootSpan,
                    renderSpans.renderSpanMap,
                    expandedSpans,
                    toggleExpanded,
                    index === renderSpans.rootSpans.length - 1,
                    true // isRoot
                )}
            </Box>
        ));
    }, [renderSpans, expandedSpans, renderSpan]);

    return (
        <Box display="flex" gap={2}>
            <Box position="relative" display="flex" flexGrow={1}>
                {renderedSpans}
            </Box>
        </Box>
    );
}

