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

package card

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
)

const cardURL = "https://a.example/.well-known/agent-card.json"

func setOpts(client func(context.Context) (*amsvc.ClientWithResponses, error), jsonMode bool) (*SetOptions, func() (string, string)) {
	ios, out, errOut := newTestIO(jsonMode)
	return &SetOptions{
			IO: ios, Client: client, Scope: baseScope(),
			Org: "acme", Proj: "triage", AgentName: "order-bot", Env: "dev", URL: cardURL,
		},
		func() (string, string) { return out.String(), errOut.String() }
}

func Test_runSet_jsonEnvelope(t *testing.T) {
	client := newClient(t, map[string]route{
		"PUT " + envPath("dev", "agent-card/source"): {status: http.StatusAccepted},
		"GET " + envPath("dev", "a2a-proxy"):         {status: http.StatusNotFound, body: map[string]any{"code": "NOT_FOUND", "message": "none"}},
	})
	o, got := setOpts(client, true)
	if err := runSet(context.Background(), o); err != nil {
		t.Fatalf("runSet: %v", err)
	}
	out, errOut := got()
	data := decodeEnvelope(t, out)["data"].(map[string]any)
	if data["environment"] != "dev" || data["url"] != cardURL || data["routing"] != "direct" {
		t.Errorf("data = %v", data)
	}
	if errOut != "" {
		t.Errorf("errOut = %q, want empty", errOut)
	}
}

func Test_runSet_text(t *testing.T) {
	client := newClient(t, map[string]route{
		"PUT " + envPath("dev", "agent-card/source"): {status: http.StatusAccepted},
		"GET " + envPath("dev", "a2a-proxy"):         {status: http.StatusNotFound, body: map[string]any{"code": "NOT_FOUND", "message": "none"}},
		"PUT " + envPath("dev", "a2a-proxy"):         {status: http.StatusAccepted},
	})
	o, got := setOpts(client, false)
	o.Proxy = true
	if err := runSet(context.Background(), o); err != nil {
		t.Fatalf("runSet: %v", err)
	}
	_, errOut := got()
	for _, want := range []string{"dev", "proxied", "amctl agent card get"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("errOut %q missing %q", errOut, want)
		}
	}
}

func Test_runSet_proxyRejectsNonWellKnownURL(t *testing.T) {
	o, _ := setOpts(unreachableClient, true)
	o.Proxy = true
	o.URL = "https://a.example/card.json"
	err := runSet(context.Background(), o)
	var ce clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != clierr.InvalidFlag {
		t.Fatalf("err = %v, want INVALID_FLAG", err)
	}
}

func Test_runSet_partialFailureEnvelope(t *testing.T) {
	client := newClient(t, map[string]route{
		"PUT " + envPath("dev", "agent-card/source"): {status: http.StatusAccepted},
		"GET " + envPath("dev", "a2a-proxy"):         {status: http.StatusNotFound, body: map[string]any{"code": "NOT_FOUND", "message": "none"}},
		"PUT " + envPath("dev", "a2a-proxy"):         {status: http.StatusBadRequest, body: map[string]any{"code": "BAD", "message": "nope"}},
	})
	o, got := setOpts(client, true)
	o.Proxy = true
	if err := runSet(context.Background(), o); err == nil {
		t.Fatal("expected error")
	}
	out, _ := got()
	e := decodeEnvelope(t, out)["error"].(map[string]any)
	if ad, _ := e["additionalData"].(map[string]any); ad["urlSaved"] != true {
		t.Errorf("additionalData = %v, want urlSaved true", e["additionalData"])
	}
}
