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

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/ampinit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"nonsense"},
		{"help"},
		{"serve", "extra"},
		{"migrate", "extra"},
		{"init"},
		{"init", "thunder"},
		{"module"},
		{"module", "list", "extra"},
		{"module", "install"},
		{"module", "install", "a", "b"},
		{"module", "enable", "a", "--with-deps"},
		{"module", "disable", "--force", "a"},
		{"module", "credential", "a"},
		{"module", "credential", "a", "w", "x"},
		{"module", "credential", "a", "w", "--subject"},
		{"module", "credential", "a", "w", "--out", "--subject", "s"},
		{"module", "install", "a", "--subject", "s"},
		{"module", "health", "a", "b"},
	} {
		assert.ErrorIs(t, run("amp", args), errUsage, "%v", args)
	}
}

func TestModuleArgs(t *testing.T) {
	a, err := parseModuleArgs("credential", []string{"--subject", "default", "ai-gateway", "gateway-controller", "--out=/amp/gw/credential"})
	assert.NoError(t, err)
	assert.Equal(t, moduleArgs{pos: []string{"ai-gateway", "gateway-controller"}, subject: "default", out: "/amp/gw/credential"}, a)
	a, err = parseModuleArgs("install", []string{"x", "--with-deps"})
	assert.NoError(t, err)
	assert.True(t, a.withDeps)
	a, err = parseModuleArgs("health", nil)
	assert.NoError(t, err)
	assert.Empty(t, a.pos)
}

func TestWriteSecretIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential")
	assert.NoError(t, writeSecret(path, "id.secret\n"))
	st, err := os.Stat(path)
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), st.Mode().Perm())
	got, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "id.secret\n", string(got))
}

func TestInitRejectsUnknownComponent(t *testing.T) {
	err := run("amp", []string{"init", "thunder", "/tmp/a", "console", "/tmp/b"})
	assert.ErrorContains(t, err, `unknown component "console"`)
}

func TestList(t *testing.T) {
	var b bytes.Buffer
	err := list(&b, []modhost.ModuleInfo{
		{Manifest: modkit.Manifest{Name: "widgets", Version: "1.0.0"}, State: modhost.StateActive},
		{Manifest: modkit.Manifest{Name: "ai-gateway", Version: "1.2.0", Provides: []string{"llm.proxy/v1"}}, State: modhost.StateAvailable},
	})
	assert.NoError(t, err)
	assert.Equal(t, ""+
		"NAME        VERSION  STATE      PROVIDES      REQUIRES\n"+
		"widgets     1.0.0    active     -             -\n"+
		"ai-gateway  1.2.0    available  llm.proxy/v1  -\n", b.String())
}

func TestConfigFlag(t *testing.T) {
	for _, c := range []struct {
		in, rest []string
		path     string
	}{
		{[]string{"--config", "a.yaml", "serve"}, []string{"serve"}, "a.yaml"},
		{[]string{"--config=b.yaml", "migrate"}, []string{"migrate"}, "b.yaml"},
		{[]string{"-config", "c.yaml"}, []string{}, "c.yaml"},
		{[]string{"-migrate"}, []string{"-migrate"}, ""},
		{nil, nil, ""},
	} {
		rest, path, err := configFlag(c.in)
		assert.NoError(t, err)
		assert.Equal(t, c.rest, rest)
		assert.Equal(t, c.path, path)
	}
	_, _, err := configFlag([]string{"--config"})
	assert.ErrorIs(t, err, errUsage)
}

func TestSecretProviderFollowsConfiguration(t *testing.T) {
	for _, name := range []string{"inhouse", "unsupported", " inhouse "} {
		p, err := secretProviderFor(name)
		if err != nil || p == nil {
			t.Fatalf("secretProviderFor(%q) = %v, %v; want a provider", name, p, err)
		}
	}
	if got, _ := secretProviderFor("unsupported"); got.Capabilities() != secretmanagersvc.StoreCapabilityReadOnly {
		t.Fatalf("unsupported must keep refusing secret writes, got capabilities %v", got.Capabilities())
	}
	for _, name := range []string{"", "vault", "Inhouse"} {
		if _, err := secretProviderFor(name); err == nil {
			t.Fatalf("secretProviderFor(%q) accepted an unknown provider", name)
		}
	}
}

