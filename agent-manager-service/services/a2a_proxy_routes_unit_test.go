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

package services

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const originCardURL = "https://agent.example/team/.well-known/agent-card.json"

// a2aTestCard is a minimal card offering the given interfaces.
func a2aTestCard(t *testing.T, interfaces ...map[string]any) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(map[string]any{"name": "Weather", "skills": []any{}, "supportedInterfaces": interfaces})
	require.NoError(t, err)
	return body
}

func a2aIface(binding, url string) map[string]any {
	return map[string]any{"protocolBinding": binding, "url": url, "protocolVersion": "1.0"}
}

func TestDeriveA2AProxyRoutesFrontsEachBindingAtItsPath(t *testing.T) {
	upstream, transports, err := deriveA2AProxyRoutes(originCardURL, a2aTestCard(t,
		a2aIface("JSONRPC", "https://agent.example/team/a2a/jsonrpc"),
		a2aIface("HTTP+JSON", "https://agent.example/team/a2a/rest"),
	))
	require.NoError(t, err)
	assert.Equal(t, "https://agent.example/team", upstream)
	assert.Equal(t, []A2ATransport{
		{ProtocolBinding: "JSONRPC", PathPrefix: "/a2a/jsonrpc"},
		{ProtocolBinding: "HTTP+JSON", PathPrefix: "/a2a/rest"},
	}, transports)
}

func TestDeriveA2AProxyRoutesTakesTheUpstreamFromTheCardURL(t *testing.T) {
	for cardURL, want := range map[string]string{
		"https://agent.example/.well-known/agent-card.json":         "https://agent.example",
		"http://agent.example:8080/a/b/.well-known/agent-card.json": "http://agent.example:8080/a/b",
	} {
		upstream, _, err := deriveA2AProxyRoutes(cardURL, a2aTestCard(t, a2aIface("JSONRPC", want+"/rpc")))
		require.NoError(t, err, cardURL)
		assert.Equal(t, want, upstream)
	}
}

// The gateway fetches the passthrough card from upstream + the well-known path, nothing else.
func TestDeriveA2AProxyRoutesRejectsACardURLTheGatewayCannotFetch(t *testing.T) {
	card := a2aTestCard(t, a2aIface("JSONRPC", "https://agent.example/rpc"))
	for _, cardURL := range []string{
		"https://agent.example/card.json",
		"https://agent.example/.well-known/agent-card.json?v=1",
		"https://agent.example//.well-known/agent-card.json",
		"ftp://agent.example/.well-known/agent-card.json",
		"https://user:pw@agent.example/.well-known/agent-card.json",
	} {
		_, _, err := deriveA2AProxyRoutes(cardURL, card)
		assert.ErrorIs(t, err, errA2ANotProxyable, cardURL)
	}
}

// The a2a-sdk default serves both bindings at the root.
func TestDeriveA2AProxyRoutesFrontsBothBindingsAtTheRoot(t *testing.T) {
	_, transports, err := deriveA2AProxyRoutes("https://agent.example/.well-known/agent-card.json", a2aTestCard(t,
		a2aIface("JSONRPC", "https://agent.example/"),
		a2aIface("HTTP+JSON", "https://agent.example"),
	))
	require.NoError(t, err)
	assert.Equal(t, []A2ATransport{{ProtocolBinding: "JSONRPC"}, {ProtocolBinding: "HTTP+JSON"}}, transports,
		"an empty remainder omits pathPrefix, and the gateway allows the same prefix on both bindings")
}

