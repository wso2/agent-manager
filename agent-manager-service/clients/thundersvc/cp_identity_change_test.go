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

package thundersvc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCPClientActivatesRoleAndGroupChangesThroughCaptureApplyAndVerify(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/configuration-versions":
			_, _ = w.Write([]byte(`{"version":"v1"}`))
		case "/gateways/gateway-dev/apply":
			_, _ = w.Write([]byte(`{}`))
		case "/gateways/gateway-dev/applied-version":
			_, _ = w.Write([]byte(`{"gatewayId":"gateway-dev","appliedVersion":"v1"}`))
		}
	}))
	defer server.Close()
	backend, err := NewCPIdentityBackend(bearerAuthorizer("cp-token"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := backend.ResolveIdentity(context.Background(), cpTestTarget(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	activator, ok := client.(EnvironmentIdentityChangeActivator)
	if !ok {
		t.Fatal("a CP client must activate role and group changes")
	}
	if err := activator.ActivateIdentityChange(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := "POST /configuration-versions,POST /gateways/gateway-dev/apply,GET /gateways/gateway-dev/applied-version"
	if got := strings.Join(calls, ","); got != want {
		t.Fatalf("CP calls = %s, want %s", got, want)
	}
}

func TestProxyResourceServerExistsReadsWithoutWriting(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		_, _ = w.Write([]byte(`{"totalResults":1,"resourceServers":[{"id":"rs-1","name":"Booking","identifier":"https://gw.example.test/booking/mcp"}]}`))
	}))
	defer server.Close()
	client := &thunderClient{baseURL: server.URL, httpClient: server.Client(), cpManaged: true}

	exists, err := client.ProxyResourceServerExists(context.Background(), "booking", "https://gw.example.test/booking/mcp")
	if err != nil || !exists {
		t.Fatalf("exists = %v, %v", exists, err)
	}
	exists, err = client.ProxyResourceServerExists(context.Background(), "weather", "https://gw.example.test/weather/mcp")
	if err != nil || exists {
		t.Fatalf("a missing resource server must be reported as absent, got %v, %v", exists, err)
	}
	for _, call := range methods {
		if !strings.HasPrefix(call, "GET ") {
			t.Fatalf("the existence check must only read, got %s", call)
		}
	}
	if _, err := client.ProxyResourceServerExists(context.Background(), "booking", "not a uri"); err == nil {
		t.Fatal("an identifier that is not a URI must be rejected")
	}
}

func TestCPTransportErrorsCarryNoGatewayOrVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	target := cpTestTarget(server.URL)
	server.Close() // nothing listens: the request fails in the transport
	client, _ := NewCPConfigurationClient(bearerAuthorizer("management-token"))

	err := client.Activate(context.Background(), target, "change")

	if err == nil {
		t.Fatal("a closed server must fail")
	}
	for _, leaked := range []string{"gateway-dev", "version="} {
		if strings.Contains(err.Error(), leaked) {
			t.Fatalf("the error %q must not carry %q", err.Error(), leaked)
		}
	}
}
