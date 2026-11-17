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
	"testing"

	"github.com/stretchr/testify/assert"

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
	} {
		assert.ErrorIs(t, run("amp", args), errUsage, "%v", args)
	}
}

func TestInitRejectsUnknownComponent(t *testing.T) {
	err := run("amp", []string{"init", "thunder", "/tmp/a", "console", "/tmp/b"})
	assert.ErrorContains(t, err, `unknown component "console"`)
}

func TestList(t *testing.T) {
	var b bytes.Buffer
	err := list(&b, []modhost.ModuleInfo{
		{Manifest: modkit.Manifest{Name: "legacy", Version: "1.0.0", Builtin: true}, State: modhost.StateActive},
		{Manifest: modkit.Manifest{Name: "ai-gateway", Version: "1.2.0", Provides: []string{"llm.proxy/v1"}}, State: modhost.StateAvailable},
	})
	assert.NoError(t, err)
	assert.Equal(t, ""+
		"NAME        VERSION  STATE      BUILTIN  PROVIDES      REQUIRES\n"+
		"legacy      1.0.0    active     yes      -             -\n"+
		"ai-gateway  1.2.0    available           llm.proxy/v1  -\n", b.String())
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
