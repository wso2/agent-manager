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

package reqorigin

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginFollowsForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/config.js", nil)
	req.Host = "amp-api:8080"
	if got := Of(req); got != "http://amp-api:8080" {
		t.Fatalf("plain: %s", got)
	}
	req.TLS = &tls.ConnectionState{}
	if got := Of(req); got != "https://amp-api:8080" {
		t.Fatalf("tls: %s", got)
	}
	req.TLS = nil
	req.Header.Set("X-Forwarded-Proto", "https, http")
	req.Header.Set("X-Forwarded-Host", "amp.example.com")
	if got := Of(req); got != "https://amp.example.com" {
		t.Fatalf("forwarded: %s", got)
	}
	req.Header.Set("X-Forwarded-Proto", "javascript")
	req.Header.Set("X-Forwarded-Host", "evil.example/'x")
	if got := Of(req); got != "http://amp-api:8080" {
		t.Fatalf("bad forwarded values must be ignored: %s", got)
	}
}
