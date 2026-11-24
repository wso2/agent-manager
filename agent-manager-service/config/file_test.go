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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	if got := ModuleConfigKey("widgets", "limits.max"); got != "MODULE_WIDGETS_LIMITS_MAX" {
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

func TestLoadPrecedence(t *testing.T) {
	// Registered first so it runs last, once the variables below are restored.
	t.Cleanup(func() {
		fileVars, fileModules = nil, Modules{}
		loadEnvs()
	})
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
		return p
	}
	t.Chdir(dir)
	write("amp.yaml", "log_level: WARN\nserver: {port: 9393}\nmodules: [legacy]\n")
	named := write("named.yaml", "log_level: DEBUG\nserver: {port: 9191}\nhttp_read_timeout_seconds: 33\nmodules: [gateway]\n")
	other := write("other.yaml", "log_level: ERROR\n")
	t.Setenv(FileEnv, "")
	t.Setenv(ModulesEnv, "")
	t.Setenv("SERVER_PORT", "7070")
	t.Setenv("LOG_LEVEL", "")
	require.NoError(t, os.Unsetenv("LOG_LEVEL"))
	t.Setenv("HTTP_READ_TIMEOUT_SECONDS", "")

	// ./amp.yaml when nothing names a file.
	require.NoError(t, Load(""))
	assert.Equal(t, "WARN", GetConfig().LogLevel)
	assert.Equal(t, 7070, GetConfig().ServerPort, "the environment wins over the file")
	_, inEnv := os.LookupEnv("LOG_LEVEL")
	assert.False(t, inEnv, "file values stay out of the process environment")

	// $AMP_CONFIG over ./amp.yaml, --config over both; only one file is read.
	t.Setenv(FileEnv, other)
	require.NoError(t, Load(""))
	assert.Equal(t, "ERROR", GetConfig().LogLevel)
	require.NoError(t, Load(named))
	assert.Equal(t, "DEBUG", GetConfig().LogLevel)
	// An empty variable is still set: the file cannot fill it, the default applies.
	assert.Equal(t, 10, GetConfig().ReadTimeoutSeconds)
	mods, err := GetModules()
	require.NoError(t, err)
	assert.Equal(t, Modules{Declared: true, List: []ModuleSpec{{Name: "gateway", Enabled: true}}}, mods)

	t.Setenv(ModulesEnv, "gateway=off, widgets")
	mods, err = GetModules()
	require.NoError(t, err)
	assert.Equal(t, Modules{Declared: true, List: []ModuleSpec{{Name: "gateway"}, {Name: "widgets", Enabled: true}}}, mods)
	t.Setenv(ModulesEnv, "gateway=maybe")
	_, err = GetModules()
	assert.Error(t, err)

	// A missing named file is an error; a missing default is not.
	assert.Error(t, Load(filepath.Join(dir, "missing.yaml")))
	t.Setenv(FileEnv, "")
	t.Chdir(t.TempDir())
	assert.NoError(t, Load(""))
}

func TestUnknownFileKeys(t *testing.T) {
	RegisterKeys("SOME_INIT_KEY")
	fileMu.Lock()
	defer fileMu.Unlock()
	readKeys = map[string]bool{"LOG_LEVEL": true}
	got := unknownKeys(map[string]string{
		"LOG_LEVEL": "x", "LOG_LEVLE": "x", "SOME_INIT_KEY": "x",
		"CONSOLE_DOCS_URL": "x", "MODULE_WIDGETS_COLOR": "x",
	})
	assert.Equal(t, []string{"LOG_LEVLE"}, got)
}

func TestModulesListDeclaration(t *testing.T) {
	for doc, want := range map[string]Modules{
		"modules:\n":                {},
		"modules:\n  # - gateway\n": {},
		"modules: [\"\"]":           {},
		"modules: []":               {Declared: true},
		"modules: [gateway]":        {Declared: true, List: []ModuleSpec{{Name: "gateway", Enabled: true}}},
		"other: 1":                  {},
	} {
		_, got, err := parseConfigFile([]byte(doc))
		require.NoError(t, err, doc)
		assert.Equal(t, want, got, doc)
	}
	for v, want := range map[string]Modules{
		",":    {},
		" , ":  {},
		"none": {Declared: true},
		"NONE": {Declared: true},
	} {
		got, err := parseModulesEnv(v)
		require.NoError(t, err, v)
		assert.Equal(t, want, got, v)
	}
}

