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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
)

// filterFlagValues holds the filter flags that need Changed to tell unset from zero.
type filterFlagValues struct {
	minDuration time.Duration
	minTokens   int64
	minSpans    int64
}

// addFilterFlags adds the filter flags list and export share. list adds the
// column each filter implies to its help text.
func addFilterFlags(cmd *cobra.Command, o *ListTracesOptions, v *filterFlagValues, list bool) {
	implies := func(flag string) string {
		if !list {
			return ""
		}
		return " (implies " + flag + ")"
	}
	fl := cmd.Flags()
	fl.StringVar(&o.Status, "status", "", "Only traces with this status: error or ok")
	fl.DurationVar(&v.minDuration, "min-duration", 0, "Only traces lasting at least this long (e.g. 500ms, 30s)")
	fl.Int64Var(&v.minTokens, "min-tokens", 0, "Only traces using at least this many tokens")
	fl.Int64Var(&v.minSpans, "min-spans", 0, "Only traces with at least this many spans")
	fl.StringVar(&o.Model, "model", "", "Only traces where a model name contains this value, ignoring case"+implies("--show-models"))
	fl.StringVar(&o.ConversationID, "conversation", "", "Only traces with exactly this conversation ID")
	fl.BoolVar(&o.NoConversation, "no-conversation", false, "Only traces with no conversation ID")
	fl.StringVar(&o.Tool, "tool", "", "Only traces that called a tool whose name contains this value, ignoring case"+implies("--show-tools"))
	fl.BoolVar(&o.ToolError, "tool-error", false, "Only traces with a failed tool, by the tool span's OTel status; with --tool, that tool failed"+implies("--show-tools"))
	fl.StringVar(&o.MCPServer, "mcp-server", "", "Only traces that used an MCP server whose name contains this value, ignoring case"+implies("--show-mcp-servers"))
}

// applyFilterFlags sets o's optional filters from the flags changed reports as set.
func applyFilterFlags(changed func(string) bool, o *ListTracesOptions, v *filterFlagValues) {
	if changed("min-duration") {
		o.MinDurationMs = int64Ptr(v.minDuration.Milliseconds())
	}
	if changed("min-tokens") {
		o.MinTokens = &v.minTokens
	}
	if changed("min-spans") {
		o.MinSpans = &v.minSpans
	}
}

// windowFlags holds --since, --start and --end.
type windowFlags struct {
	since, start, end string
}

// addWindowFlags adds --start and --end; each command adds its own --since.
func addWindowFlags(cmd *cobra.Command, w *windowFlags) {
	cmd.Flags().StringVar(&w.start, "start", "", "Window start, RFC 3339 (e.g. 2026-10-14T09:00:00Z); needs --end, not with --since")
	cmd.Flags().StringVar(&w.end, "end", "", "Window end, RFC 3339; needs --start")
}

// resolveWindow returns the --start and --end window, or else the --since window
// ending now. sinceSet says --since was given, not just defaulted.
func resolveWindow(w *windowFlags, sinceSet bool) (start, end string, err error) {
	if w.start == "" && w.end == "" {
		if w.since == "" {
			return "", "", cmdutil.FlagErrorf("--since, or --start and --end, is required")
		}
		start, end, err := cmdutil.ResolveSinceWindow(w.since)
		if err != nil {
			return "", "", cmdutil.FlagErrorf("--since: %v", err)
		}
		return start, end, nil
	}
	if sinceSet {
		return "", "", cmdutil.FlagErrorf("--since can't be used with --start and --end")
	}
	if w.start == "" || w.end == "" {
		return "", "", cmdutil.FlagErrorf("--start and --end must be set together")
	}
	s, err := time.Parse(time.RFC3339, w.start)
	if err != nil {
		return "", "", cmdutil.FlagErrorf("--start: %q is not an RFC 3339 time", w.start)
	}
	e, err := time.Parse(time.RFC3339, w.end)
	if err != nil {
		return "", "", cmdutil.FlagErrorf("--end: %q is not an RFC 3339 time", w.end)
	}
	if !s.Before(e) {
		return "", "", cmdutil.FlagErrorf("--start must be before --end")
	}
	return s.UTC().Format(time.RFC3339), e.UTC().Format(time.RFC3339), nil
}

// nextPageCommand is the command that lists the page after cursor: o's scope,
// window, sort, limit, filters and columns, with the window pinned.
func nextPageCommand(o *ListTracesOptions, cursor string) string {
	path := o.CommandPath
	if path == "" {
		path = "amctl agent traces"
	}
	args := []string{path, shellQuote(o.AgentName)}
	add := func(flag, val string) {
		if val != "" {
			args = append(args, flag, shellQuote(val))
		}
	}
	addBool := func(flag string, set bool) {
		if set {
			args = append(args, flag)
		}
	}
	addInt := func(flag string, v *int64) {
		if v != nil {
			args = append(args, flag, strconv.FormatInt(*v, 10))
		}
	}
	add("--org", o.Org)
	add("--project", o.Proj)
	add("--env", o.Env)
	add("--start", o.StartTime)
	add("--end", o.EndTime)
	add("--sort", o.SortOrder)
	add("--limit", strconv.Itoa(o.Limit))
	add("--status", o.Status)
	if o.MinDurationMs != nil {
		add("--min-duration", (time.Duration(*o.MinDurationMs) * time.Millisecond).String())
	}
	addInt("--min-tokens", o.MinTokens)
	addInt("--min-spans", o.MinSpans)
	add("--model", o.Model)
	add("--conversation", o.ConversationID)
	addBool("--no-conversation", o.NoConversation)
	add("--tool", o.Tool)
	addBool("--tool-error", o.ToolError)
	add("--mcp-server", o.MCPServer)
	addBool("--show-models", o.ShowModels)
	addBool("--show-tools", o.ShowTools)
	addBool("--show-mcp-servers", o.ShowMCPServers)
	addBool("--console-url", o.ShowConsoleURL)
	add("--cursor", cursor)
	return strings.Join(args, " ")
}

// shellSafe matches values a POSIX shell reads as one word without quotes.
var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellQuote quotes s for a POSIX shell when it needs it.
func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
