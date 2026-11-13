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

package ampinit

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/gatewaytoken"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

func setThunderEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"THUNDER_PUBLIC_URL":          "https://id.example.com",
		"THUNDER_PORT":                "8090",
		"AMP_PUBLIC_URL":              "https://api.example.com/",
		"OBSERVER_PUBLIC_URL":         "https://traces.example.com",
		"CONSOLE_URL":                 "https://console.example.com",
		"AMP_API_CLIENT_SECRET":       "s1",
		"AMP_CONSOLE_CLIENT_SECRET":   "s2",
		"AMP_PUBLISHER_CLIENT_SECRET": "s3",
		"AMP_SYSTEM_CLIENT_SECRET":    "s4",
		"AMP_API_CLIENT_ID":           "custom-api",
		"AMP_DEFAULT_USERS":           "dev:pw1, ops:pw2:admin",
	} {
		t.Setenv(k, v)
	}
}

func TestLoadThunderConfigDerivesFromURLs(t *testing.T) {
	setThunderEnv(t)
	cfg, err := LoadThunderConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ThunderScheme != "https" || cfg.ThunderHost != "id.example.com" || cfg.ThunderPort != 443 || cfg.ThunderListenPort != 8090 {
		t.Errorf("thunder URL parts = %s %s %d %d", cfg.ThunderScheme, cfg.ThunderHost, cfg.ThunderPort, cfg.ThunderListenPort)
	}
	if cfg.AMPPublicURL != "https://api.example.com" {
		t.Errorf("AMPPublicURL = %q, want the trailing slash trimmed", cfg.AMPPublicURL)
	}
	if !slices.Equal(cfg.ConsoleRedirectURIs, []string{"https://console.example.com/login"}) ||
		!slices.Equal(cfg.CORSOrigins, []string{"https://console.example.com"}) {
		t.Errorf("console defaults = %v %v", cfg.ConsoleRedirectURIs, cfg.CORSOrigins)
	}
	if cfg.APIClient.ID != "custom-api" || cfg.SystemClient.ID != "amp-system-client" {
		t.Errorf("client ids = %q %q", cfg.APIClient.ID, cfg.SystemClient.ID)
	}
	if len(cfg.Users) != 2 || cfg.Users[0].Admin || !cfg.Users[1].Admin || cfg.Users[1].Password != "pw2" {
		t.Errorf("users = %+v", cfg.Users)
	}
}

func TestLoadThunderConfigReportsEveryMissingValue(t *testing.T) {
	t.Setenv("THUNDER_PUBLIC_URL", "not a url")
	_, err := LoadThunderConfig()
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"THUNDER_PUBLIC_URL", "AMP_PUBLIC_URL", "CONSOLE_URL", "AMP_SYSTEM_CLIENT_SECRET"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}
}

func TestParseUsersRejectsBadEntries(t *testing.T) {
	for _, spec := range []string{"nopassword", "a:b:root", "a:b,a:c", ":pw"} {
		if _, err := parseUsers(spec); err == nil {
			t.Errorf("parseUsers(%q) accepted it", spec)
		}
	}
}

