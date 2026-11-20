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
