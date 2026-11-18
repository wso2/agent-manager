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

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

func setThunderEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"THUNDER_PUBLIC_URL":        "https://id.example.com",
		"THUNDER_PORT":              "8090",
		"AMP_PUBLIC_URL":            "https://api.example.com/",
		"CONSOLE_URL":               "https://console.example.com",
		"AMP_API_CLIENT_SECRET":     "s1",
		"AMP_CONSOLE_CLIENT_SECRET": "s2",
		"AMP_SYSTEM_CLIENT_SECRET":  "s4",
		"AMP_API_CLIENT_ID":         "custom-api",
		"AMP_DEFAULT_USERS":         "dev:pw1, ops:pw2:admin",
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
	servers := make([]string, 0, len(role.Permissions))
	for _, block := range role.Permissions {
		servers = append(servers, block.ResourceServerID)
	}
	if want := []string{"amp-resource-server", "amp-agent-manager-mcp-resource-srv"}; !slices.Equal(servers, want) {
		t.Fatalf("AI Lead grants on resource servers %v, want %v", servers, want)
	}
	want := make([]string, 0)
	for _, p := range rbac.PredefinedRolePermissions[rbac.RoleAILead] {
		want = append(want, p.Scope())
	}
	if !slices.Equal(role.Permissions[0].Permissions, want) {
		t.Errorf("AI Lead grants %v, want %v", role.Permissions[0].Permissions, want)
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

// TestFileAuthUsersIncludeAdminAndExtras pins the thing that would otherwise be
// found only by failing to sign in: AMP_DEFAULT_USERS names the users *besides*
// admin, so a stack left on the defaults must still get an admin account.
func TestFileAuthUsersIncludeAdminAndExtras(t *testing.T) {
	for k, v := range map[string]string{
		"AMP_ADMIN_PASSWORD": "secret",
		"AMP_DEFAULT_USERS":  "dev:pw1, ops:pw2:admin",
	} {
		t.Setenv(k, v)
	}

	cfg, err := LoadFileAuthConfig()
	if err != nil {
		t.Fatalf("LoadFileAuthConfig: %v", err)
	}

	got := map[string]bool{}
	for _, u := range cfg.Users {
		got[u.Username] = u.Admin
	}
	for name, wantAdmin := range map[string]bool{"admin": true, "dev": false, "ops": true} {
		admin, present := got[name]
		if !present {
			t.Errorf("user %q missing; got %v", name, got)
			continue
		}
		if admin != wantAdmin {
			t.Errorf("user %q admin = %v, want %v", name, admin, wantAdmin)
		}
	}
}

// TestFileAuthRefusesWithNoAdminPassword keeps the failure at start-up rather
// than at the login nobody can complete.
func TestFileAuthRefusesWithNoAdminPassword(t *testing.T) {
	t.Setenv("AMP_ADMIN_PASSWORD", "")
	t.Setenv("AMP_DEFAULT_USERS", "")
	if _, err := LoadFileAuthConfig(); err == nil {
		t.Fatal("LoadFileAuthConfig succeeded with no admin password, want an error")
	}
}

// TestFileAuthExplicitAdminWins lets an operator override the mirrored account
// through AMP_DEFAULT_USERS instead of it being silently shadowed.
func TestFileAuthExplicitAdminWins(t *testing.T) {
	t.Setenv("AMP_ADMIN_PASSWORD", "mirrored")
	t.Setenv("AMP_DEFAULT_USERS", "admin:chosen")

	cfg, err := LoadFileAuthConfig()
	if err != nil {
		t.Fatalf("LoadFileAuthConfig: %v", err)
	}
	if n := len(cfg.Users); n != 1 {
		t.Fatalf("got %d users, want 1 (the explicit admin replaces the mirrored one)", n)
	}
	if cfg.Users[0].Password != "chosen" {
		t.Errorf("admin password = %q, want the explicit one", cfg.Users[0].Password)
	}
}

// In ThunderID mode the roles come from these files, so a module's
// permissions must reach the resource server, the API client and the roles
// its manifest grants them to, or its routes would refuse everyone.
func TestRenderThunderGrantsModulePermissions(t *testing.T) {
	setThunderEnv(t)
	cfg, err := LoadThunderConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Modules = append(cfg.Modules, modkit.Manifest{
		Name:        "widgets",
		DisplayName: "Widget Shop",
		Permissions: []modkit.Permission{{Name: "widget-read", DisplayName: "Read Widgets", Description: "Read widgets"}, {Name: "widget-write"}},
		RoleGrants:  map[string][]string{rbac.RoleDeveloper: {"widget-read"}},
	})
	files, err := RenderThunder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var server struct {
		Resources []struct {
			Name    string `yaml:"name"`
			Handle  string `yaml:"handle"`
			Actions []struct {
				Name   string `yaml:"name"`
				Handle string `yaml:"handle"`
			} `yaml:"actions"`
		} `yaml:"resources"`
	}
	mustYAML(t, files["bootstrap/60-amp-resource-server.yaml"], &server)
	found := false
	for _, r := range server.Resources {
		if r.Handle != "widgets" {
			continue
		}
		found = true
		if r.Name != "Widget Shop" {
			t.Errorf("the module's resource is named %q, not its display name", r.Name)
		}
		names := map[string]string{}
		for _, a := range r.Actions {
			names[a.Handle] = a.Name
		}
		if names["widget-read"] != "Read Widgets" || names["widget-write"] != "widget-write" {
			t.Errorf("actions are named %v: a display name, or the handle when there is none", names)
		}
	}
	if !found {
		t.Error("the resource server has no resource for the module")
	}
	var app struct {
		InboundAuthConfig []struct {
			Config struct {
				Scopes []string `yaml:"scopes"`
			} `yaml:"config"`
		} `yaml:"inboundAuthConfig"`
	}
	mustYAML(t, files["bootstrap/50-amp-api-client.yaml"], &app)
	for _, scope := range []string{"amp:widgets:widget-read", "amp:widgets:widget-write"} {
		if !slices.Contains(app.InboundAuthConfig[0].Config.Scopes, scope) {
			t.Errorf("the API client cannot request %s", scope)
		}
	}
	grants := func(file string) []string {
		var role struct {
			Permissions []struct {
				ResourceServerID string   `yaml:"resourceServerId"`
				Permissions      []string `yaml:"permissions"`
			} `yaml:"permissions"`
		}
		mustYAML(t, files[file], &role)
		return role.Permissions[0].Permissions
	}
	if !slices.Contains(grants("bootstrap/62-amp-role-developer.yaml"), "amp:widgets:widget-read") {
		t.Error("the Developer role does not hold the permission the module grants it")
	}
	if slices.Contains(grants("bootstrap/63-amp-role-ai-lead.yaml"), "amp:widgets:widget-read") {
		t.Error("the AI Lead role holds a permission the module does not grant it")
	}
}

// A module named after a core resource would add a second "gateway" (or
// "agent", ...) resource to the resource server; rendering refuses it.
func TestRenderThunderRefusesAModuleNamedAfterACoreResource(t *testing.T) {
	setThunderEnv(t)
	cfg, err := LoadThunderConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Modules = append(cfg.Modules, modkit.Manifest{Name: "gateway", Permissions: []modkit.Permission{{Name: "manage"}}})
	if _, err := RenderThunder(cfg); err == nil || !strings.Contains(err.Error(), "gateway") {
		t.Fatalf("RenderThunder = %v, want a refusal naming the module", err)
	}
}
