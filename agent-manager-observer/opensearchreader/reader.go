package opensearchreader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

const (
	defaultLimit     = 100
	maxLimit         = 1000
	defaultSpanLimit = 1000
	maxSpanLimit     = 10000
)

// ErrSpanNotFound means no span matches the trace id and span id.
var ErrSpanNotFound = errors.New("span not found")

type TraceReaderImpl struct {
	s         *searcher
	namespace string
	resolver  scope.Resolver
}

var _ observer.TraceReader = (*TraceReaderImpl)(nil)

var _ observer.BatchTraceReader = (*TraceReaderImpl)(nil)

func NewTraceReader(o Options) (*TraceReaderImpl, error) {
	if strings.TrimSpace(o.DefaultNamespace) == "" {
		return nil, errors.New("a default namespace is required")
	}
	if o.Resolver == nil {
		return nil, errors.New("a scope resolver is required")
	}
	s, err := newSearcher(o)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(o.Username) == "" {
		return nil, errors.New("an OpenSearch username is required")
	}
	if o.Password == "" {
		return nil, errors.New("an OpenSearch password is required")
	}
	if o.Timeout < 0 {
		return nil, errors.New("the OpenSearch timeout must not be negative")
	}
	return &TraceReaderImpl{s: s, namespace: o.DefaultNamespace, resolver: o.Resolver}, nil
}

func (t *TraceReaderImpl) Close() error {
	t.s.http.CloseIdleConnections()
	return nil
}

func (t *TraceReaderImpl) NamespaceFor(_ string) string { return t.namespace }

func listOptions(req observer.TracesQueryRequest, def, max int, defaultDesc bool) (limit int, desc bool, err error) {
	limit = def
	if req.Limit != nil {
		limit = *req.Limit
	}
	if limit < 1 || limit > max {
		return 0, false, fmt.Errorf("limit must be between 1 and %d, got %d", max, limit)
	}
	desc = defaultDesc
	if req.SortOrder != nil {
		switch *req.SortOrder {
		case "desc":
			desc = true
		case "asc":
			desc = false
		case "":
			// not given: keep the default
		default:
			return 0, false, fmt.Errorf("sortOrder must be asc or desc, got %q", *req.SortOrder)
		}
	}
	return limit, desc, nil
}

func (t *TraceReaderImpl) QueryTraces(ctx context.Context, req observer.TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	began := time.Now()
	limit, desc, err := listOptions(req, defaultLimit, maxLimit, true)
	if err != nil {
		return nil, err
	}
	// The trace list defaults to newest first, like the ClickHouse reader.
	if req.SortOrder == nil {
		desc = true
	}
	uids, err := t.resolver.Resolve(ctx, req.SearchScope)
	if err != nil {
		return nil, err
	}

	var res listResponse
	if err := t.s.search(ctx, listQuery(uids, window{req.StartTime, req.EndTime}, desc, limit), &res); err != nil {
		return nil, fmt.Errorf("opensearch.QueryTraces: %w", err)
	}
	slog.Debug("opensearch list query", "took_ms", res.Took)

	traces := make([]observer.TraceInfo, 0, len(res.Aggregations.Traces.Buckets))
	for _, b := range res.Aggregations.Traces.Buckets {
		if b.RootSpan.DocCount == 0 {
			continue // the root span is outside the window; the adapter drops these too
		}
		traces = append(traces, b.toTraceInfo())
	}
	return &observer.TracesQueryResponse{
		Traces: traces,
		Total:  res.Aggregations.TraceCount.Distinct.Value,
		TookMs: int(time.Since(began).Milliseconds()),
	}, nil
}

