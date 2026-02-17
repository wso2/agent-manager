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

package controllers

import (
	"net/http"
	"testing"
)

func TestGetClientIP_XForwardedFor(t *testing.T) {
	tests := []struct {
		name        string
		headerValue string
		expectedIP  string
		description string
	}{
		{
			name:        "single IP",
			headerValue: "203.0.113.1",
			expectedIP:  "203.0.113.1",
			description: "Single IP should be returned as-is",
		},
		{
			name:        "multiple IPs comma separated",
			headerValue: "203.0.113.1, 198.51.100.2, 192.0.2.3",
			expectedIP:  "203.0.113.1",
			description: "Should extract only the first (leftmost) IP from proxy chain",
		},
		{
			name:        "multiple IPs with varying whitespace",
			headerValue: "203.0.113.1,198.51.100.2,  192.0.2.3",
			expectedIP:  "203.0.113.1",
			description: "Should handle whitespace variations",
		},
		{
			name:        "IP with spaces",
			headerValue: "  203.0.113.1  ",
			expectedIP:  "203.0.113.1",
			description: "Should trim whitespace from single IP",
		},
		{
			name:        "malicious bypass attempt",
			headerValue: "fake-ip-12345",
			expectedIP:  "fake-ip-12345",
			description: "Even invalid IPs should be parsed consistently to prevent bypass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Header: http.Header{},
			}
			req.Header.Set("X-Forwarded-For", tt.headerValue)

			result := getClientIP(req)
			if result != tt.expectedIP {
				t.Errorf("%s: expected %q, got %q", tt.description, tt.expectedIP, result)
			}
		})
	}
}

func TestGetClientIP_Fallback(t *testing.T) {
	tests := []struct {
		name          string
		xForwardedFor string
		xRealIP       string
		remoteAddr    string
		expectedIP    string
	}{
		{
			name:          "prefers X-Forwarded-For",
			xForwardedFor: "203.0.113.1",
			xRealIP:       "198.51.100.2",
			remoteAddr:    "192.0.2.3:8080",
			expectedIP:    "203.0.113.1",
		},
		{
			name:          "falls back to X-Real-IP",
			xForwardedFor: "",
			xRealIP:       "198.51.100.2",
			remoteAddr:    "192.0.2.3:8080",
			expectedIP:    "198.51.100.2",
		},
		{
			name:          "falls back to RemoteAddr and strips port",
			xForwardedFor: "",
			xRealIP:       "",
			remoteAddr:    "192.0.2.3:8080",
			expectedIP:    "192.0.2.3",
		},
		{
			name:          "RemoteAddr without port",
			xForwardedFor: "",
			xRealIP:       "",
			remoteAddr:    "192.0.2.3",
			expectedIP:    "192.0.2.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Header:     http.Header{},
				RemoteAddr: tt.remoteAddr,
			}

			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}

			result := getClientIP(req)
			if result != tt.expectedIP {
				t.Errorf("expected %q, got %q", tt.expectedIP, result)
			}
		})
	}
}

func TestGetClientIP_BypassPrevention(t *testing.T) {
	// Verify that rotating X-Forwarded-For values with the same client IP
	// results in the same cache key (preventing rate limit bypass)

	clientIP := "203.0.113.1"
	testCases := []string{
		"203.0.113.1",                          // Direct
		"203.0.113.1, 198.51.100.2",            // Through 1 proxy
		"203.0.113.1, 198.51.100.2, 192.0.2.3", // Through 2 proxies
		"203.0.113.1,198.51.100.2,192.0.2.3",   // No spaces
		"  203.0.113.1  , 198.51.100.2  ",      // Extra whitespace
	}

	for i, xff := range testCases {
		req := &http.Request{
			Header: http.Header{},
		}
		req.Header.Set("X-Forwarded-For", xff)

		result := getClientIP(req)
		if result != clientIP {
			t.Errorf("Test case %d: X-Forwarded-For=%q should extract %q, got %q (this would allow rate limit bypass)",
				i, xff, clientIP, result)
		}
	}
}