func TestDeriveA2AProxyRoutesLeavesABindingUnfronted(t *testing.T) {
	fronted := a2aIface("HTTP+JSON", "https://agent.example/team/rest")
	cases := map[string][]map[string]any{
		"its entries disagree on the URL": {
			a2aIface("JSONRPC", "https://agent.example/team/a"), a2aIface("JSONRPC", "https://agent.example/team/b"),
		},
		"another version":                {{"protocolBinding": "JSONRPC", "url": "https://agent.example/team/rpc", "protocolVersion": "0.3"}},
		"a non-string version":           {{"protocolBinding": "JSONRPC", "url": "https://agent.example/team/rpc", "protocolVersion": 1}},
		"another host":                   {a2aIface("JSONRPC", "https://other.example/team/rpc")},
		"another port":                   {a2aIface("JSONRPC", "https://agent.example:8443/team/rpc")},
		"another scheme":                 {a2aIface("JSONRPC", "http://agent.example/team/rpc")},
		"outside the upstream path":      {a2aIface("JSONRPC", "https://agent.example/other/rpc")},
		"a sibling of the upstream path": {a2aIface("JSONRPC", "https://agent.example/teammate/rpc")},
	}
	for name, jsonrpc := range cases {
		t.Run(name, func(t *testing.T) {
			interfaces := append(append([]map[string]any{}, jsonrpc...), fronted)
			_, transports, err := deriveA2AProxyRoutes(originCardURL, a2aTestCard(t, interfaces...))
			require.NoError(t, err)
			assert.Equal(t, []A2ATransport{{ProtocolBinding: "HTTP+JSON", PathPrefix: "/rest"}}, transports)
		})
	}
}

// The gateway rejects these as pathPrefix and would fail the whole Agent.
func TestDeriveA2AProxyRoutesLeavesNonCanonicalPathsUnfronted(t *testing.T) {
	for _, link := range []string{
		"https://agent.example/team/a2a/jsonrpc/",
		"https://agent.example/team/a2a//jsonrpc",
		"https://agent.example/team/a2a/{id}",
		"https://agent.example/team/a2a%2Fjsonrpc",
		"https://agent.example/team/a2a%20rpc",
		"https://agent.example/team/a2a%3Frpc",
		"https://agent.example/team/a2a%23x",
		"https://agent.example/team/a2a%25x",
		"https://agent.example/team/rpc?x=1",
	} {
		_, transports, err := deriveA2AProxyRoutes(originCardURL, a2aTestCard(t,
			a2aIface("JSONRPC", link), a2aIface("HTTP+JSON", "https://agent.example/team/rest")))
		require.NoError(t, err, link)
		assert.Equal(t, []A2ATransport{{ProtocolBinding: "HTTP+JSON", PathPrefix: "/rest"}}, transports, link)
	}
}

func TestDeriveA2AProxyRoutesAcceptsUnversionedEntriesAndDefaultPorts(t *testing.T) {
	_, transports, err := deriveA2AProxyRoutes(originCardURL, a2aTestCard(t,
		map[string]any{"protocolBinding": "JSONRPC", "url": "https://AGENT.example:443/team/rpc"},
	))
	require.NoError(t, err)
	assert.Equal(t, []A2ATransport{{ProtocolBinding: "JSONRPC", PathPrefix: "/rpc"}}, transports)
}

func TestDeriveA2AProxyRoutesIgnoresGRPC(t *testing.T) {
	_, transports, err := deriveA2AProxyRoutes(originCardURL, a2aTestCard(t,
		a2aIface("GRPC", "https://grpc.example:50051"),
		a2aIface("JSONRPC", "https://agent.example/team/rpc"),
	))
	require.NoError(t, err)
	assert.Equal(t, []A2ATransport{{ProtocolBinding: "JSONRPC", PathPrefix: "/rpc"}}, transports)
}

func TestDeriveA2AProxyRoutesNeedsOneFrontedBinding(t *testing.T) {
	_, _, err := deriveA2AProxyRoutes(originCardURL, a2aTestCard(t, a2aIface("GRPC", "https://grpc.example:50051")))
	require.ErrorIs(t, err, errA2ANotProxyable)
	assert.Contains(t, err.Error(), "JSONRPC is not offered")
	assert.Contains(t, err.Error(), "HTTP+JSON is not offered")
}

// The gateway's card rewrite fails the whole card on such an entry.
func TestDeriveA2AProxyRoutesRequiresABindingOnEveryInterface(t *testing.T) {
	_, _, err := deriveA2AProxyRoutes(originCardURL, a2aTestCard(t,
		a2aIface("JSONRPC", "https://agent.example/team/rpc"),
		map[string]any{"url": "https://agent.example/team/other"},
	))
	require.ErrorIs(t, err, errA2ANotProxyable)
	assert.Contains(t, err.Error(), "supportedInterfaces[1]")
}
