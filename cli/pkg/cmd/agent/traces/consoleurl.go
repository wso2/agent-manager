// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package traces

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

// selectedTraceParam is the console's open trace, the trace ID of `agent trace`.
const selectedTraceParam = "selectedTrace"

// consoleTimeRanges are the console's timeRange presets (TraceListTimeRange).
var consoleTimeRanges = []string{"10m", "30m", "1h", "3h", "6h", "12h", "1d", "3d", "7d", "30d"}

// consoleParam maps one console trace-list URL param to the CLI flag that sets it.
type consoleParam struct {
	param, flag string
	// filter marks a filter, which the URL and its flag can't both set.
	filter bool
	// window marks timeRange, startTime and endTime, which the window flags replace together.
	window bool
	// def is the console's default, left out of links.
	def string
	// condition is the --condition that sets this filter.
	condition string
	// value is the param's value for o; "" leaves it out.
	value func(o *ListTracesOptions) string
	// flagValue turns the param's value into the flag's.
	flagValue func(v string) (string, error)
}

// consoleParams maps the console's trace-list URL params to CLI flags, in the
// order links write them. The filters follow TRACE_FILTER_KEYS in
// console/workspaces/pages/traces/src/traceFilters.ts; the window, sortOrder and
// limit follow Traces.Component.tsx.
var consoleParams = []consoleParam{
	{param: "status", flag: "status", filter: true, condition: conditionErrorStatus,
		value: func(o *ListTracesOptions) string { return o.Status }, flagValue: oneOf("error", "ok")},
	{param: "minDurationMs", flag: "min-duration", filter: true, condition: conditionHighLatency,
		value: func(o *ListTracesOptions) string { return formatInt(o.MinDurationMs) }, flagValue: millis},
	{param: "minTokens", flag: "min-tokens", filter: true, condition: conditionHighTokenUsage,
		value: func(o *ListTracesOptions) string { return formatInt(o.MinTokens) }, flagValue: count},
	{param: "minSpanCount", flag: "min-spans", filter: true, condition: conditionExcessiveSteps,
		value: func(o *ListTracesOptions) string { return formatInt(o.MinSpans) }, flagValue: count},
	{param: "model", flag: "model", filter: true,
		value: func(o *ListTracesOptions) string { return o.Model }, flagValue: text},
	{param: "conversationId", flag: "conversation", filter: true,
		value: func(o *ListTracesOptions) string { return o.ConversationID }, flagValue: text},
	{param: "missing", flag: "no-conversation", filter: true,
		value: func(o *ListTracesOptions) string { return ifSet(o.NoConversation, "conversationId") }, flagValue: missingField},
	{param: "tool", flag: "tool", filter: true,
		value: func(o *ListTracesOptions) string { return o.Tool }, flagValue: text},
	{param: "toolError", flag: "tool-error", filter: true,
		value: func(o *ListTracesOptions) string { return ifSet(o.ToolError, "true") }, flagValue: oneOf("true")},
	{param: "mcpServer", flag: "mcp-server", filter: true,
		value: func(o *ListTracesOptions) string { return o.MCPServer }, flagValue: text},
	{param: "timeRange", flag: "since", window: true, def: "1h",
		value: func(o *ListTracesOptions) string { return consoleTimeRange(o.Since) }, flagValue: oneOf(consoleTimeRanges...)},
	{param: "startTime", flag: "start", window: true,
		value: func(o *ListTracesOptions) string { return ifSet(consoleTimeRange(o.Since) == "", o.StartTime) }, flagValue: rfc3339},
	{param: "endTime", flag: "end", window: true,
		value: func(o *ListTracesOptions) string { return ifSet(consoleTimeRange(o.Since) == "", o.EndTime) }, flagValue: rfc3339},
	{param: "sortOrder", flag: "sort", def: "desc",
		value: func(o *ListTracesOptions) string { return o.SortOrder }, flagValue: oneOf("asc", "desc")},
	{param: "limit", flag: "limit", def: "10",
		value: func(o *ListTracesOptions) string { return strconv.Itoa(o.Limit) }, flagValue: limit},
}

