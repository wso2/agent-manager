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
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealthProbeIPs(t *testing.T) {
	for _, tc := range []struct {
		host string
		want []string
	}{
		{"", []string{"127.0.0.1", "::1"}},
		{"0.0.0.0", []string{"127.0.0.1", "::1"}},
		{"127.0.0.2", []string{"127.0.0.2"}},
		{"::", []string{"::1", "127.0.0.1"}},
		{"[::]", []string{"::1", "127.0.0.1"}},
		{"[0:0:0:0:0:0:0:0]", []string{"::1", "127.0.0.1"}},
		{"[::1]", []string{"::1"}},
	} {
		t.Run(tc.host, func(t *testing.T) {
			ips, err := healthProbeIPs(t.Context(), tc.host, nil)
			require.NoError(t, err)
			var got []string
			for _, ip := range ips {
				got = append(got, ip.String())
			}
			require.Equal(t, tc.want, got)
		})
	}
	for _, host := range []string{"192.0.2.1", "[2001:db8::1]", "[[::1]]", "example.com"} {
		_, err := healthProbeIPs(t.Context(), host, nil)
		require.ErrorContains(t, err, "requires a loopback")
	}
}

func TestHealthProbeLocalhost(t *testing.T) {
	for _, tc := range []struct {
		name      string
		addresses []net.IP
		want      string
	}{
		{"IPv6 only", []net.IP{net.IPv6loopback}, "::1"},
		{"dual stack", []net.IP{net.IPv6loopback, net.IPv4(127, 0, 0, 1)}, "127.0.0.1"},
		{"non-loopback", []net.IP{net.IPv4(192, 0, 2, 1)}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ips, err := healthProbeIPs(t.Context(), "localhost", func(_ context.Context, network, host string) ([]net.IP, error) {
				require.Equal(t, "ip", network)
				require.Equal(t, "localhost", host)
				return tc.addresses, nil
			})
			if tc.want == "" {
				require.ErrorContains(t, err, "requires a loopback")
				return
			}
			require.NoError(t, err)
			require.Len(t, ips, 1)
			require.Equal(t, tc.want, ips[0].String())
		})
	}
}
