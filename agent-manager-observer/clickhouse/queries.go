package clickhouse

import (
	"time"

	"strings"
	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

const traceIDTimeTable = "otel_traces_trace_id_ts"
const traceRootsTable = "otel.otel_trace_roots"

type traceWindow struct{ Start, End time.Time }

func buildTraceListQuery(table string, cols tenantColumns, scope scope.UIDs, window traceWindow, isDesc bool, limit int) (string, []any) {
	// 1. Where clause for raw spans table
	where, scopeArgs := scopeWhere(cols, scope)
	if where != "" {
		where += " AND "
	}
	where += "Timestamp >= fromUnixTimestamp64Nano(?, 'UTC') " +
		"AND Timestamp <= fromUnixTimestamp64Nano(?, 'UTC') " +
		"AND addNanoseconds(Timestamp, Duration) <= fromUnixTimestamp64Nano(?, 'UTC')"

	// 2. Where clause for the narrow roots table (direct tenant columns)
	whereRoots, scopeArgsRoots := scopeWhere(cols, scope)
	if whereRoots != "" {
		whereRoots += " AND "
	}
	whereRoots += "Timestamp >= fromUnixTimestamp64Nano(?, 'UTC') " +
		"AND Timestamp <= fromUnixTimestamp64Nano(?, 'UTC') " +
		"AND addNanoseconds(Timestamp, Duration) <= fromUnixTimestamp64Nano(?, 'UTC')"

	timeArgs := []any{window.Start.UnixNano(), window.End.UnixNano(), window.End.UnixNano()}

	order := "ASC"
	if isDesc {
		order = "DESC"
	}

	var args []any
	// 1. Args for exact_total (reading otel_trace_roots)
	args = append(args, scopeArgsRoots...)
	args = append(args, timeArgs...)

	// 2. Args for IN subquery (reading top TraceIds from otel_trace_roots)
	args = append(args, scopeArgsRoots...)
	args = append(args, timeArgs...)
	args = append(args, limit)

	// 3. Args for outer aggregation (reading only matching spans from table)
	args = append(args, scopeArgs...)
	args = append(args, timeArgs...)

	query := `WITH (
		SELECT count() FROM ` + traceRootsTable + ` 
		WHERE ` + whereRoots + ` AND ParentSpanId = ''
	) AS exact_total
	SELECT TraceId,
	min(Timestamp) AS start_time,
	max(addNanoseconds(Timestamp, Duration)) AS end_time,
	uniqExact(SpanId) AS span_count,
	argMinIf(SpanId,   Timestamp, ParentSpanId = '') AS root_span_id,
	argMinIf(SpanName, Timestamp, ParentSpanId = '') AS root_span_name,
	argMinIf(SpanKind, Timestamp, ParentSpanId = '') AS root_span_kind,
	countIf(ParentSpanId = '') AS roots,
	exact_total AS total
	FROM ` + table + ` 
	WHERE TraceId IN (
		SELECT TraceId FROM ` + traceRootsTable + ` 
		WHERE ` + whereRoots + ` AND ParentSpanId = '' 
		ORDER BY Timestamp ` + order + `, TraceId ASC 
		LIMIT ?
	)
	AND ` + where + `
	GROUP BY TraceId HAVING roots > 0
	ORDER BY start_time ` + order + `, TraceId ASC`

	return query, args
}

func buildTraceSpansQuery(table string, cols tenantColumns, traceID string, scope scope.UIDs, w traceWindow, includeAttrs, desc bool, limit int) (string, []any) {

	conds := "TraceId = ?"
	args := []any{traceID}

	if scopeSQL, scopeArgs := scopeWhere(cols, scope); scopeSQL != "" {
		conds += " AND " + scopeSQL
		args = append(args, scopeArgs...)
	}

	conds += " AND Timestamp >= fromUnixTimestamp64Nano(?, 'UTC') " +
		"AND Timestamp <= fromUnixTimestamp64Nano(?, 'UTC') " +
		"AND addNanoseconds(Timestamp, Duration) <= fromUnixTimestamp64Nano(?, 'UTC')"

	args = append(args, w.Start.UnixNano(), w.End.UnixNano(), w.End.UnixNano())

	columns := "SpanId, ParentSpanId, SpanName, SpanKind, Timestamp, Duration, StatusCode, StatusMessage"

	if includeAttrs {
		columns += ", SpanAttributes, ResourceAttributes"
	}

	columns += ", uniqExact(SpanId) OVER () AS total_spans"

	order := "ASC"
	if desc {
		order = "DESC"
	}

	query := "SELECT " + columns + " FROM " + table +
		" WHERE " + conds +
		" ORDER BY Timestamp " + order + ", SpanId ASC LIMIT 1 BY SpanId LIMIT ?"
	args = append(args, limit)
	return query, args
}

func buildSpanDetailQuery(table string, cols tenantColumns, namespace, traceID, spanID string) (string, []any) {
	columns := "SpanId, ParentSpanId, SpanName, SpanKind, Timestamp, Duration, StatusCode, StatusMessage, SpanAttributes, ResourceAttributes"

	where := `TraceId = ? AND SpanId = ? AND ` + cols.ns + ` = ? 
  	AND Timestamp >= (SELECT min(Start) - 1 FROM ` + traceIDTimeTable + ` WHERE TraceId = ?)
 	AND Timestamp <= (SELECT max(End)   + 1 FROM ` + traceIDTimeTable + ` WHERE TraceId = ?)`

	args := []any{traceID, spanID, namespace, traceID, traceID}

	query := `SELECT ` + columns + `
	FROM ` + table + `
	WHERE ` + where + `
	LIMIT 1`

	return query, args
}

// buildSpansBatchQuery returns the spans of several traces in one query.
func buildSpansBatchQuery(table string, cols tenantColumns, traceIDs []string, scope scope.UIDs,
	w traceWindow, maxRows int) (string, []any) {

	marks := strings.TrimSuffix(strings.Repeat("?,", len(traceIDs)), ",") // one ? per trace id, never the ids themselves
	args := make([]any, 0, len(traceIDs)+8)
	for _, id := range traceIDs {
		args = append(args, id)
	}
	conds := []string{"TraceId IN (" + marks + ")"}
	if scopeSQL, scopeArgs := scopeWhere(cols, scope); scopeSQL != "" {
		conds = append(conds, scopeSQL)
		args = append(args, scopeArgs...)
	}
	conds = append(conds,
		"Timestamp >= fromUnixTimestamp64Nano(?, 'UTC')",
		"Timestamp <= fromUnixTimestamp64Nano(?, 'UTC')",
		"addNanoseconds(Timestamp, Duration) <= fromUnixTimestamp64Nano(?, 'UTC')")
	args = append(args, w.Start.UnixNano(), w.End.UnixNano(), w.End.UnixNano())

	query := "SELECT TraceId, SpanId, ParentSpanId, SpanName, SpanKind, Timestamp, Duration, StatusCode, StatusMessage, SpanAttributes, ResourceAttributes" +
		" FROM " + table + " WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY TraceId, Timestamp, SpanId LIMIT 1 BY TraceId, SpanId LIMIT ?"
	args = append(args, maxRows)
	return query, args
}
