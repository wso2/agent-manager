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

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const sample = `
server:
  port: 9090
  public_url: http://amp.example
db:
  driver: postgres
  host: db
cors-allowed-origin: http://localhost:3000
key_manager:
  audience: [a, b]
console:
  auth_client_id: my-console
  feature_flag_enable_user_management: true
modules:
  - gateway
  - name: widgets
    enabled: false
    config:
      color: blue
      limits: {max: 3}
`

func TestParseConfigFile(t *testing.T) {
	vars, mods, err := parseConfigFile([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"SERVER_PORT":            "9090",
		"SERVER_PUBLIC_URL":      "http://amp.example",
		"DB_DRIVER":              "postgres",
		"DB_HOST":                "db",
		"CORS_ALLOWED_ORIGIN":    "http://localhost:3000",
		"KEY_MANAGER_AUDIENCE":   "a,b",
		"CONSOLE_AUTH_CLIENT_ID": "my-console",
		"CONSOLE_FEATURE_FLAG_ENABLE_USER_MANAGEMENT": "true",
		"MODULE_WIDGETS_COLOR":                        "blue",
		"MODULE_WIDGETS_LIMITS_MAX":                   "3",
	}
	if !reflect.DeepEqual(vars, want) {
		t.Fatalf("vars:\n got %v\nwant %v", vars, want)
	}
	wantMods := Modules{Declared: true, List: []ModuleSpec{{Name: "gateway", Enabled: true}, {Name: "widgets", Enabled: false}}}
	if !reflect.DeepEqual(mods, wantMods) {
		t.Fatalf("modules: got %+v", mods)
	}
	if got := ModuleConfigEnv("widgets", "limits.max"); got != "MODULE_WIDGETS_LIMITS_MAX" {
		t.Fatalf("ModuleConfigEnv: %s", got)
	}
}

func TestParseConfigFileRejects(t *testing.T) {
	for name, doc := range map[string]string{
		"duplicate module":  "modules: [a, a]",
		"bad module name":   "modules: [Gateway]",
		"unknown entry key": "modules: [{name: a, enabld: false}]",
		"modules not list":  "modules: {a: true}",
		"conflicting keys":  "db_driver: sqlite\ndb: {driver: postgres}",
		"not yaml":          "a: [",
	} {
		if _, _, err := parseConfigFile([]byte(doc)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, _, err := parseConfigFile([]byte("modules: []")); err != nil {
		t.Errorf("empty list: %v", err)
	}
}

func TestConfigFileFillsOnlyUnsetVariables(t *testing.T) {
	// Registered first so it runs last, once the variables below are restored.
	t.Cleanup(loadEnvs)
	path := filepath.Join(t.TempDir(), "amp.yaml")
	if err := os.WriteFile(path, []byte("log_level: DEBUG\nserver: {port: 9191}\nmodules: [gateway]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(FileEnv, path)
	t.Setenv("SERVER_PORT", "7070")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv(ModulesEnv, "")
	loadEnvs()
	if got := GetConfig().ServerPort; got != 7070 {
		t.Errorf("SERVER_PORT from the environment should win, got %d", got)
	}
	if got := GetConfig().LogLevel; got != "DEBUG" {
		t.Errorf("LOG_LEVEL from the file, got %q", got)
	}
	mods, err := GetModules()
	if err != nil || !mods.Declared || len(mods.List) != 1 || mods.List[0].Name != "gateway" {
		t.Errorf("modules from the file: %+v %v", mods, err)
	}
	t.Setenv(ModulesEnv, "gateway=off, widgets")
	mods, err = GetModules()
	want := Modules{Declared: true, List: []ModuleSpec{{Name: "gateway"}, {Name: "widgets", Enabled: true}}}
	if err != nil || !reflect.DeepEqual(mods, want) {
		t.Errorf("modules from %s: %+v %v", ModulesEnv, mods, err)
	}
	t.Setenv(ModulesEnv, "gateway=maybe")
	if _, err := GetModules(); err == nil {
		t.Error("bad state accepted")
	}
}

func TestMissingDefaultFileIsFine(t *testing.T) {
	t.Setenv(FileEnv, "")
	t.Chdir(t.TempDir())
	if err := applyConfigFile(); err != nil {
		t.Fatal(err)
	}
	t.Setenv(FileEnv, filepath.Join(t.TempDir(), "missing.yaml"))
	if err := applyConfigFile(); err == nil {
		t.Fatal("a named file that is missing must be an error")
	}
}
