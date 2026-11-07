package opensearchreader

import (
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

type window struct{ Start, End time.Time }

// traceFilters: a span counts only if it starts after the window start AND ends before the window end,
// and belongs to the requested scope. Same rule as the OpenChoreo adapter.
func traceFilters(sc scope.UIDs, w window) []map[string]any {
	f := []map[string]any{
		{"range": map[string]any{"startTime": map[string]any{"gte": w.Start.UTC().Format(time.RFC3339Nano)}}},
		{"range": map[string]any{"endTime": map[string]any{"lte": w.End.UTC().Format(time.RFC3339Nano)}}},
	}
	add := func(field, value string) {
		if value != "" {
			f = append(f, map[string]any{"term": map[string]any{field: value}})
		}
	}
	add("resource.openchoreo.dev/component-uid", sc.Component)
	add("resource.openchoreo.dev/environment-uid", sc.Environment)
	add("resource.openchoreo.dev/project-uid", sc.Project)
	add("resource.openchoreo.dev/namespace", sc.Namespace)
	return f
}

func topHits(size int, sortField, order string, source []string) map[string]any {
	return map[string]any{"top_hits": map[string]any{
		"size":    size,
		"sort":    []map[string]any{{sortField: map[string]any{"order": order}}},
		"_source": source,
	}}
}

// listQuery groups spans by trace id, so the limit counts traces, not spans.
func listQuery(sc scope.UIDs, w window, desc bool, limit int) map[string]any {
	order := "asc"
	if desc {
		order = "desc"
	}
	rootFilter := map[string]any{"term": map[string]any{"parentSpanId": ""}}
	return map[string]any{
		"size":  0,
		"query": map[string]any{"bool": map[string]any{"filter": traceFilters(sc, w)}},
		"aggs": map[string]any{
			// how many traces have their root span inside the window (the total)
			"trace_count": map[string]any{
				"filter": rootFilter,
				"aggs":   map[string]any{"distinct_traces": map[string]any{"cardinality": map[string]any{"field": "traceId"}}},
			},
			"traces": map[string]any{
				"terms": map[string]any{
					"field": "traceId", "size": limit,
					"order": map[string]any{"min_start_time": order},
				},
				"aggs": map[string]any{
					"earliest_span": topHits(1, "startTime", "asc", []string{"spanId", "name", "parentSpanId", "startTime", "kind"}),
					"root_span": map[string]any{
						"filter": rootFilter,
						"aggs": map[string]any{"hit": map[string]any{"top_hits": map[string]any{
							"size": 1, "_source": []string{"spanId", "name", "startTime", "kind"},
						}}},
					},
					"latest_span":    topHits(1, "endTime", "desc", []string{"endTime"}),
					"min_start_time": map[string]any{"min": map[string]any{"field": "startTime"}},
				},
			},
		},
	}
}

// spansQuery returns the spans of one trace.
// ASSUMPTION: sorted by start time, and attributes left out unless asked for. Check against queries.go lines 40-95.
func spansQuery(sc scope.UIDs, traceID string, w window, includeAttrs, desc bool, limit int) map[string]any {
	filters := append(traceFilters(sc, w), map[string]any{"term": map[string]any{"traceId": traceID}})
	order := "asc"
	if desc {
		order = "desc"
	}
	q := map[string]any{
		"size":             limit,
		"track_total_hits": true,
		"query":            map[string]any{"bool": map[string]any{"filter": filters}},
		"sort": []map[string]any{
			{"startTime": map[string]any{"order": order}},
			{"spanId": map[string]any{"order": "asc"}},
		},
	}
	if !includeAttrs {
		q["_source"] = map[string]any{"excludes": []string{"attributes", "resource"}}
	}
	return q
}

// detailQuery returns one span in full. The adapter filters on trace id and span id only; the
// namespace condition is the same safeguard the ClickHouse reader has.
func detailQuery(namespace, traceID, spanID string) map[string]any {
	return map[string]any{
		"size": 1,
		"query": map[string]any{"bool": map[string]any{"filter": []map[string]any{
			{"term": map[string]any{"traceId": traceID}},
			{"term": map[string]any{"spanId": spanID}},
			{"term": map[string]any{"resource.openchoreo.dev/namespace": namespace}},
		}}},
	}
}