// windowParams are the rows `agent trace` reads and writes.
func windowParams() []consoleParam {
	return slices.DeleteFunc(slices.Clone(consoleParams), func(p consoleParam) bool { return !p.window })
}

// consoleTimeRange is the console preset equal to since, or "" when none is.
func consoleTimeRange(since string) string {
	d, err := cmdutil.ParseDuration(since)
	if since == "" || err != nil {
		return ""
	}
	for _, r := range consoleTimeRanges {
		if rd, _ := cmdutil.ParseDuration(r); rd == d {
			return r
		}
	}
	return ""
}

// consoleLink is the console path and query for o's scope and params, plus traceID as selectedTrace.
func consoleLink(o *ListTracesOptions, params []consoleParam, traceID string) string {
	segments := []string{"", "org", o.Org, "project", o.Proj, "agents", o.AgentName, "environment", o.Env, "observability", "traces"}
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	var query []string
	add := func(param, value string) {
		query = append(query, url.QueryEscape(param)+"="+url.QueryEscape(value))
	}
	for _, p := range params {
		if v := p.value(o); v != "" && v != p.def {
			add(p.param, v)
		}
	}
	if traceID != "" {
		add(selectedTraceParam, traceID)
	}
	link := strings.Join(segments, "/")
	if len(query) > 0 {
		link += "?" + strings.Join(query, "&")
	}
	return link
}

// printConsoleLink prints link to stderr when it's set.
func printConsoleLink(ios *iostreams.IOStreams, link string) {
	if link != "" {
		fmt.Fprintln(ios.ErrOut, "Console path: "+link)
	}
}

// withConsoleURL returns data with consoleUrl added to its JSON object, or data when link is "".
func withConsoleURL(data any, link string) (any, error) {
	if link == "" {
		return data, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	fields["consoleUrl"], _ = json.Marshal(link)
	return fields, nil
}

// consoleURL is a console trace-list URL: the route's names, when it has them, and the query.
type consoleURL struct {
	org, proj, agent, env string
	hasPath               bool
	query                 url.Values
}

// consoleRoute is the trace list route after /org/:orgId (generated-route.map.ts).
var consoleRoute = []string{"project", "", "agents", "", "environment", "", "observability", "traces"}

// parseConsoleURL reads a full console URL, a path or a "?..." query. The route
// is found by its /org/ segment, so a host or base path in front is ignored.
func parseConsoleURL(raw string) (*consoleURL, error) {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "#"); i >= 0 {
		raw = raw[:i]
	}
	path, rawQuery, _ := strings.Cut(raw, "?")
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil, cmdutil.FlagErrorf("--from-url: the query can't be read: %v", err)
	}
	u := &consoleURL{query: query}
	segments := strings.Split(path, "/")
	if !slices.Contains(segments, "org") {
		return u, nil
	}
	for at, seg := range segments {
		if seg != "org" {
			continue
		}
		if names, ok := matchRoute(segments[at+1:]); ok {
			u.org, u.proj, u.agent, u.env, u.hasPath = names[0], names[1], names[2], names[3], true
			return u, nil
		}
	}
	return nil, notTraceList()
}

// matchRoute returns the org, project, agent and environment when route is the trace list route.
func matchRoute(route []string) ([]string, bool) {
	if len(route) < len(consoleRoute)+1 {
		return nil, false
	}
	names := make([]string, 0, 4)
	for i, want := range append([]string{""}, consoleRoute...) {
		seg, err := url.PathUnescape(route[i])
		if err != nil {
			return nil, false
		}
		switch {
		case want == "" && seg == "":
			return nil, false
		case want == "":
			names = append(names, seg)
		case seg != want:
			return nil, false
		}
	}
	return names, true
}

