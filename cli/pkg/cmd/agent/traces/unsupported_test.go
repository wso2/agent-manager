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
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/clients/observersvc"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
)

// allowAllFlags lifts the observability module guard for t, so tests of the
// observer's filters keep covering the request code the module will serve.
func allowAllFlags(t *testing.T) {
	t.Helper()
	flags, conditions := moduleUnsupportedFlags, moduleUnsupportedConditions
	moduleUnsupportedFlags, moduleUnsupportedConditions = nil, nil
	t.Cleanup(func() { moduleUnsupportedFlags, moduleUnsupportedConditions = flags, conditions })
}

// unsupportedFlagArgs sets each flag in moduleUnsupportedFlags.
var unsupportedFlagArgs = map[string][]string{
	"status":           {"--status", "error"},
	"min-duration":     {"--min-duration", "1s"},
	"min-tokens":       {"--min-tokens", "10"},
	"min-spans":        {"--min-spans", "3"},
	"model":            {"--model", "gpt"},
	"conversation":     {"--conversation", "c-1"},
	"no-conversation":  {"--no-conversation"},
	"tool":             {"--tool", "search"},
	"tool-error":       {"--tool-error"},
	"mcp-server":       {"--mcp-server", "github"},
	"cursor":           {"--cursor", "c"},
	"show-models":      {"--show-models"},
	"show-tools":       {"--show-tools"},
	"show-mcp-servers": {"--show-mcp-servers"},
	"max-latency":      {"--max-latency", "100"},
	"max-tokens":       {"--max-tokens", "100"},
	"max-spans":        {"--max-spans", "5"},
}

// exportFlags are the unsupported flags export has.
var exportFlags = []string{
	"status", "min-duration", "min-tokens", "min-spans", "model", "conversation", "no-conversation",
	"tool", "tool-error", "mcp-server",
}

func TestTracesCmd_RefusesUnsupportedFlagsBeforeAnyRequest(t *testing.T) {
	for _, name := range moduleUnsupportedFlags {
		for _, cmd := range []struct {
			name string
			base []string
		}{{"list", listArgs}, {"export", append(slices.Clone(exportArgs), "--since", "1h")}} {
			if cmd.name == "export" && !slices.Contains(exportFlags, name) {
				continue
			}
			t.Run(cmd.name+"/"+name, func(t *testing.T) {
				args, ok := unsupportedFlagArgs[name]
				if !ok {
					t.Fatalf("no args for --%s", name)
				}
				run := runAmctl(t, serve(observersvc.TraceOverviewResponse{}), false, append(slices.Clone(cmd.base), args...)...)
				var flagErr *cmdutil.FlagError
				if !errors.As(run.err, &flagErr) {
					t.Fatalf("err = %v, want a flag error", run.err)
				}
				if want := "--" + name + " is not supported by the observability module yet"; !strings.Contains(run.err.Error(), want) {
					t.Errorf("err = %q, want it to contain %q", run.err, want)
				}
				if len(run.requests) != 0 {
					t.Errorf("made %d requests despite an unsupported flag", len(run.requests))
				}
			})
		}
	}
}

func TestTracesCmd_RefusesFilterConditions(t *testing.T) {
	for _, condition := range moduleUnsupportedConditions {
		t.Run(condition, func(t *testing.T) {
			run := runAmctl(t, serve(observersvc.TraceOverviewResponse{}), false, append(slices.Clone(listArgs), "--condition", condition)...)
			want := "--condition " + condition + " is not supported by the observability module yet"
			if run.err == nil || !strings.Contains(run.err.Error(), want) {
				t.Errorf("err = %v, want it to contain %q", run.err, want)
			}
			if len(run.requests) != 0 {
				t.Errorf("made %d requests despite an unsupported condition", len(run.requests))
			}
		})
	}
}

// A console URL with a filter is refused like the filter's flag.
func TestFromURL_RefusesAFilter(t *testing.T) {
	run := runAmctl(t, serve(observersvc.TraceOverviewResponse{}), false, "traces", "--from-url",
		"/org/acme/project/triage/agents/my-agent/environment/default/observability/traces?status=error")
	if run.err == nil || !strings.Contains(run.err.Error(), "--status is not supported by the observability module yet") {
		t.Errorf("err = %v, want --status refused", run.err)
	}
	if len(run.requests) != 0 {
		t.Errorf("made %d requests despite a filter in the URL", len(run.requests))
	}
}

// tool_call_fails is checked in the CLI over exported spans, which the module serves.
func TestTracesCmd_ToolCallFailsOnTheModule(t *testing.T) {
	export := observersvc.TraceExportResponse{Traces: []observersvc.FullTrace{{
		TraceOverview: observersvc.TraceOverview{TraceID: "t-fail", RootSpanID: "root"},
		Spans: []observersvc.Span{
			{SpanID: "root"},
			{SpanID: "tool", AmpAttributes: &observersvc.AmpAttributes{Kind: "tool", Status: &observersvc.SpanStatus{Error: true}}},
		},
	}}}
	run := runAmctl(t, serve(export), false, append(slices.Clone(listArgs), "--condition", "tool_call_fails")...)
	if run.err != nil {
		t.Fatalf("unexpected error: %v", run.err)
	}
	if len(run.paths) != 1 || run.paths[0] != "/api/v1/traces/export" {
		t.Errorf("paths = %v, want one export", run.paths)
	}
	if !strings.Contains(run.out, "t-fail") {
		t.Errorf("out = %q, want t-fail", run.out)
	}
}

func TestTracesCmd_HidesUnsupportedFlags(t *testing.T) {
	ios, _, _ := newTraceTestIO(false)
	f := &cmdutil.Factory{IOStreams: ios}
	list := NewTracesCmd(f)
	export, _, err := list.Find([]string{"export"})
	if err != nil {
		t.Fatalf("find export: %v", err)
	}
	for _, cmd := range []*cobra.Command{list, export} {
		usage := cmd.Flags().FlagUsages()
		for _, name := range moduleUnsupportedFlags {
			if fl := cmd.Flags().Lookup(name); fl != nil && !fl.Hidden {
				t.Errorf("%s: --%s is not hidden", cmd.Name(), name)
			}
			if strings.Contains(usage, "--"+name+" ") {
				t.Errorf("%s: help lists --%s", cmd.Name(), name)
			}
		}
		if !strings.Contains(usage, "--since") || !strings.Contains(usage, "--limit") {
			t.Errorf("%s: help is missing --since or --limit:\n%s", cmd.Name(), usage)
		}
	}
	if !strings.Contains(list.Flags().FlagUsages(), "--condition") {
		t.Error("list help is missing --condition")
	}
}

// A disabled module's routes answer the host's bare 404; the module's own 404s name what's missing.
func TestTracesCmd_ModuleNotEnabled(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{name: "module off", body: `{"message":"Not found"}`, want: "the observability module is not enabled on this server"},
		{name: "agent missing", body: `{"error":"error","message":"agent not found"}`, want: "agent not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := runAmctl(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(tc.body))
			}, false, listArgs...)
			if run.err == nil || !strings.Contains(run.err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", run.err, tc.want)
			}
		})
	}
}