func (t *TraceReaderImpl) QueryTraceSpans(ctx context.Context, traceID string, req observer.TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	began := time.Now()
	if traceID == "" {
		return nil, errors.New("opensearch.QueryTraceSpans: trace id is required")
	}
	limit, desc, err := listOptions(req, defaultSpanLimit, maxSpanLimit, false)
	if err != nil {
		return nil, err
	}
	uids, err := t.resolver.Resolve(ctx, req.SearchScope)
	if err != nil {
		return nil, err
	}

	var res spansResponse
	q := spansQuery(uids, traceID, window{req.StartTime, req.EndTime}, req.IncludeAttributes, desc, limit)
	if err := t.s.search(ctx, q, &res); err != nil {
		return nil, fmt.Errorf("opensearch.QueryTraceSpans: %w", err)
	}
	slog.Debug("opensearch span query", "took_ms", res.Took)

	spans := make([]observer.SpanInfo, 0, len(res.Hits.Hits))
	for _, h := range res.Hits.Hits {
		spans = append(spans, h.Source.toSpanInfo(req.IncludeAttributes))
	}
	return &observer.TraceSpansQueryResponse{
		Spans:  spans,
		Total:  res.Hits.Total.Value,
		TookMs: int(time.Since(began).Milliseconds()),
	}, nil
}

func (t *TraceReaderImpl) GetSpanDetails(ctx context.Context, traceID, spanID string) (*observer.SpanDetailsResponse, error) {
	if traceID == "" || spanID == "" {
		return nil, errors.New("opensearch.GetSpanDetails: trace id and span id are required")
	}
	var res spansResponse
	if err := t.s.search(ctx, detailQuery(t.namespace, traceID, spanID), &res); err != nil {
		return nil, fmt.Errorf("opensearch.GetSpanDetails: %w", err)
	}
	slog.Debug("opensearch detail query", "took_ms", res.Took)
	if len(res.Hits.Hits) == 0 {
		return nil, fmt.Errorf("opensearch.GetSpanDetails: span %q in trace %q: %w", spanID, traceID, ErrSpanNotFound)
	}
	d := res.Hits.Hits[0].Source
	start, end := d.StartTime.UTC(), d.EndTime.UTC()
	return &observer.SpanDetailsResponse{
		SpanID: d.SpanID, SpanName: d.Name, ParentSpanID: d.ParentSpanID,
		StartTime: start, EndTime: end, DurationNs: end.Sub(start).Nanoseconds(),
		// Kind is left empty on purpose: the existing path does not return it in span details either.
		Status:             &observer.SpanStatus{Code: strings.ToLower(d.Status.Code), Message: d.Status.Message},
		Attributes:         d.Attributes,
		ResourceAttributes: d.Resource,
	}, nil
}

func (t *TraceReaderImpl) QueryTraceSpansBatch(ctx context.Context, traceIDs []string, req observer.TracesQueryRequest) (map[string][]observer.SpanInfo, error) {
	out := make(map[string][]observer.SpanInfo, len(traceIDs))
	if len(traceIDs) == 0 {
		return out, nil
	}
	size := maxSpanLimit // 10,000 is also OpenSearch's default result window
	if req.Limit != nil && *req.Limit > 0 && *req.Limit < size {
		size = *req.Limit
	}
	uids, err := t.resolver.Resolve(ctx, req.SearchScope)
	if err != nil {
		return nil, err
	}
	var res spansResponse
	if err := t.s.search(ctx, batchQuery(uids, traceIDs, window{req.StartTime, req.EndTime}, size), &res); err != nil {
		return nil, fmt.Errorf("opensearch.QueryTraceSpansBatch: %w", err)
	}
	slog.Debug("opensearch batch span query", "took_ms", res.Took, "traces", len(traceIDs))
	if len(res.Hits.Hits) >= size {
		return nil, fmt.Errorf("opensearch.QueryTraceSpansBatch: row cap %d reached; the answer may be partial", size)
	}
	for _, h := range res.Hits.Hits {
		out[h.Source.TraceID] = append(out[h.Source.TraceID], h.Source.toSpanInfo(true))
	}
	return out, nil
}