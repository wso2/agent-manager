package observer

import "context"

// BatchTraceReader is an optional ability of a TraceReader: reading the spans of several
// traces in one round trip. A reader that cannot do this (the OpenChoreo HTTP client)
// simply does not implement it, and the controller keeps its per-trace calls.
type BatchTraceReader interface {
	// QueryTraceSpansBatch returns the spans, with attributes, of the given traces, keyed by trace id.
	// req carries the window and scope, like QueryTraceSpans. req.Limit is the most rows the call may return.
	// A trace missing from the result was not covered, and the caller falls back to per-trace calls.
	// If the row cap is hit, the call returns an error instead of a partial answer.
	QueryTraceSpansBatch(ctx context.Context, traceIDs []string, req TracesQueryRequest) (map[string][]SpanInfo, error)
}