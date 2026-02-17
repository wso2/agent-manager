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

// TestGetClientIP_Security verifies the security fixes for rate limit bypass
func TestGetClientIP_Security(t *testing.T) {
	t.Run("extracts only first IP from X-Forwarded-For", func(t *testing.T) {
		req := &http.Request{Header: http.Header{}}
		req.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.2, 192.0.2.3")

		result := getClientIP(req)
		expected := "203.0.113.1"

		if result != expected {
			t.Errorf("SECURITY: Multiple IPs should extract first only. Expected %q, got %q", expected, result)
		}
	})

	t.Run("prevents rate limit bypass via header rotation", func(t *testing.T) {
		// Attacker rotating proxy chain should still map to same client IP
		testCases := []string{
			"203.0.113.1",
			"203.0.113.1, 198.51.100.2",
			"203.0.113.1, 198.51.100.2, 192.0.2.3",
			"203.0.113.1,different-proxy-1,different-proxy-2",
		}

		expectedIP := "203.0.113.1"
		for _, xff := range testCases {
			req := &http.Request{Header: http.Header{}}
			req.Header.Set("X-Forwarded-For", xff)

			result := getClientIP(req)
			if result != expectedIP {
				t.Errorf("SECURITY BYPASS: Header %q should extract %q (got %q). Attacker can rotate headers to bypass rate limit!",
					xff, expectedIP, result)
			}
		}
	})

	t.Run("strips port from RemoteAddr", func(t *testing.T) {
		req := &http.Request{
			Header:     http.Header{},
			RemoteAddr: "192.0.2.3:8080",
		}

		result := getClientIP(req)
		expected := "192.0.2.3"

		if result != expected {
			t.Errorf("Should strip port from RemoteAddr. Expected %q, got %q", expected, result)
		}
	})
}
