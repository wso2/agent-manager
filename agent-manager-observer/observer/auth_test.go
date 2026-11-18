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

package observer

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthProviderUsesSuppliedBasicCredentials(t *testing.T) {
	clientID := "test-client"
	clientSecret := rand.Text()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, secret, ok := r.BasicAuth(); !ok || id != clientID || secret != clientSecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":120}`))
	}))
	defer server.Close()

	provider := NewAuthProvider(server.URL, clientID, clientSecret)
	token, err := provider.GetToken(t.Context())
	if err != nil || token != "test-token" {
		t.Fatalf("expected a token from the server using the supplied credentials, error: %v", err)
	}
}