func TestRenderThunderUsesConfigAndCatalog(t *testing.T) {
	setThunderEnv(t)
	cfg, err := LoadThunderConfig()
	if err != nil {
		t.Fatal(err)
	}
	files, err := RenderThunder(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var app struct {
		InboundAuthConfig []struct {
			Config struct {
				ClientID     string   `yaml:"clientId"`
				ClientSecret string   `yaml:"clientSecret"`
				Scopes       []string `yaml:"scopes"`
			} `yaml:"config"`
		} `yaml:"inboundAuthConfig"`
	}
	mustYAML(t, files["bootstrap/50-amp-api-client.yaml"], &app)
	got := app.InboundAuthConfig[0].Config
	if got.ClientID != "custom-api" || got.ClientSecret != "s1" {
		t.Errorf("api client = %q/%q", got.ClientID, got.ClientSecret)
	}
	// Every scope a role grants must be one the API client may request.
	for role, perms := range rbac.PredefinedRolePermissions {
		for _, p := range perms {
			if !slices.Contains(got.Scopes, p.Scope()) {
				t.Errorf("role %q grants %s, which the API client cannot request", role, p.Scope())
			}
		}
	}

	var role struct {
		Permissions []struct {
			ResourceServerID string   `yaml:"resourceServerId"`
			Permissions      []string `yaml:"permissions"`
		} `yaml:"permissions"`
	}
	mustYAML(t, files["bootstrap/63-amp-role-ai-lead.yaml"], &role)
	want := make([]string, 0)
	for _, p := range rbac.PredefinedRolePermissions[rbac.RoleAILead] {
		want = append(want, p.Scope())
	}
	if !slices.Equal(role.Permissions[0].Permissions, want) {
		t.Errorf("AI Lead grants %v, want %v", role.Permissions[0].Permissions, want)
	}
	for _, p := range role.Permissions[2].Permissions {
		if !strings.HasPrefix(p, "amp:observability:") {
			t.Errorf("observer MCP block grants %q", p)
		}
	}

	for name, want := range map[string]string{
		"bootstrap/71-amp-cors-config.yaml":      `allowedOrigins: ["https://console.example.com"]`,
		"bootstrap/60-mcp-resource-servers.yaml": `identifier: "https://api.example.com/mcp"`,
		"deployment.yaml":                        "port: 8090",
		"console-config.js":                      `scheme: "https"`,
		"users/67-amp-user-ops.yaml":             `id: "amp-user-ops"`,
		"bootstrap/65-amp-group-admins.yaml":     `id: "amp-user-ops"`,
	} {
		if !strings.Contains(string(files[name]), want) {
			t.Errorf("%s does not contain %q:\n%s", name, want, files[name])
		}
	}
	if strings.Contains(string(files["bootstrap/65-amp-group-admins.yaml"]), "amp-user-dev") {
		t.Error("a non-admin user joined the admins group")
	}
	if !strings.Contains(string(files["users/67-amp-user-dev.yaml"]), "password: \"pw1\"") {
		t.Errorf("user file lacks its credentials:\n%s", files["users/67-amp-user-dev.yaml"])
	}
	if strings.Contains(string(files["bootstrap/67-amp-default-users.yaml"]), "amp-user-") {
		t.Error("additional users must not be in the always-applied bootstrap")
	}
}

func TestWriteThunderStampFollowsBootstrap(t *testing.T) {
	dir := t.TempDir()
	cfg := SampleThunderConfig()
	if err := os.MkdirAll(filepath.Join(dir, "bootstrap"), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "bootstrap", "99-removed.yaml")
	if err := os.WriteFile(stale, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteThunder(cfg, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("a bootstrap file this run did not write was kept")
	}
	first := readFile(t, filepath.Join(dir, BootstrapStamp))
	if err := WriteThunder(cfg, dir); err != nil {
		t.Fatal(err)
	}
	if again := readFile(t, filepath.Join(dir, BootstrapStamp)); again != first {
		t.Error("the stamp changed although the configuration did not")
	}
	cfg.CORSOrigins = append(cfg.CORSOrigins, "http://localhost:3001")
	if err := WriteThunder(cfg, dir); err != nil {
		t.Fatal(err)
	}
	if changed := readFile(t, filepath.Join(dir, BootstrapStamp)); changed == first {
		t.Error("the stamp did not change with the bootstrap")
	}
	if info, _ := os.Stat(filepath.Join(dir, "bootstrap", "50-amp-api-client.yaml")); info.Mode().Perm() != 0o600 {
		t.Errorf("bootstrap file mode = %v, want 0600", info.Mode().Perm())
	}
}

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

func mustYAML(t *testing.T, data []byte, out any) {
	t.Helper()
	if err := yaml.Unmarshal(data, out); err != nil {
		t.Fatalf("parse: %v\n%s", err, data)
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
