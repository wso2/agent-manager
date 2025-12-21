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

import { Span } from '@agent-management-platform/types';
import {
  Box,
  ButtonBase,
  Chip,
  Collapse,
  IconButton,
  Stack,
  Tooltip,
  Typography,
} from '@wso2/oxygen-ui';
import { useCallback, useMemo, useState } from 'react';
import {
  Clock,
  Brain,
  ChevronDown,
  Minus,
  XCircle,
  Link,
  Shovel,
  Coins,
} from '@wso2/oxygen-ui-icons-react';

interface TraceExplorerProps {
  spans: Span[];
  onOpenAtributesClick: (span: Span) => void;
  selectedSpan: Span | null;
}

interface RenderSpan {
  span: Span;
  children: RenderSpan[];
  key: string;
  parentKey: string | null;
  childrenKeys: string[] | null;
}

function formatDuration(durationInNanos: number) {
  if (durationInNanos > 1000 * 1000 * 1000) {
    return `${(durationInNanos / (1000 * 1000 * 1000)).toFixed(2)}s`;
  }
  if (durationInNanos > 1000 * 1000) {
    return `${(durationInNanos / (1000 * 1000)).toFixed(2)}ms`;
  }
  return `${(durationInNanos / 1000).toFixed(2)}μs`;
}
const populateRenderSpans = (
  spans: Span[]
): {
  spanMap: Map<string, RenderSpan>;
  rootSpans: string[];
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
  const spanMap = new Map<string, RenderSpan>();

  sortedSpans.forEach((span) => {
    const childrenKeys = childrenMap.get(span.spanId) || null;
    spanMap.set(span.spanId, {
      span,
      children: [],
      key: span.spanId,
      parentKey: span.parentSpanId || null,
      childrenKeys: childrenKeys,
    });
  });

  return { spanMap, rootSpans };
};

