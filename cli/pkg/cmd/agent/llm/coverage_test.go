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
package llm

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

// testRoot builds root → llm with the persistent --org/--project flags the real CLI has.
func testRoot(ios *iostreams.IOStreams, client clientFn) *cobra.Command {
	f := &cmdutil.Factory{
		IOStreams: ios,
		AIGateway: client,
		Prompter:  &fakePrompter{},
		Config: func() (*config.Config, error) {
			return &config.Config{
				CurrentInstance: "default",
				Instances:       map[string]config.Instance{"default": {URL: "http://test", CurrentOrg: "acme"}},
			}, nil
		},
	}
	root := &cobra.Command{Use: "amctl", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().String("org", "", "")
	root.PersistentFlags().String("project", "", "")
	root.AddCommand(NewLLMCmd(f))
	return root
}

func failingClient(context.Context) (*aigateway.ClientWithResponses, error) {
	return nil, errors.New("no client")
}

var subcommandArgs = map[string][]string{
	"list":  {"list"},
	"get":   {"get", "--name", "primary"},
	"set":   {"set", "--name", "primary", "--provider", "openai"},
	"unset": {"unset", "--name", "primary", "--yes"},
}

func Test_commands_requireProject(t *testing.T) {
	for name, args := range subcommandArgs {
		t.Run(name, func(t *testing.T) {
			ios, _, _ := newTestIO(true)
			root := testRoot(ios, unreachableClient)
			root.SetArgs(append(append([]string{"llm"}, args...), "order-bot"))
			if err := root.Execute(); err == nil {
				t.Fatal("expected a missing-project error")
			}
		})
	}
}

func Test_commands_requireAgent(t *testing.T) {
	for name, args := range subcommandArgs {
		t.Run(name, func(t *testing.T) {
			ios, out, _ := newTestIO(true)
			root := testRoot(ios, unreachableClient)
			root.SetArgs(append(append([]string{"llm"}, args...), "--project", "triage"))
			if err := root.Execute(); err == nil {
				t.Fatal("expected a missing-agent error")
			}
			if !strings.Contains(out.String(), "agent is required") {
				t.Errorf("output = %q, want the missing-agent message", out.String())
			}
		})
	}
}

func Test_commands_runThroughCobra(t *testing.T) {
	routes := map[string]route{
		listPath:                      okJSON(listResponse(llmItem("primary", getUUID))),
		configPath("GET", getUUID):    okJSON(getResponseFixture()),
		configPath("PUT", getUUID):    okJSON(getResponseFixture()),
		configPath("DELETE", getUUID): {status: http.StatusNoContent},
	}
	for name, args := range subcommandArgs {
		t.Run(name, func(t *testing.T) {
			client, cleanup := newClient(t, routes)
			defer cleanup()
			ios, _, _ := newTestIO(true)
			root := testRoot(ios, client)
			root.SetArgs(append(append([]string{"llm"}, args...), "order-bot", "--project", "triage"))
			if err := root.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// runners invokes each command's run function against one client.
func runners(io *iostreams.IOStreams, client clientFn, agent string) map[string]func() error {
	ctx := context.Background()
	return map[string]func() error{
		"list": func() error {
			return runList(ctx, &ListOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: agent})
		},
		"get": func() error {
			o := newGetOpts(io, client)
			o.AgentName = agent
			return runGet(ctx, o)
		},
		"set": func() error {
			o := newSetOpts(io, client)
			o.AgentName, o.Provider = agent, "openai"
			return runSet(ctx, o)
		},
		"unset": func() error {
			o := newUnsetOpts(io, client)
			o.AgentName, o.Yes = agent, true
			return runUnset(ctx, o)
		},
	}
}

func Test_commands_rejectInvalidAgentName(t *testing.T) {
	ios, _, _ := newTestIO(true)
	for name, run := range runners(ios, unreachableClient, "bad/agent") {
		if err := run(); err == nil {
			t.Errorf("%s: expected an invalid-agent error", name)
		}
	}
}

func Test_commands_clientError(t *testing.T) {
	ios, _, _ := newTestIO(true)
	for name, run := range runners(ios, failingClient, "order-bot") {
		if err := run(); err == nil {
			t.Errorf("%s: expected the client error", name)
		}
	}
}

func Test_commands_listFails(t *testing.T) {
	cases := map[string]route{
		"transport": {transportErr: errors.New("drop")},
		"server":    {status: http.StatusInternalServerError, body: map[string]any{"code": "BOOM", "message": "boom"}},
	}
	for label, rt := range cases {
		client, cleanup := newClient(t, map[string]route{listPath: rt})
		ios, _, _ := newTestIO(true)
		for name, run := range runners(ios, client, "order-bot") {
			if err := run(); err == nil {
				t.Errorf("%s/%s: expected the list error", label, name)
			}
		}
		cleanup()
	}
}

func Test_runGet_transportError(t *testing.T) {
	client, cleanup := newClient(t, map[string]route{
		listPath:                   okJSON(listResponse(llmItem("primary", getUUID))),
		configPath("GET", getUUID): {transportErr: errors.New("drop")},
	})
	defer cleanup()
	ios, _, _ := newTestIO(true)
	if err := runGet(context.Background(), newGetOpts(ios, client)); err == nil {
		t.Fatal("expected a transport error")
	}
}

func Test_runSet_rejectsInvalidEnvVarNames(t *testing.T) {
	ios, _, _ := newTestIO(true)
	for _, flag := range []string{"url", "apikey"} {
		o := newSetOpts(ios, unreachableClient)
		o.Provider = "openai"
		if flag == "url" {
			o.URLEnv = "1-bad"
		} else {
			o.APIKeyEnv = "1-bad"
		}
		if err := runSet(context.Background(), o); err == nil {
			t.Errorf("--%s-env: expected a validation error", flag)
		}
	}
}

func Test_runSet_serverErrors(t *testing.T) {
	drop := route{transportErr: errors.New("drop")}
	conflict := route{status: http.StatusConflict, body: map[string]any{"code": "CONFLICT", "message": "exists"}}
	notFound := route{status: http.StatusNotFound, body: map[string]any{"code": "NOT_FOUND", "message": "gone"}}
	existing := okJSON(listResponse(llmItem("primary", getUUID)))
	cases := map[string]map[string]route{
		"create transport": {listPath: okJSON(listResponse()), postPath: drop},
		"create refused":   {listPath: okJSON(listResponse()), postPath: conflict},
		"read transport":   {listPath: existing, configPath("GET", getUUID): drop},
		"read refused":     {listPath: existing, configPath("GET", getUUID): notFound},
		"update transport": {listPath: existing, configPath("GET", getUUID): okJSON(getResponseFixture()), configPath("PUT", getUUID): drop},
		"update refused":   {listPath: existing, configPath("GET", getUUID): okJSON(getResponseFixture()), configPath("PUT", getUUID): notFound},
	}
	for label, routes := range cases {
		t.Run(label, func(t *testing.T) {
			client, cleanup := newClient(t, routes)
			defer cleanup()
			ios, _, _ := newTestIO(true)
			o := newSetOpts(ios, client)
			o.Provider = "openai"
			if err := runSet(context.Background(), o); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func Test_runUnset_deleteTransportError(t *testing.T) {
	client, cleanup := newClient(t, map[string]route{
		listPath:                      okJSON(listResponse(llmItem("primary", getUUID))),
		configPath("DELETE", getUUID): {transportErr: errors.New("drop")},
	})
	defer cleanup()
	ios, _, _ := newTestIO(true)
	o := newUnsetOpts(ios, client)
	o.Yes = true
	if err := runUnset(context.Background(), o); err == nil {
		t.Fatal("expected a transport error")
	}
}