// notTraceList is the error for a path that isn't the console's trace list.
func notTraceList() error {
	return cmdutil.FlagErrorf("--from-url: not a console trace list URL; copy it from the agent's Traces page")
}

// applyScope sets --org, --project and --env from u's path, failing when a set flag differs.
func (u *consoleURL) applyScope(fs *pflag.FlagSet) error {
	if !u.hasPath {
		return nil
	}
	for _, s := range []struct{ flag, name, value string }{
		{"org", "org", u.org},
		{"project", "project", u.proj},
		{"env", "environment", u.env},
	} {
		if fs.Lookup(s.flag) == nil {
			continue
		}
		if fs.Changed(s.flag) {
			if got, _ := fs.GetString(s.flag); got != s.value {
				return cmdutil.FlagErrorf("--from-url is for %s %q, but --%s is %q", s.name, s.value, s.flag, got)
			}
			continue
		}
		if err := fs.Set(s.flag, s.value); err != nil {
			return err
		}
	}
	return nil
}

// agentArg returns args with u's agent first, failing when args names another.
func (u *consoleURL) agentArg(args []string) ([]string, error) {
	if !u.hasPath {
		return args, nil
	}
	if len(args) > 0 && args[0] != u.agent {
		return nil, cmdutil.FlagErrorf("--from-url is for agent %q, but the command names agent %q", u.agent, args[0])
	}
	return append([]string{u.agent}, args[min(len(args), 1):]...), nil
}

// traceArgs is agentArg for `agent trace`: with a URL path, one arg is the trace ID unless it's the URL's agent.
func (u *consoleURL) traceArgs(args []string) ([]string, error) {
	if !u.hasPath || len(args) > 1 || (len(args) == 1 && args[0] == u.agent) {
		return u.agentArg(args)
	}
	return append([]string{u.agent}, args...), nil
}

// applyQuery sets cmd's flags from u's params. A filter its flag also sets is an
// error, as is one the --condition sets; the window, sort and limit flags win.
// With no window from the URL or the flags, the window is the console's 1h.
func (u *consoleURL) applyQuery(fs *pflag.FlagSet, params []consoleParam, condition string) error {
	windowByFlag := fs.Changed("since") || fs.Changed("start") || fs.Changed("end")
	custom := u.get("startTime") != "" || u.get("endTime") != ""
	windowSet := windowByFlag
	for _, p := range params {
		v := u.get(p.param)
		if v == "" || (p.window && windowByFlag) || (p.param == "timeRange" && custom) {
			continue
		}
		if fs.Changed(p.flag) {
			if p.filter {
				return cmdutil.FlagErrorf("--from-url and --%s both set %s; use one", p.flag, p.param)
			}
			continue
		}
		if p.condition != "" && p.condition == condition {
			return cmdutil.FlagErrorf("--from-url and --condition %s both set %s; use one", condition, p.param)
		}
		fv, err := p.flagValue(v)
		if err == nil {
			err = fs.Set(p.flag, fv)
		}
		if err != nil {
			return cmdutil.FlagErrorf("--from-url: %s=%q: %v", p.param, v, err)
		}
		windowSet = windowSet || p.window
	}
	if custom && !windowByFlag {
		if err := checkCustomRange(u.get("startTime"), u.get("endTime")); err != nil {
			return err
		}
	}
	if !windowSet && fs.Lookup("since") != nil {
		return fs.Set("since", "1h")
	}
	return nil
}

// checkCustomRange checks the URL's startTime and endTime are both set and in order.
func checkCustomRange(start, end string) error {
	if start == "" || end == "" {
		return cmdutil.FlagErrorf("--from-url: startTime and endTime must be set together")
	}
	s, _ := time.Parse(time.RFC3339, start)
	e, _ := time.Parse(time.RFC3339, end)
	if !s.Before(e) {
		return cmdutil.FlagErrorf("--from-url: startTime must be before endTime")
	}
	return nil
}

