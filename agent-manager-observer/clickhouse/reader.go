package clickhouse

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

var ErrSpanNotFound = errors.New("span not found")

const (
	defaultLimit     = 100
	maxLimit         = 1000
	defaultSpanLimit = 1000
	maxSpanLimit     = 10000
)

// A table name may only be letters, digits and underscores, with one optional dot.
var tableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

// Options says which table to read and how to turn names into ids.
type Options struct {
	Table            string
	Layout           string
	DefaultNamespace string
	Resolver         scope.Resolver
}

type TraceReaderImpl struct {
	db        *sql.DB
	table     string
	cols      tenantColumns
	namespace string
	resolver  scope.Resolver
}

var _ observer.TraceReader = (*TraceReaderImpl)(nil)

var _ observer.BatchTraceReader = (*TraceReaderImpl)(nil)

func NewTraceReader(db *sql.DB, opts Options) (*TraceReaderImpl, error) {
	if !tableNamePattern.MatchString(opts.Table) {
		return nil, fmt.Errorf("invalid trace table name %q", opts.Table)
	}
	var cols tenantColumns
	switch opts.Layout {
	case config.TraceLayoutExporter:
		cols = exporterTable
	case config.TraceLayoutTenant:
		cols = tenantTable
	default:
		return nil, fmt.Errorf("unknown trace table layout %q (use %q or %q)", opts.Layout, config.TraceLayoutExporter, config.TraceLayoutTenant)
	}
	if opts.Resolver == nil {
		return nil, errors.New("a scope resolver is required")
	}
	if strings.TrimSpace(opts.DefaultNamespace) == "" {
		return nil, errors.New("a default namespace is required")
	}
	return &TraceReaderImpl{db: db, table: opts.Table, cols: cols,
		namespace: opts.DefaultNamespace, resolver: opts.Resolver}, nil
}

func (t *TraceReaderImpl) Close() error {
	if t.db != nil {
		return t.db.Close()
	}
	return nil
}

// NamespaceFor resolves the namespace a trace query is scoped to.
func (t *TraceReaderImpl) NamespaceFor(_ string) string { return t.namespace }

// listOptions checks the limit and the sort order. A missing limit means 100.
func listOptions(req observer.TracesQueryRequest) (limit int, desc bool, err error) {
	limit = defaultLimit
	if req.Limit != nil {
		limit = *req.Limit
	}
	if limit < 1 || limit > maxLimit {
		return 0, false, fmt.Errorf("limit must be between 1 and %d, got %d", maxLimit, limit)
	}
	desc = true
	if req.SortOrder != nil {
		switch *req.SortOrder {
		case "asc":
			desc = false
		case "desc", "":
		default:
			return 0, false, fmt.Errorf("sortOrder must be asc or desc, got %q", *req.SortOrder)
		}
	}
	return limit, desc, nil
}

func spanListOptions(req observer.TracesQueryRequest) (limit int, desc bool, err error) {
	limit = defaultSpanLimit
	if req.Limit != nil {
		limit = *req.Limit
	}
	if limit < 1 || limit > maxSpanLimit {
		return 0, false, fmt.Errorf("limit must be between 1 and %d, got %d", maxSpanLimit, limit)
	}
	if req.SortOrder != nil {
		switch *req.SortOrder {
		case "desc":
			desc = true
		case "asc", "":
		default:
			return 0, false, fmt.Errorf("sortOrder must be asc or desc, got %q", *req.SortOrder)
		}
	}
	return limit, desc, nil
}

// QueryTraces fetches a page of traces matching the given request.
func (t *TraceReaderImpl) QueryTraces(ctx context.Context, req observer.TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	began := time.Now()

	limit, desc, err := listOptions(req)
	if err != nil {
		return nil, err
	}
	uids, err := t.resolver.Resolve(ctx, req.SearchScope)
	if err != nil {
		return nil, err
	}

	query, args := buildTraceListQuery(t.table, t.cols, uids,
		traceWindow{Start: req.StartTime, End: req.EndTime}, desc, limit)

	rows, err := t.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("clickhouse.QueryTraces: %w", err)
	}
	defer rows.Close()

	traces := make([]observer.TraceInfo, 0, limit)
	total := 0
	for rows.Next() {
		var (
			tr                         observer.TraceInfo
			start, end                 time.Time
			spanCount, roots, rowTotal uint64
		)
		// The order here must match the SELECT list in buildTraceListQuery.
		if err := rows.Scan(&tr.TraceID, &start, &end, &spanCount,
			&tr.RootSpanID, &tr.RootSpanName, &tr.RootSpanKind, &roots, &rowTotal); err != nil {
			return nil, fmt.Errorf("clickhouse.QueryTraces: read row: %w", err)
		}
		tr.StartTime = start.UTC()
		tr.EndTime = end.UTC()
		tr.DurationNs = end.Sub(start).Nanoseconds()
		tr.SpanCount = int(spanCount)
		tr.TraceName = tr.RootSpanName
		total = int(rowTotal) // the same number on every row
		traces = append(traces, tr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse.QueryTraces: %w", err)
	}

	return &observer.TracesQueryResponse{
		Traces: traces,
		Total:  total,
		TookMs: int(time.Since(began).Milliseconds()),
	}, nil
}

