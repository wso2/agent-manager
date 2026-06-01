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

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/auth"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func TestRunLogin_TypedErrorPassthrough(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.JSON = true

	opts := &LoginOptions{
		IO:  io,
		URL: "https://example.com",
		Authenticate: func(_ context.Context, _ auth.LoginOptions) (*config.Instance, error) {
			return nil, clierr.New(clierr.Unauthorized, "client credentials rejected (401)")
		},
	}

	err := runLogin(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	env := decodeLoginEnvelope(t, out.String())
	errBody, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing 'error' field: %v", env)
	}
	if got := errBody["code"]; got != clierr.Unauthorized {
		t.Errorf("code = %q, want %q (typed error must not be re-wrapped as Transport)", got, clierr.Unauthorized)
	}
}

func TestRunLogin_PlainErrorBecomesTransport(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.JSON = true

	opts := &LoginOptions{
		IO:  io,
		URL: "https://example.com",
		Authenticate: func(_ context.Context, _ auth.LoginOptions) (*config.Instance, error) {
			return nil, errors.New("dial tcp: connection refused")
		},
	}

	err := runLogin(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	env := decodeLoginEnvelope(t, out.String())
	errBody, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing 'error' field: %v", env)
	}
	if got := errBody["code"]; got != clierr.Transport {
		t.Errorf("code = %q, want %q (untyped errors must be wrapped as Transport)", got, clierr.Transport)
	}
}

func TestRunLogin_CancelledErrorPassthrough(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.JSON = true

	opts := &LoginOptions{
		IO:  io,
		URL: "https://example.com",
		Authenticate: func(_ context.Context, _ auth.LoginOptions) (*config.Instance, error) {
			return nil, clierr.New(clierr.AuthLoginCancelled, "browser login cancelled")
		},
	}

	err := runLogin(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	env := decodeLoginEnvelope(t, out.String())
	errBody, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing 'error' field: %v", env)
	}
	if got := errBody["code"]; got != clierr.AuthLoginCancelled {
		t.Errorf("code = %q, want %q", got, clierr.AuthLoginCancelled)
	}
}

func TestRunLogin_MissingURL(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.JSON = true

	opts := &LoginOptions{
		IO:  io,
		URL: "",
		Authenticate: func(_ context.Context, _ auth.LoginOptions) (*config.Instance, error) {
			t.Fatal("Authenticate should not be called when --url is missing")
			return nil, nil
		},
	}

	err := runLogin(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for missing --url")
	}

	env := decodeLoginEnvelope(t, out.String())
	errBody, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing 'error' field: %v", env)
	}
	if got := errBody["code"]; got != clierr.InvalidFlag {
		t.Errorf("code = %q, want %q", got, clierr.InvalidFlag)
	}
}

func decodeLoginEnvelope(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("decode envelope: %v\nbody=%q", err, raw)
	}
	return m
}