// warnUnknown prints the URL params the CLI doesn't read, so a dropped filter isn't silent.
func (u *consoleURL) warnUnknown(w io.Writer) {
	var unknown []string
	for param := range u.query {
		known := param == selectedTraceParam || slices.ContainsFunc(consoleParams, func(p consoleParam) bool { return p.param == param })
		if !known {
			unknown = append(unknown, param)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		fmt.Fprintf(w, "Warning: --from-url ignored params the CLI doesn't support: %s\n", strings.Join(unknown, ", "))
	}
}

// get returns the param's first value, trimmed.
func (u *consoleURL) get(param string) string {
	return strings.TrimSpace(u.query.Get(param))
}

// wholeNumber matches the non-negative integers the console and the API take.
var wholeNumber = regexp.MustCompile(`^\d+$`)

// oneOf accepts only the given values.
func oneOf(values ...string) func(string) (string, error) {
	return func(v string) (string, error) {
		if slices.Contains(values, v) {
			return v, nil
		}
		if len(values) > 2 {
			return "", fmt.Errorf("must be one of %s", strings.Join(values, ", "))
		}
		return "", fmt.Errorf("must be %s", strings.Join(values, " or "))
	}
}

// text passes a text filter through.
func text(v string) (string, error) { return v, nil }

// count accepts a whole number, at least 0.
func count(v string) (string, error) {
	if !wholeNumber.MatchString(v) {
		return "", errors.New("must be a whole number, at least 0")
	}
	return v, nil
}

// millis turns whole milliseconds into a duration.
func millis(v string) (string, error) {
	if _, err := count(v); err != nil {
		return "", err
	}
	if n, err := strconv.ParseInt(v, 10, 64); err != nil || n > math.MaxInt64/int64(time.Millisecond) {
		return "", errors.New("is too large")
	}
	return v + "ms", nil
}

// missingField accepts conversationId, the only field the console filters as missing.
func missingField(v string) (string, error) {
	for _, field := range strings.Split(v, ",") {
		if field != "conversationId" {
			return "", errors.New("only conversationId can be missing")
		}
	}
	return "true", nil
}

// rfc3339 accepts an ISO time, as the console writes it.
func rfc3339(v string) (string, error) {
	if _, err := time.Parse(time.RFC3339, v); err != nil {
		return "", errors.New("must be an ISO time, like 2026-10-14T09:00:00Z")
	}
	return v, nil
}

// limit accepts the page sizes --limit takes.
func limit(v string) (string, error) {
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 100 {
		return "", errors.New("must be between 1 and 100")
	}
	return v, nil
}

// ifSet returns v when set is true.
func ifSet(set bool, v string) string {
	if set {
		return v
	}
	return ""
}

// formatInt formats v, or "" when it's nil.
func formatInt(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

// applyFromURL sets the list's scope and flags from a console URL and returns args with its agent.
func applyFromURL(cmd *cobra.Command, errOut io.Writer, raw string, args []string, condition string) ([]string, error) {
	u, err := parseConsoleURL(raw)
	if err != nil {
		return nil, err
	}
	if err := u.applyScope(cmd.Flags()); err != nil {
		return nil, err
	}
	if args, err = u.agentArg(args); err != nil {
		return nil, err
	}
	u.warnUnknown(errOut)
	return args, u.applyQuery(cmd.Flags(), consoleParams, condition)
}

// addConsoleFlags adds --console-url and --from-url.
func addConsoleFlags(cmd *cobra.Command, consoleURL *bool, fromURL *string) {
	cmd.Flags().BoolVar(consoleURL, "console-url", false, "Print the console path that shows this view; paste it after your console's host")
	cmd.Flags().StringVar(fromURL, "from-url", "", "Take the scope, filters and window from a console URL, path or \"?...\" query")
}