func TestInitWritesSecrets(t *testing.T) {
	for _, k := range []string{"IDP_CLIENT_SECRET", "AMP_API_CLIENT_SECRET", "AMP_SECRETS_DIR", "AMP_DATA_DIR"} {
		t.Setenv(k, "")
		assert.NoError(t, os.Unsetenv(k))
	}
	dir := filepath.Join(t.TempDir(), "secrets")
	assert.NoError(t, run("amp", []string{"init", "secrets", dir}))
	assert.FileExists(t, filepath.Join(dir, "amp-api-client-secret"))
	assert.FileExists(t, filepath.Join(dir, "admin-password"))
}

func TestInitThunderImpliesSecrets(t *testing.T) {
	for k, v := range map[string]string{
		"THUNDER_BASE_URL": "http://thunder.example:8080", "SERVER_PUBLIC_URL": "http://localhost:9000",
		"CONSOLE_URL": "http://localhost:9000",
	} {
		t.Setenv(k, v)
	}
	for _, k := range []string{
		"IDP_CLIENT_SECRET", "AMP_API_CLIENT_SECRET", "AMP_CONSOLE_CLIENT_SECRET",
		"THUNDER_CLIENT_SECRET", "AMP_SYSTEM_CLIENT_SECRET", "AMP_SECRETS_DIR",
	} {
		t.Setenv(k, "")
		assert.NoError(t, os.Unsetenv(k))
	}
	data := t.TempDir()
	t.Setenv("AMP_DATA_DIR", data)
	assert.NoError(t, run("amp", []string{"init", "thunder", filepath.Join(t.TempDir(), "thunder")}))
	assert.FileExists(t, filepath.Join(data, "secrets", "amp-system-client-secret"))
}

func secretsFixture(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AMP_SECRETS_DIR", dir)
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "admin-password"), []byte("pw-value"), 0o600))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "encryption-key"), []byte("key-value"), 0o600))
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, "jwt"), 0o700))
}

func TestSecretList(t *testing.T) {
	secretsFixture(t)
	var b bytes.Buffer
	assert.NoError(t, secretCmd([]string{"list"}, &b))
	assert.Equal(t, "admin-password\nencryption-key\n", b.String())
}

func TestSecretShow(t *testing.T) {
	secretsFixture(t)
	var b bytes.Buffer
	assert.NoError(t, secretCmd([]string{"show", "admin-password"}, &b))
	assert.Equal(t, "pw-value\n", b.String())
}

func TestSecretShowUnknown(t *testing.T) {
	secretsFixture(t)
	var b bytes.Buffer
	err := secretCmd([]string{"show", "../etc/passwd"}, &b)
	assert.ErrorContains(t, err, "admin-password")
	assert.Empty(t, b.String())
}

// The observability store's passwords show only where their directory is set,
// as in amp-init, never in a server that lacks it.
func TestSecretListIncludesOwnDirOnlyWhereSet(t *testing.T) {
	secretsFixture(t)
	store := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(store, "setup-password"), []byte("setup-value"), 0o600))
	assert.NoError(t, os.WriteFile(filepath.Join(store, "unrelated"), []byte("x"), 0o600))

	t.Setenv(ampinit.ClickHouseSecretsDir, "")
	var b bytes.Buffer
	assert.NoError(t, secretCmd([]string{"list"}, &b))
	assert.Equal(t, "admin-password\nencryption-key\n", b.String())
	assert.Error(t, secretCmd([]string{"show", "setup-password"}, &b))

	t.Setenv(ampinit.ClickHouseSecretsDir, store)
	b.Reset()
	assert.NoError(t, secretCmd([]string{"list"}, &b))
	assert.Equal(t, "admin-password\nencryption-key\nsetup-password\n", b.String())
	b.Reset()
	assert.NoError(t, secretCmd([]string{"show", "setup-password"}, &b))
	assert.Equal(t, "setup-value\n", b.String())
}
