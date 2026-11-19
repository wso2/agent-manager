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

package gwsvc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The port rule is the load-bearing one: the agent sandbox NetworkPolicy permits egress to
// any destination on exactly three ports — 80 and 443 via its public ipBlock rule, and 53
// TCP+UDP via its `to:`-less DNS rule — so refusing all three is what stops a stored
// runtimeUrl from redirecting a sandboxed agent's LLM/MCP traffic — API key included — off
// the cluster. The host-shape rule is defence in depth only.
func TestValidateGatewayRuntimeURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "empty is legal — means no internal address", input: "", wantErr: false},
		{name: "whitespace only is legal", input: "   ", wantErr: false},
		{
			name:  "the chart's value",
			input: "http://api-platform-acme-dev-gw-gateway-gateway-runtime.acme-dev:22893",
		},
		{name: "dotless service name", input: "http://gateway-runtime:22893"},
		{name: "name.namespace.svc", input: "http://runtime.acme-dev.svc:22893"},
		{name: "fully qualified cluster form", input: "http://runtime.acme-dev.svc.cluster.local:22893"},
		{name: "https is allowed", input: "https://runtime.acme-dev:8443"},
		{name: "private IP", input: "http://10.4.2.9:22893"},
		{name: "loopback for local dev", input: "http://localhost:22893"},

		{name: "public FQDN on 443 rejected", input: "https://evil.example.com:443", wantErr: true},
		{name: "public FQDN on 80 rejected", input: "http://evil.example.com:80", wantErr: true},
		{name: "port 443 rejected even cluster-local", input: "https://runtime.acme-dev:443", wantErr: true},
		{name: "port 80 rejected even cluster-local", input: "http://runtime.acme-dev:80", wantErr: true},
		{
			name:    "port 53 rejected — sandbox DNS egress rule reaches any destination",
			input:   "http://exfil.dev:53",
			wantErr: true,
		},
		{name: "port 53 rejected even cluster-local", input: "http://runtime.acme-dev:53", wantErr: true},
		{name: "link-local metadata address rejected", input: "http://169.254.169.254:22893", wantErr: true},
		{name: "missing port rejected", input: "http://runtime.acme-dev", wantErr: true},
		{name: "non-numeric port rejected", input: "http://runtime.acme-dev:abc", wantErr: true},
		{name: "non-http scheme rejected", input: "ftp://runtime.acme-dev:22893", wantErr: true},
		{name: "scheme-less value rejected", input: "runtime.acme-dev:22893", wantErr: true},
		{name: "userinfo rejected", input: "http://user:pw@runtime.acme-dev:22893", wantErr: true},
		{name: "query rejected", input: "http://runtime.acme-dev:22893?x=1", wantErr: true},
		{name: "fragment rejected", input: "http://runtime.acme-dev:22893#f", wantErr: true},
		{name: "trailing slash rejected", input: "http://runtime.acme-dev:22893/", wantErr: true},
		{name: "path rejected", input: "http://runtime.acme-dev:22893/a/b", wantErr: true},
		{name: "public four-label host rejected by shape", input: "http://a.b.c.d.example.com:22893", wantErr: true},
		{name: "public IP rejected", input: "http://8.8.8.8:22893", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGatewayRuntimeURL(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, utils.ErrBadRequest)
				return
			}
			require.NoError(t, err)
		})
	}
}