// reloadAfter restores the settings once the test's variables are restored;
// call it before the test changes any.
func reloadAfter(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		fileVars, fileModules = nil, Modules{}
		loadEnvs()
	})
}

func loadWith(t *testing.T, yaml string) error {
	t.Helper()
	t.Setenv("ENV_FILE_PATH", "")
	t.Setenv(FileEnv, "")
	t.Setenv(ModulesEnv, "")
	p := filepath.Join(t.TempDir(), "amp.yaml")
	require.NoError(t, os.WriteFile(p, []byte(yaml), 0o600))
	return Load(p)
}

func secretFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestFileSuffixFromEnv(t *testing.T) {
	reloadAfter(t)
	t.Setenv("DB_PASSWORD", "")
	require.NoError(t, os.Unsetenv("DB_PASSWORD"))
	t.Setenv("DB_PASSWORD_FILE", secretFile(t, "pw\n"))
	require.NoError(t, loadWith(t, "log_level: INFO\n"))
	assert.Equal(t, "pw", Value("DB_PASSWORD"))
}

func TestFileSuffixPrecedence(t *testing.T) {
	reloadAfter(t)
	b, c, d := secretFile(t, "b"), "c", secretFile(t, "d")

	t.Setenv("DB_PASSWORD", "a")
	t.Setenv("DB_PASSWORD_FILE", b)
	require.NoError(t, loadWith(t, "db: {password: "+c+"}\n"))
	assert.Equal(t, "a", Value("DB_PASSWORD"), "the variable wins over its _FILE")

	require.NoError(t, os.Unsetenv("DB_PASSWORD"))
	require.NoError(t, loadWith(t, "db: {password: "+c+"}\n"))
	assert.Equal(t, "b", Value("DB_PASSWORD"), "the environment's _FILE wins over the file")

	t.Setenv("DB_PASSWORD_FILE", "")
	require.NoError(t, os.Unsetenv("DB_PASSWORD_FILE"))
	require.NoError(t, loadWith(t, "db: {password_file: "+d+"}\n"))
	assert.Equal(t, "d", Value("DB_PASSWORD"), "the file's _file is read")
}

func TestFileSuffixMissingFile(t *testing.T) {
	reloadAfter(t)
	t.Setenv("DB_PASSWORD", "")
	require.NoError(t, os.Unsetenv("DB_PASSWORD"))
	t.Setenv("DB_PASSWORD_FILE", "/nope/amp-db-password")
	err := loadWith(t, "log_level: INFO\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_PASSWORD_FILE")
}

func TestFileSuffixKeysAreKnown(t *testing.T) {
	reloadAfter(t)
	require.NoError(t, loadWith(t, "db: {password_file: "+secretFile(t, "x")+"}\n"))
	fileMu.Lock()
	defer fileMu.Unlock()
	assert.Empty(t, unknownKeys(map[string]string{"DB_PASSWORD_FILE": "x"}))
}

func TestGatewayTokenFileStillWorks(t *testing.T) {
	reloadAfter(t)
	key := ModuleConfigKey("ai-gateway", "default_gateway_token_file")
	t.Setenv(key, "/amp/gw/registration-token")
	require.NoError(t, loadWith(t, "log_level: INFO\n"))
	assert.Equal(t, "/amp/gw/registration-token", Value(key))
}

// A setting another package registers is read during Load too, so a _FILE
// it cannot read fails Load instead of only being logged later.
func TestFileSuffixMissingFileForRegisteredKey(t *testing.T) {
	reloadAfter(t)
	RegisterKeys("AMP_TEST_REGISTERED_PORT")
	t.Setenv("AMP_TEST_REGISTERED_PORT", "")
	require.NoError(t, os.Unsetenv("AMP_TEST_REGISTERED_PORT"))
	t.Setenv("AMP_TEST_REGISTERED_PORT_FILE", "/nope/amp-port")
	err := loadWith(t, "log_level: INFO\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AMP_TEST_REGISTERED_PORT_FILE")
}