// QueryTraceSpans fetches the spans of one trace.
func (t *TraceReaderImpl) QueryTraceSpans(ctx context.Context, traceID string, req observer.TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	began := time.Now()

	if traceID == "" {
		return nil, errors.New("clickhouse.QueryTraceSpans: trace id is required")
	}
	limit, desc, err := spanListOptions(req)
	if err != nil {
		return nil, err
	}
	uids, err := t.resolver.Resolve(ctx, req.SearchScope)
	if err != nil {
		return nil, err
	}

	query, args := buildTraceSpansQuery(t.table, t.cols, traceID, uids,
		traceWindow{Start: req.StartTime, End: req.EndTime}, req.IncludeAttributes, desc, limit)

	rows, err := t.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("clickhouse.QueryTraceSpans: %w", err)
	}
	defer rows.Close()

	spans := make([]observer.SpanInfo, 0)
	total := 0
	for rows.Next() {
		var (
			sp        observer.SpanInfo
			start     time.Time
			duration  uint64
			code, msg string
			attrs     map[string]string
			resAttrs  map[string]string
			rowTotal  uint64
		)
		// The order here must match the SELECT list in buildTraceSpansQuery.
		dest := []any{&sp.SpanID, &sp.ParentSpanID, &sp.SpanName, &sp.Kind, &start, &duration, &code, &msg}
		if req.IncludeAttributes {
			dest = append(dest, &attrs, &resAttrs)
		}

		dest = append(dest, &rowTotal)

		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("clickhouse.QueryTraceSpans: read row: %w", err)
		}
		total = int(rowTotal)
		sp.StartTime = start.UTC()
		sp.EndTime = sp.StartTime.Add(time.Duration(duration))
		sp.DurationNs = int64(duration)
		sp.Status = &observer.SpanStatus{Code: strings.ToLower(code), Message: msg}
		if req.IncludeAttributes {
			sp.Attributes = attributesToAny(attrs)
			sp.ResourceAttributes = attributesToAny(resAttrs)
		}
		spans = append(spans, sp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse.QueryTraceSpans: %w", err)
	}

	return &observer.TraceSpansQueryResponse{
		Spans:  spans,
		Total:  total,
		TookMs: int(time.Since(began).Milliseconds()),
	}, nil
}

// GetSpanDetails fetches one span in full, including both attribute maps.
func (t *TraceReaderImpl) GetSpanDetails(ctx context.Context, traceID, spanID string) (*observer.SpanDetailsResponse, error) {
	if traceID == "" || spanID == "" {
		return nil, errors.New("clickhouse.GetSpanDetails: trace id and span id are required")
	}
	query, args := buildSpanDetailQuery(t.table, t.cols, t.namespace, traceID, spanID)

	var (
		out       observer.SpanDetailsResponse
		start     time.Time
		duration  uint64
		code, msg string
		attrs     map[string]string
		resAttrs  map[string]string
	)

	var kind string

	err := t.db.QueryRowContext(ctx, query, args...).Scan(
		&out.SpanID, &out.ParentSpanID, &out.SpanName, &kind,
		&start, &duration, &code, &msg, &attrs, &resAttrs)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("clickhouse.GetSpanDetails: span %q in trace %q: %w", spanID, traceID, ErrSpanNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("clickhouse.GetSpanDetails: %w", err)
	}

	out.StartTime = start.UTC()
	out.EndTime = out.StartTime.Add(time.Duration(duration))
	out.DurationNs = int64(duration)
	out.Status = &observer.SpanStatus{Code: strings.ToLower(code), Message: msg}
	out.Attributes = attributesToAny(attrs)
	out.ResourceAttributes = attributesToAny(resAttrs)
	return &out, nil
}

// QueryTraceSpansBatch returns the spans, with attributes, of several traces in one query.
func (t *TraceReaderImpl) QueryTraceSpansBatch(ctx context.Context, traceIDs []string, req observer.TracesQueryRequest) (map[string][]observer.SpanInfo, error) {
	out := make(map[string][]observer.SpanInfo, len(traceIDs))
	if len(traceIDs) == 0 {
		return out, nil
	}
	maxRows := maxSpanLimit
	if req.Limit != nil && *req.Limit > 0 && *req.Limit < maxRows {
		maxRows = *req.Limit
	}
	uids, err := t.resolver.Resolve(ctx, req.SearchScope)
	if err != nil {
		return nil, err
	}
	query, args := buildSpansBatchQuery(t.table, t.cols, traceIDs, uids,
		traceWindow{Start: req.StartTime, End: req.EndTime}, maxRows)

	rows, err := t.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("clickhouse.QueryTraceSpansBatch: %w", err)
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var (
			traceID   string
			sp        observer.SpanInfo
			start     time.Time
			duration  uint64
			code, msg string
			attrs     map[string]string
			resAttrs  map[string]string
		)
		if err := rows.Scan(&traceID, &sp.SpanID, &sp.ParentSpanID, &sp.SpanName, &sp.Kind,
			&start, &duration, &code, &msg, &attrs, &resAttrs); err != nil {
			return nil, fmt.Errorf("clickhouse.QueryTraceSpansBatch: read row: %w", err)
		}
		sp.StartTime = start.UTC()
		sp.EndTime = sp.StartTime.Add(time.Duration(duration))
		sp.DurationNs = int64(duration)
		sp.Status = &observer.SpanStatus{Code: strings.ToLower(code), Message: msg}
		sp.Attributes = attributesToAny(attrs)
		sp.ResourceAttributes = attributesToAny(resAttrs)
		out[traceID] = append(out[traceID], sp)
		n++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse.QueryTraceSpansBatch: %w", err)
	}
	if n >= maxRows {
		return nil, fmt.Errorf("clickhouse.QueryTraceSpansBatch: row cap %d reached; the answer may be partial", maxRows)
	}
	return out, nil
}
