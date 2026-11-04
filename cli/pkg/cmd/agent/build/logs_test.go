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

package build

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clients/observersvc"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

// missingBuildName is the build the test agent-manager server reports as not found.
const missingBuildName = "build-missing"

// newBuildTestClient serves the GetAgent (buildable check), GetAgentBuilds
// (latest-build resolution) and GetBuild (explicit-build existence check)
// preconditions used by build/logs.go, so callers only exercise the Observer
// client under test.
func newBuildTestClient(t *testing.T, latestBuildName string) func(context.Context) (*amsvc.ClientWithResponses, error) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/builds/"+missingBuildName):
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(amsvc.ErrorResponse{Code: "BUILD_NOT_FOUND", Message: "Build not found"})
		case strings.Contains(r.URL.Path, "/builds/"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(amsvc.BuildDetailsResponse{AgentName: "my-agent", BuildName: path.Base(r.URL.Path)})
		case strings.HasSuffix(r.URL.Path, "/builds"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(amsvc.BuildsListResponse{
				Builds: []amsvc.BuildResponse{{BuildName: latestBuildName}},
			})
		default:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(amsvc.AgentResponse{
				Name:         "my-agent",
				DisplayName:  "My Agent",
				ProjectName:  "triage",
				Provisioning: amsvc.Provisioning{Type: amsvc.ProvisioningTypeInternal},
			})
		}
	}))
	t.Cleanup(server.Close)
	client, err := amsvc.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatalf("new amsvc client: %v", err)
	}
	return func(context.Context) (*amsvc.ClientWithResponses, error) { return client, nil }
}

func newObserverBuildLogsClient(t *testing.T, status int, body any) func(context.Context) (*observersvc.Client, error) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/build-logs" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			_ = json.NewEncoder(w).Encode(body)
		}
	}))
	t.Cleanup(server.Close)
	client, err := observersvc.NewClient(server.URL)
	if err != nil {
		t.Fatalf("new observer client: %v", err)
	}
	return func(context.Context) (*observersvc.Client, error) { return client, nil }
}

func buildTestScope() render.Scope {
	return render.Scope{Instance: "default", Org: "acme", Project: "triage", Agent: "my-agent"}
}

func TestBuildLogs_UsesExplicitBuildName(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = false
	amClient := newBuildTestClient(t, "should-not-be-used")
	observer := newObserverBuildLogsClient(t, http.StatusOK, observersvc.LogsResponse{
		Logs: []observersvc.LogEntry{
			{Log: "Cloning repository"},
			{Log: "Build succeeded"},
		},
		TotalCount: 2,
	})

	err := runLogs(context.Background(), &LogsOptions{
		IO: ios, Client: amClient, Observer: observer, Scope: buildTestScope(),
		Org: "acme", Proj: "triage", AgentName: "my-agent", BuildName: "build-explicit",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Build succeeded") {
		t.Errorf("output should contain build log line, got %q", got)
	}
}

func TestBuildLogs_ResolvesLatestBuildWhenNameOmitted(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = false
	amClient := newBuildTestClient(t, "latest-build-99")
	var gotBuildName string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBuildName = r.URL.Query().Get("buildName")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(observersvc.LogsResponse{
			Logs: []observersvc.LogEntry{{Log: "from latest build"}},
		})
	}))
	defer server.Close()
	observerClient, err := observersvc.NewClient(server.URL)
	if err != nil {
		t.Fatalf("new observer client: %v", err)
	}
	observer := func(context.Context) (*observersvc.Client, error) { return observerClient, nil }

	err = runLogs(context.Background(), &LogsOptions{
		IO: ios, Client: amClient, Observer: observer, Scope: buildTestScope(),
		Org: "acme", Proj: "triage", AgentName: "my-agent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBuildName != "latest-build-99" {
		t.Errorf("buildName sent to observer = %q, want %q", gotBuildName, "latest-build-99")
	}
	if !strings.Contains(out.String(), "from latest build") {
		t.Errorf("output should contain resolved-build log line, got %q", out.String())
	}
}

func TestBuildLogs_JSONOutput(t *testing.T) {
	ios, out, _ := newBuildTestIO(true)
	amClient := newBuildTestClient(t, "build-1")
	observer := newObserverBuildLogsClient(t, http.StatusOK, observersvc.LogsResponse{
		Logs:       []observersvc.LogEntry{{Log: "hello"}},
		TotalCount: 1,
	})

	err := runLogs(context.Background(), &LogsOptions{
		IO: ios, Client: amClient, Observer: observer, Scope: buildTestScope(),
		Org: "acme", Proj: "triage", AgentName: "my-agent", BuildName: "build-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v\nbody=%s", err, out.String())
	}
	if _, ok := envelope.Data["logs"]; !ok {
		t.Fatalf("expected \"logs\" key in JSON envelope: %s", out.String())
	}
}

func TestBuildLogs_ObserverErrorMapsToCLIError(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.JSON = false
	amClient := newBuildTestClient(t, "build-1")
	observer := newObserverBuildLogsClient(t, http.StatusBadGateway, nil)

	err := runLogs(context.Background(), &LogsOptions{
		IO: ios, Client: amClient, Observer: observer, Scope: buildTestScope(),
		Org: "acme", Proj: "triage", AgentName: "my-agent", BuildName: "build-1",
	})
	if err == nil {
		t.Fatal("expected error from observer failure")
	}
}

func TestBuildLogs_UnknownBuildReportsNotFound(t *testing.T) {
	ios, out, _ := newBuildTestIO(true)
	amClient := newBuildTestClient(t, "build-1")
	observer := func(context.Context) (*observersvc.Client, error) {
		return nil, errors.New("observer should not be constructed")
	}

	err := runLogs(context.Background(), &LogsOptions{
		IO: ios, Client: amClient, Observer: observer, Scope: buildTestScope(),
		Org: "acme", Proj: "triage", AgentName: "my-agent", BuildName: missingBuildName,
	})
	if err == nil {
		t.Fatal("expected error for unknown build")
	}
	var envelope struct {
		Error struct {
			Status int    `json:"status"`
			Code   string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v\nbody=%s", err, out.String())
	}
	if envelope.Error.Status != http.StatusNotFound || envelope.Error.Code != "BUILD_NOT_FOUND" {
		t.Errorf("error = %d %q, want 404 BUILD_NOT_FOUND", envelope.Error.Status, envelope.Error.Code)
	}
}

func TestBuildLogs_RejectsExternalAgent(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.JSON = false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(amsvc.AgentResponse{
			Name:         "ext-agent",
			DisplayName:  "Ext Agent",
			ProjectName:  "triage",
			Provisioning: amsvc.Provisioning{Type: "external"},
		})
	}))
	defer server.Close()
	amClient, err := amsvc.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatalf("new amsvc client: %v", err)
	}

	err = runLogs(context.Background(), &LogsOptions{
		IO: ios,
		Client: func(context.Context) (*amsvc.ClientWithResponses, error) {
			return amClient, nil
		},
		Observer: func(context.Context) (*observersvc.Client, error) {
			return nil, errors.New("observer should not be constructed")
		},
		Scope: buildTestScope(), Org: "acme", Proj: "triage", AgentName: "ext-agent", BuildName: "build-1",
	})
	if err == nil {
		t.Fatal("expected error for externally-provisioned agent")
	}
}

func newBuildTestIO(jsonMode bool) (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	ios, _, out, errOut := iostreams.Test()
	ios.JSON = jsonMode
	return ios, out, errOut
}
