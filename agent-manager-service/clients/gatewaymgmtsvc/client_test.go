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

package gatewaymgmtsvc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFindRestAPIByArtifactID(t *testing.T) {
	const artifactID = "8af36e12-6df2-4c58-b64f-d70fc8d4fecd"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/rest-apis" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "admin" || password != "secret" {
			t.Fatalf("unexpected basic auth: ok=%v username=%q password=%q", ok, username, password)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"count":  2,
			"apis": []map[string]interface{}{
				{
					"metadata": map[string]interface{}{
						"name": "other-api",
						"annotations": map[string]string{
							artifactIDAnnotation: "other-artifact",
						},
					},
					"status": map[string]string{"id": "other-api"},
				},
				{
					"metadata": map[string]interface{}{
						"name": "travel-agent-api",
						"annotations": map[string]string{
							artifactIDAnnotation: artifactID,
						},
					},
					"status": map[string]string{"id": "travel-agent-api"},
				},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:    server.URL,
		Username:   "admin",
		Password:   "secret",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	restAPIID, err := client.FindRestAPIByArtifactID(t.Context(), artifactID)
	if err != nil {
		t.Fatalf("FindRestAPIByArtifactID returned error: %v", err)
	}
	if restAPIID != "travel-agent-api" {
		t.Fatalf("expected RestAPI ID travel-agent-api, got %q", restAPIID)
	}
}

func TestFindRestAPIByArtifactIDFromDetailComponentUIDLabel(t *testing.T) {
	const artifactID = "2986043c-4661-4a4e-8f94-dcaa3f8a5bbf"
	const restAPIID = "custom2-default-50810e8d"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "admin" || password != "secret" {
			t.Fatalf("unexpected basic auth: ok=%v username=%q password=%q", ok, username, password)
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest-apis":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "success",
				"count":  1,
				"apis": []map[string]interface{}{
					{
						"id":          restAPIID,
						"displayName": "custom2",
						"context":     "/default-default-custom2",
						"status":      "deployed",
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/rest-apis/"+restAPIID:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "success",
				"api": map[string]interface{}{
					"id": restAPIID,
					"configuration": map[string]interface{}{
						"metadata": map[string]interface{}{
							"name": restAPIID,
							"labels": map[string]string{
								componentUIDLabel: artifactID,
							},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:    server.URL,
		Username:   "admin",
		Password:   "secret",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	foundRestAPIID, err := client.FindRestAPIByArtifactID(t.Context(), artifactID)
	if err != nil {
		t.Fatalf("FindRestAPIByArtifactID returned error: %v", err)
	}
	if foundRestAPIID != restAPIID {
		t.Fatalf("expected RestAPI ID %s, got %q", restAPIID, foundRestAPIID)
	}
}

func TestFindRestAPIByArtifactIDNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"count":  1,
			"apis": []map[string]interface{}{
				{
					"metadata": map[string]interface{}{
						"name": "other-api",
						"annotations": map[string]string{
							artifactIDAnnotation: "other-artifact",
						},
					},
					"status": map[string]string{"id": "other-api"},
				},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:    server.URL,
		Username:   "admin",
		Password:   "secret",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	_, err = client.FindRestAPIByArtifactID(t.Context(), "missing-artifact")
	if err == nil {
		t.Fatalf("expected not-found error")
	}
	if !strings.Contains(err.Error(), "RestAPI not found") {
		t.Fatalf("expected RestAPI not found error, got %v", err)
	}
}
