// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealthProbeIP(t *testing.T) {
	for _, tc := range []struct{ host, want string }{
		{"", "127.0.0.1"},
		{"0.0.0.0", "127.0.0.1"},
		{"127.0.0.2", "127.0.0.2"},
		{"::", "::1"},
		{"[::]", "::1"},
		{"[::1]", "::1"},
	} {
		t.Run(tc.host, func(t *testing.T) {
			ip, err := healthProbeIP(t.Context(), tc.host)
			require.NoError(t, err)
			require.Equal(t, tc.want, ip.String())
		})
	}
	for _, host := range []string{"192.0.2.1", "[2001:db8::1]", "[[::1]]", "example.com"} {
		_, err := healthProbeIP(t.Context(), host)
		require.ErrorContains(t, err, "requires a loopback")
	}
}
