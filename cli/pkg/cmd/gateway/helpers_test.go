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

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

type capturedRequest struct {
	called   bool
	method   string
	path     string
	rawQuery string
}

func newTestClient(t *testing.T, status int, body any) (func(context.Context) (*aigateway.ClientWithResponses, error), *capturedRequest, func()) {
	t.Helper()
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.called = true
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			if err := json.NewEncoder(w).Encode(body); err != nil {
				t.Errorf("encode response: %v", err)
			}
		}
	}))
	client, err := aigateway.NewClientWithResponses(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("new client: %v", err)
	}
	return func(context.Context) (*aigateway.ClientWithResponses, error) { return client, nil }, captured, server.Close
}

// newTestIO returns streams in JSON mode, where assertions read the envelope.
func newTestIO() (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	io, _, out, errOut := iostreams.Test()
	io.SetTerminal(true, true, true)
	io.JSON = true
	return io, out, errOut
}

// newTextTestIO returns streams in text mode with stdout marked as a terminal,
// which is what the empty-list notice keys off.
func newTextTestIO(stdoutTTY bool) (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	io, _, out, errOut := iostreams.Test()
	io.SetTerminal(stdoutTTY, stdoutTTY, stdoutTTY)
	return io, out, errOut
}

func decodeEnvelope(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("decode envelope: %v\nbody=%q", err, raw)
	}
	return m
}

func baseScope() render.Scope {
	return render.Scope{Instance: "default", Org: "acme"}
}

// testGatewayCmd builds a root → gateway cobra tree for tests that exercise flag
// parsing and validation through the RunE path.
func testGatewayCmd(t *testing.T, ios *iostreams.IOStreams, clientFn func(context.Context) (*aigateway.ClientWithResponses, error)) *cobra.Command {
	t.Helper()
	f := &cmdutil.Factory{
		IOStreams: ios,
		AIGateway: clientFn,
		Config: func() (*config.Config, error) {
			return &config.Config{
				CurrentInstance: "default",
				Instances: map[string]config.Instance{
					"default": {URL: "http://test", CurrentOrg: "acme"},
				},
			}, nil
		},
	}
	root := &cobra.Command{Use: "amctl", SilenceErrors: true, SilenceUsage: true}
	cmdutil.EnableOrgOverride(root, f)
	root.AddCommand(NewGatewayCmd(f))
	return root
}