export function TraceExplorer(props: TraceExplorerProps) {
  const { spans, onOpenAtributesClick, selectedSpan } = props;
  const renderSpan = useCallback(
    (
      key: string,
      spanMap: Map<string, RenderSpan>,
      expandedSpans: Record<string, boolean>,
      toggleExpanded: (key: string) => void,
      isLastChild?: boolean,
      isRoot?: boolean
    ) => {
      const span = spanMap.get(key);
      if (!span) {
        return null;
      }
      const expanded = expandedSpans[key];
      const hasChildren = span.childrenKeys && span.childrenKeys.length > 0;
      return (
        <Stack key={key} spacing={1} width="100%">
          {/* Connecting lines - only show for non-root nodes */}
          {!isRoot && (
            <>
              {/* Horizontal line */}
              <Box
                position="absolute"
                sx={{
                  width: 32,
                  height: 44,
                  borderLeft: isLastChild ? `2px solid` : 'none',
                  borderBottom: `2px solid`,
                  borderColor: 'divider',
                  left: -32,
                  top: -10,
                  borderBottomLeftRadius: isLastChild ? 8 : 0,
                }}
              />
              {/* Vertical line continuing down (only if not last child) */}
              {!isLastChild && (
                <Box
                  position="absolute"
                  sx={{
                    width: 1,
                    height: '100%',
                    borderLeft: `2px solid`,
                    borderColor: 'divider',
                    left: -32,
                    top: -18,
                  }}
                />
              )}
            </>
          )}
          <ButtonBase
            onClick={() => onOpenAtributesClick(span.span)}
            sx={{
              width: '100%',
            }}
          >
            <Stack
              direction="row"
              width="100%"
              justifyContent="space-between"
              sx={{
                border: `1px solid`,
                borderColor: selectedSpan?.spanId === span.span.spanId ? 'primary.main' : 'divider',
                borderRadius: 0.5,
                backgroundColor: 'background.paper',
                px: 1,
                transition: 'all 0.2s ease-in-out',
                '&:hover': {
                  backgroundColor: 'background.default',
                },
              }}
            >
              <Stack
                direction="row"
                spacing={1}
                flexGrow={1}
                alignItems="center"
              >
                <IconButton
                  disabled={!hasChildren}
                  onClick={(e) => {
                    e.stopPropagation();
                    e.preventDefault();
                    toggleExpanded(key);
                  }}
                  size="small"
                  color="primary"
                >
                  {hasChildren ? (
                    <>
                      <Box
                        component="span"
                        sx={{
                          transform: expanded
                            ? 'rotate(180deg)'
                            : 'rotate(0deg)',
                          display: 'inline-flex',
                          transition: 'transform 0.2s ease-in-out',
                        }}
                      >
                        <ChevronDown size={16} />
                      </Box>
                    </>
                  ) : (
                    <Minus size={16} />
                  )}
                </IconButton>
                <Stack direction="row" color="primary.main" alignItems="center">
                  {span.span.ampAttributes?.kind === 'llm' && (
                    <Brain size={16} />
                  )}
                  {span.span.ampAttributes?.kind === 'chain' && (
                    <Link size={16} />
                  )}
                  {span.span.ampAttributes?.kind === 'tool' && (
                    <Shovel size={16} />
                  )}
                </Stack>
                <Stack direction="column" p={0.5} alignItems="start">
                  <Stack direction="row" spacing={1} alignItems="center">
                    <Typography variant="h6">{span.span.name}</Typography>
                    {span.span.ampAttributes?.status?.error && (
                      <Stack
                        justifyContent="center"
                        sx={{ color: 'error.main' }}
                      >
                        <XCircle size={16} />
                      </Stack>
                    )}
                    <Chip
                      icon={<Clock size={16} />}
                      label={formatDuration(span.span.durationInNanos)}
                      size="small"
                      variant="outlined"
                    />
                  </Stack>
                  <Typography variant="caption" fontFamily="monospace" noWrap>
                    #{span.span.spanId}
                  </Typography>
                </Stack>
              </Stack>
              <Stack direction="row" spacing={1} alignItems="center">
                {span.span.ampAttributes?.model && (
                  <Tooltip title={span.span.ampAttributes?.model}>
                    <Chip
                      icon={<Brain size={16} />}
                      label="Model"
                      size="small"
                      variant="outlined"
                    />
                  </Tooltip>
                )}
                {span.span.ampAttributes?.tools && (
                  <Tooltip
                    title={span.span.ampAttributes?.tools
                      .map((tool) => tool.name)
                      .join(', ')}
                  >
                    <Chip
                      icon={<Shovel size={16} />}
                      label="Tools"
                      size="small"
                      variant="outlined"
                    />
                  </Tooltip>
                )}
                {span.span.ampAttributes?.tokenUsage && (
                  <Tooltip
                    title={`Used ${span.span.ampAttributes?.tokenUsage.inputTokens} input tokens, ${span.span.ampAttributes?.tokenUsage.outputTokens} output tokens`}
                  >
                    <Chip
                      icon={<Coins size={16} />}
                      label="Tokens"
                      size="small"
                      variant="outlined"
                    />
                  </Tooltip>
                )}
              </Stack>
            </Stack>
          </ButtonBase>
          {hasChildren && (
            <Collapse in={expanded} unmountOnExit>
              <Box
                display="flex"
                flexDirection="column"
                pl={4}
                position="relative"
              >
                {span.childrenKeys?.map((childKey, index) => (
                  <Box key={childKey} display="flex" position="relative">
                    {renderSpan(
                      childKey,
                      spanMap,
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
        </Stack>
      );
    },
    [onOpenAtributesClick, selectedSpan]
  );

  const [expandedSpans, setExpandedSpans] = useState<Record<string, boolean>>(
    () => {
      return spans.reduce(
        (acc, span) => {
          acc[span.spanId] = !span.parentSpanId;
          return acc;
        },
        {} as Record<string, boolean>
      );
    }
  );

  const renderingSpans = useMemo(() => populateRenderSpans(spans), [spans]);

  const renderedSpans = useMemo(() => {
    const toggleExpanded = (key: string) => {
      setExpandedSpans((prev) => ({
        ...prev,
        [key]: !prev[key],
      }));
    };
    return renderingSpans.rootSpans.map((rootSpan, index) => (
      <Stack key={rootSpan}>
        {renderSpan(
          rootSpan,
          renderingSpans.spanMap,
          expandedSpans,
          toggleExpanded,
          index === renderingSpans.rootSpans.length - 1,
          true // isRoot
        )}
      </Stack>
    ));
  }, [renderingSpans, expandedSpans, renderSpan]);

  return (
    <Stack direction="column" spacing={2}>
      {renderedSpans}
    </Stack>
  );
}
