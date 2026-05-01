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

package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	amsvc "github.com/wso2/agent-manager/internal/amctl/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/amctl/iostreams"
	"github.com/wso2/agent-manager/internal/amctl/render"
)

type capturedRequest struct {
	called      bool
	method      string
	path        string
	contentType string
	body        []byte
}

func newTestClient(t *testing.T, status int, body any) (func(context.Context) (*amsvc.ClientWithResponses, error), *capturedRequest, func()) {
	t.Helper()
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.called = true
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.contentType = r.Header.Get("Content-Type")
		if r.Body != nil {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
			} else {
				captured.body = raw
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			if err := json.NewEncoder(w).Encode(body); err != nil {
				t.Errorf("encode response: %v", err)
			}
		}
	}))
	client, err := amsvc.NewClientWithResponses(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("new client: %v", err)
	}
	return func(context.Context) (*amsvc.ClientWithResponses, error) { return client, nil }, captured, server.Close
}

func unreachableClient(context.Context) (*amsvc.ClientWithResponses, error) {
	return nil, errors.New("client should not be constructed")
}

type fakePrompter struct {
	confirmDeletionErr error
	confirmDeletionArg string
	calls              int
}

func (p *fakePrompter) ConfirmDeletion(required string) error {
	p.calls++
	p.confirmDeletionArg = required
	return p.confirmDeletionErr
}

func newTestIO(canPrompt bool) (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	io, _, out, errOut := iostreams.Test()
	io.SetTerminal(canPrompt, canPrompt, canPrompt)
	io.JSON = true
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
