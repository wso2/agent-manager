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

package gwinit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/gatewaytoken"
)

func gatewayConfig() *GatewayConfig {
	return &GatewayConfig{
		ControlPlaneHost: "amp-api:9243", AdminPassword: "pw", AMPJWKSURL: "http://amp-api:8080/jwks",
		ThunderIssuer: "http://thunder.amp.localhost:8080", ThunderJWKSURL: "http://thunder:8080/oauth2/jwks",
		Host: "default-default.gateway.localhost", ListenerPort: 19080,
	}
}

func TestWriteGatewayKeepsGeneratedMaterial(t *testing.T) {
	dir := t.TempDir()
	if err := WriteGateway(gatewayConfig(), dir); err != nil {
		t.Fatal(err)
	}
	token := readFile(t, filepath.Join(dir, RegistrationTokenFile))
	if _, err := gatewaytoken.Prefix(token); err != nil {
		t.Fatalf("generated token %q: %v", token, err)
	}
	key := readFile(t, filepath.Join(dir, gatewayAESKeyFile))
	cert := readFile(t, filepath.Join(dir, gatewayCertFile))
	if len(key) != 32 || !strings.Contains(cert, "BEGIN CERTIFICATE") {
		t.Fatalf("key length %d, cert %q", len(key), cert)
	}
	conf := readFile(t, filepath.Join(dir, GatewayConfigFile))
	for _, want := range []string{`token = "` + token + `"`, `host = "amp-api:9243"`, "listener_port = 19080", `gatewayhost = "default-default.gateway.localhost"`} {
		if !strings.Contains(conf, want) {
			t.Errorf("config.toml lacks %q", want)
		}
	}

	if err := WriteGateway(gatewayConfig(), dir); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, RegistrationTokenFile)) != token ||
		readFile(t, filepath.Join(dir, gatewayAESKeyFile)) != key ||
		readFile(t, filepath.Join(dir, gatewayCertFile)) != cert {
		t.Error("a rerun replaced generated material")
	}

	configured, _ := gatewaytoken.New()
	cfg := gatewayConfig()
	cfg.RegistrationToken = configured
	if err := WriteGateway(cfg, dir); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, RegistrationTokenFile)) != configured ||
		!strings.Contains(readFile(t, filepath.Join(dir, GatewayConfigFile)), configured) {
		t.Error("a configured token did not replace the kept one")
	}
}

func TestLoadGatewayConfigRejectsMalformedToken(t *testing.T) {
	for k, v := range map[string]string{
		"GATEWAY_CONTROLPLANE_HOST": "amp-api:9243", "GATEWAY_ADMIN_PASSWORD": "pw",
		"GATEWAY_AMP_JWKS_URL": "http://a/jwks", "THUNDER_PUBLIC_URL": "http://t:8080",
		"GATEWAY_THUNDER_JWKS_URL": "http://t/jwks", "GATEWAY_HOST": "gw.localhost",
		"DEFAULT_GATEWAY_TOKEN": "too-short",
	} {
		t.Setenv(k, v)
	}
	if _, err := LoadGatewayConfig(); err == nil || !strings.Contains(err.Error(), "DEFAULT_GATEWAY_TOKEN") {
		t.Fatalf("err = %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
