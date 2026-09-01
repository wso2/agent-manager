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

package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestDiscoverUsesAPIProtectedResourceScopes(t *testing.T) {
	wantScopes := []string{
		"amp:agent:create",
		"amp:agent:token-manage",
		"amp:deployment-pipeline:read",
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(ProtectedResourceMetadata{
				Resource:             server.URL,
				AuthorizationServers: []string{server.URL},
				ScopesSupported:      wantScopes,
			})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(AuthorizationServerMetadata{
				Issuer:                server.URL,
				AuthorizationEndpoint: server.URL + "/oauth2/authorize",
				TokenEndpoint:         server.URL + "/oauth2/token",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	discovery, err := Discover(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if !reflect.DeepEqual(discovery.ScopesSupported, wantScopes) {
		t.Fatalf("CLI scopes = %v, want %v", discovery.ScopesSupported, wantScopes)
	}
}
