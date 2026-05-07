package traceobserversvc

// TraceListParams holds query parameters for listing/exporting traces.
type TraceListParams struct {
	Organization string
	Project      string
	Component    string
	Environment  string
	StartTime    string
	EndTime      string
	Limit        int
	Offset       int
	SortOrder    string
}

// TraceDetailsParams holds query parameters for fetching a specific trace.
type TraceDetailsParams struct {
	TraceID      string
	Organization string
	Project      string
	Component    string
	Environment  string
	SortOrder    string
	Limit        int
	StartTime    string
	EndTime      string
	ParentSpan   *bool
}

// SpanDetailsParams holds query parameters for fetching a specific span in a trace.
type SpanDetailsParams struct {
	TraceID      string
	SpanID       string
	Organization string
	Project      string
	Component    string
	Environment  string
}
